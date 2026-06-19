package logic

import (
	"sie-vocab-server/client"
)

// ChatHandler AI 翻译业务编排
type ChatHandler struct {
	apiKey string
}

// NewChatHandler 创建 ChatHandler
func NewChatHandler(apiKey string) *ChatHandler {
	return &ChatHandler{apiKey: apiKey}
}

// Chat 调用 DeepSeek 翻译单词，返回 AI 原始回复
func (h *ChatHandler) Chat(message string) (string, error) {
	return client.CallDeepSeek(h.apiKey, message)
}
