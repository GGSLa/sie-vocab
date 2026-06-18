package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"sie-vocab-server/model"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// CallDeepSeek 调用 DeepSeek API 进行单词翻译
func CallDeepSeek(apiKey, message string) (string, error) {
	return CallDeepSeekWithSystem(apiKey, model.SystemPrompt, message)
}

// CallDeepSeekWithSystem 调用 DeepSeek API，使用自定义 system prompt
func CallDeepSeekWithSystem(apiKey, systemPrompt, message string) (string, error) {
	body := map[string]interface{}{
		"model": "deepseek-chat",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": message},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", model.DeepSeekURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("DeepSeek 返回空响应")
	}

	return result.Choices[0].Message.Content, nil
}
