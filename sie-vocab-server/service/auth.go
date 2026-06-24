package service

import (
	"encoding/json"
	"log"
	"net/http"

	"sie-vocab-server/logic"
	"sie-vocab-server/model"
)

// HandleRegister 用户注册
func HandleRegister(h *logic.AuthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.RegisterResponse{Error: "只接受 POST 请求"})
			return
		}
		var req model.RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.RegisterResponse{Error: "请求格式错误"})
			return
		}
		if req.Username == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, model.RegisterResponse{Error: "用户名和密码不能为空"})
			return
		}
		resp, err := h.Register(req.Username, req.Password)
		if err != nil {
			log.Printf("❌ 注册失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, model.RegisterResponse{Error: "注册失败，请稍后重试"})
			return
		}
		if resp.Error != "" {
			writeJSON(w, http.StatusBadRequest, resp)
			return
		}
		log.Printf("👤 新用户注册: %s", resp.Username)
		writeJSON(w, http.StatusOK, resp)
	}
}

// HandleLogin 用户登录
func HandleLogin(h *logic.AuthHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.LoginResponse{Error: "只接受 POST 请求"})
			return
		}
		var req model.LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.LoginResponse{Error: "请求格式错误"})
			return
		}
		if req.Username == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, model.LoginResponse{Error: "用户名和密码不能为空"})
			return
		}
		resp, err := h.Login(req.Username, req.Password)
		if err != nil {
			log.Printf("❌ 登录失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, model.LoginResponse{Error: "登录失败，请稍后重试"})
			return
		}
		if resp.Error != "" {
			writeJSON(w, http.StatusUnauthorized, resp)
			return
		}
		log.Printf("🔑 用户登录: %s", resp.Username)
		writeJSON(w, http.StatusOK, resp)
	}
}
