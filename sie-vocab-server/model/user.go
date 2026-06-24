package model

import "time"

// User 用户账号
type User struct {
	ID           int       `gorm:"primaryKey;column:id"`
	Username     string    `gorm:"column:username;uniqueIndex;size:100"`
	PasswordHash string    `gorm:"column:password_hash;size:255"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名
func (User) TableName() string { return "users" }

// ── 请求/响应 ──

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}
