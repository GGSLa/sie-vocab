package service

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"sie-vocab-server/model"

	"sie-vocab-server/logic"
)

// contextUserID 用于从 context 中提取 userID（字符串键，与 main.go 保持一致）
const contextUserID = "userID"

// getUserID 从请求 context 中提取认证用户的 ID
func getUserID(r *http.Request) int {
	if uid, ok := r.Context().Value(contextUserID).(int); ok {
		return uid
	}
	return 0
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ────────── AI 翻译 ──────────

// HandleChat AI 翻译接口
func HandleChat(h *logic.ChatHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, model.ChatResponse{Error: "只接受 POST 请求"})
			return
		}
		var req model.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, model.ChatResponse{Error: "请求格式错误"})
			return
		}
		if req.Message == "" {
			writeJSON(w, http.StatusBadRequest, model.ChatResponse{Error: "消息不能为空"})
			return
		}
		log.Printf("📩 收到消息: %s", req.Message)
		reply, err := h.Chat(req.Message)
		if err != nil {
			log.Printf("❌ DeepSeek 调用失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, model.ChatResponse{Error: "AI 服务异常，请稍后重试"})
			return
		}
		log.Printf("✅ DeepSeek 回复长度: %d 字节", len(reply))
		writeJSON(w, http.StatusOK, model.ChatResponse{Reply: reply})
	}
}

// ────────── 单词 ──────────

// HandleWordQuery 查询单词族
func HandleWordQuery(h *logic.WordQueryHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.QueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Word == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word 为空"})
			return
		}
		userID := getUserID(r)
		resp, err := h.Query(req.Word, userID)
		if err != nil {
			log.Printf("❌ 查询单词失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "数据库查询失败"})
			return
		}
		if resp.Found {
			log.Printf("📚 从数据库找到 %d 个相关单词", len(resp.Data.Words))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// HandleWordSave 保存单个单词
func HandleWordSave(h *logic.WordSaveHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var entry model.WordEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
			return
		}
		if entry.Word == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "word 不能为空"})
			return
		}
		userID := getUserID(r)
		if err := h.Save(entry, userID); err != nil {
			log.Printf("❌ 保存单词失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存失败"})
			return
		}
		log.Printf("💾 已保存单词: %s", entry.Word)
		writeJSON(w, http.StatusOK, model.SaveResult{Success: true})
	}
}

// HandleWordSaveAll 批量保存单词
func HandleWordSaveAll(h *logic.WordSaveAllHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.SaveAllRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
			return
		}
		userID := getUserID(r)
		count, err := h.SaveAll(req.Words, userID)
		if err != nil {
			log.Printf("❌ 批量保存失败: %v", err)
		}
		log.Printf("💾 批量保存完成: %d/%d 个单词", count, len(req.Words))
		writeJSON(w, http.StatusOK, model.SaveResult{Success: true, Count: count})
	}
}

// ────────── 复习 — 每日模式 ──────────

// HandleReviewRandom 每日模式随机抽词
func HandleReviewRandom(h *logic.ReviewRandomHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		userID := getUserID(r)
		resp, allDone, err := h.GetRandom(userID)
		if err != nil {
			log.Printf("❌ 随机抽取复习单词失败: %v", err)
			writeJSON(w, http.StatusOK, model.ReviewErrorResponse{
				Error:   "抽取复习单词失败",
				AllDone: allDone,
			})
			return
		}
		// 批次完成（非错误，是正常状态）
		if resp.BatchDone {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"batch_done":  true,
				"can_more":    resp.CanMore,
				"batch_drawn": resp.BatchDrawn,
				"batch_total": resp.BatchTotal,
			})
			return
		}
		log.Printf("🎲 复习抽词: %s (id=%d)", resp.Word.Word, resp.WordID)
		writeJSON(w, http.StatusOK, resp)
	}
}

// HandleReviewRecord 每日模式记录复习
func HandleReviewRecord(h *logic.ReviewRecordHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.ReviewRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WordID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word_id 无效"})
			return
		}
		userID := getUserID(r)
		result, err := h.Record(req.WordID, userID)
		if err != nil {
			log.Printf("❌ 记录复习失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "记录复习失败"})
			return
		}
		log.Printf("📝 已记录复习: word_id=%d (词:%d 族:%d 下次:%s 批次:%d/%d 剩余:%d)",
			req.WordID, result.WordCount, result.BaseCount, result.NextDate,
			result.BatchDrawn, result.BatchTotal, result.BatchRemaining)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":          true,
			"word_count":       result.WordCount,
			"base_count":       result.BaseCount,
			"next_review_date": result.NextDate,
			"batch_drawn":      result.BatchDrawn,
			"batch_total":      result.BatchTotal,
			"batch_remaining":  result.BatchRemaining,
		})
	}
}

// ────────── 复习 — 自由模式 ──────────

// HandleReviewNextBatch "再来一批"生成新词池
func HandleReviewNextBatch(h *logic.ReviewNextBatchHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		userID := getUserID(r)
		resp, err := h.NextBatch(userID)
		if err != nil {
			log.Printf("❌ 生成下一批失败: %v", err)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"error":    "没有更多可复习的单词",
				"all_done": true,
			})
			return
		}
		log.Printf("📦 新一批复习词池: 首词 %s (id=%d)", resp.Word.Word, resp.WordID)
		writeJSON(w, http.StatusOK, resp)
	}
}

// ────────── 复习 — 自由模式 ──────────

// HandleReviewFreeRandom 自由模式随机抽词
func HandleReviewFreeRandom(h *logic.ReviewFreeRandomHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		userID := getUserID(r)
		resp, err := h.GetRandom(userID)
		if err != nil {
			log.Printf("❌ 自由复习抽词失败: %v", err)
			writeJSON(w, http.StatusOK, model.ReviewErrorResponse{Error: "抽取复习单词失败"})
			return
		}
		log.Printf("🎲 自由复习抽词: %s (id=%d)", resp.Word.Word, resp.WordID)
		writeJSON(w, http.StatusOK, resp)
	}
}

// HandleReviewFreeRecord 自由模式记录复习
func HandleReviewFreeRecord(h *logic.ReviewFreeRecordHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.ReviewRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WordID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word_id 无效"})
			return
		}
		userID := getUserID(r)
		result, err := h.Record(req.WordID, userID)
		if err != nil {
			log.Printf("❌ 记录自由复习失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "记录失败"})
			return
		}
		log.Printf("📝 自由复习记录: word_id=%d（不计入统计，当前词:%d 族:%d 下次:%s）",
			req.WordID, result.WordCount, result.BaseCount, result.NextDate)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":          true,
			"word_count":       result.WordCount,
			"base_count":       result.BaseCount,
			"next_review_date": result.NextDate,
		})
	}
}

// ────────── 总览 ──────────

// HandleOverview 月度总览
func HandleOverview(h *logic.OverviewHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.OverviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			now := time.Now()
			req.Year = now.Year()
			req.Month = int(now.Month())
		}
		if req.Year == 0 {
			req.Year = time.Now().Year()
		}
		if req.Month < 1 || req.Month > 12 {
			req.Month = int(time.Now().Month())
		}
		userID := getUserID(r)
		data, err := h.GetOverview(req.Year, req.Month, userID)
		if err != nil {
			log.Printf("❌ 获取总览数据失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取总览数据失败"})
			return
		}
		log.Printf("📊 总览: year=%d month=%d words=%d reviews=%d streak=%d",
			req.Year, req.Month, data.TotalWords, data.TotalReviews, data.Streak)
		writeJSON(w, http.StatusOK, data)
	}
}

// ────────── 教材阅读 ──────────

// HandleReaderChunk 阅读页 AI 分析
func HandleReaderChunk(h *logic.ReaderChunkHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.ReaderChunkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Page <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page 参数无效"})
			return
		}
		if req.BookID <= 0 {
			req.BookID = 1
		}
		userID := getUserID(r)
		result, err := h.GetChunk(req.BookID, req.Page, userID)
		if err != nil {
			log.Printf("❌ 获取阅读块失败 (book=%d, page=%d): %v", req.BookID, req.Page, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取阅读内容失败"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleReaderDefaultBook 获取默认书籍（最近阅读 > 第一本 > 无书提示）
func HandleReaderDefaultBook(h *logic.ReaderProgressHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}
		userID := getUserID(r)
		progress, err := h.GetDefaultBook(userID)
		if err != nil {
			if err.Error() == "没有书籍" {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"error":    "没有书籍，请先上传 PDF",
					"no_books": true,
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取默认书籍失败"})
			return
		}
		writeJSON(w, http.StatusOK, progress)
	}
}

// HandleReaderProgress 阅读进度（GET 加载 / POST 保存）
func HandleReaderProgress(h *logic.ReaderProgressHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		// 提取 book_id（query param 或 body，默认 1）
		getBookID := func() int {
			if idStr := r.URL.Query().Get("book"); idStr != "" {
				if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
					return id
				}
			}
			return 1
		}
		switch r.Method {
		case http.MethodGet:
			progress, err := h.Load(getBookID(), userID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "加载进度失败"})
				return
			}
			writeJSON(w, http.StatusOK, progress)
		case http.MethodPost:
			var req model.SaveProgressRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
				return
			}
			if req.BookID <= 0 {
				req.BookID = 1
			}
			if err := h.Save(req.BookID, req.CurrentPage, req.CurrentChunk, req.Section, userID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存进度失败"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "不支持的请求方法"})
		}
	}
}

// HandleReaderTOC 目录大纲
func HandleReaderTOC(h *logic.ReaderTOCHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}
		bookID := 1
		if idStr := r.URL.Query().Get("book"); idStr != "" {
			if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
				bookID = id
			}
		}
		userID := getUserID(r)
		result, err := h.GetTOC(bookID, userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取目录失败"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleReaderBreakdownSentence 句子深度拆解（不缓存，实时调 AI）
func HandleReaderBreakdownSentence(h *logic.ReaderBreakdownHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		var req model.BreakdownSentenceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sentence == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 sentence 为空"})
			return
		}
		if len(req.Sentence) > 2000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "句子过长，请选中 2000 字符以内的文本"})
			return
		}
		result, err := h.Breakdown(req.Sentence)
		if err != nil {
			log.Printf("❌ 句子拆解失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "拆解失败，请稍后重试"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleReaderPageImage PDF 页图片渲染
func HandleReaderPageImage(h *logic.ReaderPageImageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}
		pageStr := r.URL.Query().Get("page")
		if pageStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page 参数必填"})
			return
		}
		page, err := strconv.Atoi(pageStr)
		if err != nil || page <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page 参数无效"})
			return
		}
		bookID := 1
		if idStr := r.URL.Query().Get("book"); idStr != "" {
			if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
				bookID = id
			}
		}
		userID := getUserID(r)
		data, err := h.GetPageImage(bookID, page, userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "PDF 渲染失败"})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	}
}

// ────────── 书架 ──────────

// HandleBookshelfList 书架列表
func HandleBookshelfList(h *logic.BookshelfHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}
		userID := getUserID(r)
		result, err := h.List(userID)
		if err != nil {
			log.Printf("❌ 获取书架列表失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取书架列表失败"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleBookshelfGetSingle 获取单本书（含阅读进度）
func HandleBookshelfGetSingle(h *logic.BookshelfHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数必填"})
			return
		}
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}
		userID := getUserID(r)
		result, err := h.GetSingle(id, userID)
		if err != nil {
			log.Printf("❌ 获取书籍失败 (id=%d): %v", id, err)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "书籍不存在"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleBookshelfCreate 上传新书（multipart/form-data）
func HandleBookshelfCreate(h *logic.BookshelfHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件过大或格式错误"})
			return
		}

		title := r.FormValue("title")
		author := r.FormValue("author")
		description := r.FormValue("description")
		ocrLang := r.FormValue("ocr_lang")

		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请选择 PDF 文件"})
			return
		}
		defer file.Close()

		pdfData, err := io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "读取文件失败"})
			return
		}
		if len(pdfData) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件为空"})
			return
		}
		if len(pdfData) < 4 || string(pdfData[:4]) != "%PDF" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件不是有效的 PDF"})
			return
		}

		userID := getUserID(r)
		book, err := h.Create(title, author, description, ocrLang, pdfData, userID)
		if err != nil {
			log.Printf("❌ 上传书籍失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "上传书籍失败，请稍后重试"})
			return
		}
		log.Printf("📚 新书上架: id=%d title=%q pages=%d", book.ID, book.Title, book.PageCount)
		writeJSON(w, http.StatusOK, book)
	}
}

// HandleBookshelfDelete 删除书籍
func HandleBookshelfDelete(h *logic.BookshelfHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 DELETE 请求"})
			return
		}
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数必填"})
			return
		}
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id 参数无效"})
			return
		}
		userID := getUserID(r)
		if err := h.Delete(id, userID); err != nil {
			log.Printf("❌ 删除书籍失败 (id=%d): %v", id, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "删除书籍失败，请稍后重试"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	}
}
