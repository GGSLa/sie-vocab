package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordSaveAllHandler 批量保存业务编排
type WordSaveAllHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewWordSaveAllHandler 创建 WordSaveAllHandler
func NewWordSaveAllHandler(familyRepo *repo.WordFamilyRepo) *WordSaveAllHandler {
	return &WordSaveAllHandler{familyRepo: familyRepo}
}

// SaveAll 批量保存单词
func (h *WordSaveAllHandler) SaveAll(words []model.WordEntry) (int, error) {
	return h.familyRepo.SaveWords(words)
}
