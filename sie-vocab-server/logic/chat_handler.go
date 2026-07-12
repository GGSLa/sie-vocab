package logic

import (
	"encoding/json"
	"log"
	"strings"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ChatHandler AI 翻译业务编排
type ChatHandler struct {
	apiKey          string
	globalCacheRepo *repo.GlobalWordCacheRepo
}

// NewChatHandler 创建 ChatHandler
func NewChatHandler(apiKey string, globalCacheRepo *repo.GlobalWordCacheRepo) *ChatHandler {
	return &ChatHandler{apiKey: apiKey, globalCacheRepo: globalCacheRepo}
}

// Chat 调用 DeepSeek 翻译单词，返回 AI 原始回复
func (h *ChatHandler) Chat(message string) (string, error) {
	reply, err := client.CallDeepSeek(h.apiKey, message)
	if err != nil {
		return "", err
	}

	// 解析 AI 回复，将首次出现的词写入全局缓存（已存在则跳过）
	words := parseAIWords(reply)
	for _, w := range words {
		if err := h.globalCacheRepo.InsertWord(w); err != nil {
			log.Printf("[cache] 全局缓存写入失败 %q: %v", w.Word, err)
		}
	}
	if len(words) > 0 {
		log.Printf("[cache] AI 翻译结果已尝试写入全局缓存: %d 词", len(words))
	}

	return reply, nil
}

// parseAIWords 解析 AI 回复中的 words 数组，前端同款逻辑
func parseAIWords(reply string) []model.WordEntry {
	// 尝试直接解析
	var resp model.WordsResponse
	if json.Unmarshal([]byte(reply), &resp) == nil && len(resp.Words) > 0 {
		return resp.Words
	}
	// 去掉 markdown 代码围栏后重试
	cleaned := strings.TrimSpace(reply)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	if json.Unmarshal([]byte(cleaned), &resp) == nil && len(resp.Words) > 0 {
		return resp.Words
	}
	return nil
}
