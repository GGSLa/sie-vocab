package repo

import (
	"fmt"

	"sie-vocab-server/model"

	"gorm.io/gorm"
)

// DailyPoolRepo 词池纯 DB 操作（无业务判断）
type DailyPoolRepo struct {
	db *gorm.DB
}

// NewDailyPoolRepo 创建 DailyPoolRepo
func NewDailyPoolRepo(db *gorm.DB) *DailyPoolRepo {
	return &DailyPoolRepo{db: db}
}

// InsertPoolBatch 批量插入池词
func (r *DailyPoolRepo) InsertPoolBatch(userID int, poolDate string, batchNum int, words []model.PoolWord) error {
	if len(words) == 0 {
		return nil
	}
	for _, w := range words {
		if err := r.db.Exec(
			`INSERT INTO daily_word_pool (user_id, pool_date, batch_num, word_id, word, family_root, is_due, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, poolDate, batchNum, w.WordID, w.Word, w.FamilyRoot, w.IsDue, w.SortOrder,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetUndrawnWord 从当前批次随机取一条未抽取词（返回 word_id + word 字符串）
func (r *DailyPoolRepo) GetUndrawnWord(userID int, poolDate string, batchNum int) (wordID int, word string, err error) {
	err = r.db.Raw(
		`SELECT word_id, word FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ? AND batch_num = ? AND drawn = 0
		 ORDER BY RAND() LIMIT 1`,
		userID, poolDate, batchNum,
	).Row().Scan(&wordID, &word)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, "", fmt.Errorf("批次已耗尽")
		}
		return 0, "", err
	}
	return wordID, word, nil
}

// MarkDrawn 标记词池中某词为已抽取
func (r *DailyPoolRepo) MarkDrawn(userID int, wordID int, poolDate string) error {
	return r.db.Exec(
		`UPDATE daily_word_pool SET drawn = 1 WHERE user_id = ? AND word_id = ? AND pool_date = ? AND drawn = 0`,
		userID, wordID, poolDate,
	).Error
}

// CountRemaining 当前批次剩余未抽取数
func (r *DailyPoolRepo) CountRemaining(userID int, poolDate string, batchNum int) (int, error) {
	var count int64
	err := r.db.Raw(
		`SELECT COUNT(*) FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ? AND batch_num = ? AND drawn = 0`,
		userID, poolDate, batchNum,
	).Row().Scan(&count)
	return int(count), err
}

// CountBatchTotal 当前批次总词数
func (r *DailyPoolRepo) CountBatchTotal(userID int, poolDate string, batchNum int) (int, error) {
	var count int64
	err := r.db.Raw(
		`SELECT COUNT(*) FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ? AND batch_num = ?`,
		userID, poolDate, batchNum,
	).Row().Scan(&count)
	return int(count), err
}

// CountDrawn 当前批次已抽取数
func (r *DailyPoolRepo) CountDrawn(userID int, poolDate string, batchNum int) (int, error) {
	var count int64
	err := r.db.Raw(
		`SELECT COUNT(*) FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ? AND batch_num = ? AND drawn = 1`,
		userID, poolDate, batchNum,
	).Row().Scan(&count)
	return int(count), err
}

// GetActiveBatch 获取当前活跃批次号（最大 batch_num），无则返回 0
func (r *DailyPoolRepo) GetActiveBatch(userID int, poolDate string) (int, error) {
	var batch int
	err := r.db.Raw(
		`SELECT COALESCE(MAX(batch_num), 0) FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ?`,
		userID, poolDate,
	).Row().Scan(&batch)
	return batch, err
}

// GetPooledFamilies 获取今日已入池的所有族根
func (r *DailyPoolRepo) GetPooledFamilies(userID int, poolDate string) ([]string, error) {
	var families []string
	err := r.db.Raw(
		`SELECT DISTINCT family_root FROM daily_word_pool
		 WHERE user_id = ? AND pool_date = ?`,
		userID, poolDate,
	).Pluck("family_root", &families).Error
	return families, err
}

// CountAvailableFamilies 统计还可入池的族数（到期/未到期分别计数）
// excludeFamilies 应包含：今日已复习的族 + 今日已入池的族
func (r *DailyPoolRepo) CountAvailableFamilies(userID int, poolDate string, excludeFamilies []string) (due int, nonDue int, err error) {
	// 到期族
	dueQuery := r.db.Raw(
		`SELECT COUNT(DISTINCT COALESCE(base_word, word))
		 FROM words
		 WHERE user_id = ? AND type = '基础词'
		   AND (next_review_date IS NULL OR next_review_date <= ?)`,
		userID, poolDate,
	)
	if len(excludeFamilies) > 0 {
		dueQuery = r.db.Raw(
			`SELECT COUNT(DISTINCT COALESCE(base_word, word))
			 FROM words
			 WHERE user_id = ? AND type = '基础词'
			   AND (next_review_date IS NULL OR next_review_date <= ?)
			   AND COALESCE(base_word, word) NOT IN ?`,
			userID, poolDate, excludeFamilies,
		)
	}
	var dueCount int64
	if err := dueQuery.Row().Scan(&dueCount); err != nil {
		return 0, 0, err
	}

	// 未到期族
	nonDueQuery := r.db.Raw(
		`SELECT COUNT(DISTINCT COALESCE(base_word, word))
		 FROM words
		 WHERE user_id = ? AND type = '基础词'
		   AND next_review_date > ?`,
		userID, poolDate,
	)
	if len(excludeFamilies) > 0 {
		nonDueQuery = r.db.Raw(
			`SELECT COUNT(DISTINCT COALESCE(base_word, word))
			 FROM words
			 WHERE user_id = ? AND type = '基础词'
			   AND next_review_date > ?
			   AND COALESCE(base_word, word) NOT IN ?`,
			userID, poolDate, excludeFamilies,
		)
	}
	var nonDueCount int64
	if err := nonDueQuery.Row().Scan(&nonDueCount); err != nil {
		return 0, 0, err
	}

	return int(dueCount), int(nonDueCount), nil
}
