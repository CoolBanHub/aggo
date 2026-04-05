package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"gorm.io/gorm"
)

// UpsertUserMemory 创建或更新用户记忆（每个用户一条记录）
func (s *SQLStore) UpsertUserMemory(ctx context.Context, userMemory *builtin.UserMemory) error {
	if userMemory == nil {
		return errors.New("记忆对象不能为空")
	}
	if userMemory.UserID == "" {
		return errors.New("用户ID不能为空")
	}
	if userMemory.Memory == "" {
		return errors.New("记忆内容不能为空")
	}

	// 设置时间戳
	now := time.Now()
	if userMemory.CreatedAt.IsZero() {
		userMemory.CreatedAt = now
	}
	userMemory.UpdatedAt = now

	// 转换为数据库模型
	model := &UserMemoryModel{}
	model.FromUserMemory(userMemory)

	// 使用 GORM 的 Clauses 实现 upsert（OnConflict）
	// 主键是 UserID，所以会自动判断是创建还是更新
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).
		Save(model).Error; err != nil {
		return fmt.Errorf("保存用户记忆到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// GetUserMemory 获取用户的记忆
func (s *SQLStore) GetUserMemory(ctx context.Context, userID string) (*builtin.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var model UserMemoryModel
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).
		Where("user_id = ?", userID).First(&model).Error; err != nil {
		// 如果记录不存在，返回nil而不是错误
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取用户记忆失败: %v", err)
	}

	return model.ToUserMemory(), nil
}

// ClearUserMemory 清空用户记忆
func (s *SQLStore) ClearUserMemory(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).
		Where("user_id = ?", userID).Delete(&UserMemoryModel{}).Error; err != nil {
		return fmt.Errorf("清空用户记忆失败: %v", err)
	}

	return nil
}
