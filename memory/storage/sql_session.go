package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/gookit/slog"
)

// SaveMessage 保存对话消息
func (s *SQLStore) SaveMessage(ctx context.Context, message *memory.ConversationMessage) error {
	if message == nil {
		return errors.New("消息对象不能为空")
	}
	if message.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if message.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 如果没有ID，生成一个
	if message.ID == "" {
		message.ID = utils.GetULID()
	}

	// 设置时间戳
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// 转换为数据库模型
	model := &ConversationMessageModel{}
	model.FromConversationMessage(message)

	// 保存到数据库
	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).Create(model).Error; err != nil {
		return fmt.Errorf("保存消息到%s失败: %v", s.db.Config.Dialector.Name(), err)
	}

	return nil
}

// GetMessages 获取会话的消息历史
func (s *SQLStore) GetMessages(ctx context.Context, sessionID string, userID string, limit int) ([]*memory.ConversationMessage, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var models []ConversationMessageModel
	query := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("created_at DESC")

	if limit > 0 {
		// 获取最新的limit条消息
		query = query.Limit(limit)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("获取消息历史失败: %v", err)
	}

	// 转换为业务模型
	var messages []*memory.ConversationMessage
	for _, model := range models {
		messages = append(messages, model.ToConversationMessage())
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// DeleteMessages 删除会话的消息历史
func (s *SQLStore) DeleteMessages(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	if err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).Where("session_id = ? AND user_id = ?", sessionID, userID).
		Delete(&ConversationMessageModel{}).Error; err != nil {
		return fmt.Errorf("删除消息历史失败: %v", err)
	}

	return nil
}

// CleanupOldMessages 清理指定时间之前的消息
func (s *SQLStore) CleanupOldMessages(ctx context.Context, userID string, before time.Time) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	result := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).
		Where("user_id = ? AND created_at < ?", userID, before).
		Delete(&ConversationMessageModel{})

	if result.Error != nil {
		return fmt.Errorf("清理旧消息失败: %v", result.Error)
	}

	if result.RowsAffected > 0 {
		slog.Infof("SQL存储清理了 %d 条旧消息，用户: %s", result.RowsAffected, userID)
	}

	return nil
}

// CleanupMessagesByLimit 按数量限制清理消息，保留最新的N条
func (s *SQLStore) CleanupMessagesByLimit(ctx context.Context, userID, sessionID string, keepLimit int) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if keepLimit <= 0 {
		return errors.New("保留数量必须大于0")
	}

	// 获取消息总数
	var totalCount int64
	countErr := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Count(&totalCount).Error

	if countErr != nil {
		return fmt.Errorf("获取消息总数失败: %v", countErr)
	}

	if int(totalCount) <= keepLimit {
		return nil // 消息数量在限制内，无需清理
	}

	// 找到要保留的消息的最早创建时间
	var earliestKeepTime time.Time
	err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).
		Select("created_at").
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("created_at DESC").
		Limit(1).
		Offset(keepLimit - 1).
		Scan(&earliestKeepTime).Error

	if err != nil {
		return fmt.Errorf("获取清理边界时间失败: %v", err)
	}

	// 删除早于边界时间的所有消息
	result := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).
		Where("user_id = ? AND session_id = ? AND created_at < ?", userID, sessionID, earliestKeepTime).
		Delete(&ConversationMessageModel{})

	if result.Error != nil {
		return fmt.Errorf("按限制清理消息失败: %v", result.Error)
	}

	if result.RowsAffected > 0 {
		slog.Infof("SQL存储按限制清理了 %d 条旧消息，会话: %s, 用户: %s", result.RowsAffected, sessionID, userID)
	}

	return nil
}

// GetMessageCount 获取消息总数
func (s *SQLStore) GetMessageCount(ctx context.Context, userID, sessionID string) (int, error) {
	if userID == "" {
		return 0, errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return 0, errors.New("会话ID不能为空")
	}

	var count int64
	err := s.db.WithContext(ctx).Table(s.tableNameProvider.GetConversationMessageTableName()).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("获取消息总数失败: %v", err)
	}

	return int(count), nil
}
