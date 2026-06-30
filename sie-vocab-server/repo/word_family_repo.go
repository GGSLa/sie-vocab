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
func (r *WordFamilyRepo) QueryWordFamily(word string, userID int) ([]model.WordEntry, error) {
	// 1. 查单词基本信息
	var wID int
	var wBaseWord sql.NullString
	var wType, wPos string
	var wDeriv sql.NullString
	err := r.db.Raw(
		"SELECT id, base_word, type, pos, derivation FROM words WHERE user_id = ? AND word = ?",
		userID, word,
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
		 WHERE user_id = ? AND (word = ? OR base_word = ?)
		 ORDER BY CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id`,
		userID, familyRoot, familyRoot,
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

		// Meanings
		var meanings []struct {
			Domain string
			Text   string
		}
		r.db.Raw("SELECT domain, text FROM meanings WHERE word_id = ? AND user_id = ?", row.ID, userID).Scan(&meanings)
		for _, m := range meanings {
			entry.Meanings = append(entry.Meanings, model.Meaning{Domain: m.Domain, Text: m.Text})
		}

		// Examples
		var examples []struct {
			En string
			Zh string
		}
		r.db.Raw("SELECT en, zh FROM examples WHERE word_id = ? AND user_id = ? ORDER BY sort_order", row.ID, userID).Scan(&examples)
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
func (r *WordFamilyRepo) GetDueWordForReview(userID int) (*model.WordEntry, int, error) {
	today := Today4AM()

	// 0. 检查今日复习数量是否已达上限
	var todayCount int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs WHERE user_id = ? AND review_date = ?", userID, today).Row().Scan(&todayCount)
	if todayCount >= 30 {
		return nil, 0, fmt.Errorf("今日复习已达上限（30词）")
	}

	// 1. 随机选一个到期且今日未被复习过的基础词族
	var familyRoot string
	err := r.db.Raw(`
		SELECT word FROM words
		WHERE user_id = ? AND type = '基础词'
		  AND (next_review_date IS NULL OR next_review_date <= ?)
		  AND word NOT IN (
			SELECT DISTINCT COALESCE(w2.base_word, w2.word)
			FROM review_logs rl
			JOIN words w2 ON rl.word_id = w2.id AND rl.user_id = w2.user_id
			WHERE rl.user_id = ? AND rl.review_date = ?
		  )
		ORDER BY RAND() LIMIT 1`, userID, today, userID, today).Row().Scan(&familyRoot)
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
		WHERE user_id = ? AND (word = ? OR base_word = ?)
		  AND (next_review_date IS NULL OR next_review_date <= ?)
		  AND id NOT IN (
			SELECT word_id FROM review_logs WHERE user_id = ? AND review_date = ?
		  )
		ORDER BY RAND() LIMIT 1`, userID, familyRoot, familyRoot, today, userID, today).Row().Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	// 3. 获取完整词族数据
	family, err := r.QueryWordFamily(pickedWord, userID)
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
func (r *WordFamilyRepo) RecordReview(wordID int, userID int) (newCount int, nextDate string, err error) {
	today := Today4AM()

	// 1. INSERT IGNORE 防止同日重复记录
	result := r.db.Exec(
		"INSERT IGNORE INTO review_logs (user_id, word_id, review_date) VALUES (?, ?, ?)",
		userID, wordID, today,
	)
	if result.Error != nil {
		return 0, "", result.Error
	}

	if result.RowsAffected == 0 {
		// 今日已记录过，读取当前状态返回
		var nd sql.NullString
		err = r.db.Raw(
			"SELECT review_count, next_review_date FROM words WHERE id = ? AND user_id = ?",
			wordID, userID,
		).Row().Scan(&newCount, &nd)
		if err != nil {
			return 0, "", err
		}
		if nd.Valid && nd.String != "" {
			nextDate = nd.String
		} else {
			// 修复遗留数据
			r.db.Exec(
				"UPDATE words SET next_review_date = DATE_ADD(?, INTERVAL 1 DAY), updated_at = NOW() WHERE id = ? AND user_id = ?",
				today, wordID, userID,
			)
			r.db.Raw("SELECT next_review_date FROM words WHERE id = ? AND user_id = ?", wordID, userID).Row().Scan(&nextDate)
		}
		return
	}

	// 本次是该单词今日首次复习 → 更新每日快照
	r.db.Exec(`
		INSERT INTO daily_stats (user_id, review_date, word_count, total_words, is_completed)
		VALUES (?, ?, 1, (SELECT COUNT(*) FROM words WHERE user_id = ?),
			LEAST(30, (SELECT COUNT(*) FROM words WHERE user_id = ?)) <= 1)
		ON DUPLICATE KEY UPDATE
			word_count = word_count + 1,
			is_completed = (word_count + 1 >= LEAST(30, total_words))
	`, userID, today, userID, userID)

	// 2. 检查是否过期
	var currentCount int
	var isOverdue bool
	err = r.db.Raw(`
		SELECT review_count, next_review_date IS NOT NULL AND next_review_date < ?
		FROM words WHERE id = ? AND user_id = ?`, today, wordID, userID,
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
		"UPDATE words SET review_count = ?, next_review_date = DATE_ADD(?, INTERVAL ? DAY), updated_at = NOW() WHERE id = ? AND user_id = ?",
		newCount, today, interval, wordID, userID,
	).Error
	if err != nil {
		return 0, "", err
	}

	r.db.Raw("SELECT next_review_date FROM words WHERE id = ? AND user_id = ?", wordID, userID).Row().Scan(&nextDate)
	return newCount, nextDate, nil
}

// GetWordReviewStats 获取单词的复习统计
// wordCount: 该单词本身的复习次数
// baseCount: 该单词所属词族的总复习次数
// nextDate: 下次复习日期
func (r *WordFamilyRepo) GetWordReviewStats(wordID int, userID int) (wordCount int, baseCount int, nextDate string, err error) {
	// 1. 查询单词信息
	var wWord, wType string
	var wBaseWord sql.NullString
	var wNextReview sql.NullString
	err = r.db.Raw(
		"SELECT word, type, base_word, review_count, next_review_date FROM words WHERE id = ? AND user_id = ?",
		wordID, userID,
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
		JOIN words w ON rl.word_id = w.id AND rl.user_id = w.user_id
		WHERE rl.user_id = ? AND (w.word = ? OR w.base_word = ?)
	`, userID, familyRoot, familyRoot).Row().Scan(&bc)
	baseCount = int(bc)

	return wordCount, baseCount, nextDate, nil
}

// ---------- 自由模式 ----------

// GetRandomWordForFreeReview 自由模式：完全随机抽词
func (r *WordFamilyRepo) GetRandomWordForFreeReview(userID int) (*model.WordEntry, int, error) {
	// 1. 随机选一个基础词族
	var familyRoot string
	err := r.db.Raw(
		"SELECT word FROM words WHERE user_id = ? AND type = '基础词' ORDER BY RAND() LIMIT 1",
		userID,
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
		"SELECT id, word FROM words WHERE user_id = ? AND (word = ? OR base_word = ?) ORDER BY RAND() LIMIT 1",
		userID, familyRoot, familyRoot,
	).Row().Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	// 3. 获取完整数据
	family, err := r.QueryWordFamily(pickedWord, userID)
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

// ---------- 词池生成辅助查询（纯 DB，无业务判断）----------

// GetDueFamilies 随机取 N 个到期的、未在排除列表中的基础词族根
func (r *WordFamilyRepo) GetDueFamilies(userID int, poolDate string, exclude []string, limit int) ([]string, error) {
	query := `SELECT COALESCE(base_word, word) AS family_root
		FROM words
		WHERE user_id = ? AND type = '基础词'
		  AND (next_review_date IS NULL OR next_review_date <= ?)`
	args := []interface{}{userID, poolDate}

	if len(exclude) > 0 {
		query += ` AND COALESCE(base_word, word) NOT IN (?)`
		args = append(args, exclude)
	}

	query += ` GROUP BY family_root ORDER BY RAND() LIMIT ?`
	args = append(args, limit)

	var families []string
	err := r.db.Raw(query, args...).Pluck("family_root", &families).Error
	return families, err
}

// GetNonDueFamilies 取 N 个未到期的、未在排除列表中的基础词族根，按最早到期日排序
func (r *WordFamilyRepo) GetNonDueFamilies(userID int, poolDate string, exclude []string, limit int) ([]string, error) {
	query := `SELECT COALESCE(base_word, word) AS family_root,
			MIN(next_review_date) AS earliest_due
		FROM words
		WHERE user_id = ? AND type = '基础词'
		  AND next_review_date > ?`
	args := []interface{}{userID, poolDate}

	if len(exclude) > 0 {
		query += ` AND COALESCE(base_word, word) NOT IN (?)`
		args = append(args, exclude)
	}

	query += ` GROUP BY family_root ORDER BY earliest_due ASC LIMIT ?`
	args = append(args, limit)

	var families []string
	err := r.db.Raw(query, args...).Pluck("family_root", &families).Error
	return families, err
}

// PickWordFromFamily 从指定词族中选一个单词（到期优先，随机选）
func (r *WordFamilyRepo) PickWordFromFamily(userID int, familyRoot, poolDate string) (wordID int, word string, err error) {
	// 优先选到期单词
	err = r.db.Raw(`
		SELECT id, word FROM words
		WHERE user_id = ? AND (word = ? OR base_word = ?)
		  AND (next_review_date IS NULL OR next_review_date <= ?)
		ORDER BY RAND() LIMIT 1
	`, userID, familyRoot, familyRoot, poolDate).Row().Scan(&wordID, &word)
	if err == nil {
		return
	}

	// 无到期词，选最早到期的
	err = r.db.Raw(`
		SELECT id, word FROM words
		WHERE user_id = ? AND (word = ? OR base_word = ?)
		ORDER BY next_review_date ASC LIMIT 1
	`, userID, familyRoot, familyRoot).Row().Scan(&wordID, &word)
	return
}

// ---------- 总览 ----------

// GetMonthOverview 获取月度总览数据
func (r *WordFamilyRepo) GetMonthOverview(year, month int, userID int) (*model.OverviewResponse, error) {
	resp := &model.OverviewResponse{
		Year:        year,
		Month:       month,
		MonthlyData: []model.DayOverview{},
	}

	// Total words (仅统计基础词 / base words only)
	var tw int64
	r.db.Raw("SELECT COUNT(*) FROM words WHERE user_id = ? AND type = '基础词'", userID).Row().Scan(&tw)
	resp.TotalWords = int(tw)

	// Total reviews
	var tr int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs WHERE user_id = ?", userID).Row().Scan(&tr)
	resp.TotalReviews = int(tr)

	// Today's review count
	today := Today4AM()
	var td int64
	r.db.Raw("SELECT COUNT(*) FROM review_logs WHERE user_id = ? AND review_date = ?", userID, today).Row().Scan(&td)
	resp.TodayReviewed = int(td)

	// Monthly data from daily_stats
	var stats []DailyStats
	r.db.Where("user_id = ? AND YEAR(review_date) = ? AND MONTH(review_date) = ?", userID, year, month).
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
	dates, _ := r.getDistinctReviewDates(userID)
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

func (r *WordFamilyRepo) getDistinctReviewDates(userID int) ([]string, error) {
	var dates []string
	err := r.db.Raw(
		"SELECT DISTINCT review_date FROM review_logs WHERE user_id = ? ORDER BY review_date DESC",
		userID,
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
func (r *WordFamilyRepo) SaveWord(entry model.WordEntry, userID int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Upsert words (含 user_id) — 基础词优先：已存在的基础词不被同 word 的衍生词覆盖
		if err := tx.Exec(`
			INSERT INTO words (user_id, word, base_word, type, pos, derivation)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				type = IF(type = '基础词', '基础词', VALUES(type)),
				base_word = IF(type = '基础词', base_word, VALUES(base_word)),
				derivation = IF(type = '基础词', derivation, VALUES(derivation)),
				pos = VALUES(pos),
				updated_at = NOW()
		`, userID, entry.Word, entry.BaseWord, entry.Type, entry.Pos, entry.Derivation).Error; err != nil {
			return err
		}

		// 2. Get word ID
		var wordID int
		if err := tx.Raw("SELECT id FROM words WHERE user_id = ? AND word = ?", userID, entry.Word).Row().Scan(&wordID); err != nil {
			return err
		}

		// 3. Delete old meanings & examples
		tx.Exec("DELETE FROM meanings WHERE word_id = ? AND user_id = ?", wordID, userID)
		tx.Exec("DELETE FROM examples WHERE word_id = ? AND user_id = ?", wordID, userID)

		// 4. Insert meanings
		for _, m := range entry.Meanings {
			if err := tx.Exec(
				"INSERT INTO meanings (user_id, word_id, domain, text) VALUES (?, ?, ?, ?)",
				userID, wordID, m.Domain, m.Text,
			).Error; err != nil {
				return err
			}
		}

		// 5. Insert examples
		for i, e := range entry.Examples {
			if err := tx.Exec(
				"INSERT INTO examples (user_id, word_id, en, zh, sort_order) VALUES (?, ?, ?, ?, ?)",
				userID, wordID, e.En, e.Zh, i,
			).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// SaveWords 批量保存单词 — 先去重：同 word 时基础词优先，合并释义和例句
func (r *WordFamilyRepo) SaveWords(entries []model.WordEntry, userID int) (int, error) {
	// 预去重：同 word 的多个条目，基础词优先，合并释义和例句
	seen := make(map[string]*model.WordEntry)
	for i := range entries {
		w := entries[i].Word
		if w == "" {
			continue
		}
		if existing, ok := seen[w]; ok {
			// 冲突：基础词优先
			if entries[i].Type == "基础词" && existing.Type != "基础词" {
				// 新条目是基础词，保留新条目，合并旧条目的 meanings/examples
				entries[i].Meanings = append(entries[i].Meanings, existing.Meanings...)
				entries[i].Examples = append(entries[i].Examples, existing.Examples...)
				seen[w] = &entries[i]
			} else {
				// 保留 existing，把新条目的 meanings/examples 合并到 existing
				existing.Meanings = append(existing.Meanings, entries[i].Meanings...)
				existing.Examples = append(existing.Examples, entries[i].Examples...)
			}
		} else {
			seen[w] = &entries[i]
		}
	}

	count := 0
	for _, entry := range seen {
		if err := r.SaveWord(*entry, userID); err != nil {
			continue
		}
		count++
	}
	return count, nil
}
