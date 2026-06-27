package logic

import (
	"fmt"
	"strings"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// InviteHandler 邀请业务逻辑
type InviteHandler struct {
	invitationRepo *repo.InvitationRepo
	userRepo       *repo.UserRepo
}

// NewInviteHandler 创建 InviteHandler
func NewInviteHandler(invitationRepo *repo.InvitationRepo, userRepo *repo.UserRepo) *InviteHandler {
	return &InviteHandler{invitationRepo: invitationRepo, userRepo: userRepo}
}

// Invite 已有用户邀请新用户名
func (h *InviteHandler) Invite(inviterUserID int, invitedUsername string) (*model.InviteResponse, error) {
	invitedUsername = strings.TrimSpace(invitedUsername)

	if len(invitedUsername) < 3 || len(invitedUsername) > 50 {
		return &model.InviteResponse{Error: "用户名需要 3-50 个字符"}, nil
	}

	// 检查是否已被注册
	existing, err := h.userRepo.FindByUsername(invitedUsername)
	if err == nil && existing != nil {
		return &model.InviteResponse{Error: "该用户名已被注册"}, nil
	}

	// 检查是否已有未使用的邀请
	_, err = h.invitationRepo.FindUnusedByUsername(invitedUsername)
	if err == nil {
		return &model.InviteResponse{Error: "该用户名已被邀请，等待注册"}, nil
	}

	// 创建邀请
	if err := h.invitationRepo.Create(inviterUserID, invitedUsername); err != nil {
		return nil, fmt.Errorf("创建邀请失败: %v", err)
	}

	return &model.InviteResponse{
		Success:  true,
		Username: invitedUsername,
	}, nil
}
