package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"sie-vocab-server/auth"
	"sie-vocab-server/client"
	"sie-vocab-server/logic"
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
	"sie-vocab-server/service"
)

// contextUserID 用于在 context 中存储用户 ID（字符串键，跨包可用）
const contextUserID = "userID"

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	db, err := repo.InitDB(cfg.MySQL)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	log.Println("✅ 数据库连接成功")

	// 初始化 AI 限流器
	client.InitRateLimiter(cfg.DeepSeekRPM, cfg.DeepSeekMaxConcurrent)
	log.Printf("⏱️ AI 限流: %d RPM / %d 并发", cfg.DeepSeekRPM, cfg.DeepSeekMaxConcurrent)

	// ── Repo 层 ──
	wordRepo := repo.NewWordRepo(db)
	meaningRepo := repo.NewMeaningRepo(db)
	exampleRepo := repo.NewExampleRepo(db)
	reviewLogRepo := repo.NewReviewLogRepo(db)
	freeReviewLogRepo := repo.NewFreeReviewLogRepo(db)
	dailyStatsRepo := repo.NewDailyStatsRepo(db)
	readerCacheRepo := repo.NewReaderCacheRepo(db)
	familyRepo := repo.NewWordFamilyRepo(db)
	bookRepo := repo.NewBookRepo(db)
	progressRepo := repo.NewReaderProgressRepo(db)
	userRepo := repo.NewUserRepo(db)
	invitationRepo := repo.NewInvitationRepo(db)

	// ── Logic 层 ──
	chatHandler := logic.NewChatHandler(cfg.DeepSeekAPIKey)
	wordQueryHandler := logic.NewWordQueryHandler(familyRepo)
	wordSaveHandler := logic.NewWordSaveHandler(familyRepo)
	wordSaveAllHandler := logic.NewWordSaveAllHandler(familyRepo)
	reviewRandomHandler := logic.NewReviewRandomHandler(familyRepo)
	reviewRecordHandler := logic.NewReviewRecordHandler(familyRepo)
	reviewFreeRandomHandler := logic.NewReviewFreeRandomHandler(familyRepo)
	reviewFreeRecordHandler := logic.NewReviewFreeRecordHandler(familyRepo, freeReviewLogRepo)
	overviewHandler := logic.NewOverviewHandler(familyRepo)
	readerChunkHandler := logic.NewReaderChunkHandler(cfg.DeepSeekAPIKey, bookRepo, readerCacheRepo)
	readerProgressHandler := logic.NewReaderProgressHandler(progressRepo, bookRepo)
	readerTOCHandler := logic.NewReaderTOCHandler(bookRepo, readerCacheRepo)
	readerPageImageHandler := logic.NewReaderPageImageHandler(bookRepo)
	bookshelfHandler := logic.NewBookshelfHandler(bookRepo, progressRepo, readerCacheRepo, cfg.UploadDir, cfg.OCRLanguage)
	authHandler := logic.NewAuthHandler(userRepo, invitationRepo, cfg.JWTSecret)
	inviteHandler := logic.NewInviteHandler(invitationRepo, userRepo)

	// 消除未使用变量警告（部分 repo 仅在 WordFamilyRepo 内部使用）
	_ = wordRepo
	_ = meaningRepo
	_ = exampleRepo
	_ = reviewLogRepo
	_ = dailyStatsRepo

	exePath, _ := os.Executable()
	staticDir := filepath.Join(filepath.Dir(exePath), "..", "sie-vocab-web")

	// ── JWT 认证中间件 ──
	withAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 安全头
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; script-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")

			// JWT 认证 — 优先从 Authorization 头获取，回退到 URL query 参数（用于 <img> 等无法设头的请求）
			authHeader := r.Header.Get("Authorization")
			tokenStr := ""
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				tokenStr = r.URL.Query().Get("token")
			}
			if tokenStr == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "未授权访问"})
				return
			}
			userID, err := auth.ValidateToken(tokenStr, cfg.JWTSecret)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "token 无效或已过期"})
				return
			}

			// 将 userID 存入 context
			ctx := context.WithValue(r.Context(), contextUserID, userID)
			next(w, r.WithContext(ctx))
		}
	}

	// ── 认证路由（无需 JWT） ──
	http.HandleFunc("/api/auth/register", service.HandleRegister(authHandler))
	http.HandleFunc("/api/auth/login", service.HandleLogin(authHandler))

	// ── 用户路由（需 JWT） ──
	http.HandleFunc("/api/user/info", withAuth(service.HandleUserInfo(authHandler)))
	http.HandleFunc("/api/invite", withAuth(service.HandleInvite(inviteHandler)))

	// ── API 路由（JWT 认证保护） ──
	http.HandleFunc("/api/chat", withAuth(service.HandleChat(chatHandler)))
	http.HandleFunc("/api/word/query", withAuth(service.HandleWordQuery(wordQueryHandler)))
	http.HandleFunc("/api/word/save", withAuth(service.HandleWordSave(wordSaveHandler)))
	http.HandleFunc("/api/word/save-all", withAuth(service.HandleWordSaveAll(wordSaveAllHandler)))
	http.HandleFunc("/api/review/random", withAuth(service.HandleReviewRandom(reviewRandomHandler)))
	http.HandleFunc("/api/review/record", withAuth(service.HandleReviewRecord(reviewRecordHandler)))
	http.HandleFunc("/api/review/free/random", withAuth(service.HandleReviewFreeRandom(reviewFreeRandomHandler)))
	http.HandleFunc("/api/review/free/record", withAuth(service.HandleReviewFreeRecord(reviewFreeRecordHandler)))
	http.HandleFunc("/api/review/overview", withAuth(service.HandleOverview(overviewHandler)))
	http.HandleFunc("/api/reader/chunk", withAuth(service.HandleReaderChunk(readerChunkHandler)))
	http.HandleFunc("/api/reader/last-book", withAuth(service.HandleReaderDefaultBook(readerProgressHandler)))
	http.HandleFunc("/api/reader/progress", withAuth(service.HandleReaderProgress(readerProgressHandler)))
	http.HandleFunc("/api/reader/toc", withAuth(service.HandleReaderTOC(readerTOCHandler)))
	http.HandleFunc("/api/reader/page-image", withAuth(service.HandleReaderPageImage(readerPageImageHandler)))

	// 书架
	http.HandleFunc("/api/books", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("id") != "" {
				service.HandleBookshelfGetSingle(bookshelfHandler)(w, r)
			} else {
				service.HandleBookshelfList(bookshelfHandler)(w, r)
			}
		case http.MethodPost:
			service.HandleBookshelfCreate(bookshelfHandler)(w, r)
		case http.MethodDelete:
			service.HandleBookshelfDelete(bookshelfHandler)(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "不支持的请求方法"})
		}
	}))

	// 静态文件（无需认证，前端 JS 自行处理重定向）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
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
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("配置文件中 jwt_secret 不能为空")
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
	if cfg.SIE_PDFPath == "" {
		cfg.SIE_PDFPath = filepath.Join(filepath.Dir(exePath), "..", "SIE.pdf")
	}
	if cfg.OCRLanguage == "" {
		cfg.OCRLanguage = "eng"
	}
	if cfg.UploadDir == "" {
		cfg.UploadDir = filepath.Join(filepath.Dir(exePath), "..", "uploads")
	}
	if cfg.DeepSeekRPM <= 0 {
		cfg.DeepSeekRPM = 10
	}
	if cfg.DeepSeekMaxConcurrent <= 0 {
		cfg.DeepSeekMaxConcurrent = 3
	}
	return &cfg, nil
}

// 消除 gorm 包未使用警告（repo 层已通过 import 使用）
var _ *gorm.DB
