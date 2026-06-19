package model

// MySQLConfig 数据库连接配置
type MySQLConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// Config 应用配置
type Config struct {
	DeepSeekAPIKey        string      `json:"deepseek_api_key"`
	Port                  string      `json:"port"`
	APIToken              string      `json:"api_token"` // 可选，设置后 /api/* 需 Bearer 认证
	MySQL                 MySQLConfig `json:"mysql"`
	SIE_PDFPath           string      `json:"sie_pdf_path"`
	SIE_ProgressPath      string      `json:"sie_progress_path"`
	OCRLanguage           string      `json:"ocr_language"`
	UploadDir             string      `json:"upload_dir"`
	DeepSeekRPM           int         `json:"deepseek_rpm"`
	DeepSeekMaxConcurrent int         `json:"deepseek_max_concurrent"`
}
