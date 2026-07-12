package logic

import (
	"fmt"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewNextBatchHandler "再来一批"业务编排
type ReviewNextBatchHandler struct {
	randomHandler *ReviewRandomHandler
}

// NewReviewNextBatchHandler 创建 ReviewNextBatchHandler
func NewReviewNextBatchHandler(randomHandler *ReviewRandomHandler) *ReviewNextBatchHandler {
	return &ReviewNextBatchHandler{randomHandler: randomHandler}
}

// NextBatch 生成下一批词池，返回第一个单词
func (h *ReviewNextBatchHandler) NextBatch(userID int) (*model.ReviewRandomResponse, error) {
	poolDate := repo.Today4AM()

	// 获取当前批次号
	batch, err := h.randomHandler.poolRepo.GetActiveBatch(userID, poolDate)
	if err != nil {
		return nil, err
	}

	// 检查是否还能生成
	canMore, err := h.randomHandler.canGenerateMore(userID, poolDate)
	if err != nil || !canMore {
		return nil, fmt.Errorf("没有更多可复习的单词")
	}

	// 生成下一批
	newBatch := batch + 1
	if err := h.randomHandler.GeneratePool(userID, newBatch); err != nil {
		return nil, err
	}

	// 抽取第一个单词
	wordID, word, err := h.randomHandler.poolRepo.GetUndrawnWord(userID, poolDate, newBatch)
	if err != nil {
		return nil, fmt.Errorf("新批次生成失败")
	}

	family, err := h.randomHandler.familyRepo.QueryWordFamily(word, userID)
	if err != nil {
		return nil, err
	}

	// 新批次进度（刚生成，已抽取 0）
	drawn, _ := h.randomHandler.poolRepo.CountDrawn(userID, poolDate, newBatch)
	total, _ := h.randomHandler.poolRepo.CountBatchTotal(userID, poolDate, newBatch)

	for _, entry := range family {
		if entry.Word == word {
			return &model.ReviewRandomResponse{
				WordID:     wordID,
				Word:       entry,
				BatchDrawn: drawn,
				BatchTotal: total,
			}, nil
		}
	}

	return nil, fmt.Errorf("单词 %s 未在词族中找到", word)
}
