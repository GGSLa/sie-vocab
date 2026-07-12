package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordSaveHandler 保存单词业务编排
type WordSaveHandler struct {
	familyRepo      *repo.WordFamilyRepo
	globalCacheRepo *repo.GlobalWordCacheRepo
}

// NewWordSaveHandler 创建 WordSaveHandler
func NewWordSaveHandler(familyRepo *repo.WordFamilyRepo, globalCacheRepo *repo.GlobalWordCacheRepo) *WordSaveHandler {
	return &WordSaveHandler{familyRepo: familyRepo, globalCacheRepo: globalCacheRepo}
}

// Save 保存单个单词（个人表 + 全局缓存）
func (h *WordSaveHandler) Save(entry model.WordEntry, userID int) error {
	if err := h.familyRepo.SaveWord(entry, userID); err != nil {
		return err
	}
	// 同步更新全局缓存（best-effort，忽略错误）
	_ = h.globalCacheRepo.UpsertWord(entry)
	return nil
}
