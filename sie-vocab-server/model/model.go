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
	DeepSeekAPIKey string      `json:"deepseek_api_key"`
	Port           string      `json:"port"`
	MySQL          MySQLConfig `json:"mysql"`
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
	En        string `json:"en"`
	Zh        string `json:"zh"`
	SortOrder int    `json:"-"`
}

type WordsResponse struct {
	Words []WordEntry `json:"words"`
}
