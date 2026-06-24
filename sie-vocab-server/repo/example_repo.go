package repo

import (
	"gorm.io/gorm"
)

// Example GORM model for examples table
type Example struct {
	ID        int    `gorm:"primaryKey;column:id"`
	UserID    int    `gorm:"column:user_id;index"`
	WordID    int    `gorm:"column:word_id"`
	En        string `gorm:"column:en"`
	Zh        string `gorm:"column:zh"`
	SortOrder int    `gorm:"column:sort_order;default:0"`
}

// TableName overrides the default table name
func (Example) TableName() string { return "examples" }

// ExampleRepo 管理 examples 表的 CRUD
type ExampleRepo struct {
	db *gorm.DB
}

// NewExampleRepo 创建 ExampleRepo
func NewExampleRepo(db *gorm.DB) *ExampleRepo {
	return &ExampleRepo{db: db}
}

// FindByWordID 查找某单词的所有例句，按 sort_order 排序
func (r *ExampleRepo) FindByWordID(wordID int, userID int) ([]Example, error) {
	var examples []Example
	err := r.db.Where("word_id = ? AND user_id = ?", wordID, userID).Order("sort_order").Find(&examples).Error
	return examples, err
}

// DeleteByWordID 删除某单词的所有例句
func (r *ExampleRepo) DeleteByWordID(tx *gorm.DB, wordID int, userID int) error {
	return tx.Where("word_id = ? AND user_id = ?", wordID, userID).Delete(&Example{}).Error
}

// BatchInsert 批量插入例句
func (r *ExampleRepo) BatchInsert(tx *gorm.DB, examples []Example) error {
	if len(examples) == 0 {
		return nil
	}
	return tx.Create(&examples).Error
}
