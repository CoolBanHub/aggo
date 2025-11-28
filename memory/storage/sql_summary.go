package storage

import (
	"errors"
	"fmt"
	"time"

	"context"

	"github.com/CoolBanHub/aggo/memory"
	"gorm.io/gorm"
)

// SaveSessionSummary 保存会话摘要
func (s *SQLStore) SaveSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 设置时间戳
	now := time.Now()
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = now
	}
	summary.UpdatedAt = now

	// 转换为数据库模型
	model := &SessionSummaryModel{}
	model.FromSessionSummary(summary)

	// 使用UPSERT语义
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetSessionSummaryTableName()).Save(model).Error; err != nil {
		return fmt.Errorf("保存会话摘要到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// GetSessionSummary 获取会话摘要
func (s *SQLStore) GetSessionSummary(ctx context.Context, sessionID string, userID string) (*memory.SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var model SessionSummaryModel
	err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetSessionSummaryTableName()).Where("session_id = ? AND user_id = ?", sessionID, userID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 摘要不存在
		}
		return nil, fmt.Errorf("获取会话摘要失败: %v", err)
	}

	return model.ToSessionSummary(), nil
}

// UpdateSessionSummary 更新会话摘要
func (s *SQLStore) UpdateSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 检查摘要是否存在
	var exists bool
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetSessionSummaryTableName()).
		Select("count(*) > 0").Where("session_id = ? AND user_id = ?", summary.SessionID, summary.UserID).
		Find(&exists).Error; err != nil {
		return fmt.Errorf("检查会话摘要是否存在失败: %v", err)
	}
	if !exists {
		return errors.New("会话摘要不存在")
	}

	// 更新时间戳
	summary.UpdatedAt = time.Now()

	// 转换为数据库模型
	model := &SessionSummaryModel{}
	model.FromSessionSummary(summary)

	// 更新数据库
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetSessionSummaryTableName()).Save(model).Error; err != nil {
		return fmt.Errorf("更新会话摘要到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// DeleteSessionSummary 删除会话摘要
func (s *SQLStore) DeleteSessionSummary(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	result := s.db.WithContext(ctx).Table(s.tableNameProvider.GetSessionSummaryTableName()).Delete(&SessionSummaryModel{}, "session_id = ? AND user_id = ?", sessionID, userID)
	if result.Error != nil {
		return fmt.Errorf("删除会话摘要失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("会话摘要不存在")
	}

	return nil
}
