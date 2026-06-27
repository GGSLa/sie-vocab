package logic

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
)

// ReaderBreakdownHandler 句子拆解业务编排
type ReaderBreakdownHandler struct {
	apiKey string
}

// NewReaderBreakdownHandler 创建 ReaderBreakdownHandler
func NewReaderBreakdownHandler(apiKey string) *ReaderBreakdownHandler {
	return &ReaderBreakdownHandler{apiKey: apiKey}
}

// Breakdown 对用户选中的句子做深度拆解（不缓存，实时调 DeepSeek）
func (h *ReaderBreakdownHandler) Breakdown(sentence string) (*model.BreakdownSentenceResponse, error) {
	log.Printf("🔍 句子拆解请求: len=%d text=%.80s", len(sentence), sentence)

	reply, err := client.CallDeepSeekWithSystem(h.apiKey, model.BreakdownSystemPrompt, sentence)
	if err != nil {
		return nil, fmt.Errorf("AI 调用失败: %v", err)
	}

	result, err := parseBreakdownReply(reply)
	if err != nil {
		log.Printf("❌ 解析句子拆解回复失败: %v\n原始回复: %.200s", err, reply)
		return &model.BreakdownSentenceResponse{
			Error: "AI 回复解析失败，请重试",
		}, nil
	}

	log.Printf("✅ 句子拆解完成: vocab=%d phrases=%d grammar=%d",
		len(result.Vocabulary), len(result.Phrases), len(result.Grammar))
	return result, nil
}

func parseBreakdownReply(reply string) (*model.BreakdownSentenceResponse, error) {
	var result model.BreakdownSentenceResponse
	if err := json.Unmarshal([]byte(reply), &result); err == nil {
		return &result, nil
	}
	cleaned := reply
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("解析 AI 回复 JSON 失败: %v", err)
	}
	return &result, nil
}
