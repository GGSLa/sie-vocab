package logic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
		log.Printf("❌ 解析句子拆解回复失败: %v (响应长度=%d)", err, len(reply))
		// 保存完整原始回复到文件方便排查 / Save raw reply for debugging
		os.WriteFile("/tmp/breakdown_raw.txt", []byte(reply), 0644)
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

	// 1. Try raw parse
	if err := json.Unmarshal([]byte(reply), &result); err == nil {
		return &result, nil
	}

	// 2. Strip markdown code fences
	cleaned := strings.TrimSpace(reply)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	// 3. Extract JSON using brace counting
	start := strings.Index(cleaned, "{")
	if start < 0 {
		return nil, fmt.Errorf("未找到 JSON 开头")
	}
	depth := 0
	end := -1
	for i := start; i < len(cleaned); i++ {
		ch := cleaned[i]
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		} else if ch == '"' {
			i++
			for i < len(cleaned) {
				if cleaned[i] == '\\' {
					i += 2
				} else if cleaned[i] == '"' {
					break
				} else {
					i++
				}
			}
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("未找到匹配的 JSON 结束括号")
	}
	cleaned = cleaned[start : end+1]

	// 4. Try parsing cleaned JSON
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil {
		return &result, nil
	}

	// 5. Escape literal control chars inside JSON strings (AI sometimes outputs real newlines)
	sanitized := sanitizeJSONStrings(cleaned)
	if err := json.Unmarshal([]byte(sanitized), &result); err != nil {
		return nil, fmt.Errorf("解析 AI 回复 JSON 失败: %v", err)
	}
	return &result, nil
}

// sanitizeJSONStrings escapes literal control characters (\n \r \t) that
// appear inside JSON string values. DeepSeek sometimes outputs real newlines
// inside long text fields like usage_notes, which breaks JSON parsing.
func sanitizeJSONStrings(raw string) string {
	var buf bytes.Buffer
	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' && inString {
			buf.WriteByte(ch)
			escaped = true
			continue
		}

		if ch == '"' {
			inString = !inString
			buf.WriteByte(ch)
			continue
		}

		if inString {
			switch ch {
			case '\n':
				buf.WriteString("\\n")
			case '\r':
				buf.WriteString("\\r")
			case '\t':
				buf.WriteByte(ch) // tabs are valid in JSON
			default:
				buf.WriteByte(ch)
			}
		} else {
			buf.WriteByte(ch)
		}
	}

	return buf.String()
}
