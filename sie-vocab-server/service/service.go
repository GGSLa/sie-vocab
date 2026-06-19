package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
	"sie-vocab-server/pdf"
	"sie-vocab-server/repo"
)

// HandleChat AI 翻译接口
func HandleChat(cfg *model.Config) http.HandlerFunc {
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

		reply, err := client.CallDeepSeek(cfg.DeepSeekAPIKey, req.Message)
		if err != nil {
			log.Printf("❌ DeepSeek 调用失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, model.ChatResponse{Error: fmt.Sprintf("DeepSeek 调用失败: %v", err)})
			return
		}

		log.Printf("✅ DeepSeek 回复长度: %d 字节", len(reply))
		writeJSON(w, http.StatusOK, model.ChatResponse{Reply: reply})
	}
}

// HandleWordQuery 查询单词（数据库）
func HandleWordQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req model.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Word == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word 为空"})
		return
	}

	words, err := repo.QueryWordFamily(req.Word)
	if err != nil {
		log.Printf("❌ 查询单词失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "数据库查询失败"})
		return
	}

	if len(words) == 0 {
		writeJSON(w, http.StatusOK, model.QueryResponse{Found: false})
		return
	}

	log.Printf("📚 从数据库找到 %d 个相关单词", len(words))
	writeJSON(w, http.StatusOK, model.QueryResponse{
		Found: true,
		Data:  &model.WordsResponse{Words: words},
	})
}

// HandleWordSave 保存单个单词
func HandleWordSave(w http.ResponseWriter, r *http.Request) {
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

	if err := repo.SaveWord(entry); err != nil {
		log.Printf("❌ 保存单词失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存失败"})
		return
	}

	log.Printf("💾 已保存单词: %s", entry.Word)
	writeJSON(w, http.StatusOK, model.SaveResult{Success: true})
}

// HandleWordSaveAll 批量保存单词
func HandleWordSaveAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req model.SaveAllRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
		return
	}

	count, err := repo.SaveWords(req.Words)
	if err != nil {
		log.Printf("❌ 批量保存失败: %v", err)
	}
	log.Printf("💾 批量保存完成: %d/%d 个单词", count, len(req.Words))
	writeJSON(w, http.StatusOK, model.SaveResult{Success: true, Count: count})
}

// ---------- 复习 ----------

// HandleReviewRandom 随机抽取复习单词
func HandleReviewRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	entry, wordID, err := repo.GetDueWordForReview()
	if err != nil {
		log.Printf("❌ 随机抽取复习单词失败: %v", err)
		allDone := err.Error() == "所有单词均已排期，暂无到期复习的单词" ||
			err.Error() == "今日复习已达上限（30词）"
		writeJSON(w, http.StatusOK, model.ReviewErrorResponse{
			Error:   "抽取复习单词失败",
			AllDone: allDone,
		})
		return
	}

	log.Printf("🎲 复习抽词: %s (id=%d)", entry.Word, wordID)
	writeJSON(w, http.StatusOK, model.ReviewRandomResponse{
		WordID: wordID,
		Word:   *entry,
	})
}

// HandleReviewRecord 记录复习
func HandleReviewRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req model.ReviewRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WordID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word_id 无效"})
		return
	}

	_, nextDate, err := repo.RecordReview(req.WordID)
	if err != nil {
		log.Printf("❌ 记录复习失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "记录复习失败"})
		return
	}

	wCount, bCount, nd, _ := repo.GetWordReviewStats(req.WordID)
	// 优先使用 RecordReview 返回的日期
	if nextDate == "" {
		nextDate = nd
	}

	log.Printf("📝 已记录复习: word_id=%d (词:%d 族:%d 下次:%s)", req.WordID, wCount, bCount, nextDate)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":          true,
		"word_count":       wCount,
		"base_count":       bCount,
		"next_review_date": nextDate,
	})
}

// ---------- 自由复习 ----------

// HandleReviewFreeRandom 自由模式随机抽词
func HandleReviewFreeRandom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	entry, wordID, err := repo.GetRandomWordForFreeReview()
	if err != nil {
		log.Printf("❌ 自由复习抽词失败: %v", err)
		writeJSON(w, http.StatusOK, model.ReviewErrorResponse{Error: "抽取复习单词失败"})
		return
	}

	log.Printf("🎲 自由复习抽词: %s (id=%d)", entry.Word, wordID)
	writeJSON(w, http.StatusOK, model.ReviewRandomResponse{
		WordID: wordID,
		Word:   *entry,
	})
}

// HandleReviewFreeRecord 记录自由复习
func HandleReviewFreeRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
		return
	}

	var req model.ReviewRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WordID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误或 word_id 无效"})
		return
	}

	if err := repo.RecordFreeReview(req.WordID); err != nil {
		log.Printf("❌ 记录自由复习失败: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "记录失败"})
		return
	}

	wCount, bCount, nextDate, _ := repo.GetWordReviewStats(req.WordID)

	log.Printf("📝 自由复习记录: word_id=%d（不计入统计，当前词:%d 族:%d 下次:%s）", req.WordID, wCount, bCount, nextDate)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":          true,
		"word_count":       wCount,
		"base_count":       bCount,
		"next_review_date": nextDate,
	})
}

// ---------- 教材阅读 ----------

// HandleReaderChunk returns AI-analyzed content for a single PDF page.
// If the page ends mid-paragraph, the跨页 paragraph is completed from the next page.
func HandleReaderChunk(cfg *model.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 POST 请求"})
			return
		}

		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Page <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page 参数无效"})
			return
		}

		log.Printf("📖 阅读请求: page=%d", req.Page)

		// 1. Check cache
		cached, err := repo.GetCachedReaderPage(req.Page)
		if err != nil {
			log.Printf("⚠️ 查询 reader_cache 失败: %v", err)
		}
		if cached != nil {
			log.Printf("✅ reader 缓存命中: page=%d, chunks=%d", req.Page, cached.TotalChunks)
			writeJSON(w, http.StatusOK, cached)
			return
		}

		// 2. Extract current page text
		pageText, err := pdf.ExtractPageTextStructured(cfg.SIE_PDFPath, req.Page)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "PDF 提取失败"})
			return
		}
		if pageText == "" {
			writeJSON(w, http.StatusOK, model.ReaderChunkResponse{
				Page:    req.Page,
				PageEnd: req.Page + 1,
				Error:   "该页无文本内容",
			})
			return
		}

		// 3. Check if page ends mid-paragraph: no double-newline at end
		pageText = strings.TrimSpace(pageText)
		if needsCrossPageCompletion(pageText) {
				// Page ends mid-paragraph — fetch next page's first body paragraph
				nextText, err := pdf.ExtractPageTextStructured(cfg.SIE_PDFPath, req.Page+1)
				if err == nil && nextText != "" {
					nextText = strings.TrimSpace(nextText)
					firstPara := extractFirstBodyParagraph(nextText)
					if firstPara != "" {
						pageText += "\n" + firstPara
						log.Printf("📎 跨页段落补齐: page=%d, 从 page=%d 取了 %d 字", req.Page, req.Page+1, len(firstPara))
					}
				}
			}

			log.Printf("📄 PDF 提取: page=%d, 总长=%d", req.Page, len(pageText))

		// 4. Call DeepSeek
		reply, err := client.CallDeepSeekWithSystem(cfg.DeepSeekAPIKey, model.ReaderSystemPrompt, pageText)
		if err != nil {
			log.Printf("❌ DeepSeek 调用失败: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "AI 分析失败"})
			return
		}

		// 5. Parse response
		result, err := parseReaderReply(reply)
		if err != nil {
			log.Printf("❌ 解析 DeepSeek 回复失败: %v\n原始回复: %.200s", err, reply)
			writeJSON(w, http.StatusOK, model.ReaderChunkResponse{
				Page:    req.Page,
				PageEnd: req.Page + 1,
				Error:   "AI 回复解析失败，请重试",
			})
			return
		}
		result.Page = req.Page
		result.PageEnd = req.Page + 1
		result.TotalChunks = len(result.Chunks)

		// 6. Cache
		go func() {
			if err := repo.SaveCachedReaderPage(req.Page, result.Section, pageText, result); err != nil {
				log.Printf("❌ 保存 reader_cache 失败: %v", err)
			}
		}()

		log.Printf("✅ reader 分析完成: page=%d section=%q chunks=%d", req.Page, result.Section, result.TotalChunks)
		writeJSON(w, http.StatusOK, result)
	}
}

// HandleReaderProgress handles GET (load) and POST (save) for reading progress.
func HandleReaderProgress(cfg *model.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			p := loadProgress(cfg.SIE_ProgressPath)
			writeJSON(w, http.StatusOK, p)
		case http.MethodPost:
			var req model.SaveProgressRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式错误"})
				return
			}
			p := loadProgress(cfg.SIE_ProgressPath)
			p.CurrentPage = req.CurrentPage
			p.CurrentChunk = req.CurrentChunk
			if req.Section != "" {
				p.CurrentSection = req.Section
			}
			p.LastRead = time.Now().Format("2006-01-02")
			saveProgress(cfg.SIE_ProgressPath, p)
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "不支持的请求方法"})
		}
	}
}

// HandleReaderPageImage renders a PDF page as PNG image.
// GET /api/reader/page-image?page=67
func HandleReaderPageImage(cfg *model.Config) http.HandlerFunc {
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
		var page int
		if _, err := fmt.Sscanf(pageStr, "%d", &page); err != nil || page <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page 参数无效"})
			return
		}

		// Check cache
		cacheDir := "/tmp/sie-page-images"
		cachePath := fmt.Sprintf("%s/page-%d.png", cacheDir, page)
		if data, err := os.ReadFile(cachePath); err == nil {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Write(data)
			return
		}

		// Render page to PNG using pdftoppm
		os.MkdirAll(cacheDir, 0755)
		tmpPrefix := fmt.Sprintf("%s/tmp-%d", cacheDir, page)
		cmd := exec.Command("pdftoppm",
			"-png", "-r", "150",
			"-f", fmt.Sprintf("%d", page),
			"-l", fmt.Sprintf("%d", page),
			"-singlefile",
			cfg.SIE_PDFPath, tmpPrefix,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			log.Printf("❌ pdftoppm 渲染失败 (page=%d): %v\nstderr: %s", page, err, stderr.String())
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "PDF 渲染失败"})
			return
		}

		// Read rendered image and cache
		tmpPath := tmpPrefix + ".png"
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "读取渲染图片失败"})
			return
		}
		os.Rename(tmpPath, cachePath)

		log.Printf("🖼️ 渲染 PDF 页面: page=%d, size=%d bytes", page, len(data))
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	}
}

// ---------- reader 辅助 ----------

func loadProgress(path string) *model.ReaderProgress {
	data, err := os.ReadFile(path)
	if err != nil {
		return &model.ReaderProgress{
			CurrentPage:      67,
			CurrentChunk:     0,
			CurrentSection:   "Chapter 5: Securities Underwriting",
			CompletedSections: []string{},
			LastRead:         time.Now().Format("2006-01-02"),
		}
	}
	var p model.ReaderProgress
	if err := json.Unmarshal(data, &p); err != nil {
		return &model.ReaderProgress{
			CurrentPage:      67,
			CurrentChunk:     0,
			CurrentSection:   "Chapter 5: Securities Underwriting",
			CompletedSections: []string{},
			LastRead:         time.Now().Format("2006-01-02"),
		}
	}
	return &p
}

func saveProgress(path string, p *model.ReaderProgress) {
	data, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(path, data, 0644)
}

func parseReaderReply(reply string) (*model.ReaderChunkResponse, error) {
	var result model.ReaderChunkResponse
	if err := json.Unmarshal([]byte(reply), &result); err == nil {
		return &result, nil
	}
	cleaned := reply
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("解析 AI 回复 JSON 失败: %v", err)
	}
	return &result, nil
}

// HandleReaderTOC returns the PDF outline (from bookmarks) plus cached page numbers.
func HandleReaderTOC(cfg *model.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "只接受 GET 请求"})
			return
		}

		// Get PDF outline (built-in bookmarks)
		outline, err := pdf.ExtractOutline(cfg.SIE_PDFPath)
		if err != nil {
			log.Printf("⚠️ 提取 PDF 大纲失败，回退到缓存页面: %v", err)
			// Fallback: use cached pages
			entries, err2 := repo.GetAllCachedPages()
			if err2 != nil {
				log.Printf("❌ 获取缓存页面也失败: %v", err2)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "获取目录失败"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"outline":      nil,
				"entries":      entries,
				"cached_pages": nil,
			})
			return
		}

		// Get cached page numbers for visual markers
		entries, _ := repo.GetAllCachedPages()
		cachedPages := make(map[int]bool)
		for _, e := range entries {
			cachedPages[e.Page] = true
		}

		log.Printf("📑 TOC: %d 大纲条目, %d 已缓存页面", countOutlineItems(outline), len(cachedPages))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"outline":      outline,
			"cached_pages": cachedPages,
		})
	}
}

func countOutlineItems(items []pdf.TocItem) int {
	n := len(items)
	for _, item := range items {
		n += countOutlineItems(item.Children)
	}
	return n
}

// ---------- 工具 ----------

// needsCrossPageCompletion checks whether the page text appears to end mid-paragraph.
// Returns false if the page ends with a heading, sentence-ending punctuation, or blank line.
func needsCrossPageCompletion(pageText string) bool {
	if pageText == "" || strings.HasSuffix(pageText, "\n\n") {
		return false
	}
	last := lastLine(pageText)
	if last == "" || strings.HasPrefix(last, "#") {
		return false
	}
	if isSentenceEnd(last) {
		return false
	}
	return true
}

// isSentenceEnd returns true if the line ends with sentence-closing punctuation.
func isSentenceEnd(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '.' || last == '!' || last == '?' || last == '"' || last == ')' || last == ':'
}

// lastLine returns the last non-empty line of text.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

// extractFirstBodyParagraph returns the first body paragraph from structured text,
// skipping any heading lines (# / ## / ###).
func extractFirstBodyParagraph(s string) string {
	lines := strings.Split(s, "\n")
	var paraLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(paraLines) > 0 {
				break // end of first paragraph
			}
			continue
		}
		// Skip heading lines
		if strings.HasPrefix(trimmed, "#") {
			if len(paraLines) > 0 {
				break // heading after body text ends the paragraph
			}
			continue
		}
		paraLines = append(paraLines, trimmed)
	}
	return strings.Join(paraLines, " ")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
