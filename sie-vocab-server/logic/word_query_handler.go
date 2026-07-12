package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordQueryHandler 查词业务编排
type WordQueryHandler struct {
	familyRepo      *repo.WordFamilyRepo
	globalCacheRepo *repo.GlobalWordCacheRepo
}

// NewWordQueryHandler 创建 WordQueryHandler
func NewWordQueryHandler(familyRepo *repo.WordFamilyRepo, globalCacheRepo *repo.GlobalWordCacheRepo) *WordQueryHandler {
	return &WordQueryHandler{familyRepo: familyRepo, globalCacheRepo: globalCacheRepo}
}

// Query 查询单词族（个人表 → 全局缓存 → 未找到）
func (h *WordQueryHandler) Query(word string, userID int) (*model.QueryResponse, error) {
	// 1. 查个人表
	words, err := h.familyRepo.QueryWordFamily(word, userID)
	if err != nil {
		return nil, err
	}
	if len(words) > 0 {
		return &model.QueryResponse{
			Found:  true,
			Source: "personal",
			Data:   &model.WordsResponse{Words: words},
		}, nil
	}

	// 2. 查全局缓存
	globalWords, err := h.globalCacheRepo.FindFamilyByWord(word)
	if err != nil {
		return nil, err
	}
	if len(globalWords) > 0 {
		return &model.QueryResponse{
			Found:  true,
			Source: "global",
			Data:   &model.WordsResponse{Words: globalWords},
		}, nil
	}

	// 3. 都没找到
	return &model.QueryResponse{Found: false}, nil
}
