package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordQueryHandler 查词业务编排
type WordQueryHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewWordQueryHandler 创建 WordQueryHandler
func NewWordQueryHandler(familyRepo *repo.WordFamilyRepo) *WordQueryHandler {
	return &WordQueryHandler{familyRepo: familyRepo}
}

// Query 查询单词族
func (h *WordQueryHandler) Query(word string) (*model.QueryResponse, error) {
	words, err := h.familyRepo.QueryWordFamily(word)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return &model.QueryResponse{Found: false}, nil
	}
	return &model.QueryResponse{
		Found: true,
		Data:  &model.WordsResponse{Words: words},
	}, nil
}
