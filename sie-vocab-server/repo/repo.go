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
