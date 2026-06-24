package logic

import (
	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// OverviewHandler 总览业务编排
type OverviewHandler struct {
	familyRepo *repo.WordFamilyRepo
}

// NewOverviewHandler 创建 OverviewHandler
func NewOverviewHandler(familyRepo *repo.WordFamilyRepo) *OverviewHandler {
	return &OverviewHandler{familyRepo: familyRepo}
}

// GetOverview 获取月度总览数据
func (h *OverviewHandler) GetOverview(year, month int, userID int) (*model.OverviewResponse, error) {
	return h.familyRepo.GetMonthOverview(year, month, userID)
}
