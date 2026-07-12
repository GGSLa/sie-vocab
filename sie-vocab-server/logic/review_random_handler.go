package logic

import (
	"fmt"

	"sie-vocab-server/model"
	"sie-vocab-server/repo"
)

// ReviewRandomHandler 每日模式抽词业务编排（含词池生成）
type ReviewRandomHandler struct {
	familyRepo      *repo.WordFamilyRepo
	poolRepo        *repo.DailyPoolRepo
	reviewLogRepo   *repo.ReviewLogRepo
}

// NewReviewRandomHandler 创建 ReviewRandomHandler
func NewReviewRandomHandler(familyRepo *repo.WordFamilyRepo, poolRepo *repo.DailyPoolRepo, reviewLogRepo *repo.ReviewLogRepo) *ReviewRandomHandler {
	return &ReviewRandomHandler{
		familyRepo:    familyRepo,
		poolRepo:      poolRepo,
		reviewLogRepo: reviewLogRepo,
	}
}

// GetRandom 抽取一个到期复习单词（词池模式）
func (h *ReviewRandomHandler) GetRandom(userID int) (*model.ReviewRandomResponse, bool, error) {
	poolDate := repo.Today4AM()

	// 1. 检查 / 创建词池
	batch, err := h.poolRepo.GetActiveBatch(userID, poolDate)
	if err != nil {
		return nil, false, err
	}
	if batch == 0 {
		batch = 1
		if err := h.GeneratePool(userID, batch); err != nil {
			return nil, false, err
		}
	}

	// 2. 从池中抽词
	wordID, word, err := h.poolRepo.GetUndrawnWord(userID, poolDate, batch)
	if err != nil {
		// 批次已耗尽 — 检查还能不能再来一批
		canMore, checkErr := h.canGenerateMore(userID, poolDate)
		if checkErr != nil {
			canMore = false
		}
		if !canMore {
			// 真正无词可用
			return nil, true, fmt.Errorf("所有单词均已复习")
		}
		drawn, _ := h.poolRepo.CountDrawn(userID, poolDate, batch)
		total, _ := h.poolRepo.CountBatchTotal(userID, poolDate, batch)
		return &model.ReviewRandomResponse{BatchDone: true, CanMore: true, BatchDrawn: drawn, BatchTotal: total}, false, nil
	}

	// 3. 获取完整词族数据
	family, err := h.familyRepo.QueryWordFamily(word, userID)
	if err != nil {
		return nil, false, err
	}

	// 4. 批次进度
	drawn, _ := h.poolRepo.CountDrawn(userID, poolDate, batch)
	total, _ := h.poolRepo.CountBatchTotal(userID, poolDate, batch)

	for _, entry := range family {
		if entry.Word == word {
			return &model.ReviewRandomResponse{
				WordID:     wordID,
				Word:       entry,
				BatchDrawn: drawn,
				BatchTotal: total,
			}, false, nil
		}
	}

	return nil, false, fmt.Errorf("单词 %s 未在词族中找到", word)
}

// GeneratePool 生成词池：选 30 个族各取一词（到期优先，不足补非到期按剩余时间排）
func (h *ReviewRandomHandler) GeneratePool(userID int, batchNum int) error {
	poolDate := repo.Today4AM()

	// ── 收集排除列表：已复习族 + 已入池族 ──
	reviewed, _ := h.reviewLogRepo.GetReviewedFamilies(userID)
	pooled, _ := h.poolRepo.GetPooledFamilies(userID, poolDate)
	exclude := append(reviewed, pooled...)

	// ── 选到期族 ──
	dueFamilies, err := h.familyRepo.GetDueFamilies(userID, poolDate, exclude, 30)
	if err != nil {
		return fmt.Errorf("查询到期词族失败: %w", err)
	}

	// ── 不足 30 补非到期族（按剩余时间排序）──
	allFamilies := dueFamilies
	if len(dueFamilies) < 30 {
		exclude = append(exclude, dueFamilies...)
		nonDue, err := h.familyRepo.GetNonDueFamilies(userID, poolDate, exclude, 30-len(dueFamilies))
		if err != nil {
			return fmt.Errorf("查询非到期词族失败: %w", err)
		}
		allFamilies = append(allFamilies, nonDue...)
	}

	if len(allFamilies) == 0 {
		return fmt.Errorf("没有可入池的词族")
	}

	// ── 每个族选一词 ──
	var poolWords []model.PoolWord
	for i, family := range allFamilies {
		wordID, word, err := h.familyRepo.PickWordFromFamily(userID, family, poolDate)
		if err != nil {
			continue // 跳过选不出的族
		}
		isDue := i < len(dueFamilies)
		poolWords = append(poolWords, model.PoolWord{
			WordID:     wordID,
			Word:       word,
			FamilyRoot: family,
			IsDue:      isDue,
			SortOrder:  i, // 到期族在前（0~N），非到期按剩余时间排
		})
	}

	if len(poolWords) == 0 {
		return fmt.Errorf("没有可入池的单词")
	}

	return h.poolRepo.InsertPoolBatch(userID, poolDate, batchNum, poolWords)
}

// canGenerateMore 检查是否还能生成下一批
func (h *ReviewRandomHandler) canGenerateMore(userID int, poolDate string) (bool, error) {
	reviewed, _ := h.reviewLogRepo.GetReviewedFamilies(userID)
	pooled, _ := h.poolRepo.GetPooledFamilies(userID, poolDate)
	exclude := append(reviewed, pooled...)

	due, nonDue, err := h.poolRepo.CountAvailableFamilies(userID, poolDate, exclude)
	if err != nil {
		return false, err
	}
	return (due + nonDue) > 0, nil
}
