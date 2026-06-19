package model

// ReviewRandomResponse 抽词响应
type ReviewRandomResponse struct {
	WordID int       `json:"word_id"`
	Word   WordEntry `json:"word"`
}

// ReviewErrorResponse 复习错误响应（含 all_done 标记）
type ReviewErrorResponse struct {
	Error   string `json:"error"`
	AllDone bool   `json:"all_done,omitempty"`
}

// ReviewRecordRequest 记录复习请求
type ReviewRecordRequest struct {
	WordID int `json:"word_id"`
}

// ReviewRecordResult 记录复习结果
type ReviewRecordResult struct {
	WordCount int
	BaseCount int
	NextDate  string
}
