package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const deepseekURL = "https://api.deepseek.com/v1/chat/completions"

const systemPrompt = `你是一个为 SIE（Securities Industry Essentials）考试准备的英语单词分析助手。用户输入一个英语单词，你需要返回一个严格的 JSON 对象。

## 分析步骤

1. 确定基础词：衍生词必须追溯到基础词；基础词本身就是它自己。
2. 输出顺序：基础词 → 用户输入的词 → 该基础词的其他常见衍生词。
3. 词性使用英文简写：n. / v. / vi. / vt. / adj. / adv. / prep. / conj. / pron. / aux. / art. / num. / int. 等。
4. 若该词在金融/证券领域有与日常不同的专业含义，必须在 meanings 中分别列出"金融"和"日常"两个 domain。
5. 每个词的 examples 至少 2 条；若金融和日常含义不同，各至少 1 条。

## JSON Schema（必须严格遵守）

{
  "words": [
    {
      "word": "go",
      "type": "基础词",
      "pos": "v.",
      "baseWord": null,
      "derivation": null,
      "meanings": [
        {"domain": "日常", "text": "去；走；进行"}
      ],
      "examples": [
        {"en": "I go to school every day.", "zh": "我每天去学校。"},
        {"en": "Let's go for a walk.", "zh": "我们去散步吧。"}
      ]
    },
    {
      "word": "went",
      "type": "衍生词",
      "pos": "v.",
      "baseWord": "go",
      "derivation": "过去式",
      "meanings": [
        {"domain": "日常", "text": "去（go的过去式）"}
      ],
      "examples": [
        {"en": "He went to the bank yesterday.", "zh": "他昨天去了银行。"}
      ]
    }
  ]
}

## 字段说明
- word: 单词本身
- type: "基础词" 或 "衍生词"
- pos: 词性英文简写
- baseWord: 基础词名称（衍生词必填，基础词填 null）
- derivation: 衍生关系说明，如"过去式"、"复数"、"名词形式"等（衍生词必填，基础词填 null）
- meanings: 含义数组，每个元素包含 domain（"日常"/"金融"）和 text（中文释义）
- examples: 例句数组，每个元素包含 en（英文）和 zh（中文翻译）

## 铁律（违反即错误）
- 只输出 JSON 对象本身，不要用 ` + "```json```" + ` 包裹，不要在 JSON 前后添加任何文字。
- 输出必须以 { 开头，以 } 结尾。
- JSON 必须是合法的，可以被 JSON.parse() 直接解析。
- 如果用户输入的不是单个单词（短语或句子），返回：{"error": "请输入单个英语单词，不支持短语或句子。"}
- 如果单词无法识别，返回：{"error": "无法识别该单词，请检查拼写。"}`

var httpClient = &http.Client{Timeout: 60 * time.Second}
var db *sql.DB

type MySQLConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Config struct {
	DeepSeekAPIKey string      `json:"deepseek_api_key"`
	Port           string      `json:"port"`
	MySQL          MySQLConfig `json:"mysql"`
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

// ---------- JSON 数据模型 ----------

type WordEntry struct {
	Word       string    `json:"word"`
	Type       string    `json:"type"`
	Pos        string    `json:"pos"`
	BaseWord   *string   `json:"baseWord"`
	Derivation *string   `json:"derivation"`
	Meanings   []Meaning `json:"meanings"`
	Examples   []Example `json:"examples"`
}

type Meaning struct {
	Domain string `json:"domain"`
	Text   string `json:"text"`
}

type Example struct {
	En       string `json:"en"`
	Zh       string `json:"zh"`
	SortOrder int   `json:"-"`
}

type WordsResponse struct {
	Words []WordEntry `json:"words"`
}

type QueryRequest struct {
	Word string `json:"word"`
}

type QueryResponse struct {
	Found bool          `json:"found"`
	Data  *WordsResponse `json:"data,omitempty"`
}

type SaveOneRequest WordEntry

type SaveAllRequest struct {
	Words []WordEntry `json:"words"`
}

type SaveResult struct {
	Success bool `json:"success"`
	Count   int  `json:"count,omitempty"`
}

// ---------- 配置加载 ----------

func loadConfig() (*Config, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取可执行文件路径失败: %v", err)
	}
	configPath := filepath.Join(filepath.Dir(exePath), "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %v", configPath, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}
	if cfg.DeepSeekAPIKey == "" {
		return nil, fmt.Errorf("配置文件中 deepseek_api_key 不能为空")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.MySQL.Host == "" {
		cfg.MySQL.Host = "localhost"
	}
	if cfg.MySQL.Port == "" {
		cfg.MySQL.Port = "3306"
	}
	return &cfg, nil
}

// ---------- 数据库初始化 ----------

func initDB(cfg MySQLConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %v", err)
	}
	d.SetMaxOpenConns(10)
	d.SetMaxIdleConns(5)
	d.SetConnMaxLifetime(5 * time.Minute)
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接测试失败: %v", err)
	}
	return d, nil
}

// ---------- 主函数 ----------

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err = initDB(cfg.MySQL)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()
	log.Println("✅ 数据库连接成功")

	exePath, _ := os.Executable()
	staticDir := filepath.Join(filepath.Dir(exePath), "..", "sie-vocab-web")

	// API 路由
	http.HandleFunc("/api/chat", handleChat(cfg))
	http.HandleFunc("/api/word/query", handleWordQuery)
	http.HandleFunc("/api/word/save", handleWordSave)
	http.HandleFunc("/api/word/save-all", handleWordSaveAll)

	// 静态文件（前端）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		http.FileServer(http.Dir(staticDir)).ServeHTTP(w, r)
	})

	log.Printf("🚀 服务启动: http://0.0.0.0:%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

// ==================== AI 翻译 ====================

func handleChat(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, chatResponse{Error: "只接受 POST 请求"})
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "请求格式错误"})
			return
		}
		if req.Message == "" {
			writeJSON(w, http.StatusBadRequest, chatResponse{Error: "消息不能为空"})
			return
		}

		log.Printf("📩 收到消息: %s", req.Message)

		reply, err := callDeepSeek(cfg.DeepSeekAPIKey, req.Message)
		if err != nil {
			log.Printf("❌ DeepSeek 调用失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, chatResponse{Error: fmt.Sprintf("DeepSeek 调用失败: %v", err)})
			return
		}

		log.Printf("✅ DeepSeek 回复长度: %d 字节", len(reply))
		writeJSON(w, http.StatusOK, chatResponse{Reply: reply})
	}
}

func callDeepSeek(apiKey, message string) (string, error) {
	body := map[string]interface{}{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": message},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", deepseekURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("DeepSeek 返回空响应")
	}

	return result.Choices[0].Message.Content, nil
}

// ==================== 单词查询（数据库） ====================

func handleWordQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Word == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word 为空"})
		return
	}

	words, err := queryWordFamily(req.Word)
	if err != nil {
		log.Printf("❌ 查询单词失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "数据库查询失败"})
		return
	}

	if len(words) == 0 {
		writeJSON(w, http.StatusOK, QueryResponse{Found: false})
		return
	}

	log.Printf("📚 从数据库找到 %d 个相关单词", len(words))
	writeJSON(w, http.StatusOK, QueryResponse{
		Found: true,
		Data:  &WordsResponse{Words: words},
	})
}

func queryWordFamily(word string) ([]WordEntry, error) {
	// 1. 查该词本身，获取 base_word
	var id int
	var baseWord sql.NullString
	var wType, pos string
	var derivation sql.NullString

	err := db.QueryRow("SELECT id, base_word, type, pos, derivation FROM words WHERE word = ?", word).
		Scan(&id, &baseWord, &wType, &pos, &derivation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 2. 确定族根（base_word 非空则用 base_word，否则该词本身是基础词）
	familyRoot := word
	if baseWord.Valid && baseWord.String != "" {
		familyRoot = baseWord.String
	}

	// 3. 查整个族
	rows, err := db.Query(
		`SELECT id, word, base_word, type, pos, derivation FROM words
		 WHERE word = ? OR base_word = ?
		 ORDER BY CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id`,
		familyRoot, familyRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []WordEntry
	for rows.Next() {
		var wid int
		var w string
		var bw sql.NullString
		var wt, p string
		var d sql.NullString
		if err := rows.Scan(&wid, &w, &bw, &wt, &p, &d); err != nil {
			return nil, err
		}
		entry := WordEntry{
			Word: w,
			Type: wt,
			Pos:  p,
		}
		if bw.Valid {
			entry.BaseWord = &bw.String
		}
		if d.Valid {
			entry.Derivation = &d.String
		}

		// 查 meanings
		mRows, err := db.Query("SELECT domain, text FROM meanings WHERE word_id = ?", wid)
		if err != nil {
			return nil, err
		}
		for mRows.Next() {
			var domain, text string
			if err := mRows.Scan(&domain, &text); err != nil {
				mRows.Close()
				return nil, err
			}
			entry.Meanings = append(entry.Meanings, Meaning{Domain: domain, Text: text})
		}
		mRows.Close()

		// 查 examples
		eRows, err := db.Query("SELECT en, zh FROM examples WHERE word_id = ? ORDER BY sort_order", wid)
		if err != nil {
			return nil, err
		}
		for eRows.Next() {
			var en, zh string
			if err := eRows.Scan(&en, &zh); err != nil {
				eRows.Close()
				return nil, err
			}
			entry.Examples = append(entry.Examples, Example{En: en, Zh: zh})
		}
		eRows.Close()

		words = append(words, entry)
	}
	return words, nil
}

// ==================== 单词保存 ====================

func handleWordSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var entry WordEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
		return
	}
	if entry.Word == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "word 不能为空"})
		return
	}

	if err := saveWordToDB(entry); err != nil {
		log.Printf("❌ 保存单词失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存失败"})
		return
	}

	log.Printf("💾 已保存单词: %s", entry.Word)
	writeJSON(w, http.StatusOK, SaveResult{Success: true})
}

func handleWordSaveAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req SaveAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
		return
	}

	count := 0
	for _, entry := range req.Words {
		if entry.Word == "" {
			continue
		}
		if err := saveWordToDB(entry); err != nil {
			log.Printf("❌ 保存 %s 失败: %v", entry.Word, err)
			continue
		}
		count++
	}

	log.Printf("💾 批量保存完成: %d/%d 个单词", count, len(req.Words))
	writeJSON(w, http.StatusOK, SaveResult{Success: true, Count: count})
}

func saveWordToDB(entry WordEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// upsert words
	_, err = tx.Exec(
		`INSERT INTO words (word, base_word, type, pos, derivation)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   base_word = VALUES(base_word),
		   type = VALUES(type),
		   pos = VALUES(pos),
		   derivation = VALUES(derivation),
		   updated_at = NOW()`,
		entry.Word, entry.BaseWord, entry.Type, entry.Pos, entry.Derivation)
	if err != nil {
		return err
	}

	// 获取 word_id
	var wordID int
	err = tx.QueryRow("SELECT id FROM words WHERE word = ?", entry.Word).Scan(&wordID)
	if err != nil {
		return err
	}

	// 删除旧 meanings + examples
	tx.Exec("DELETE FROM meanings WHERE word_id = ?", wordID)
	tx.Exec("DELETE FROM examples WHERE word_id = ?", wordID)

	// 插入 meanings
	for _, m := range entry.Meanings {
		_, err := tx.Exec("INSERT INTO meanings (word_id, domain, text) VALUES (?, ?, ?)", wordID, m.Domain, m.Text)
		if err != nil {
			return err
		}
	}

	// 插入 examples
	for i, e := range entry.Examples {
		_, err := tx.Exec("INSERT INTO examples (word_id, en, zh, sort_order) VALUES (?, ?, ?, ?)", wordID, e.En, e.Zh, i)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ---------- 工具函数 ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
