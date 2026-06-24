package logic

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// validOCRLangs OCR 语言码白名单
var validOCRLangs = map[string]bool{
	"eng": true, "chi_sim": true, "chi_tra": true,
	"jpn": true, "kor": true, "fra": true, "deu": true, "spa": true,
	"chi_sim+eng": true, "chi_tra+eng": true,
}

// BookshelfHandler 书架管理业务编排
type BookshelfHandler struct {
	bookRepo       *repo.BookRepo
	progressRepo   *repo.ReaderProgressRepo
	cacheRepo      *repo.ReaderCacheRepo
	uploadDir      string
	defaultOCRLang string
}

// NewBookshelfHandler 创建 BookshelfHandler
func NewBookshelfHandler(
	bookRepo *repo.BookRepo,
	progressRepo *repo.ReaderProgressRepo,
	cacheRepo *repo.ReaderCacheRepo,
	uploadDir, defaultOCRLang string,
) *BookshelfHandler {
	return &BookshelfHandler{
		bookRepo:       bookRepo,
		progressRepo:   progressRepo,
		cacheRepo:      cacheRepo,
		uploadDir:      uploadDir,
		defaultOCRLang: defaultOCRLang,
	}
}

// List 获取所有书籍（含阅读进度）
func (h *BookshelfHandler) List(userID int) (*model.BookListResponse, error) {
	books, err := h.bookRepo.FindAll(userID)
	if err != nil {
		return nil, err
	}
	if books == nil {
		books = []model.Book{}
	}
	return &model.BookListResponse{Books: books}, nil
}

// GetSingle 获取单本书籍（含阅读进度）
func (h *BookshelfHandler) GetSingle(bookID int, userID int) (*model.BookWithProgress, error) {
	book, err := h.bookRepo.FindByID(bookID, userID)
	if err != nil {
		return nil, err
	}
	progress, _ := h.progressRepo.Load(bookID, userID)
	return &model.BookWithProgress{
		Book:           *book,
		CurrentPage:    progress.CurrentPage,
		CurrentChunk:   progress.CurrentChunk,
		CurrentSection: progress.CurrentSection,
	}, nil
}

// Create 保存上传的 PDF 并创建书籍记录
func (h *BookshelfHandler) Create(title, author, description, ocrLang string, pdfData []byte, userID int) (*model.Book, error) {
	if ocrLang == "" {
		ocrLang = h.defaultOCRLang
	}
	if !validOCRLangs[ocrLang] {
		log.Printf("⚠️ 非法的 OCR 语言: %q，回退到默认值 %q", ocrLang, h.defaultOCRLang)
		ocrLang = h.defaultOCRLang
	}
	if title == "" {
		title = "未命名教材"
	}

	// 确保上传目录存在
	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %v", err)
	}

	// 先在 DB 中创建记录（获取 ID）
	row := &repo.Book{
		UserID:      userID,
		Title:       title,
		Author:      author,
		Description: description,
		PDFPath:     "", // 稍后更新
		OCRLang:     ocrLang,
	}
	if err := h.bookRepo.Create(row); err != nil {
		return nil, fmt.Errorf("创建书籍记录失败: %v", err)
	}

	// 用 book ID 作为文件名，保存 PDF
	fileName := fmt.Sprintf("%d.pdf", row.ID)
	filePath := filepath.Join(h.uploadDir, fileName)
	if err := os.WriteFile(filePath, pdfData, 0644); err != nil {
		// 清理 DB 记录
		h.bookRepo.Delete(row.ID, userID)
		return nil, fmt.Errorf("保存 PDF 文件失败: %v", err)
	}

	// 更新 DB 中的 pdf_path 和 page_count
	pageCount := detectPageCount(filePath)
	if err := h.bookRepo.UpdatePDFInfo(row.ID, filePath, pageCount, userID); err != nil {
		log.Printf("⚠️ 更新 pdf_path 失败 (book=%d): %v", row.ID, err)
	}

	// 重新读取完整记录
	book, err := h.bookRepo.FindByID(row.ID, userID)
	if err != nil {
		return nil, err
	}

	log.Printf("📚 新书上架: id=%d title=%q pages=%d ocr=%s", book.ID, book.Title, book.PageCount, book.OCRLang)
	return book, nil
}

// Delete 删除书籍及其关联数据
func (h *BookshelfHandler) Delete(bookID int, userID int) error {
	book, err := h.bookRepo.FindByID(bookID, userID)
	if err != nil {
		return fmt.Errorf("书籍不存在: id=%d", bookID)
	}

	// 删除关联数据（cache + progress）
	if err := h.bookRepo.DeleteCacheByBook(bookID); err != nil {
		log.Printf("⚠️ 删除 reader_cache 失败 (book=%d): %v", bookID, err)
	}
	if err := h.bookRepo.DeleteProgressByBook(bookID, userID); err != nil {
		log.Printf("⚠️ 删除 reader_progress 失败 (book=%d): %v", bookID, err)
	}

	// 删除 DB 记录
	if err := h.bookRepo.Delete(bookID, userID); err != nil {
		return fmt.Errorf("删除书籍记录失败: %v", err)
	}

	// 删除 PDF 文件
	if book.PDFPath != "" {
		os.Remove(book.PDFPath)
	}

	log.Printf("🗑️ 书籍已删除: id=%d title=%q", bookID, book.Title)
	return nil
}

// detectPageCount 使用 pdfinfo 检测 PDF 总页数，失败时返回 0
func detectPageCount(pdfPath string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdfinfo", pdfPath)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ pdfinfo 失败 (path=%s): %v", pdfPath, err)
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				n, err := strconv.Atoi(parts[1])
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}
