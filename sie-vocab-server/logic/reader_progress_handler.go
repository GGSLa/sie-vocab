package logic

import (
	"time"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReaderProgressHandler 阅读进度加载/保存业务编排（DB 持久化）
type ReaderProgressHandler struct {
	progressRepo *repo.ReaderProgressRepo
}

// NewReaderProgressHandler 创建 ReaderProgressHandler
func NewReaderProgressHandler(progressRepo *repo.ReaderProgressRepo) *ReaderProgressHandler {
	return &ReaderProgressHandler{progressRepo: progressRepo}
}

// Load 从 DB 加载阅读进度（无记录时返回默认值）
func (h *ReaderProgressHandler) Load(bookID int) (*model.ReaderProgress, error) {
	return h.progressRepo.Load(bookID)
}

// Save 保存阅读进度到 DB
func (h *ReaderProgressHandler) Save(bookID, currentPage, currentChunk int, section string) error {
	p, err := h.progressRepo.Load(bookID)
	if err != nil {
		return err
	}
	p.BookID = bookID
	p.CurrentPage = currentPage
	p.CurrentChunk = currentChunk
	if section != "" {
		p.CurrentSection = section
	}
	p.LastRead = time.Now().Format("2006-01-02")
	return h.progressRepo.Save(bookID, p)
}
