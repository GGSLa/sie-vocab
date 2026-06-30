package repo

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// columnExists 检查列是否已存在
func columnExists(db *gorm.DB, table, column string) bool {
	var count int64
	db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		table, column).Scan(&count)
	return count > 0
}

// indexExists 检查索引是否已存在
func indexExists(db *gorm.DB, table, index string) bool {
	var count int64
	db.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, index).Scan(&count)
	return count > 0
}

// findUniqueIndexOnColumn 查找指定表指定列上的唯一索引名称（非主键）
func findUniqueIndexOnColumn(db *gorm.DB, table, column string) string {
	var name string
	db.Raw(`
		SELECT INDEX_NAME FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		AND COLUMN_NAME = ? AND INDEX_NAME != 'PRIMARY'
		AND NON_UNIQUE = 0 LIMIT 1
	`, table, column).Scan(&name)
	return name
}

// safeExec 执行 SQL，忽略 "Duplicate column" 等错误
func safeExec(db *gorm.DB, sql string, label string) {
	if err := db.Exec(sql).Error; err != nil {
		log.Printf("⚠️  迁移步骤 [%s] 跳过（可能已执行过）: %v", label, err)
	} else {
		log.Printf("✅ 迁移步骤 [%s] 完成", label)
	}
}

// fixUniqueIndex 安全替换唯一索引：找旧索引 → 删除 → 建新复合索引
func fixUniqueIndex(db *gorm.DB, table, oldColumn, newIndexName, newColumns string) {
	if indexExists(db, table, newIndexName) {
		return // 已迁移完成
	}
	// 查找旧唯一索引名
	oldName := findUniqueIndexOnColumn(db, table, oldColumn)
	if oldName != "" {
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", table, oldName)).Error; err != nil {
			log.Printf("⚠️  删除旧索引 %s.%s 失败: %v", table, oldName, err)
			return
		}
		log.Printf("🗑️  已删除旧索引: %s.%s", table, oldName)
	}
	safeExec(db, fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX %s (%s)", table, newIndexName, newColumns), newIndexName)
}

// AutoMigrate 自动迁移数据库结构（用户系统）
func AutoMigrate(db *gorm.DB) error {
	log.Println("🔧 开始数据库迁移...")

	// ── 1. 创建 users 表 ──
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(100) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE INDEX idx_users_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`).Error; err != nil {
		return fmt.Errorf("创建 users 表失败: %v", err)
	}
	log.Println("✅ users 表就绪")

	// ── 2. 各表添加 user_id 列 ──

	// books
	if !columnExists(db, "books", "user_id") {
		safeExec(db, "ALTER TABLE books ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "books.user_id")
		safeExec(db, "ALTER TABLE books ADD INDEX idx_books_user_id (user_id)", "books.idx_user_id")
	}

	// words — 重建唯一索引
	if !columnExists(db, "words", "user_id") {
		safeExec(db, "ALTER TABLE words ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "words.user_id")
	}
	fixUniqueIndex(db, "words", "word", "idx_words_user_word", "user_id, word")

	// meanings
	if !columnExists(db, "meanings", "user_id") {
		safeExec(db, "ALTER TABLE meanings ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "meanings.user_id")
		safeExec(db, "ALTER TABLE meanings ADD INDEX idx_meanings_user_id (user_id)", "meanings.idx_user_id")
	}

	// examples
	if !columnExists(db, "examples", "user_id") {
		safeExec(db, "ALTER TABLE examples ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "examples.user_id")
		safeExec(db, "ALTER TABLE examples ADD INDEX idx_examples_user_id (user_id)", "examples.idx_user_id")
	}

	// review_logs — 重建唯一索引
	if !columnExists(db, "review_logs", "user_id") {
		safeExec(db, "ALTER TABLE review_logs ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "review_logs.user_id")
	}
	fixUniqueIndex(db, "review_logs", "word_id", "idx_review_logs_user_word_date", "user_id, word_id, review_date")

	// free_review_logs
	if !columnExists(db, "free_review_logs", "user_id") {
		safeExec(db, "ALTER TABLE free_review_logs ADD COLUMN user_id INT NOT NULL DEFAULT 0 AFTER id", "free_review_logs.user_id")
		safeExec(db, "ALTER TABLE free_review_logs ADD INDEX idx_free_review_logs_user_id (user_id)", "free_review_logs.idx_user_id")
	}

	// daily_stats — 重建主键
	if !columnExists(db, "daily_stats", "user_id") {
		safeExec(db, "ALTER TABLE daily_stats ADD COLUMN user_id INT NOT NULL DEFAULT 0 FIRST", "daily_stats.user_id")
		db.Exec("ALTER TABLE daily_stats DROP PRIMARY KEY")
		safeExec(db, "ALTER TABLE daily_stats ADD PRIMARY KEY (user_id, review_date)", "daily_stats.pk")
	}

	// reader_progress — 重建主键（需先删外键约束）
	if !columnExists(db, "reader_progress", "user_id") {
		safeExec(db, "ALTER TABLE reader_progress ADD COLUMN user_id INT NOT NULL DEFAULT 0 FIRST", "reader_progress.user_id")
		// Try to drop FK if exists
		db.Exec("ALTER TABLE reader_progress DROP FOREIGN KEY reader_progress_ibfk_1")
		db.Exec("ALTER TABLE reader_progress DROP PRIMARY KEY")
		safeExec(db, "ALTER TABLE reader_progress ADD PRIMARY KEY (user_id, book_id)", "reader_progress.pk")
	}

	// ── 3. 创建 invitations 表 ──
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS invitations (
			id INT AUTO_INCREMENT PRIMARY KEY,
			inviter_user_id INT NOT NULL,
			invited_username VARCHAR(100) NOT NULL,
			used TINYINT(1) NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE INDEX idx_invitations_username (invited_username),
			INDEX idx_invitations_inviter (inviter_user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`).Error; err != nil {
		return fmt.Errorf("创建 invitations 表失败: %v", err)
	}
	log.Println("✅ invitations 表就绪")

	// ── 4. 创建 daily_word_pool 表 ──
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS daily_word_pool (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			user_id INT NOT NULL,
			pool_date DATE NOT NULL,
			batch_num INT NOT NULL DEFAULT 1,
			word_id INT NOT NULL,
			word VARCHAR(100) NOT NULL,
			family_root VARCHAR(100) NOT NULL,
			is_due TINYINT(1) NOT NULL DEFAULT 1,
			sort_order INT NOT NULL DEFAULT 0,
			drawn TINYINT(1) NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_lookup (user_id, pool_date, batch_num, drawn)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`).Error; err != nil {
		return fmt.Errorf("创建 daily_word_pool 表失败: %v", err)
	}
	log.Println("✅ daily_word_pool 表就绪")

	log.Println("✅ 数据库迁移完成")
	return nil
}
