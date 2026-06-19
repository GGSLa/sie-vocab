package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewFreeRandomHandler 自由模式抽词业务编排
type ReviewFreeRandomHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewReviewFreeRandomHandler 创建 ReviewFreeRandomHandler
func NewReviewFreeRandomHandler(familyRepo *repo.WordFamilyRepo) *ReviewFreeRandomHandler {
	return &ReviewFreeRandomHandler{familyRepo: familyRepo}
}

// GetRandom 随机抽取一个单词（无间隔约束）
func (h *ReviewFreeRandomHandler) GetRandom() (*model.ReviewRandomResponse, error) {
	entry, wordID, err := h.familyRepo.GetRandomWordForFreeReview()
	if err != nil {
		return nil, err
	}
	return &model.ReviewRandomResponse{
		WordID: wordID,
		Word:   *entry,
	}, nil
}
