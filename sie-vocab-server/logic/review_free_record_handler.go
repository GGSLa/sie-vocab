package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewFreeRecordHandler 自由模式记录复习业务编排
type ReviewFreeRecordHandler struct {
	familyRepo    *repo.WordFamilyRepo
	freeLogRepo   *repo.FreeReviewLogRepo
}

// NewReviewFreeRecordHandler 创建 ReviewFreeRecordHandler
func NewReviewFreeRecordHandler(familyRepo *repo.WordFamilyRepo, freeLogRepo *repo.FreeReviewLogRepo) *ReviewFreeRecordHandler {
	return &ReviewFreeRecordHandler{familyRepo: familyRepo, freeLogRepo: freeLogRepo}
}

// Record 记录自由复习（不更新 words 计数）
func (h *ReviewFreeRecordHandler) Record(wordID int) (*model.ReviewRecordResult, error) {
	if err := h.freeLogRepo.Insert(wordID); err != nil {
		return nil, err
	}

	wCount, bCount, nextDate, _ := h.familyRepo.GetWordReviewStats(wordID)
	return &model.ReviewRecordResult{
		WordCount: wCount,
		BaseCount: bCount,
		NextDate:  nextDate,
	}, nil
}
