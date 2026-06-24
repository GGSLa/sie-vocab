package repo

import (
	"encoding/json"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sie-vocab-server/model"
)

// ReaderProgress GORM model for reader_progress table
type ReaderProgress struct {
	UserID            int    `gorm:"primaryKey;column:user_id"`
	BookID            int    `gorm:"primaryKey;column:book_id"`
	CurrentPage       int    `gorm:"column:current_page"`
	CurrentChunk      int    `gorm:"column:current_chunk"`
	CurrentSection    string `gorm:"column:current_section"`
	CompletedSections string `gorm:"column:completed_sections"` // JSON array
	LastRead          string `gorm:"column:last_read"`           // YYYY-MM-DD
}

// TableName overrides the default table name
func (ReaderProgress) TableName() string { return "reader_progress" }

// ReaderProgressRepo 管理阅读进度的 DB 持久化
type ReaderProgressRepo struct {
	db *gorm.DB
}

// NewReaderProgressRepo 创建 ReaderProgressRepo
func NewReaderProgressRepo(db *gorm.DB) *ReaderProgressRepo {
	return &ReaderProgressRepo{db: db}
}

// Load 读取指定书的阅读进度，若无记录则返回默认值
func (r *ReaderProgressRepo) Load(bookID int, userID int) (*model.ReaderProgress, error) {
	var rp ReaderProgress
	err := r.db.Where("user_id = ? AND book_id = ?", userID, bookID).First(&rp).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &model.ReaderProgress{
				BookID:            bookID,
				CurrentPage:       1,
				CurrentChunk:      0,
				CurrentSection:    "",
				CompletedSections: []string{},
				LastRead:          "",
			}, nil
		}
		return nil, err
	}
	var completed []string
	if rp.CompletedSections != "" {
		json.Unmarshal([]byte(rp.CompletedSections), &completed)
	}
	if completed == nil {
		completed = []string{}
	}
	return &model.ReaderProgress{
		BookID:            rp.BookID,
		CurrentPage:       rp.CurrentPage,
		CurrentChunk:      rp.CurrentChunk,
		CurrentSection:    rp.CurrentSection,
		CompletedSections: completed,
		LastRead:          rp.LastRead,
	}, nil
}

// FindLastReadBookID 返回最近阅读的 book_id（按 last_read DESC），无记录返回 0
func (r *ReaderProgressRepo) FindLastReadBookID(userID int) (int, error) {
	var rp ReaderProgress
	err := r.db.Where("user_id = ?", userID).Order("last_read DESC").First(&rp).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}
	return rp.BookID, nil
}

// Save 保存阅读进度（UPSERT）
func (r *ReaderProgressRepo) Save(bookID int, progress *model.ReaderProgress, userID int) error {
	completedJSON, _ := json.Marshal(progress.CompletedSections)
	rp := ReaderProgress{
		UserID:            userID,
		BookID:            bookID,
		CurrentPage:       progress.CurrentPage,
		CurrentChunk:      progress.CurrentChunk,
		CurrentSection:    progress.CurrentSection,
		CompletedSections: string(completedJSON),
		LastRead:          progress.LastRead,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "book_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"current_page", "current_chunk", "current_section",
			"completed_sections", "last_read",
		}),
	}).Create(&rp).Error
}
