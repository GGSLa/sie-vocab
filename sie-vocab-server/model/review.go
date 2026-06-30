package model

// ReviewRandomResponse 抽词响应
type ReviewRandomResponse struct {
	WordID    int       `json:"word_id"`
	Word      WordEntry `json:"word"`
	BatchDone bool      `json:"batch_done,omitempty"` // 本批已完成（30词）
	CanMore   bool      `json:"can_more,omitempty"`   // 还能生成下一批
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
	WordCount      int
	BaseCount      int
	NextDate       string
	BatchRemaining int // 当前批次剩余未抽取数
}
