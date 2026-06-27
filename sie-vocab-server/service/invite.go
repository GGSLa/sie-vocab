package service

import (
	"encoding/json"
	"log"
	"net/http"

	"sie-vocab-server/logic"
	"sie-vocab-server/model"
)

// HandleInvite 创建邀请（需认证）
func HandleInvite(h *logic.InviteHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.InviteResponse{Error: "只接受 POST 请求"})
			return
		}
		var req model.InviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.InviteResponse{Error: "请求格式错误"})
			return
		}
		if req.Username == "" {
			writeJSON(w, http.StatusBadRequest, model.InviteResponse{Error: "用户名不能为空"})
			return
		}

		userID := getUserID(r)
		resp, err := h.Invite(userID, req.Username)
		if err != nil {
			log.Printf("❌ 创建邀请失败 (inviter=%d, target=%s): %v", userID, req.Username, err)
			writeJSON(w, http.StatusInternalServerError, model.InviteResponse{Error: "创建邀请失败，请稍后重试"})
			return
		}
		if resp.Error != "" {
			writeJSON(w, http.StatusBadRequest, resp)
			return
		}
		log.Printf("📨 用户 %d 邀请了 %s", userID, req.Username)
		writeJSON(w, http.StatusOK, resp)
	}
}
