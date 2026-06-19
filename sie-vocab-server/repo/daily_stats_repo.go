package repo

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sie-vocab-server/model"
)

// DailyStats GORM model for daily_stats table
type DailyStats struct {
	ReviewDate  string `gorm:"primaryKey;column:review_date"`
	WordCount   int    `gorm:"column:word_count"`
	TotalWords  int    `gorm:"column:total_words"`
	IsCompleted bool   `gorm:"column:is_completed"`
}

// TableName overrides the default table name
func (DailyStats) TableName() string { return "daily_stats" }

// DailyStatsRepo 管理 daily_stats 表的 CRUD（每日复习快照）
type DailyStatsRepo struct {
	db *gorm.DB
}

// NewDailyStatsRepo 创建 DailyStatsRepo
func NewDailyStatsRepo(db *gorm.DB) *DailyStatsRepo {
	return &DailyStatsRepo{db: db}
}

// UpsertToday 更新或插入今日复习快照
// INSERT: word_count=1, total_words=当前单词总数
// UPDATE: word_count = word_count + 1
// is_completed 自动计算
func (r *DailyStatsRepo) UpsertToday() error {
	today := Today4AM()

	// Use raw SQL for the complex upsert with subquery and LEAST
	return r.db.Exec(`
		INSERT INTO daily_stats (review_date, word_count, total_words, is_completed)
		VALUES (?, 1, (SELECT COUNT(*) FROM words),
			LEAST(30, (SELECT COUNT(*) FROM words)) <= 1)
		ON DUPLICATE KEY UPDATE
			word_count = word_count + 1,
			is_completed = (word_count + 1 >= LEAST(30, total_words))
	`, today).Error
}

// FindByMonth 查询某月所有日期的快照
func (r *DailyStatsRepo) FindByMonth(year, month int) ([]model.DayOverview, error) {
	var stats []DailyStats
	err := r.db.Where("YEAR(review_date) = ? AND MONTH(review_date) = ?", year, month).
		Order("review_date").
		Find(&stats).Error
	if err != nil {
		return nil, err
	}

	result := make([]model.DayOverview, 0, len(stats))
	for _, s := range stats {
		date := s.ReviewDate
		if len(date) >= 10 {
			date = date[:10]
		}
		result = append(result, model.DayOverview{
			Date:        date,
			ReviewCount: s.WordCount,
			IsCompleted: s.IsCompleted,
		})
	}
	return result, nil
}

// GetToday 获取今日快照
func (r *DailyStatsRepo) GetToday() (*DailyStats, error) {
	today := Today4AM()
	var s DailyStats
	err := r.db.Where("review_date = ?", today).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// EnsureToday 确保今日快照存在（不存在则创建）
func (r *DailyStatsRepo) EnsureToday() error {
	today := Today4AM()
	var count int64
	if err := r.db.Model(&DailyStats{}).Where("review_date = ?", today).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return r.db.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&DailyStats{ReviewDate: today, WordCount: 0}).Error
	}
	return nil
}
