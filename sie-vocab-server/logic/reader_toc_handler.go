package logic

import (
	"log"

	"sie-vocab-server/pdf"
	"sie-vocab-server/repo"
)

// ReaderTOCHandler 目录大纲业务编排
type ReaderTOCHandler struct {
	pdfPath   string
	cacheRepo *repo.ReaderCacheRepo
}

// NewReaderTOCHandler 创建 ReaderTOCHandler
func NewReaderTOCHandler(pdfPath string, cacheRepo *repo.ReaderCacheRepo) *ReaderTOCHandler {
	return &ReaderTOCHandler{pdfPath: pdfPath, cacheRepo: cacheRepo}
}

// TOCResult 目录结果
type TOCResult struct {
	Outline     []pdf.TocItem     `json:"outline"`
	CachedPages map[int]bool      `json:"cached_pages"`
	Entries     []repo.TocEntry   `json:"entries,omitempty"`
}

// GetTOC 获取 PDF 大纲 + 已缓存页面标记
func (h *ReaderTOCHandler) GetTOC() (*TOCResult, error) {
	result := &TOCResult{}

	// Get PDF outline (built-in bookmarks)
	outline, err := pdf.ExtractOutline(h.pdfPath)
	if err != nil {
		log.Printf("⚠️ 提取 PDF 大纲失败，回退到缓存页面: %v", err)
		entries, err2 := h.cacheRepo.AllCachedPages()
		if err2 != nil {
			log.Printf("❌ 获取缓存页面也失败: %v", err2)
			return nil, err2
		}
		result.Entries = entries
		return result, nil
	}
	result.Outline = outline

	// Get cached page numbers for visual markers
	entries, _ := h.cacheRepo.AllCachedPages()
	cachedPages := make(map[int]bool)
	for _, e := range entries {
		cachedPages[e.Page] = true
	}
	result.CachedPages = cachedPages

	log.Printf("📑 TOC: %d 大纲条目, %d 已缓存页面", countOutlineItems(outline), len(cachedPages))
	return result, nil
}

func countOutlineItems(items []pdf.TocItem) int {
	n := len(items)
	for _, item := range items {
		n += countOutlineItems(item.Children)
	}
	return n
}
