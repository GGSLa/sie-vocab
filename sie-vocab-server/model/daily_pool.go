package model

// PoolWord 词池中的单个单词条目
type PoolWord struct {
	WordID     int
	Word       string
	FamilyRoot string
	IsDue      bool
	SortOrder  int
}
