package logic

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
	"sie-vocab-server/pdf"
	"sie-vocab-server/repo"
)

// flightKey 用于 in-flight 请求去重：{bookID, page}
type flightKey struct {
	bookID int
	page   int
}

// ReaderChunkHandler 阅读页 AI 分析业务编排
type ReaderChunkHandler struct {
	apiKey    string
	bookRepo  *repo.BookRepo
	cacheRepo *repo.ReaderCacheRepo

	// in-flight request tracker
	flights   map[flightKey]chan struct{}
	flightsMu sync.Mutex
}

// NewReaderChunkHandler 创建 ReaderChunkHandler
func NewReaderChunkHandler(apiKey string, bookRepo *repo.BookRepo, cacheRepo *repo.ReaderCacheRepo) *ReaderChunkHandler {
	return &ReaderChunkHandler{
		apiKey:    apiKey,
		bookRepo:  bookRepo,
		cacheRepo: cacheRepo,
		flights:   make(map[flightKey]chan struct{}),
	}
}

// GetChunk 获取指定书籍指定页的 AI 分析结果（含缓存、去重、跨页补全）
func (h *ReaderChunkHandler) GetChunk(bookID, page int, userID int) (*model.ReaderChunkResponse, error) {
	log.Printf("📖 阅读请求: book=%d page=%d", bookID, page)

	// 获取书籍信息（pdf路径 + OCR语言）
	book, err := h.bookRepo.FindByID(bookID, userID)
	if err != nil {
		return nil, fmt.Errorf("书籍不存在: book_id=%d", bookID)
	}
	pdfPath := book.PDFPath
	ocrLang := book.OCRLang
	if ocrLang == "" {
		ocrLang = "eng"
	}

	// 1. Check cache
	cached, err := h.cacheRepo.FindByPage(bookID, page)
	if err == nil && cached != nil {
		log.Printf("✅ reader 缓存命中: book=%d page=%d chunks=%d", bookID, page, cached.TotalChunks)
		cached.BookID = bookID
		return cached, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Printf("⚠️ 查询 reader_cache 失败: %v", err)
	}

	// 2. Deduplicate concurrent requests for the same (book, page)
	key := flightKey{bookID: bookID, page: page}
	h.flightsMu.Lock()
	if ch, exists := h.flights[key]; exists {
		h.flightsMu.Unlock()
		log.Printf("⏳ reader flight wait: book=%d page=%d (another request in progress)", bookID, page)
		select {
		case <-ch:
		case <-time.After(120 * time.Second):
			log.Printf("⚠️ reader flight timeout: book=%d page=%d", bookID, page)
			return nil, fmt.Errorf("请求超时，请稍后重试")
		}
		if cached2, err2 := h.cacheRepo.FindByPage(bookID, page); err2 == nil && cached2 != nil {
			log.Printf("✅ reader 缓存命中 (after wait): book=%d page=%d chunks=%d", bookID, page, cached2.TotalChunks)
			cached2.BookID = bookID
			return cached2, nil
		}
	} else {
		ch := make(chan struct{})
		h.flights[key] = ch
		h.flightsMu.Unlock()

		defer func() {
			h.flightsMu.Lock()
			delete(h.flights, key)
			h.flightsMu.Unlock()
			close(ch)
		}()
	}

	// 3. Extract current page text (hybrid: text layer preferred, OCR fallback)
	pageText, err := pdf.ExtractPageTextHybrid(pdfPath, page, ocrLang)
	if err != nil {
		return nil, fmt.Errorf("PDF 提取失败: %v", err)
	}
	if pageText == "" {
		return &model.ReaderChunkResponse{
			BookID:  bookID,
			Page:    page,
			PageEnd: page + 1,
			Error:   "该页无文本内容（文字层和 OCR 均未提取到内容）",
		}, nil
	}

	// 4. Cross-page paragraph handling
	// 4a. Check if first paragraph is continuation from previous page → remove it
	if page > 1 {
		prevText, err := pdf.ExtractPageTextHybrid(pdfPath, page-1, ocrLang)
		if err == nil && prevText != "" {
			lastParaPrev := pdf.GetLastParagraph(prevText)
			if lastParaPrev != "" && pdf.IsParagraphContinued(lastParaPrev) {
				trimmed := pdf.RemoveFirstParagraph(pageText)
				if trimmed != "" {
					log.Printf("📎 首段归前页: book=%d page=%d, 前页末段未闭合，本页首段移除 %d→%d 字", bookID, page, len(pageText), len(trimmed))
					pageText = trimmed
				}
			}
		}
	}

	// 4b. Check if last paragraph continues to next page → append continuation
	lastParaCurr := pdf.GetLastParagraph(pageText)
	if lastParaCurr != "" && pdf.IsParagraphContinued(lastParaCurr) {
		nextText, err := pdf.ExtractPageTextHybrid(pdfPath, page+1, ocrLang)
		if err == nil && nextText != "" {
			firstParaNext := pdf.GetFirstParagraph(nextText)
			if firstParaNext != "" {
				pageText += "\n" + firstParaNext
				log.Printf("📎 末段跨页补齐: book=%d page=%d, 从 page=%d 取了 %d 字", bookID, page, page+1, len(firstParaNext))
			}
		}
	}

	log.Printf("📄 PDF 提取: book=%d page=%d, 总长=%d", bookID, page, len(pageText))

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
			BookID:  bookID,
			Page:    page,
			PageEnd: page + 1,
			Error:   "AI 回复解析失败，请重试",
		}, nil
	}
	result.BookID = bookID
	result.Page = page
	result.PageEnd = page + 1
	result.TotalChunks = len(result.Chunks)

	// 7. Cache
	if err := h.cacheRepo.SavePage(bookID, page, result.Section, pageText, result); err != nil {
		log.Printf("❌ 保存 reader_cache 失败: %v", err)
	}

	log.Printf("✅ reader 分析完成: book=%d page=%d section=%q chunks=%d", bookID, page, result.Section, result.TotalChunks)
	return result, nil
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
