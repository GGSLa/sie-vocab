package logic

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"gorm.io/gorm"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
	"sie-vocab-server/pdf"
	"sie-vocab-server/repo"
)

// ReaderChunkHandler 阅读页 AI 分析业务编排
type ReaderChunkHandler struct {
	apiKey    string
	pdfPath   string
	cacheRepo *repo.ReaderCacheRepo

	// in-flight request tracker: page → channel that closes when AI completes
	flights   map[int]chan struct{}
	flightsMu sync.Mutex
}

// NewReaderChunkHandler 创建 ReaderChunkHandler
func NewReaderChunkHandler(apiKey, pdfPath string, cacheRepo *repo.ReaderCacheRepo) *ReaderChunkHandler {
	return &ReaderChunkHandler{
		apiKey:    apiKey,
		pdfPath:   pdfPath,
		cacheRepo: cacheRepo,
		flights:   make(map[int]chan struct{}),
	}
}

// GetChunk 获取指定页的 AI 分析结果（含缓存、去重、跨页补全）
func (h *ReaderChunkHandler) GetChunk(page int) (*model.ReaderChunkResponse, error) {
	log.Printf("📖 阅读请求: page=%d", page)

	// 1. Check cache
	cached, err := h.cacheRepo.FindByPage(page)
	if err == nil && cached != nil {
		log.Printf("✅ reader 缓存命中: page=%d, chunks=%d", page, cached.TotalChunks)
		return cached, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("⚠️ 查询 reader_cache 失败: %v", err)
	}

	// 2. Deduplicate concurrent requests for the same page
	h.flightsMu.Lock()
	if ch, exists := h.flights[page]; exists {
		h.flightsMu.Unlock()
		log.Printf("⏳ reader flight wait: page=%d (another request in progress)", page)
		<-ch
		if cached2, err2 := h.cacheRepo.FindByPage(page); err2 == nil && cached2 != nil {
			log.Printf("✅ reader 缓存命中 (after wait): page=%d, chunks=%d", page, cached2.TotalChunks)
			return cached2, nil
		}
	} else {
		ch := make(chan struct{})
		h.flights[page] = ch
		h.flightsMu.Unlock()

		defer func() {
			h.flightsMu.Lock()
			delete(h.flights, page)
			h.flightsMu.Unlock()
			close(ch)
		}()
	}

	// 3. Extract current page text
	pageText, err := pdf.ExtractPageTextStructured(h.pdfPath, page)
	if err != nil {
		return nil, fmt.Errorf("PDF 提取失败: %v", err)
	}
	if pageText == "" {
		return &model.ReaderChunkResponse{
			Page:    page,
			PageEnd: page + 1,
			Error:   "该页无文本内容",
		}, nil
	}

	// 4. Cross-page completion
	pageText = strings.TrimSpace(pageText)
	if needsCrossPageCompletion(pageText) {
		nextText, err := pdf.ExtractPageTextStructured(h.pdfPath, page+1)
		if err == nil && nextText != "" {
			nextText = strings.TrimSpace(nextText)
			firstPara := extractFirstBodyParagraph(nextText)
			if firstPara != "" {
				pageText += "\n" + firstPara
				log.Printf("📎 跨页段落补齐: page=%d, 从 page=%d 取了 %d 字", page, page+1, len(firstPara))
			}
		}
	}

	log.Printf("📄 PDF 提取: page=%d, 总长=%d", page, len(pageText))

	// 5. Call DeepSeek
	reply, err := client.CallDeepSeekWithSystem(h.apiKey, model.ReaderSystemPrompt, pageText)
	if err != nil {
		return nil, fmt.Errorf("AI 分析失败: %v", err)
	}

	// 6. Parse response
	result, err := parseReaderReply(reply)
	if err != nil {
		log.Printf("❌ 解析 DeepSeek 回复失败: %v\n原始回复: %.200s", err, reply)
		return &model.ReaderChunkResponse{
			Page:    page,
			PageEnd: page + 1,
			Error:   "AI 回复解析失败，请重试",
		}, nil
	}
	result.Page = page
	result.PageEnd = page + 1
	result.TotalChunks = len(result.Chunks)

	// 7. Cache
	if err := h.cacheRepo.SavePage(page, result.Section, pageText, result); err != nil {
		log.Printf("❌ 保存 reader_cache 失败: %v", err)
	}

	log.Printf("✅ reader 分析完成: page=%d section=%q chunks=%d", page, result.Section, result.TotalChunks)
	return result, nil
}

// ---------- 跨页辅助（从 service 层迁移） ----------

func needsCrossPageCompletion(pageText string) bool {
	if pageText == "" || strings.HasSuffix(pageText, "\n\n") {
		return false
	}
	last := lastLine(pageText)
	if last == "" || strings.HasPrefix(last, "#") {
		return false
	}
	if isSentenceEnd(last) {
		return false
	}
	return true
}

func isSentenceEnd(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '.' || last == '!' || last == '?' || last == '"' || last == ')' || last == ':'
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func extractFirstBodyParagraph(s string) string {
	lines := strings.Split(s, "\n")
	var paraLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(paraLines) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if len(paraLines) > 0 {
				break
			}
			continue
		}
		paraLines = append(paraLines, trimmed)
	}
	return strings.Join(paraLines, " ")
}

func parseReaderReply(reply string) (*model.ReaderChunkResponse, error) {
	var result model.ReaderChunkResponse
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
