package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
	"sie-vocab-server/service"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := repo.InitDB(cfg.MySQL)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()
	log.Println("✅ 数据库连接成功")

	exePath, _ := os.Executable()
	staticDir := filepath.Join(filepath.Dir(exePath), "..", "sie-vocab-web")

	// API 路由
	http.HandleFunc("/api/chat", service.HandleChat(cfg))
	http.HandleFunc("/api/word/query", service.HandleWordQuery)
	http.HandleFunc("/api/word/save", service.HandleWordSave)
	http.HandleFunc("/api/word/save-all", service.HandleWordSaveAll)

	// 静态文件
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		http.FileServer(http.Dir(staticDir)).ServeHTTP(w, r)
	})

	log.Printf("🚀 服务启动: http://0.0.0.0:%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, nil))
}

func loadConfig() (*model.Config, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("获取可执行文件路径失败: %v", err)
	}
	configPath := filepath.Join(filepath.Dir(exePath), "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %v", configPath, err)
	}
	var cfg model.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}
	if cfg.DeepSeekAPIKey == "" {
		return nil, fmt.Errorf("配置文件中 deepseek_api_key 不能为空")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.MySQL.Host == "" {
		cfg.MySQL.Host = "localhost"
	}
	if cfg.MySQL.Port == "" {
		cfg.MySQL.Port = "3306"
	}
	return &cfg, nil
}
