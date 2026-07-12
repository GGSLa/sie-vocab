package model

import "database/sql"

// GlobalWordCacheRow 用于从 global_word_cache 表扫描行数据
type GlobalWordCacheRow struct {
	ID           int64          `gorm:"column:id"`
	Word         string         `gorm:"column:word"`
	BaseWord     sql.NullString `gorm:"column:base_word"`
	Type         string         `gorm:"column:type"`
	Pos          string         `gorm:"column:pos"`
	Derivation   sql.NullString `gorm:"column:derivation"`
	MeaningsJSON string         `gorm:"column:meanings_json"`
	ExamplesJSON string         `gorm:"column:examples_json"`
}
