package repo

import (
	"encoding/json"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sie-vocab-server/model"
)

// ReaderCache GORM model for reader_cache table (composite PK: book_id + page)
type ReaderCache struct {
	BookID       int    `gorm:"primaryKey;column:book_id"`
	Page         int    `gorm:"primaryKey;column:page"`
	SectionTitle string `gorm:"column:section_title"`
	RawText      string `gorm:"column:raw_text"`
	AIResponse   string `gorm:"column:ai_response"`
}

// TableName overrides the default table name
func (ReaderCache) TableName() string { return "reader_cache" }

// TocEntry holds a single TOC item (one cached page).
type TocEntry struct {
	Page    int    `json:"page"`
	Section string `json:"section"`
}

// ReaderCacheRepo 管理 reader_cache 表的 CRUD（AI 分析缓存）
type ReaderCacheRepo struct {
	db *gorm.DB
}

// NewReaderCacheRepo 创建 ReaderCacheRepo
func NewReaderCacheRepo(db *gorm.DB) *ReaderCacheRepo {
	return &ReaderCacheRepo{db: db}
}

// FindByPage 按 book_id + 页码查找缓存
func (r *ReaderCacheRepo) FindByPage(bookID, page int) (*model.ReaderChunkResponse, error) {
	var c ReaderCache
	err := r.db.Where("book_id = ? AND page = ?", bookID, page).First(&c).Error
	if err != nil {
		return nil, err
	}
	var result model.ReaderChunkResponse
	if err := json.Unmarshal([]byte(c.AIResponse), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SavePage 缓存一页的 AI 分析结果（UPSERT on book_id+page）
func (r *ReaderCacheRepo) SavePage(bookID, page int, sectionTitle, rawText string, response *model.ReaderChunkResponse) error {
	response.BookID = bookID
	jsonBytes, _ := json.Marshal(response)
	c := ReaderCache{
		BookID:       bookID,
		Page:         page,
		SectionTitle: sectionTitle,
		RawText:      rawText,
		AIResponse:   string(jsonBytes),
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "book_id"}, {Name: "page"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"section_title", "raw_text", "ai_response",
		}),
	}).Create(&c).Error
}

// AllCachedPages 获取指定书的所有已缓存页面
func (r *ReaderCacheRepo) AllCachedPages(bookID int) ([]TocEntry, error) {
	var entries []TocEntry
	err := r.db.Model(&ReaderCache{}).
		Where("book_id = ?", bookID).
		Select("page, section_title as section").
		Order("page ASC").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []TocEntry{}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Page < entries[j].Page })
	return entries, nil
}

// FindByContentHash 通过内容哈希跨书查找缓存（相同 PDF 不同 book_id 共享）
func (r *ReaderCacheRepo) FindByContentHash(hash string, page int) (*model.ReaderChunkResponse, error) {
	if hash == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var c ReaderCache
	err := r.db.Table("reader_cache rc").
		Select("rc.*").
		Joins("JOIN books b ON rc.book_id = b.id").
		Where("b.content_hash = ? AND rc.page = ?", hash, page).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	var result model.ReaderChunkResponse
	if err := json.Unmarshal([]byte(c.AIResponse), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AllCachedPagesByHash 通过内容哈希获取所有同内容书的已缓存页面（去重）
func (r *ReaderCacheRepo) AllCachedPagesByHash(hash string) ([]TocEntry, error) {
	if hash == "" {
		return []TocEntry{}, nil
	}
	var entries []TocEntry
	err := r.db.Table("reader_cache rc").
		Select("rc.page, rc.section_title as section").
		Joins("JOIN books b ON rc.book_id = b.id").
		Where("b.content_hash = ?", hash).
		Order("rc.page ASC").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []TocEntry{}
	}
	// Deduplicate by page (multiple books with same hash may have same page cached)
	seen := make(map[int]bool)
	deduped := make([]TocEntry, 0, len(entries))
	for _, e := range entries {
		if !seen[e.Page] {
			seen[e.Page] = true
			deduped = append(deduped, e)
		}
	}
	sort.Slice(deduped, func(i, j int) bool { return deduped[i].Page < deduped[j].Page })
	return deduped, nil
}
