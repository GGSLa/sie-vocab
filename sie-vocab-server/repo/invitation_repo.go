package repo

import (
	"gorm.io/gorm"

	"sie-vocab-server/model"
)

// InvitationRepo 邀请数据访问层
type InvitationRepo struct {
	db *gorm.DB
}

// NewInvitationRepo 创建 InvitationRepo
func NewInvitationRepo(db *gorm.DB) *InvitationRepo {
	return &InvitationRepo{db: db}
}

// Create 创建邀请记录（inviter 已存在用户邀请一个新用户名）
func (r *InvitationRepo) Create(inviterUserID int, invitedUsername string) error {
	inv := &model.Invitation{
		InviterUserID:   inviterUserID,
		InvitedUsername: invitedUsername,
		Used:            false,
	}
	return r.db.Create(inv).Error
}

// FindByUsername 查找指定用户名的邀请记录（未使用且未被占用）
func (r *InvitationRepo) FindUnusedByUsername(username string) (*model.Invitation, error) {
	var inv model.Invitation
	err := r.db.Where("invited_username = ? AND used = false", username).First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// MarkUsed 将邀请标记为已使用
func (r *InvitationRepo) MarkUsed(id int) error {
	return r.db.Model(&model.Invitation{}).Where("id = ?", id).Update("used", true).Error
}

// HasAnyUser 检查是否存在任何用户（用于判断是否为首个用户）
func (r *InvitationRepo) HasAnyUser() (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Count(&count).Error
	return count > 0, err
}
