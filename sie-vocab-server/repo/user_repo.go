package repo

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"sie-vocab-server/model"
)

// UserRepo 用户数据访问层
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo 创建 UserRepo
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// FindByUsername 按用户名查找用户
func (r *UserRepo) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create 创建新用户
func (r *UserRepo) Create(user *model.User) error {
	return r.db.Create(user).Error
}

// FindByID 按 ID 查找用户
func (r *UserRepo) FindByID(id int, user *model.User) error {
	return r.db.Where("id = ?", id).First(user).Error
}

// CountAll 统计用户总数
func (r *UserRepo) CountAll() (int, error) {
	var count int64
	err := r.db.Model(&model.User{}).Count(&count).Error
	return int(count), err
}

// ClaimOrphanedData 将 user_id=0 的孤儿数据分配给指定用户
// 在第一个用户注册时调用，将已有的所有数据归于该用户
func (r *UserRepo) ClaimOrphanedData(userID int) error {
	tables := []string{
		"words", "meanings", "examples",
		"review_logs", "free_review_logs",
		"daily_stats", "books", "reader_progress",
	}

	for _, table := range tables {
		result := r.db.Exec(
			fmt.Sprintf("UPDATE %s SET user_id = ? WHERE user_id = 0", table),
			userID,
		)
		if result.Error != nil {
			return fmt.Errorf("迁移 %s 表数据失败: %v", table, result.Error)
		}
		if result.RowsAffected > 0 {
			log.Printf("📦 孤儿数据迁移: %s 表 %d 行 → user_id=%d", table, result.RowsAffected, userID)
		}
	}
	return nil
}
