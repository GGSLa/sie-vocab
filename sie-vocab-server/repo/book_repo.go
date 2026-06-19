package repo

import (
	"gorm.io/gorm"

	"sie-vocab-server/model"
)

// Book GORM model for books table
type Book struct {
	ID          int    `gorm:"primaryKey;column:id"`
	Title       string `gorm:"column:title"`
	Author      string `gorm:"column:author"`
	Description string `gorm:"column:description"`
	PDFPath     string `gorm:"column:pdf_path"`
	OCRLang     string `gorm:"column:ocr_lang"`
	PageCount   int    `gorm:"column:page_count"`
}

// TableName overrides the default table name
func (Book) TableName() string { return "books" }

// BookRepo 管理 books 表的 CRUD
type BookRepo struct {
	db *gorm.DB
}

// NewBookRepo 创建 BookRepo
func NewBookRepo(db *gorm.DB) *BookRepo {
	return &BookRepo{db: db}
}

// Create 插入新书
func (r *BookRepo) Create(b *Book) error {
	return r.db.Create(b).Error
}

// FindAll 查询所有书
func (r *BookRepo) FindAll() ([]model.Book, error) {
	var rows []Book
	err := r.db.Order("id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	books := make([]model.Book, len(rows))
	for i, row := range rows {
		books[i] = bookRowToModel(row)
	}
	return books, nil
}

// FindByID 按 ID 查书
func (r *BookRepo) FindByID(id int) (*model.Book, error) {
	var row Book
	err := r.db.Where("id = ?", id).First(&row).Error
	if err != nil {
		return nil, err
	}
	b := bookRowToModel(row)
	return &b, nil
}

// Delete 按 ID 删书
func (r *BookRepo) Delete(id int) error {
	return r.db.Where("id = ?", id).Delete(&Book{}).Error
}

// UpdatePDFInfo 更新书的 PDF 路径和总页数
func (r *BookRepo) UpdatePDFInfo(id int, pdfPath string, pageCount int) error {
	return r.db.Model(&Book{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pdf_path":   pdfPath,
		"page_count": pageCount,
	}).Error
}

// DeleteCacheByBook 删除指定书的所有 reader_cache 记录
func (r *BookRepo) DeleteCacheByBook(bookID int) error {
	return r.db.Where("book_id = ?", bookID).Delete(&ReaderCache{}).Error
}

// DeleteProgressByBook 删除指定书的阅读进度
func (r *BookRepo) DeleteProgressByBook(bookID int) error {
	return r.db.Where("book_id = ?", bookID).Delete(&ReaderProgress{}).Error
}

// bookRowToModel 将 GORM row 转为 model.Book
func bookRowToModel(row Book) model.Book {
	return model.Book{
		ID:          row.ID,
		Title:       row.Title,
		Author:      row.Author,
		Description: row.Description,
		PDFPath:     row.PDFPath,
		OCRLang:     row.OCRLang,
		PageCount:   row.PageCount,
	}
}
