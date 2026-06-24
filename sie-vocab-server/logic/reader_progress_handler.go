package logic

import (
	"fmt"
	"time"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReaderProgressHandler 阅读进度加载/保存业务编排（DB 持久化）
type ReaderProgressHandler struct {
	progressRepo *repo.ReaderProgressRepo
	bookRepo     *repo.BookRepo
}

// NewReaderProgressHandler 创建 ReaderProgressHandler
func NewReaderProgressHandler(progressRepo *repo.ReaderProgressRepo, bookRepo *repo.BookRepo) *ReaderProgressHandler {
	return &ReaderProgressHandler{progressRepo: progressRepo, bookRepo: bookRepo}
}

// GetDefaultBook 获取默认书籍：优先最近阅读的，否则第一本，无书则返回 error
func (h *ReaderProgressHandler) GetDefaultBook() (*model.ReaderProgress, error) {
	// 1. 尝试找到最近阅读的书籍
	lastBookID, err := h.progressRepo.FindLastReadBookID()
	if err != nil {
		return nil, fmt.Errorf("查询最近阅读记录失败: %v", err)
	}
	if lastBookID > 0 {
		return h.progressRepo.Load(lastBookID)
	}

	// 2. 无阅读记录，选第一本书
	books, err := h.bookRepo.FindAll()
	if err != nil {
		return nil, fmt.Errorf("查询书籍列表失败: %v", err)
	}
	if len(books) == 0 {
		return nil, fmt.Errorf("没有书籍")
	}

	return h.progressRepo.Load(books[0].ID)
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
