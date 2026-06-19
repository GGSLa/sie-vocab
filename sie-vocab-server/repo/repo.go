package repo

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"sie-vocab-server/model"
)

// DB 全局数据库连接
var DB *sql.DB

// InitDB 初始化数据库连接
func InitDB(cfg model.MySQLConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	d, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库连接失败: %v", err)
	}
	d.SetMaxOpenConns(10)
	d.SetMaxIdleConns(5)
	d.SetConnMaxLifetime(5 * time.Minute)
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连接测试失败: %v", err)
	}
	DB = d
	return d, nil
}

// QueryWordFamily 按单词查询整个词族
func QueryWordFamily(word string) ([]model.WordEntry, error) {
	var id int
	var baseWord sql.NullString
	var wType, pos string
	var derivation sql.NullString

	err := DB.QueryRow("SELECT id, base_word, type, pos, derivation FROM words WHERE word = ?", word).
		Scan(&id, &baseWord, &wType, &pos, &derivation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	familyRoot := word
	if baseWord.Valid && baseWord.String != "" {
		familyRoot = baseWord.String
	}

	rows, err := DB.Query(
		`SELECT id, word, base_word, type, pos, derivation FROM words
		 WHERE word = ? OR base_word = ?
		 ORDER BY CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id`,
		familyRoot, familyRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []model.WordEntry
	for rows.Next() {
		var wid int
		var w string
		var bw sql.NullString
		var wt, p string
		var d sql.NullString
		if err := rows.Scan(&wid, &w, &bw, &wt, &p, &d); err != nil {
			return nil, err
		}
		entry := model.WordEntry{
			Word: w,
			Type: wt,
			Pos:  p,
		}
		if bw.Valid {
			entry.BaseWord = &bw.String
		}
		if d.Valid {
			entry.Derivation = &d.String
		}

		mRows, err := DB.Query("SELECT domain, text FROM meanings WHERE word_id = ?", wid)
		if err != nil {
			return nil, err
		}
		for mRows.Next() {
			var domain, text string
			if err := mRows.Scan(&domain, &text); err != nil {
				mRows.Close()
				return nil, err
			}
			entry.Meanings = append(entry.Meanings, model.Meaning{Domain: domain, Text: text})
		}
		mRows.Close()

		eRows, err := DB.Query("SELECT en, zh FROM examples WHERE word_id = ? ORDER BY sort_order", wid)
		if err != nil {
			return nil, err
		}
		for eRows.Next() {
			var en, zh string
			if err := eRows.Scan(&en, &zh); err != nil {
				eRows.Close()
				return nil, err
			}
			entry.Examples = append(entry.Examples, model.Example{En: en, Zh: zh})
		}
		eRows.Close()

		words = append(words, entry)
	}
	return words, nil
}

// SaveWord 保存单个单词（含 meanings 和 examples），使用 upsert
func SaveWord(entry model.WordEntry) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO words (word, base_word, type, pos, derivation)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   base_word = VALUES(base_word),
		   type = VALUES(type),
		   pos = VALUES(pos),
		   derivation = VALUES(derivation),
		   updated_at = NOW()`,
		entry.Word, entry.BaseWord, entry.Type, entry.Pos, entry.Derivation)
	if err != nil {
		return err
	}

	var wordID int
	err = tx.QueryRow("SELECT id FROM words WHERE word = ?", entry.Word).Scan(&wordID)
	if err != nil {
		return err
	}

	tx.Exec("DELETE FROM meanings WHERE word_id = ?", wordID)
	tx.Exec("DELETE FROM examples WHERE word_id = ?", wordID)

	for _, m := range entry.Meanings {
		_, err := tx.Exec("INSERT INTO meanings (word_id, domain, text) VALUES (?, ?, ?)", wordID, m.Domain, m.Text)
		if err != nil {
			return err
		}
	}

	for i, e := range entry.Examples {
		_, err := tx.Exec("INSERT INTO examples (word_id, en, zh, sort_order) VALUES (?, ?, ?, ?)", wordID, e.En, e.Zh, i)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ---------- 复习 ----------

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

// GetDueWordForReview 随机抽取一个到期的基础词族，从中随机选一个单词返回
// 到期判断：next_review_date IS NULL（从未复习）或 <= 今日（已到复习日）
// 同时排除今日已复习过的词族（保证一天一词族一次）
// 每日上限 30 词
func GetDueWordForReview() (*model.WordEntry, int, error) {
	today := "DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR))"

	// 0. 检查今日复习数量是否已达上限
	var todayCount int
	DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM review_logs WHERE review_date = %s", today)).Scan(&todayCount)
	if todayCount >= 30 {
		return nil, 0, fmt.Errorf("今日复习已达上限（30词）")
	}

	// 1. 随机选一个到期且今日未被复习过的基础词族
	var familyRoot string
	query := fmt.Sprintf(`
		SELECT word FROM words
		WHERE type = '基础词'
		  AND (next_review_date IS NULL OR next_review_date <= %s)
		  AND word NOT IN (
			SELECT DISTINCT COALESCE(w2.base_word, w2.word)
			FROM review_logs rl
			JOIN words w2 ON rl.word_id = w2.id
			WHERE rl.review_date = %s
		  )
		ORDER BY RAND() LIMIT 1`, today, today)
	err := DB.QueryRow(query).Scan(&familyRoot)
	if err == sql.ErrNoRows {
		return nil, 0, fmt.Errorf("所有单词均已排期，暂无到期复习的单词")
	}
	if err != nil {
		return nil, 0, err
	}

	// 2. 从该词族中随机选一个到期且今日未复习的单词
	var pickedWord string
	var pickedID int
	err = DB.QueryRow(
		fmt.Sprintf(`SELECT id, word FROM words
			WHERE (word = ? OR base_word = ?)
			  AND (next_review_date IS NULL OR next_review_date <= %s)
			  AND id NOT IN (
			    SELECT word_id FROM review_logs WHERE review_date = %s
			  )
			ORDER BY RAND() LIMIT 1`, today, today),
		familyRoot, familyRoot).Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	// 3. 获取完整数据
	family, err := QueryWordFamily(pickedWord)
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

// updateDailyStats 更新每日复习快照（INSERT 时记录当日单词总数，后续 UPDATE 仅增量计数）
func updateDailyStats() {
	todayExpr := "DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR))"
	_, err := DB.Exec(`
		INSERT INTO daily_stats (review_date, word_count, total_words, is_completed)
		VALUES (` + todayExpr + `, 1, (SELECT COUNT(*) FROM words),
			LEAST(30, (SELECT COUNT(*) FROM words)) <= 1)
		ON DUPLICATE KEY UPDATE
			word_count = word_count + 1,
			is_completed = (word_count + 1 >= LEAST(30, total_words))
	`)
	if err != nil {
		log.Printf("⚠️ 更新每日快照失败: %v", err)
	}
}

// RecordReview 记录复习并更新间隔（每天每词只记一次，每日模式）
func RecordReview(wordID int) (newCount int, nextDate string, err error) {
	// 1. INSERT IGNORE 防止同日重复记录
	result, err := DB.Exec(
		"INSERT IGNORE INTO review_logs (word_id, review_date) VALUES (?, DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR)))",
		wordID)
	if err != nil {
		return 0, "", err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// 今日已记录过，读取当前状态返回
		var nd sql.NullString
		err = DB.QueryRow("SELECT review_count, next_review_date FROM words WHERE id = ?", wordID).
			Scan(&newCount, &nd)
		if nd.Valid {
			nextDate = nd.String
		} else {
			// 修复遗留数据：有 review_logs 但没有 next_review_date
			DB.Exec(`UPDATE words SET next_review_date = DATE_ADD(DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR)), INTERVAL 1 DAY), updated_at = NOW() WHERE id = ?`, wordID)
			DB.QueryRow("SELECT next_review_date FROM words WHERE id = ?", wordID).Scan(&nd)
			if nd.Valid {
				nextDate = nd.String
			}
		}
		return
	}

	// 本次是该单词今日首次复习 → 更新每日快照表
	updateDailyStats()

	// 2. 检查是否过期：next_review_date < 今天 → 重置计数（遗忘惩罚）
	var currentCount int
	var isOverdue bool
	err = DB.QueryRow(fmt.Sprintf(
		`SELECT review_count, next_review_date IS NOT NULL AND next_review_date < %s FROM words WHERE id = ?`,
		"DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR))"),
		wordID).Scan(&currentCount, &isOverdue)
	if err != nil {
		return 0, "", err
	}

	newCount = currentCount + 1
	interval := nextReviewInterval(newCount)
	if isOverdue {
		interval = 1 // 过期：计数不重置，仅间隔回到 1 天
	}

	// MySQL UPDATE + 回读 next_review_date（兼容 5.7 无 RETURNING）
	_, err = DB.Exec(
		`UPDATE words SET review_count = ?, next_review_date = DATE_ADD(DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR)), INTERVAL ? DAY), updated_at = NOW()
		 WHERE id = ?`,
		newCount, interval, wordID)
	if err != nil {
		return 0, "", err
	}

	var nd sql.NullString
	err = DB.QueryRow("SELECT next_review_date FROM words WHERE id = ?", wordID).Scan(&nd)
	if err != nil {
		return 0, "", err
	}
	if nd.Valid {
		nextDate = nd.String
	}

	return newCount, nextDate, nil
}

// GetWordReviewStats 获取单词的复习统计（仅每日模式，自由模式不计入）
// wordCount: 该单词本身被复习的次数（从 words.review_count 读取）
// baseCount: 该单词所属词族的总复习次数（所有单词都有，衍生词取基础词的词族）
// nextDate: 下次复习日期（NULL 时返回空字符串）
func GetWordReviewStats(wordID int) (wordCount int, baseCount int, nextDate string, err error) {
	// 1. 查询单词信息 + review_count + next_review_date
	var word, wType string
	var baseWord sql.NullString
	var nextReview sql.NullString
	err = DB.QueryRow("SELECT word, type, base_word, review_count, next_review_date FROM words WHERE id = ?", wordID).
		Scan(&word, &wType, &baseWord, &wordCount, &nextReview)
	if err != nil {
		return 0, 0, "", err
	}
	if nextReview.Valid {
		nextDate = nextReview.String
	}

	// 2. 词族总复习次数（衍生词追溯到基础词来计算）
	familyRoot := word
	if wType != "基础词" && baseWord.Valid && baseWord.String != "" {
		familyRoot = baseWord.String
	}
	err = DB.QueryRow(`
		SELECT COUNT(*) FROM review_logs rl
		JOIN words w ON rl.word_id = w.id
		WHERE w.word = ? OR w.base_word = ?
	`, familyRoot, familyRoot).Scan(&baseCount)
	if err != nil {
		return wordCount, 0, nextDate, err
	}

	return wordCount, baseCount, nextDate, nil
}

// ---------- 自由复习 ----------

// GetRandomWordForFreeReview 自由模式随机抽词（无每日约束）
func GetRandomWordForFreeReview() (*model.WordEntry, int, error) {
	var familyRoot string
	err := DB.QueryRow("SELECT word FROM words WHERE type = '基础词' ORDER BY RAND() LIMIT 1").Scan(&familyRoot)
	if err == sql.ErrNoRows {
		return nil, 0, fmt.Errorf("数据库中没有单词")
	}
	if err != nil {
		return nil, 0, err
	}

	var pickedWord string
	var pickedID int
	err = DB.QueryRow(
		"SELECT id, word FROM words WHERE word = ? OR base_word = ? ORDER BY RAND() LIMIT 1",
		familyRoot, familyRoot).Scan(&pickedID, &pickedWord)
	if err != nil {
		return nil, 0, err
	}

	family, err := QueryWordFamily(pickedWord)
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

// RecordFreeReview 记录自由复习
func RecordFreeReview(wordID int) error {
	_, err := DB.Exec("INSERT INTO free_review_logs (word_id) VALUES (?)", wordID)
	return err
}

// ---------- 总览 ----------

// GetMonthOverview returns overview statistics for a given month.
func GetMonthOverview(year, month int) (*model.OverviewResponse, error) {
	resp := &model.OverviewResponse{
		Year:        year,
		Month:       month,
		MonthlyData: []model.DayOverview{},
	}

	// Total words (current count, for display)
	if err := DB.QueryRow("SELECT COUNT(*) FROM words").Scan(&resp.TotalWords); err != nil {
		return nil, err
	}

	// Total reviews (daily mode only)
	if err := DB.QueryRow("SELECT COUNT(*) FROM review_logs").Scan(&resp.TotalReviews); err != nil {
		return nil, err
	}

	// Today's review count (4AM boundary)
	todayExpr := "DATE(DATE_SUB(NOW(), INTERVAL 4 HOUR))"
	if err := DB.QueryRow("SELECT COUNT(*) FROM review_logs WHERE review_date = " + todayExpr).Scan(&resp.TodayReviewed); err != nil {
		return nil, err
	}

	// Monthly daily breakdown — reads from snapshot table (historically immutable)
	rows, err := DB.Query(
		"SELECT review_date, word_count, is_completed FROM daily_stats WHERE YEAR(review_date)=? AND MONTH(review_date)=? ORDER BY review_date",
		year, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var d model.DayOverview
		var rd string
		if err := rows.Scan(&rd, &d.ReviewCount, &d.IsCompleted); err != nil {
			return nil, err
		}
		if len(rd) >= 10 {
			d.Date = rd[:10]
		} else {
			d.Date = rd
		}
		resp.MonthlyData = append(resp.MonthlyData, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Streak: consecutive days from most recent review date backwards
	streakRows, err := DB.Query("SELECT DISTINCT review_date FROM review_logs ORDER BY review_date DESC")
	if err != nil {
		return nil, err
	}
	defer streakRows.Close()

	var dates []string
	for streakRows.Next() {
		var d string
		if err := streakRows.Scan(&d); err != nil {
			return nil, err
		}
		if len(d) >= 10 {
			dates = append(dates, d[:10])
		} else {
			dates = append(dates, d)
		}
	}

	if len(dates) > 0 {
		// Determine "today" in 4AM terms
		var todayStr string
		if err := DB.QueryRow("SELECT " + todayExpr).Scan(&todayStr); err != nil {
			return nil, err
		}
		if len(todayStr) >= 10 {
			todayStr = todayStr[:10]
		}

		dateSet := make(map[string]bool, len(dates))
		for _, d := range dates {
			dateSet[d] = true
		}

		// Start streak check from most recent date (dates[0])
		// If mostRecentDate == today or mostRecentDate == yesterday, count includes it
		checkDate := dates[0]
		streak := 0
		for dateSet[checkDate] {
			streak++
			// Move one day back
			t, _ := time.Parse("2006-01-02", checkDate)
			checkDate = t.Add(-24 * time.Hour).Format("2006-01-02")
		}
		resp.Streak = streak
	}

	return resp, nil
}

// SaveWords 批量保存单词
func SaveWords(entries []model.WordEntry) (int, error) {
	count := 0
	for _, entry := range entries {
		if entry.Word == "" {
			continue
		}
		if err := SaveWord(entry); err != nil {
			log.Printf("❌ 保存 %s 失败: %v", entry.Word, err)
			continue
		}
		count++
	}
	return count, nil
}
