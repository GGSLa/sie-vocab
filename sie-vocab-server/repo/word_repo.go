package repo

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sie-vocab-server/model"
)

// Word GORM model for words table
type Word struct {
	ID             int     `gorm:"primaryKey;column:id"`
	Word           string  `gorm:"column:word;uniqueIndex"`
	BaseWord       *string `gorm:"column:base_word"`
	Type           string  `gorm:"column:type"`
	Pos            string  `gorm:"column:pos"`
	Derivation     *string `gorm:"column:derivation"`
	ReviewCount    int     `gorm:"column:review_count;default:0"`
	NextReviewDate *string `gorm:"column:next_review_date"`
}

// TableName overrides the default table name
func (Word) TableName() string { return "words" }

// WordRepo 管理 words 表的 CRUD
type WordRepo struct {
	db *gorm.DB
}

// NewWordRepo 创建 WordRepo
func NewWordRepo(db *gorm.DB) *WordRepo {
	return &WordRepo{db: db}
}

// FindByWord 按单词拼写查找
func (r *WordRepo) FindByWord(word string) (*Word, error) {
	var w Word
	err := r.db.Where("word = ?", word).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// FindByID 按 ID 查找
func (r *WordRepo) FindByID(id int) (*Word, error) {
	var w Word
	err := r.db.Where("id = ?", id).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// FindByFamilyRoot 查找词族所有单词（word = root OR base_word = root）
// 结果按基础词优先排列
func (r *WordRepo) FindByFamilyRoot(root string) ([]Word, error) {
	var words []Word
	err := r.db.Where("word = ? OR base_word = ?", root, root).
		Order("CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id").
		Find(&words).Error
	return words, err
}

// UpsertWord INSERT ... ON DUPLICATE KEY UPDATE
func (r *WordRepo) UpsertWord(entry model.WordEntry) error {
	w := Word{
		Word:       entry.Word,
		BaseWord:   entry.BaseWord,
		Type:       entry.Type,
		Pos:        entry.Pos,
		Derivation: entry.Derivation,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "word"}},
		DoUpdates: clause.AssignmentColumns([]string{"base_word", "type", "pos", "derivation", "updated_at"}),
	}).Create(&w).Error
}

// GetIDByWord 按单词获取 ID
func (r *WordRepo) GetIDByWord(word string) (int, error) {
	var w Word
	err := r.db.Select("id").Where("word = ?", word).First(&w).Error
	if err != nil {
		return 0, err
	}
	return w.ID, nil
}

// FindFamilyRoots 获取所有基础词（用于随机抽词）
func (r *WordRepo) FindFamilyRoots() ([]string, error) {
	var roots []string
	err := r.db.Model(&Word{}).Select("word").Where("type = ?", "基础词").Pluck("word", &roots).Error
	return roots, err
}

// CountAll 统计单词总数
func (r *WordRepo) CountAll() (int, error) {
	var count int64
	err := r.db.Model(&Word{}).Count(&count).Error
	return int(count), err
}

// UpdateReview 更新复习计数和下次复习日期
func (r *WordRepo) UpdateReview(wordID int, reviewCount int, nextDate string) error {
	return r.db.Model(&Word{}).Where("id = ?", wordID).Updates(map[string]interface{}{
		"review_count":    reviewCount,
		"next_review_date": nextDate,
	}).Error
}

// FixMissingNextReviewDate 修复遗留数据：有 review_logs 但没有 next_review_date 的单词
func (r *WordRepo) FixMissingNextReviewDate(wordID int) error {
	today := Today4AM()
	return r.db.Exec(
		"UPDATE words SET next_review_date = DATE_ADD(?, INTERVAL 1 DAY), updated_at = NOW() WHERE id = ?",
		today, wordID).Error
}
