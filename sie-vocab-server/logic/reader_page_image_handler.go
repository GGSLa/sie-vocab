package logic

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"sie-vocab-server/repo"
)

// ReaderPageImageHandler PDF 页图片渲染业务编排
type ReaderPageImageHandler struct {
	bookRepo *repo.BookRepo
}

// NewReaderPageImageHandler 创建 ReaderPageImageHandler
func NewReaderPageImageHandler(bookRepo *repo.BookRepo) *ReaderPageImageHandler {
	os.MkdirAll(pageImageCacheDir, 0755)
	return &ReaderPageImageHandler{bookRepo: bookRepo}
}

const pageImageCacheDir = "/tmp/sie-page-images"

// GetPageImage 获取指定书指定页的 PNG 图片（缓存到磁盘）
func (h *ReaderPageImageHandler) GetPageImage(bookID, page int) ([]byte, error) {
	book, err := h.bookRepo.FindByID(bookID)
	if err != nil {
		return nil, fmt.Errorf("书籍不存在: book_id=%d", bookID)
	}

	// 缓存路径: /tmp/sie-page-images/book-{id}-page-{n}.png
	cachePath := filepath.Join(pageImageCacheDir, fmt.Sprintf("book-%d-page-%d.png", bookID, page))

	// Check disk cache
	if data, err := os.ReadFile(cachePath); err == nil {
		return data, nil
	}

	// Render page to PNG using pdftoppm
	tmpPrefix := filepath.Join(pageImageCacheDir, fmt.Sprintf("tmp-book%d-p%d", bookID, page))

	cmd := exec.Command("pdftoppm",
		"-png", "-r", "150",
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		"-singlefile",
		book.PDFPath, tmpPrefix,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("❌ pdftoppm 渲染失败 (book=%d page=%d): %v\nstderr: %s", bookID, page, err, stderr.String())
		return nil, fmt.Errorf("PDF 渲染失败: %v", err)
	}

	// Read rendered image and cache
	tmpPath := tmpPrefix + ".png"
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("读取渲染图片失败: %v", err)
	}
	os.Rename(tmpPath, cachePath)

	log.Printf("🖼️ 渲染 PDF 页面: book=%d page=%d, size=%d bytes", bookID, page, len(data))
	return data, nil
}
