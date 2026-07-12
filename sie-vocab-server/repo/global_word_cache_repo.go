package repo

import (
	"database/sql"
	"encoding/json"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sie-vocab-server/model"
)

// GlobalWordCache GORM model for global_word_cache table (no user_id, shared across all users)
type GlobalWordCache struct {
	ID           int64          `gorm:"primaryKey;column:id;autoIncrement"`
	Word         string         `gorm:"column:word;uniqueIndex:idx_global_word_cache_word"`
	BaseWord     sql.NullString `gorm:"column:base_word"`
	Type         string         `gorm:"column:type;default:基础词"`
	Pos          string         `gorm:"column:pos"`
	Derivation   sql.NullString `gorm:"column:derivation"`
	MeaningsJSON string         `gorm:"column:meanings_json"`
	ExamplesJSON string         `gorm:"column:examples_json"`
}

// TableName overrides the default table name
func (GlobalWordCache) TableName() string { return "global_word_cache" }

// GlobalWordCacheRepo 管理 global_word_cache 表的 CRUD（全局共享缓存，无 user_id）
type GlobalWordCacheRepo struct {
	db *gorm.DB
}

// NewGlobalWordCacheRepo 创建 GlobalWordCacheRepo
func NewGlobalWordCacheRepo(db *gorm.DB) *GlobalWordCacheRepo {
	return &GlobalWordCacheRepo{db: db}
}

// FindByWord 按单词查找单条缓存记录，未找到返回 nil, nil
func (r *GlobalWordCacheRepo) FindByWord(word string) (*model.WordEntry, error) {
	var c GlobalWordCache
	err := r.db.Where("word = ?", word).First(&c).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rowToWordEntry(&c), nil
}

// FindFamily 查找词族中所有单词（word = root 或 base_word = root），基础词在前
func (r *GlobalWordCacheRepo) FindFamily(root string) ([]model.WordEntry, error) {
	var rows []GlobalWordCache
	err := r.db.Where("word = ? OR base_word = ?", root, root).
		Order("CASE WHEN type = '基础词' THEN 0 ELSE 1 END, id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	words := make([]model.WordEntry, 0, len(rows))
	for i := range rows {
		words = append(words, *rowToWordEntry(&rows[i]))
	}
	return words, nil
}

// FindFamilyByWord 先查单词确定词根，再查整个词族（镜像 WordFamilyRepo.QueryWordFamily 逻辑）
func (r *GlobalWordCacheRepo) FindFamilyByWord(word string) ([]model.WordEntry, error) {
	// 1. 查单词
	entry, err := r.FindByWord(word)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	// 2. 确定词根
	root := word
	if entry.BaseWord != nil && *entry.BaseWord != "" {
		root = *entry.BaseWord
	}

	// 3. 查词族
	return r.FindFamily(root)
}

// UpsertWord 插入或更新全局缓存中的单词（保存时调用，覆盖更新）
func (r *GlobalWordCacheRepo) UpsertWord(entry model.WordEntry) error {
	c := buildGlobalWordCache(entry)
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "word"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"base_word", "type", "pos", "derivation",
			"meanings_json", "examples_json",
		}),
	}).Create(&c).Error
}

// InsertWord 仅在词不存在时插入（AI 翻译时调用，已存在则跳过）
func (r *GlobalWordCacheRepo) InsertWord(entry model.WordEntry) error {
	c := buildGlobalWordCache(entry)
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "word"}},
		DoNothing: true,
	}).Create(&c).Error
}

// buildGlobalWordCache 将 WordEntry 转换为 DB 行结构
func buildGlobalWordCache(entry model.WordEntry) GlobalWordCache {
	meaningsJSON, _ := json.Marshal(entry.Meanings)
	examplesJSON, _ := json.Marshal(entry.Examples)

	c := GlobalWordCache{
		Word:         entry.Word,
		Type:         entry.Type,
		Pos:          entry.Pos,
		MeaningsJSON: string(meaningsJSON),
		ExamplesJSON: string(examplesJSON),
	}
	if entry.BaseWord != nil {
		c.BaseWord = sql.NullString{String: *entry.BaseWord, Valid: true}
	}
	if entry.Derivation != nil {
		c.Derivation = sql.NullString{String: *entry.Derivation, Valid: true}
	}
	return c
}

// rowToWordEntry 将 DB 行转换为 WordEntry
func rowToWordEntry(c *GlobalWordCache) *model.WordEntry {
	entry := &model.WordEntry{
		Word: c.Word,
		Type: c.Type,
		Pos:  c.Pos,
	}
	if c.BaseWord.Valid && c.BaseWord.String != "" {
		bw := c.BaseWord.String
		entry.BaseWord = &bw
	}
	if c.Derivation.Valid && c.Derivation.String != "" {
		d := c.Derivation.String
		entry.Derivation = &d
	}

	// 反序列化 meanings
	if c.MeaningsJSON != "" {
		var meanings []model.Meaning
		if json.Unmarshal([]byte(c.MeaningsJSON), &meanings) == nil {
			entry.Meanings = meanings
		}
	}
	if entry.Meanings == nil {
		entry.Meanings = []model.Meaning{}
	}

	// 反序列化 examples
	if c.ExamplesJSON != "" {
		var examples []model.Example
		if json.Unmarshal([]byte(c.ExamplesJSON), &examples) == nil {
			entry.Examples = examples
		}
	}
	if entry.Examples == nil {
		entry.Examples = []model.Example{}
	}

	return entry
}
