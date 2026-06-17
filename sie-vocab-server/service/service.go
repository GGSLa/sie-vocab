package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"sie-vocab-server/client"
	"sie-vocab-server/model"
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

// ---------- 工具 ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
