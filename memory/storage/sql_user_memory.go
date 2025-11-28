package storage

import (
	"errors"
	"fmt"
	"time"

	"context"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/utils"
)

// SaveUserMemory 保存用户记忆
func (s *SQLStore) SaveUserMemory(ctx context.Context, memory *memory.UserMemory) error {
	if memory == nil {
		return errors.New("记忆对象不能为空")
	}
	if memory.UserID == "" {
		return errors.New("用户ID不能为空")
	}
	if memory.Memory == "" {
		return errors.New("记忆内容不能为空")
	}

	// 如果没有ID，生成一个
	if memory.ID == "" {
		memory.ID = utils.GetULID()
	}

	// 设置时间戳
	now := time.Now()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now

	// 转换为数据库模型
	model := &UserMemoryModel{}
	model.FromUserMemory(memory)

	// 保存到数据库
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Create(model).Error; err != nil {
		return fmt.Errorf("保存记忆到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// GetUserMemories 获取用户的记忆列表
func (s *SQLStore) GetUserMemories(ctx context.Context, userID string, limit int, retrieval memory.MemoryRetrieval) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	var models []UserMemoryModel
	query := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Where("user_id = ?", userID)

	if limit > 0 {
		// 根据检索方式确定排序和限制
		switch retrieval {
		case memory.RetrievalLastN:
			// 先按更新时间降序获取最近的N条记忆
			query = query.Order("updated_at DESC")
		case memory.RetrievalFirstN:
			// 按创建时间升序获取最早的N条记忆
			query = query.Order("created_at ASC")
		default:
			query = query.Order("updated_at DESC")
		}
		query = query.Limit(limit)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("获取用户记忆失败: %v", err)
	}

	var memories []*memory.UserMemory
	for _, model := range models {
		memories = append(memories, model.ToUserMemory())
	}

	return memories, nil
}

// UpdateUserMemory 更新用户记忆
func (s *SQLStore) UpdateUserMemory(ctx context.Context, userMemory *memory.UserMemory) error {
	if userMemory == nil {
		return errors.New("记忆对象不能为空")
	}
	if userMemory.ID == "" {
		return errors.New("记忆ID不能为空")
	}
	if userMemory.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 检查记忆是否存在
	var exists bool
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).
		Select("count(*) > 0").Where("id = ?", userMemory.ID).
		Find(&exists).Error; err != nil {
		return fmt.Errorf("检查记忆是否存在失败: %v", err)
	}
	if !exists {
		return errors.New("记忆不存在")
	}

	// 更新时间戳
	userMemory.UpdatedAt = time.Now()

	// 转换为数据库模型
	model := &UserMemoryModel{}
	model.FromUserMemory(userMemory)

	// 更新数据库
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Save(model).Error; err != nil {
		return fmt.Errorf("更新记忆到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// DeleteUserMemory 删除用户记忆
func (s *SQLStore) DeleteUserMemory(ctx context.Context, memoryID string) error {
	if memoryID == "" {
		return errors.New("记忆ID不能为空")
	}

	result := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Delete(&UserMemoryModel{}, "id = ?", memoryID)
	if result.Error != nil {
		return fmt.Errorf("删除记忆失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("记忆不存在")
	}

	return nil
}

func (s *SQLStore) DeleteUserMemoriesByIds(ctx context.Context, userID string, memoryIDs []string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if len(memoryIDs) == 0 {
		return nil
	}
	result := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Delete(&UserMemoryModel{}, "user_id = ? and id in ?", userID, memoryIDs)
	if result.Error != nil {
		return fmt.Errorf("删除记忆失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("记忆不存在")
	}

	return nil
}

// ClearUserMemories 清空用户的所有记忆
func (s *SQLStore) ClearUserMemories(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Where("user_id = ?", userID).Delete(&UserMemoryModel{}).Error; err != nil {
		return fmt.Errorf("清空用户记忆失败: %v", err)
	}

	return nil
}

// SearchUserMemories 搜索用户记忆（基于内容关键词搜索）
func (s *SQLStore) SearchUserMemories(ctx context.Context, userID string, query string, limit int) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	if query == "" {
		return []*memory.UserMemory{}, nil
	}

	var models []UserMemoryModel
	searchQuery := "%" + query + "%"

	// 根据数据库类型使用不同的搜索语法
	dbQuery := s.db.WithContext(ctx).Table(s.tableNameProvider.GetUserMemoryTableName()).Where("user_id = ?", userID)
	if s.db.Config.Dialector.Name() == DialectPostgreSQL {
		// PostgreSQL使用ILIKE进行大小写不敏感的搜索
		dbQuery = dbQuery.Where("memory ILIKE ? OR input ILIKE ?", searchQuery, searchQuery)
	} else {
		// MySQL和SQLite使用LIKE
		dbQuery = dbQuery.Where("memory LIKE ? OR input LIKE ?", searchQuery, searchQuery)
	}

	dbQuery = dbQuery.Order("updated_at DESC")

	if limit > 0 {
		dbQuery = dbQuery.Limit(limit)
	}

	if err := dbQuery.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("搜索用户记忆失败: %v", err)
	}

	// 转换为业务模型
	var memories []*memory.UserMemory
	for _, model := range models {
		memories = append(memories, model.ToUserMemory())
	}

	return memories, nil
}
