package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordSaveAllHandler 批量保存业务编排
type WordSaveAllHandler struct {
	familyRepo      *repo.WordFamilyRepo
	globalCacheRepo *repo.GlobalWordCacheRepo
}

// NewWordSaveAllHandler 创建 WordSaveAllHandler
func NewWordSaveAllHandler(familyRepo *repo.WordFamilyRepo, globalCacheRepo *repo.GlobalWordCacheRepo) *WordSaveAllHandler {
	return &WordSaveAllHandler{familyRepo: familyRepo, globalCacheRepo: globalCacheRepo}
}

// SaveAll 批量保存单词（个人表 + 全局缓存）
func (h *WordSaveAllHandler) SaveAll(words []model.WordEntry, userID int) (int, error) {
	count, err := h.familyRepo.SaveWords(words, userID)
	// 同步更新全局缓存（best-effort，忽略错误）
	for _, entry := range words {
		_ = h.globalCacheRepo.UpsertWord(entry)
	}
	return count, err
}
