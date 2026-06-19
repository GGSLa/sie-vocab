package repo

import (
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"

	"sie-vocab-server/model"
)

// WordFamilyRepo 联表查询仓库（词族查询、复习抽词、统计、总览）
// 跨 words / meanings / examples / review_logs 等表
type WordFamilyRepo struct {
	db *gorm.DB
}

// NewWordFamilyRepo 创建 WordFamilyRepo
func NewWordFamilyRepo(db *gorm.DB) *WordFamilyRepo {
	return &WordFamilyRepo{db: db}
}

// DB 返回底层 gorm.DB（供 logic 层事务使用）
func (r *WordFamilyRepo) DB() *gorm.DB {
	return r.db
}

// QueryWordFamily 按单词查询整个词族（含 meanings + examples）
func (r *WordFamilyRepo) QueryWordFamily(word string) ([]model.WordEntry, error) {
	// 1. 查单词基本信息
	var wID int
	var wBaseWord sql.NullString
	var wType, wPos string
	var wDeriv sql.NullString
	err := r.db.Raw(
		"SELECT id, base_word, type, pos, derivation FROM words WHERE word = ?", word,
	).Row().Scan(&wID, &wBaseWord, &wType, &wPos, &wDeriv)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// 2. 确定词族根
	familyRoot := word
	if wBaseWord.Valid && wBaseWord.String != "" {
		familyRoot = wBaseWord.String
	}

	// 3. 查词族所有单词
	var rows []Row
	err = r.db.Raw(
		`SELECT id, word, base_word, type, pos, derivation FROM words
		 WHERE word = ? OR base_word = ?
		 ORDER BY CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id`,
		familyRoot, familyRoot,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// 4. 为每个单词填充 meanings 和 examples
	words := make([]model.WordEntry, 0, len(rows))
	for _, row := range rows {
		entry := model.WordEntry{
			Word: row.Word,
			Type: row.Type,
			Pos:  row.Pos,
		}
		if row.BaseWord.Valid && row.BaseWord.String != "" {
			bw := row.BaseWord.String
			entry.BaseWord = &bw
		}
		if row.Derivation.Valid && row.Derivation.String != "" {
			d := row.Derivation.String
			entry.Derivation = &d
		}

		// Meanings — scan into slice (works with GORM Scan)
		var meanings []struct {
			Domain string
			Text   string
		}
		r.db.Raw("SELECT domain, text FROM meanings WHERE word_id = ?", row.ID).Scan(&meanings)
		for _, m := range meanings {
			entry.Meanings = append(entry.Meanings, model.Meaning{Domain: m.Domain, Text: m.Text})
		}

		// Examples — scan into slice (works with GORM Scan)
		var examples []struct {
			En string
			Zh string
		}
		r.db.Raw("SELECT en, zh FROM examples WHERE word_id = ? ORDER BY sort_order", row.ID).Scan(&examples)
		for _, e := range examples {
			entry.Examples = append(entry.Examples, model.Example{En: e.En, Zh: e.Zh})
		}

		words = append(words, entry)
	}
	return words, nil
}

// Row 查询中间结果
type Row struct {
	ID         int
	Word       string
	BaseWord   sql.NullString `gorm:"column:base_word"`
	Type       string
	Pos        string
	Derivation sql.NullString `gorm:"column:derivation"`
}

// ---------- 复习抽词 ----------

// GetDueWordForReview 每日模式：随机抽取一个到期且今日未复习的单词
// 返回完整 WordEntry、word_id、错误
func (r *WordFamilyRepo) GetDueWordForReview() (*model.WordEntry, int, error) {
	today := Today4AM()

	// 0. 检查今日复习数量是否已达上限
	var todayCount int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs WHERE review_date = ?", today).Row().Scan(&todayCount)
	if todayCount >= 30 {
		return nil, 0, fmt.Errorf("今日复习已达上限（30词）")
	}

	// 1. 随机选一个到期且今日未被复习过的基础词族
	var familyRoot string
	err := r.db.Raw(`
		SELECT word FROM words
		WHERE type = '基础词'
		  AND (next_review_date IS NULL OR next_review_date <= ?)
		  AND word NOT IN (
			SELECT DISTINCT COALESCE(w2.base_word, w2.word)
			FROM review_logs rl
			JOIN words w2 ON rl.word_id = w2.id
			WHERE rl.review_date = ?
		  )
		ORDER BY RAND() LIMIT 1`, today, today).Row().Scan(&familyRoot)
	if err != nil {
		if err == sql.ErrNoRows || familyRoot == "" {
			return nil, 0, fmt.Errorf("所有单词均已排期，暂无到期复习的单词")
		}
		return nil, 0, err
	}

	// 2. 从该词族中随机选一个到期且今日未复习的单词
	var pickedID int
	var pickedWord string
	err = r.db.Raw(`
		SELECT id, word FROM words
		WHERE (word = ? OR base_word = ?)
		  AND (next_review_date IS NULL OR next_review_date <= ?)
		  AND id NOT IN (
			SELECT word_id FROM review_logs WHERE review_date = ?
		  )
		ORDER BY RAND() LIMIT 1`, familyRoot, familyRoot, today, today).Row().Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	// 3. 获取完整词族数据
	family, err := r.QueryWordFamily(pickedWord)
	if err != nil {
		return nil, 0, err
	}

	for _, entry := range family {
		if entry.Word == pickedWord {
			return &entry, pickedID, nil
		}
	}

	return nil, 0, fmt.Errorf("单词 %s 未在词族中找到", pickedWord)
}

// nextReviewInterval 根据复习次数计算下次复习间隔（天）
func nextReviewInterval(reviewCount int) int {
	switch {
	case reviewCount <= 1:
		return 1
	case reviewCount == 2:
		return 2
	case reviewCount == 3:
		return 4
	case reviewCount == 4:
		return 7
	case reviewCount == 5:
		return 15
	default:
		return 30
	}
}

// RecordReview 记录每日模式复习，更新间隔
// 返回 (newCount, nextDate, error)
func (r *WordFamilyRepo) RecordReview(wordID int) (newCount int, nextDate string, err error) {
	today := Today4AM()

	// 1. INSERT IGNORE 防止同日重复记录
	result := r.db.Exec(
		"INSERT IGNORE INTO review_logs (word_id, review_date) VALUES (?, ?)",
		wordID, today,
	)
	if result.Error != nil {
		return 0, "", result.Error
	}

	if result.RowsAffected == 0 {
		// 今日已记录过，读取当前状态返回
		var nd sql.NullString
		err = r.db.Raw(
			"SELECT review_count, next_review_date FROM words WHERE id = ?", wordID,
		).Row().Scan(&newCount, &nd)
		if err != nil {
			return 0, "", err
		}
		if nd.Valid && nd.String != "" {
			nextDate = nd.String
		} else {
			// 修复遗留数据
			r.db.Exec(
				"UPDATE words SET next_review_date = DATE_ADD(?, INTERVAL 1 DAY), updated_at = NOW() WHERE id = ?",
				today, wordID,
			)
			r.db.Raw("SELECT next_review_date FROM words WHERE id = ?", wordID).Row().Scan(&nextDate)
		}
		return
	}

	// 本次是该单词今日首次复习 → 更新每日快照
	r.db.Exec(`
		INSERT INTO daily_stats (review_date, word_count, total_words, is_completed)
		VALUES (?, 1, (SELECT COUNT(*) FROM words),
			LEAST(30, (SELECT COUNT(*) FROM words)) <= 1)
		ON DUPLICATE KEY UPDATE
			word_count = word_count + 1,
			is_completed = (word_count + 1 >= LEAST(30, total_words))
	`, today)

	// 2. 检查是否过期
	var currentCount int
	var isOverdue bool
	err = r.db.Raw(`
		SELECT review_count, next_review_date IS NOT NULL AND next_review_date < ?
		FROM words WHERE id = ?`, today, wordID,
	).Row().Scan(&currentCount, &isOverdue)
	if err != nil {
		return 0, "", err
	}

	newCount = currentCount + 1
	interval := nextReviewInterval(newCount)
	if isOverdue {
		interval = 1
	}

	// 3. UPDATE words
	err = r.db.Exec(
		"UPDATE words SET review_count = ?, next_review_date = DATE_ADD(?, INTERVAL ? DAY), updated_at = NOW() WHERE id = ?",
		newCount, today, interval, wordID,
	).Error
	if err != nil {
		return 0, "", err
	}

	r.db.Raw("SELECT next_review_date FROM words WHERE id = ?", wordID).Row().Scan(&nextDate)
	return newCount, nextDate, nil
}

// GetWordReviewStats 获取单词的复习统计
// wordCount: 该单词本身的复习次数
// baseCount: 该单词所属词族的总复习次数
// nextDate: 下次复习日期
func (r *WordFamilyRepo) GetWordReviewStats(wordID int) (wordCount int, baseCount int, nextDate string, err error) {
	// 1. 查询单词信息
	var wWord, wType string
	var wBaseWord sql.NullString
	var wNextReview sql.NullString
	err = r.db.Raw(
		"SELECT word, type, base_word, review_count, next_review_date FROM words WHERE id = ?",
		wordID,
	).Row().Scan(&wWord, &wType, &wBaseWord, &wordCount, &wNextReview)
	if err != nil {
		return 0, 0, "", err
	}
	if wNextReview.Valid {
		nextDate = wNextReview.String
	}

	// 2. 词族总复习次数
	familyRoot := wWord
	if wType != "基础词" && wBaseWord.Valid && wBaseWord.String != "" {
		familyRoot = wBaseWord.String
	}
	var bc int64
	r.db.Raw(`
		SELECT COUNT(*) FROM review_logs rl
		JOIN words w ON rl.word_id = w.id
		WHERE w.word = ? OR w.base_word = ?
	`, familyRoot, familyRoot).Row().Scan(&bc)
	baseCount = int(bc)

	return wordCount, baseCount, nextDate, nil
}

// ---------- 自由模式 ----------

// GetRandomWordForFreeReview 自由模式：完全随机抽词
func (r *WordFamilyRepo) GetRandomWordForFreeReview() (*model.WordEntry, int, error) {
	// 1. 随机选一个基础词族
	var familyRoot string
	err := r.db.Raw(
		"SELECT word FROM words WHERE type = '基础词' ORDER BY RAND() LIMIT 1",
	).Row().Scan(&familyRoot)
	if err != nil {
		if err == sql.ErrNoRows || familyRoot == "" {
			return nil, 0, fmt.Errorf("数据库中没有单词")
		}
		return nil, 0, err
	}

	// 2. 从该词族中随机选一个单词
	var pickedID int
	var pickedWord string
	err = r.db.Raw(
		"SELECT id, word FROM words WHERE word = ? OR base_word = ? ORDER BY RAND() LIMIT 1",
		familyRoot, familyRoot,
	).Row().Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	// 3. 获取完整数据
	family, err := r.QueryWordFamily(pickedWord)
	if err != nil {
		return nil, 0, err
	}

	for _, entry := range family {
		if entry.Word == pickedWord {
			return &entry, pickedID, nil
		}
	}

	return nil, 0, fmt.Errorf("单词 %s 未在词族中找到", pickedWord)
}

// ---------- 总览 ----------

// GetMonthOverview 获取月度总览数据
func (r *WordFamilyRepo) GetMonthOverview(year, month int) (*model.OverviewResponse, error) {
	resp := &model.OverviewResponse{
		Year:        year,
		Month:       month,
		MonthlyData: []model.DayOverview{},
	}

	// Total words
	var tw int64
	r.db.Raw("SELECT COUNT(*) FROM words").Row().Scan(&tw)
	resp.TotalWords = int(tw)

	// Total reviews
	var tr int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs").Row().Scan(&tr)
	resp.TotalReviews = int(tr)

	// Today's review count
	today := Today4AM()
	var td int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs WHERE review_date = ?", today).Row().Scan(&td)
	resp.TodayReviewed = int(td)

	// Monthly data from daily_stats
	var stats []DailyStats
	r.db.Where("YEAR(review_date) = ? AND MONTH(review_date) = ?", year, month).
		Order("review_date").
		Find(&stats)

	for _, s := range stats {
		date := s.ReviewDate
		if len(date) >= 10 {
			date = date[:10]
		}
		resp.MonthlyData = append(resp.MonthlyData, model.DayOverview{
			Date:        date,
			ReviewCount: s.WordCount,
			IsCompleted: s.IsCompleted,
		})
	}

	// Streak: consecutive days from most recent review date backwards
	dates, _ := r.getDistinctReviewDates()
	if len(dates) > 0 {
		dateSet := make(map[string]bool, len(dates))
		for _, d := range dates {
			dateSet[d] = true
		}

		checkDate := dates[0]
		streak := 0
		for dateSet[checkDate] {
			streak++
			t, _ := time.Parse("2006-01-02", checkDate)
			checkDate = t.Add(-24 * time.Hour).Format("2006-01-02")
		}
		resp.Streak = streak
	}

	return resp, nil
}

func (r *WordFamilyRepo) getDistinctReviewDates() ([]string, error) {
	var dates []string
	err := r.db.Raw(
		"SELECT DISTINCT review_date FROM review_logs ORDER BY review_date DESC",
	).Pluck("review_date", &dates).Error
	// Truncate to YYYY-MM-DD
	for i, d := range dates {
		if len(d) >= 10 {
			dates[i] = d[:10]
		}
	}
	return dates, err
}

// ---------- 单词保存（事务） ----------

// SaveWord 在事务中保存单个单词（含 meanings + examples）
func (r *WordFamilyRepo) SaveWord(entry model.WordEntry) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Upsert words
		if err := tx.Exec(`
			INSERT INTO words (word, base_word, type, pos, derivation)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				base_word = VALUES(base_word),
				type = VALUES(type),
				pos = VALUES(pos),
				derivation = VALUES(derivation),
				updated_at = NOW()
		`, entry.Word, entry.BaseWord, entry.Type, entry.Pos, entry.Derivation).Error; err != nil {
			return err
		}

		// 2. Get word ID
		var wordID int
		if err := tx.Raw("SELECT id FROM words WHERE word = ?", entry.Word).Row().Scan(&wordID); err != nil {
			return err
		}

		// 3. Delete old meanings & examples
		tx.Exec("DELETE FROM meanings WHERE word_id = ?", wordID)
		tx.Exec("DELETE FROM examples WHERE word_id = ?", wordID)

		// 4. Insert meanings
		for _, m := range entry.Meanings {
			if err := tx.Exec(
				"INSERT INTO meanings (word_id, domain, text) VALUES (?, ?, ?)",
				wordID, m.Domain, m.Text,
			).Error; err != nil {
				return err
			}
		}

		// 5. Insert examples
		for i, e := range entry.Examples {
			if err := tx.Exec(
				"INSERT INTO examples (word_id, en, zh, sort_order) VALUES (?, ?, ?, ?)",
				wordID, e.En, e.Zh, i,
			).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// SaveWords 批量保存单词
func (r *WordFamilyRepo) SaveWords(entries []model.WordEntry) (int, error) {
	count := 0
	for _, entry := range entries {
		if entry.Word == "" {
			continue
		}
		if err := r.SaveWord(entry); err != nil {
			continue
		}
		count++
	}
	return count, nil
}
