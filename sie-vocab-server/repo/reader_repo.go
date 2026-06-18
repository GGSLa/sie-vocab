package repo

import (
	"database/sql"
	"encoding/json"

	"sie-vocab-server/model"
)

// GetCachedReaderPage retrieves a cached reader response for a given page.
// Returns nil, nil if no cache entry exists.
func GetCachedReaderPage(page int) (*model.ReaderChunkResponse, error) {
	var aiResponse string
	err := DB.QueryRow("SELECT ai_response FROM reader_cache WHERE page = ?", page).Scan(&aiResponse)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result model.ReaderChunkResponse
	if err := json.Unmarshal([]byte(aiResponse), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SaveCachedReaderPage caches a reader response for a given page.
// Uses INSERT ... ON DUPLICATE KEY UPDATE for idempotent writes.
func SaveCachedReaderPage(page int, sectionTitle, rawText string, response *model.ReaderChunkResponse) error {
	jsonBytes, _ := json.Marshal(response)
	_, err := DB.Exec(
		`INSERT INTO reader_cache (page, section_title, raw_text, ai_response)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   section_title = VALUES(section_title),
		   raw_text = VALUES(raw_text),
		   ai_response = VALUES(ai_response),
		   updated_at = NOW()`,
		page, sectionTitle, rawText, string(jsonBytes))
	return err
}
