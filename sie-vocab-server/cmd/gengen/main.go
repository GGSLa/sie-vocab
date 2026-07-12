// GORM Gen 代码生成器
// 从 config.json 读取数据库连接，连接 MySQL，生成类型安全的模型和查询代码
//
// 用法:
//
//	go run ./cmd/gengen/ -config ../sie-vocab-bin/config.json
//	go run ./cmd/gengen/                          # 自动搜索 config.json
//
// 配置文件优先级: -config flag > SIE_VOCAB_CONFIG 环境变量 > ./config.json > ../config.json > ../sie-vocab-bin/config.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gen"
)

// mysqlConfig 数据库连接配置（对应 config.json 中的 mysql 字段）
type mysqlConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// appConfig 仅包含生成器需要的字段
type appConfig struct {
	MySQL mysqlConfig `json:"mysql"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	fmt.Println("已连接到数据库，开始生成代码...")

	// 配置 GORM Gen
	g := gen.NewGenerator(gen.Config{
		OutPath:      "repo/gen/query",    // 查询构建器输出目录（package query）
		ModelPkgPath: "repo/gen/model",   // 模型 struct 输出目录（package model），不用 "gen" 避免与 gorm.io/gen 冲突
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface | gen.WithoutContext,
	})

	g.UseDB(db)

	// 为数据库中所有表生成模型和基础 CRUD 查询
	// DATE 列映射为 string（匹配现有代码中大量使用的字符串日期模式，如 Today4AM()）
	g.ApplyBasic(g.GenerateAllTable(
		gen.FieldTypeReg(`_date$`, "string"),   // next_review_date, review_date, pool_date 等
		gen.FieldType("last_read", "string"),    // reader_progress.last_read
	)...)

	g.Execute()

	fmt.Println("✅ GORM Gen 代码生成完成！")
	fmt.Println("   模型:   repo/gen/*.go")
	fmt.Println("   查询:   repo/gen/query/*.go")
	return nil
}

// loadConfig 读取 config.json，优先级从高到低：
//  1. -config 命令行参数
//  2. SIE_VOCAB_CONFIG 环境变量
//  3. ./config.json（项目根目录）
//  4. ../config.json（sie-vocab-server 上一级）
//  5. ../sie-vocab-bin/config.json（本地运行目录）
func loadConfig() (*appConfig, error) {
	var configPath string
	flag.StringVar(&configPath, "config", "", "config.json 文件路径")
	flag.Parse()

	if configPath == "" {
		configPath = os.Getenv("SIE_VOCAB_CONFIG")
	}

	searchPaths := []string{"./config.json", "../config.json", "../sie-vocab-bin/config.json"}
	if configPath == "" {
		for _, p := range searchPaths {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	if configPath == "" {
		return nil, fmt.Errorf("未找到 config.json。请使用 -config 参数指定路径，或设置 SIE_VOCAB_CONFIG 环境变量\n搜索路径: %v", searchPaths)
	}

	fmt.Printf("读取配置: %s\n", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", configPath, err)
	}

	var cfg appConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", configPath, err)
	}

	// 应用默认值（与 main.go loadConfig 一致）
	if cfg.MySQL.Host == "" {
		cfg.MySQL.Host = "localhost"
	}
	if cfg.MySQL.Port == "" {
		cfg.MySQL.Port = "3306"
	}

	return &cfg, nil
}
