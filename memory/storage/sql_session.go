package storage

import (
	"errors"
	"fmt"
	"time"

	"context"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/utils"
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
