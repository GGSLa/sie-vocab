package repo

import (
	"gorm.io/gorm"
)

// Meaning GORM model for meanings table
type Meaning struct {
	ID     int    `gorm:"primaryKey;column:id"`
	WordID int    `gorm:"column:word_id"`
	Domain string `gorm:"column:domain"`
	Text   string `gorm:"column:text"`
}

// TableName overrides the default table name
func (Meaning) TableName() string { return "meanings" }

// MeaningRepo 管理 meanings 表的 CRUD
type MeaningRepo struct {
	db *gorm.DB
}

// NewMeaningRepo 创建 MeaningRepo
func NewMeaningRepo(db *gorm.DB) *MeaningRepo {
	return &MeaningRepo{db: db}
}

// FindByWordID 查找某单词的所有释义
func (r *MeaningRepo) FindByWordID(wordID int) ([]Meaning, error) {
	var meanings []Meaning
	err := r.db.Where("word_id = ?", wordID).Find(&meanings).Error
	return meanings, err
}

// DeleteByWordID 删除某单词的所有释义
func (r *MeaningRepo) DeleteByWordID(tx *gorm.DB, wordID int) error {
	return tx.Where("word_id = ?", wordID).Delete(&Meaning{}).Error
}

// BatchInsert 批量插入释义
func (r *MeaningRepo) BatchInsert(tx *gorm.DB, meanings []Meaning) error {
	if len(meanings) == 0 {
		return nil
	}
	return tx.Create(&meanings).Error
}
