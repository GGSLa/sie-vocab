// backfill_hash — 一次性工具：为旧书计算 PDF 内容 SHA-256 哈希
// 用法：编译后 scp 到服务器运行一次，跑完即删
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Config struct {
	MySQL MySQLConfig `json:"mysql"`
}

type MySQLConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Book struct {
	ID      int    `gorm:"column:id"`
	PDFPath string `gorm:"column:pdf_path"`
}

func main() {
	// 读配置
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("无法读取配置文件 %s: %v", configPath, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("解析配置失败: %v", err)
	}

	// 连 MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接 MySQL 失败: %v", err)
	}

	// 查待回填的书
	var books []Book
	if err := db.Table("books").Where("content_hash = '' AND pdf_path != ''").Find(&books).Error; err != nil {
		log.Fatalf("查询 books 失败: %v", err)
	}
	if len(books) == 0 {
		log.Println("没有需要回填的书，退出")
		return
	}

	log.Printf("找到 %d 本待回填的书", len(books))
	ok := 0
	for _, b := range books {
		f, err := os.Open(b.PDFPath)
		if err != nil {
			log.Printf("⚠️  book=%d 无法打开 PDF %s: %v", b.ID, b.PDFPath, err)
			continue
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			log.Printf("⚠️  book=%d 读取失败: %v", b.ID, err)
			continue
		}
		f.Close()
		hash := hex.EncodeToString(h.Sum(nil))
		if err := db.Table("books").Where("id = ?", b.ID).Update("content_hash", hash).Error; err != nil {
			log.Printf("⚠️  book=%d 更新失败: %v", b.ID, err)
			continue
		}
		log.Printf("✅ book=%d hash=%s", b.ID, hash[:16])
		ok++
	}
	log.Printf("完成: %d/%d", ok, len(books))
}
