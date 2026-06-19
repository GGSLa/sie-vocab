package logic

import (
	"encoding/json"
	"os"
	"time"

	"sie-vocab-server/model"
)

// ReaderProgressHandler 阅读进度加载/保存业务编排
type ReaderProgressHandler struct {
	progressPath string
}

// NewReaderProgressHandler 创建 ReaderProgressHandler
func NewReaderProgressHandler(progressPath string) *ReaderProgressHandler {
	return &ReaderProgressHandler{progressPath: progressPath}
}

// Load 加载阅读进度（文件不存在时返回默认值）
func (h *ReaderProgressHandler) Load() *model.ReaderProgress {
	data, err := os.ReadFile(h.progressPath)
	if err != nil {
		return defaultProgress()
	}
	var p model.ReaderProgress
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultProgress()
	}
	return &p
}

// Save 保存阅读进度
func (h *ReaderProgressHandler) Save(currentPage, currentChunk int, section string) error {
	p := h.Load()
	p.CurrentPage = currentPage
	p.CurrentChunk = currentChunk
	if section != "" {
		p.CurrentSection = section
	}
	p.LastRead = time.Now().Format("2006-01-02")
	return h.write(p)
}

func (h *ReaderProgressHandler) write(p *model.ReaderProgress) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.progressPath, data, 0644)
}

func defaultProgress() *model.ReaderProgress {
	return &model.ReaderProgress{
		CurrentPage:      67,
		CurrentChunk:     0,
		CurrentSection:   "Chapter 5: Securities Underwriting",
		CompletedSections: []string{},
		LastRead:         time.Now().Format("2006-01-02"),
	}
}
