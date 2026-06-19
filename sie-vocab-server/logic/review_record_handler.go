package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewRecordHandler 每日模式记录复习业务编排
type ReviewRecordHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewReviewRecordHandler 创建 ReviewRecordHandler
func NewReviewRecordHandler(familyRepo *repo.WordFamilyRepo) *ReviewRecordHandler {
	return &ReviewRecordHandler{familyRepo: familyRepo}
}

// Record 记录一次复习，返回统计信息
func (h *ReviewRecordHandler) Record(wordID int) (*model.ReviewRecordResult, error) {
	_, nextDate, err := h.familyRepo.RecordReview(wordID)
	if err != nil {
		return nil, err
	}

	wCount, bCount, nd, _ := h.familyRepo.GetWordReviewStats(wordID)
	if nextDate == "" {
		nextDate = nd
	}

	return &model.ReviewRecordResult{
		WordCount: wCount,
		BaseCount: bCount,
		NextDate:  nextDate,
	}, nil
}
