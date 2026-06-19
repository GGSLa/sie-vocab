package model

// ReaderChunkResponse 阅读页 AI 分析响应
type ReaderChunkResponse struct {
	BookID      int     `json:"book_id"`
	Page        int     `json:"page"`
	PageEnd     int     `json:"page_end"`
	PageLabel   string  `json:"page_label"`
	Section     string  `json:"section"`
	Chunks      []Chunk `json:"chunks"`
	TotalChunks int     `json:"total_chunks"`
	Error       string  `json:"error,omitempty"`
}

// Chunk 单个段落分析块
type Chunk struct {
	En      string        `json:"en"`
	Zh      string        `json:"zh"`
	Vocab   []VocabEntry  `json:"vocab"`
	Grammar []GrammarNote `json:"grammar"`
}

// VocabEntry 词汇条目
type VocabEntry struct {
	Word       string `json:"word"`
	Pos        string `json:"pos"`
	Definition string `json:"definition"`
	Example    string `json:"example"`
}

// GrammarNote 语法笔记
type GrammarNote struct {
	Point  string `json:"point"`
	Detail string `json:"detail"`
}

// ReaderProgress 阅读进度
type ReaderProgress struct {
	BookID            int      `json:"book_id"`
	CurrentPage       int      `json:"current_page"`
	CurrentChunk      int      `json:"current_chunk"`
	CurrentSection    string   `json:"current_section"`
	CompletedSections []string `json:"completed_sections"`
	LastRead          string   `json:"last_read"`
}

// SaveProgressRequest 保存进度请求
type SaveProgressRequest struct {
	BookID       int    `json:"book_id"`
	CurrentPage  int    `json:"current_page"`
	CurrentChunk int    `json:"current_chunk"`
	Section      string `json:"section"`
}

// ReaderChunkRequest 取 chunk 请求
type ReaderChunkRequest struct {
	BookID int `json:"book_id"`
	Page   int `json:"page"`
}
