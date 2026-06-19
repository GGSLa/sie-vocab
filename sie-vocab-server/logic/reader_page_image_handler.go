package logic

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
)

// ReaderPageImageHandler PDF 页图片渲染业务编排
type ReaderPageImageHandler struct {
	pdfPath string
}

// NewReaderPageImageHandler 创建 ReaderPageImageHandler
func NewReaderPageImageHandler(pdfPath string) *ReaderPageImageHandler {
	return &ReaderPageImageHandler{pdfPath: pdfPath}
}

const pageImageCacheDir = "/tmp/sie-page-images"

// GetPageImage 获取 PDF 单页的 PNG 图片（缓存到磁盘）
// 返回图片字节数据和错误
func (h *ReaderPageImageHandler) GetPageImage(page int) ([]byte, error) {
	cachePath := fmt.Sprintf("%s/page-%d.png", pageImageCacheDir, page)

	// Check disk cache
	if data, err := os.ReadFile(cachePath); err == nil {
		return data, nil
	}

	// Render page to PNG using pdftoppm
	os.MkdirAll(pageImageCacheDir, 0755)
	tmpPrefix := fmt.Sprintf("%s/tmp-%d", pageImageCacheDir, page)

	cmd := exec.Command("pdftoppm",
		"-png", "-r", "150",
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		"-singlefile",
		h.pdfPath, tmpPrefix,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("❌ pdftoppm 渲染失败 (page=%d): %v\nstderr: %s", page, err, stderr.String())
		return nil, fmt.Errorf("PDF 渲染失败: %v", err)
	}

	// Read rendered image and cache
	tmpPath := tmpPrefix + ".png"
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("读取渲染图片失败: %v", err)
	}
	os.Rename(tmpPath, cachePath)

	log.Printf("🖼️ 渲染 PDF 页面: page=%d, size=%d bytes", page, len(data))
	return data, nil
}
