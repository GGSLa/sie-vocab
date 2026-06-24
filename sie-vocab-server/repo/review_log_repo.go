package repo

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReviewLog GORM model for review_logs table
type ReviewLog struct {
	ID         int    `gorm:"primaryKey;column:id"`
	UserID     int    `gorm:"column:user_id;uniqueIndex:idx_review_logs_user_word_date"`
	WordID     int    `gorm:"column:word_id;uniqueIndex:idx_review_logs_user_word_date"`
	ReviewDate string `gorm:"column:review_date;uniqueIndex:idx_review_logs_user_word_date"`
}

// TableName overrides the default table name
func (ReviewLog) TableName() string { return "review_logs" }

// ReviewLogRepo 管理 review_logs 表的 CRUD（每日模式复习记录）
type ReviewLogRepo struct {
	db *gorm.DB
}

// NewReviewLogRepo 创建 ReviewLogRepo
func NewReviewLogRepo(db *gorm.DB) *ReviewLogRepo {
	return &ReviewLogRepo{db: db}
}

// CountToday 统计今日（4AM 边界）已复习词数
func (r *ReviewLogRepo) CountToday(userID int) (int, error) {
	today := Today4AM()
	var count int64
	err := r.db.Model(&ReviewLog{}).Where("user_id = ? AND review_date = ?", userID, today).Count(&count).Error
	return int(count), err
}

// InsertIgnore insert ignore（防同日重复记录）
// 返回 rowsAffected: 1=新记录, 0=已存在
func (r *ReviewLogRepo) InsertIgnore(wordID int, userID int) (int64, error) {
	today := Today4AM()
	result := r.db.Clauses(clause.Insert{Modifier: "IGNORE"}).
		Create(&ReviewLog{UserID: userID, WordID: wordID, ReviewDate: today})
	return result.RowsAffected, result.Error
}

// HasReviewedToday 检查某单词族今日是否已复习过
func (r *ReviewLogRepo) HasReviewedToday(w *WordRepo, userID int) ([]string, error) {
	today := Today4AM()
	var roots []string
	err := r.db.Model(&ReviewLog{}).
		Select("DISTINCT COALESCE(w2.base_word, w2.word)").
		Joins("JOIN words w2 ON review_logs.word_id = w2.id AND review_logs.user_id = w2.user_id").
		Where("review_logs.user_id = ? AND review_logs.review_date = ?", userID, today).
		Pluck("COALESCE(w2.base_word, w2.word)", &roots).Error
	return roots, err
}

// GetDistinctDates 获取所有不重复的复习日期（按日期降序）
func (r *ReviewLogRepo) GetDistinctDates(userID int) ([]string, error) {
	var dates []string
	err := r.db.Model(&ReviewLog{}).
		Select("DISTINCT review_date").
		Where("user_id = ?", userID).
		Order("review_date DESC").
		Pluck("review_date", &dates).Error
	return dates, err
}

// CountTotal 统计每日模式总复习次数
func (r *ReviewLogRepo) CountTotal(userID int) (int, error) {
	var count int64
	err := r.db.Model(&ReviewLog{}).Where("user_id = ?", userID).Count(&count).Error
	return int(count), err
}

// CountByWordFamily 统计某词族的总复习次数
func (r *ReviewLogRepo) CountByWordFamily(familyRoot string, userID int) (int, error) {
	var count int64
	err := r.db.Model(&ReviewLog{}).
		Joins("JOIN words w ON review_logs.word_id = w.id AND review_logs.user_id = w.user_id").
		Where("review_logs.user_id = ? AND (w.word = ? OR w.base_word = ?)", userID, familyRoot, familyRoot).
		Count(&count).Error
	return int(count), err
}
