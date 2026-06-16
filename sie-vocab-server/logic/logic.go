package logic

import (
	"encoding/json"
	"fmt"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// Translate 调用 AI 翻译单词并解析为结构化结果
func Translate(word, apiKey string) (*model.WordsResponse, error) {
	reply, err := client.CallDeepSeek(apiKey, word)
	if err != nil {
		return nil, fmt.Errorf("AI 翻译失败: %v", err)
	}
	return parseReplyJSON(reply)
}

// GetOrTranslate 先查数据库，无结果则调用 AI 翻译
func GetOrTranslate(word, apiKey string) (*model.WordsResponse, error) {
	words, err := repo.QueryWordFamily(word)
	if err != nil {
		return nil, fmt.Errorf("数据库查询失败: %v", err)
	}
	if len(words) > 0 {
		return &model.WordsResponse{Words: words}, nil
	}
	return Translate(word, apiKey)
}

func parseReplyJSON(reply string) (*model.WordsResponse, error) {
	var result model.WordsResponse

	// 直接解析
	if err := json.Unmarshal([]byte(reply), &result); err == nil {
		return &result, nil
	}

	// 尝试剥离 ```json ... ``` 包裹
	cleaned := reply
	for len(cleaned) > 0 && cleaned[0] != '{' {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] != '}' {
		cleaned = cleaned[:len(cleaned)-1]
	}

	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("解析 AI 返回 JSON 失败: %v", err)
	}
	return &result, nil
}
