package model

// OverviewRequest 总览请求
type OverviewRequest struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

// DayOverview 单日概览
type DayOverview struct {
	Date        string `json:"date"`
	ReviewCount int    `json:"review_count"`
	IsCompleted bool   `json:"is_completed"`
}

// OverviewResponse 月度总览响应
type OverviewResponse struct {
	TotalWords    int           `json:"total_words"`
	TotalReviews  int           `json:"total_reviews"`
	Streak        int           `json:"streak"`
	TodayReviewed int           `json:"today_reviewed"`
	MonthlyData   []DayOverview `json:"monthly_data"`
	Month         int           `json:"month"`
	Year          int           `json:"year"`
}
