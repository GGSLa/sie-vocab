package model

import "time"

// Invitation 邀请记录
type Invitation struct {
	ID              int       `gorm:"primaryKey;column:id"`
	InviterUserID   int       `gorm:"column:inviter_user_id;index"`
	InvitedUsername string    `gorm:"column:invited_username;size:100;uniqueIndex"`
	Used            bool      `gorm:"column:used;default:false"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

// TableName 指定表名
func (Invitation) TableName() string { return "invitations" }

// ── 请求/响应 ──

// InviteRequest 邀请请求
type InviteRequest struct {
	Username string `json:"username"`
}

// InviteResponse 邀请响应
type InviteResponse struct {
	Success  bool   `json:"success"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

// UserInfoResponse 用户信息响应
type UserInfoResponse struct {
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"` // ISO 8601 格式
}
