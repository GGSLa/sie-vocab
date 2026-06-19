package model

// WordEntry 单词条目（API 数据模型，与 DB 行结构不同）
type WordEntry struct {
	Word       string    `json:"word"`
	Type       string    `json:"type"`
	Pos        string    `json:"pos"`
	BaseWord   *string   `json:"baseWord"`
	Derivation *string   `json:"derivation"`
	Meanings   []Meaning `json:"meanings"`
	Examples   []Example `json:"examples"`
}

// Meaning 释义
type Meaning struct {
	Domain string `json:"domain"`
	Text   string `json:"text"`
}

// Example 例句
type Example struct {
	En        string `json:"en"`
	Zh        string `json:"zh"`
	SortOrder int    `json:"-"`
}

// WordsResponse 词族响应
type WordsResponse struct {
	Words []WordEntry `json:"words"`
}
