package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// WordSaveHandler 保存单词业务编排
type WordSaveHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewWordSaveHandler 创建 WordSaveHandler
func NewWordSaveHandler(familyRepo *repo.WordFamilyRepo) *WordSaveHandler {
	return &WordSaveHandler{familyRepo: familyRepo}
}

// Save 保存单个单词
func (h *WordSaveHandler) Save(entry model.WordEntry) error {
	return h.familyRepo.SaveWord(entry)
}
