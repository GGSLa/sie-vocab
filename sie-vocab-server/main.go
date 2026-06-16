package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const deepseekURL = "https://api.deepseek.com/v1/chat/completions"

var httpClient = &http.Client{Timeout: 60 * time.Second}

type Config struct {
	DeepSeekAPIKey string `json:"deepseek_api_key"`
	Port           string `json:"port"`
}

type chatRequest struct {
	Message string `json:"message"`
}

type chatResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

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
	return &cfg, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	exePath, _ := os.Executable()
	staticDir := filepath.Join(filepath.Dir(exePath), "..", "sie-vocab-web")

	// API 路由
	http.HandleFunc("/api/chat", handleChat(cfg))

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

		log.Printf("✅ DeepSeek 回复: %s", reply)
		writeJSON(w, http.StatusOK, chatResponse{Reply: reply})
	}
}

func callDeepSeek(apiKey, message string) (string, error) {
	body := map[string]interface{}{
		"model": "deepseek-chat",
		"messages": []map[string]string{
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
