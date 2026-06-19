package model

// ---------- 常量 ----------

const DeepSeekURL = "https://api.deepseek.com/v1/chat/completions"

const SystemPrompt = `你是一个为 SIE（Securities Industry Essentials）考试准备的英语单词分析助手。用户输入一个英语单词，你需要返回一个严格的 JSON 对象。

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

// ---------- 配置 ----------

type MySQLConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Config struct {
	DeepSeekAPIKey   string      `json:"deepseek_api_key"`
	Port             string      `json:"port"`
	MySQL            MySQLConfig `json:"mysql"`
	SIE_PDFPath      string      `json:"sie_pdf_path"`
	SIE_ProgressPath string      `json:"sie_progress_path"`
}

// ---------- 请求/响应 ----------

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

type QueryRequest struct {
	Word string `json:"word"`
}

type QueryResponse struct {
	Found bool           `json:"found"`
	Data  *WordsResponse `json:"data,omitempty"`
}

type SaveAllRequest struct {
	Words []WordEntry `json:"words"`
}

type SaveResult struct {
	Success bool `json:"success"`
	Count   int  `json:"count,omitempty"`
}

// ---------- 复习 ----------

type ReviewRandomResponse struct {
	WordID int       `json:"word_id"`
	Word   WordEntry `json:"word"`
}

type ReviewErrorResponse struct {
	Error   string `json:"error"`
	AllDone bool   `json:"all_done,omitempty"`
}

type ReviewRecordRequest struct {
	WordID int `json:"word_id"`
}

// ---------- 总览 ----------

type OverviewRequest struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

type DayOverview struct {
	Date        string `json:"date"`
	ReviewCount int    `json:"review_count"`
	IsCompleted bool   `json:"is_completed"`
}

type OverviewResponse struct {
	TotalWords    int           `json:"total_words"`
	TotalReviews  int           `json:"total_reviews"`
	Streak        int           `json:"streak"`
	TodayReviewed int           `json:"today_reviewed"`
	MonthlyData   []DayOverview `json:"monthly_data"`
	Month         int           `json:"month"`
	Year          int           `json:"year"`
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

// ---------- 阅读器 ----------

const ReaderSystemPrompt = `你是一个 SIE（Securities Industry Essentials）考试教材的阅读辅助助手。你的任务是为英语学习者处理教材文本。

对于给定的文本，你需要：
1. 保留原始英文文本
2. 提供准确的中文翻译
3. 提取重要的词汇（尤其是金融/证券专业术语），包括单词、词性和中文释义
4. 标注值得注意的语法点或句式结构

## JSON Schema（必须严格遵守）

{
  "page_label": "51",
  "section": "Chapter 5: Securities Underwriting",
  "chunks": [
    {
      "en": "All issuers of securities need a starting point...",
      "zh": "所有证券发行人需要一个起点...",
      "vocab": [
        {
          "word": "underwriting",
          "pos": "n.",
          "definition": "承销",
          "example": "The underwriting process involves several key players."
        }
      ],
      "grammar": [
        {
          "point": "被动语态",
          "detail": "must be registered 使用被动语态，强调动作的承受者..."
        }
      ]
    }
  ]
}

## 标题标记（重要）
- 输入文本中已经用 Markdown 标题标记标出了原文的各级标题：
  "# "   = 章节大标题（Page-level chapter title）
  "## "  = 节标题（Major section heading）
  "### " = 小节标题（Sub-section heading）
- **这些标题是原文中字体较大、加粗的真实标题，请在分段时优先以标题为边界**
- 每个 chunk 应尽量从一个标题开始，到下一个标题前结束
- 不要把标题和其下的正文分到不同的 chunk 中（标题应和紧随其后的正文在同一个 chunk 里）
- section 字段应使用输入文本中 "# " 级别标题的文字内容（去掉 # 前缀），如果输入中没有该级别标题，则使用第一个 "## " 标题

## 分段规则
- 将输入文本分为 1 到 3 个 chunk，尽量以 "## " 或 "### " 标题为自然边界
- 每个 chunk 包含 1 到 3 个自然段落
- 每个 chunk 必须有完整意义，不可在句子中间断开
- 如果输入文本只有一个有效段落，则只输出 1 个 chunk
- 每个 chunk 的 vocab 包含 3 到 8 个词汇
- 每个 chunk 的 grammar 包含 1 到 3 个语法点
- 不要提取人名（如 Steven）和无关冠词/介词作为词汇

## 铁律（违反即错误）
- 只输出 JSON 对象本身，不要用 json 代码块包裹
- 输出必须以 { 开头，以 } 结尾
- JSON 必须是合法的，可以被 JSON.parse() 直接解析
- 如果输入无法解析为教材文本，返回：{"error": "无法解析该文本"}
- 不要使用斜体标记，用加粗代替强调`

type ReaderChunkResponse struct {
	Page        int     `json:"page"`
	PageEnd     int     `json:"page_end"`
	PageLabel   string  `json:"page_label"`
	Section     string  `json:"section"`
	Chunks      []Chunk `json:"chunks"`
	TotalChunks int     `json:"total_chunks"`
	Error       string  `json:"error,omitempty"`
}

type Chunk struct {
	En      string        `json:"en"`
	Zh      string        `json:"zh"`
	Vocab   []VocabEntry  `json:"vocab"`
	Grammar []GrammarNote `json:"grammar"`
}

type VocabEntry struct {
	Word       string `json:"word"`
	Pos        string `json:"pos"`
	Definition string `json:"definition"`
	Example    string `json:"example"`
}

type GrammarNote struct {
	Point  string `json:"point"`
	Detail string `json:"detail"`
}

type ReaderProgress struct {
	CurrentPage      int      `json:"current_page"`
	CurrentChunk     int      `json:"current_chunk"`
	CurrentSection   string   `json:"current_section"`
	CompletedSections []string `json:"completed_sections"`
	LastRead         string   `json:"last_read"`
}

type SaveProgressRequest struct {
	CurrentPage  int    `json:"current_page"`
	CurrentChunk int    `json:"current_chunk"`
	Section      string `json:"section"`
}

type Example struct {
	En        string `json:"en"`
	Zh        string `json:"zh"`
	SortOrder int    `json:"-"`
}

type WordsResponse struct {
	Words []WordEntry `json:"words"`
}
