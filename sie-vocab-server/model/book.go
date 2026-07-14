package model

import "time"

// Book 书架中的一本 PDF 教材
type Book struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	PDFPath     string    `json:"pdf_path"`
	OCRLang     string    `json:"ocr_lang"`
	PageCount   int       `json:"page_count"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BookListResponse 书架列表 API 响应
type BookListResponse struct {
	Books []Book `json:"books"`
}

// BookWithProgress 带阅读进度的书籍信息
type BookWithProgress struct {
	Book
	CurrentPage    int    `json:"current_page"`
	CurrentChunk   int    `json:"current_chunk"`
	CurrentSection string `json:"current_section"`
}
