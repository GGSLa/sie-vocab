package repo

import (
	"gorm.io/gorm"
)

// FreeReviewLog GORM model for free_review_logs table
type FreeReviewLog struct {
	ID     int `gorm:"primaryKey;column:id"`
	WordID int `gorm:"column:word_id"`
}

// TableName overrides the default table name
func (FreeReviewLog) TableName() string { return "free_review_logs" }

// FreeReviewLogRepo 管理 free_review_logs 表的 CRUD（自由模式复习记录）
type FreeReviewLogRepo struct {
	db *gorm.DB
}

// NewFreeReviewLogRepo 创建 FreeReviewLogRepo
func NewFreeReviewLogRepo(db *gorm.DB) *FreeReviewLogRepo {
	return &FreeReviewLogRepo{db: db}
}

// Insert 插入一条自由复习记录
func (r *FreeReviewLogRepo) Insert(wordID int) error {
	return r.db.Create(&FreeReviewLog{WordID: wordID}).Error
}
