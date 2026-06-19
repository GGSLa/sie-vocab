package model

// ChatRequest AI 翻译请求
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse AI 翻译响应
type ChatResponse struct {
	Reply string `json:"reply,omitempty"`
	Error string `json:"error,omitempty"`
}

// QueryRequest 查词请求
type QueryRequest struct {
	Word string `json:"word"`
}

// QueryResponse 查词响应
type QueryResponse struct {
	Found bool           `json:"found"`
	Data  *WordsResponse `json:"data,omitempty"`
}

// SaveAllRequest 批量保存请求
type SaveAllRequest struct {
	Words []WordEntry `json:"words"`
}

// SaveResult 保存结果
type SaveResult struct {
	Success bool `json:"success"`
	Count   int  `json:"count,omitempty"`
}
