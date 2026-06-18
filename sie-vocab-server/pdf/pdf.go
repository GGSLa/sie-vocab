package pdf

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ExtractPageText extracts plain text from a single PDF page using pdftotext.
// Returns cleaned text with normalized whitespace.
func ExtractPageText(pdfPath string, page int) (string, error) {
	cmd := exec.Command("pdftotext",
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		"-layout",
		pdfPath,
		"-",
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed (page %d): %v\nstderr: %s", page, err, stderr.String())
	}
	return cleanText(out.String()), nil
}

// cleanText normalizes whitespace in extracted PDF text:
// - Collapses multiple spaces/tabs into single space
// - Removes leading/trailing whitespace from each line
// - Preserves paragraph breaks (blank lines)
func cleanText(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		// Replace tabs with spaces, collapse multiple spaces
		trimmed := strings.Join(strings.Fields(line), " ")
		cleaned = append(cleaned, trimmed)
	}
	result := strings.Join(cleaned, "\n")
	// Collapse 3+ newlines to 2 (preserve paragraph breaks, remove excess)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}
