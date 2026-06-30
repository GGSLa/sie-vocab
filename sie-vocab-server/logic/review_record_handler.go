package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewRecordHandler 每日模式记录复习业务编排
type ReviewRecordHandler struct {
	familyRepo *repo.WordFamilyRepo
	poolRepo   *repo.DailyPoolRepo
}

// NewReviewRecordHandler 创建 ReviewRecordHandler
func NewReviewRecordHandler(familyRepo *repo.WordFamilyRepo, poolRepo *repo.DailyPoolRepo) *ReviewRecordHandler {
	return &ReviewRecordHandler{familyRepo: familyRepo, poolRepo: poolRepo}
}

// Record 记录一次复习，返回统计信息（含当前批次剩余数）
func (h *ReviewRecordHandler) Record(wordID int, userID int) (*model.ReviewRecordResult, error) {
	poolDate := repo.Today4AM()

	// 1. 记录复习（复用现有逻辑）
	_, nextDate, err := h.familyRepo.RecordReview(wordID, userID)
	if err != nil {
		return nil, err
	}

	// 2. 标记词池中已抽取
	_ = h.poolRepo.MarkDrawn(userID, wordID, poolDate)

	// 3. 获取统计
	wCount, bCount, nd, _ := h.familyRepo.GetWordReviewStats(wordID, userID)
	if nextDate == "" {
		nextDate = nd
	}

	// 4. 当前批次剩余
	batch, _ := h.poolRepo.GetActiveBatch(userID, poolDate)
	remaining, _ := h.poolRepo.CountRemaining(userID, poolDate, batch)

	return &model.ReviewRecordResult{
		WordCount:      wCount,
		BaseCount:      bCount,
		NextDate:       nextDate,
		BatchRemaining: remaining,
	}, nil
}
