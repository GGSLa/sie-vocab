package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewRandomHandler 每日模式抽词业务编排
type ReviewRandomHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewReviewRandomHandler 创建 ReviewRandomHandler
func NewReviewRandomHandler(familyRepo *repo.WordFamilyRepo) *ReviewRandomHandler {
	return &ReviewRandomHandler{familyRepo: familyRepo}
}

// GetRandom 抽取一个到期复习单词
// 返回 (response, allDone, error)
func (h *ReviewRandomHandler) GetRandom(userID int) (*model.ReviewRandomResponse, bool, error) {
	entry, wordID, err := h.familyRepo.GetDueWordForReview(userID)
	if err != nil {
		allDone := err.Error() == "所有单词均已排期，暂无到期复习的单词" ||
			err.Error() == "今日复习已达上限（30词）"
		return nil, allDone, err
	}
	return &model.ReviewRandomResponse{
		WordID: wordID,
		Word:   *entry,
	}, false, nil
}
