package logic

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"sie-vocab-server/auth"
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// AuthHandler 用户认证业务逻辑
type AuthHandler struct {
	userRepo       *repo.UserRepo
	invitationRepo *repo.InvitationRepo
	jwtSecret      string
}

// NewAuthHandler 创建 AuthHandler
func NewAuthHandler(userRepo *repo.UserRepo, invitationRepo *repo.InvitationRepo, jwtSecret string) *AuthHandler {
	return &AuthHandler{userRepo: userRepo, invitationRepo: invitationRepo, jwtSecret: jwtSecret}
}

// Register 注册新用户
func (h *AuthHandler) Register(username, password string) (*model.RegisterResponse, error) {
	username = strings.TrimSpace(username)

	// 验证用户名
	if len(username) < 3 || len(username) > 50 {
		return &model.RegisterResponse{Error: "用户名需要 3-50 个字符"}, nil
	}

	// 验证密码长度
	if len(password) < 6 {
		return &model.RegisterResponse{Error: "密码至少需要 6 个字符"}, nil
	}

	// 检查用户名是否已存在
	existing, err := h.userRepo.FindByUsername(username)
	if err == nil && existing != nil {
		return &model.RegisterResponse{Error: "用户名已被注册"}, nil
	}

	// 检查邀请：首个用户无需邀请，后续用户必须被邀请
	hasAny, err := h.invitationRepo.HasAnyUser()
	if err != nil {
		return nil, fmt.Errorf("检查用户数失败: %v", err)
	}
	var pendingInvitation *model.Invitation
	if hasAny {
		// 非首个用户，必须检查邀请
		inv, err := h.invitationRepo.FindUnusedByUsername(username)
		if err != nil || inv == nil {
			return &model.RegisterResponse{Error: "该用户名未被邀请注册，请联系已有用户获取邀请"}, nil
		}
		pendingInvitation = inv
	}

	// 哈希密码
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码哈希失败: %v", err)
	}

	// 创建用户
	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
	}
	if err := h.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %v", err)
	}

	// 非首个用户：标记邀请为已使用
	if pendingInvitation != nil {
		h.invitationRepo.MarkUsed(pendingInvitation.ID)
	}

	// 如果是第一个用户，将孤儿数据分配给他
	totalUsers, _ := h.userRepo.CountAll()
	if totalUsers == 1 {
		if err := h.userRepo.ClaimOrphanedData(user.ID); err != nil {
			// 不阻塞注册，仅记录日志
		}
	}

	// 生成 JWT
	token, err := auth.GenerateToken(user.ID, h.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("生成 token 失败: %v", err)
	}

	return &model.RegisterResponse{
		Token:    token,
		Username: username,
	}, nil
}

// Login 用户登录
func (h *AuthHandler) Login(username, password string) (*model.LoginResponse, error) {
	username = strings.TrimSpace(username)

	// 查找用户
	user, err := h.userRepo.FindByUsername(username)
	if err != nil {
		return &model.LoginResponse{Error: "用户名或密码错误"}, nil
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return &model.LoginResponse{Error: "用户名或密码错误"}, nil
	}

	// 生成 JWT
	token, err := auth.GenerateToken(user.ID, h.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("生成 token 失败: %v", err)
	}

	return &model.LoginResponse{
		Token:    token,
		Username: username,
	}, nil
}

// GetUserInfo 获取当前用户信息
func (h *AuthHandler) GetUserInfo(userID int) (*model.UserInfoResponse, error) {
	var user model.User
	err := h.userRepo.FindByID(userID, &user)
	if err != nil {
		return nil, fmt.Errorf("用户不存在: %v", err)
	}
	return &model.UserInfoResponse{
		Username:  user.Username,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}
