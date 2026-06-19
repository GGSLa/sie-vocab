package repo

import (
	"database/sql"
	"encoding/json"
	"sort"

	"sie-vocab-server/model"
)

// TocEntry holds a single TOC item (one cached page).
type TocEntry struct {
	Page    int    `json:"page"`
	Section string `json:"section"`
}

// GetAllCachedPages returns all cached pages with their section titles, sorted by page.
func GetAllCachedPages() ([]TocEntry, error) {
	rows, err := DB.Query("SELECT page, section_title FROM reader_cache ORDER BY page ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []TocEntry
	for rows.Next() {
		var e TocEntry
		if err := rows.Scan(&e.Page, &e.Section); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	// If no cached pages yet, return empty slice (not null)
	if entries == nil {
		entries = []TocEntry{}
	}
	// Already sorted by DB query, but ensure determinism
	sort.Slice(entries, func(i, j int) bool { return entries[i].Page < entries[j].Page })
	return entries, nil
}

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
