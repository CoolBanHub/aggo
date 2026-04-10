package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/gookit/slog"
)

// MemoryStore 内存存储实现
// 这是一个基于内存的记忆存储实现，适合测试和开发环境
type MemoryStore struct {
	// 读写锁，保证并发安全
	mu sync.RWMutex

	// 用户记忆存储 map[userID]*UserMemory
	userMemories map[string]*builtin.UserMemory

	// 会话摘要存储 map[sessionID+userID]*SessionSummary
	sessionSummaries map[string]*builtin.SessionSummary

	// 对话消息存储 map[sessionID+userID][]*ConversationMessage
	messages map[string][]*builtin.ConversationMessage
}

// NewMemoryStore 创建新的内存存储实例
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		userMemories:     make(map[string]*builtin.UserMemory),
		sessionSummaries: make(map[string]*builtin.SessionSummary),
		messages:         make(map[string][]*builtin.ConversationMessage),
	}
}

func (m *MemoryStore) AutoMigrate() error {
	return nil
}

// generateKey 生成会话相关的复合键
func (m *MemoryStore) generateKey(sessionID, userID string) string {
	return fmt.Sprintf("%s:%s", sessionID, userID)
}

// UpsertUserMemory 创建或更新用户记忆（每个用户一条记录）
func (m *MemoryStore) UpsertUserMemory(ctx context.Context, userMemory *builtin.UserMemory) error {
	if userMemory == nil {
		return errors.New("记忆对象不能为空")
	}
	if userMemory.UserID == "" {
		return errors.New("用户ID不能为空")
	}
	if userMemory.Memory == "" {
		return errors.New("记忆内容不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 设置时间戳
	now := time.Now()
	if userMemory.CreatedAt.IsZero() {
		userMemory.CreatedAt = now
	}
	userMemory.UpdatedAt = now

	// 保存记忆
	m.userMemories[userMemory.UserID] = userMemory
	return nil
}

// GetUserMemory 获取用户的记忆
func (m *MemoryStore) GetUserMemory(ctx context.Context, userID string) (*builtin.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	userMemory, exists := m.userMemories[userID]
	if !exists {
		return nil, nil
	}

	return userMemory, nil
}

// ClearUserMemory 清空用户记忆
func (m *MemoryStore) ClearUserMemory(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.userMemories, userID)
	return nil
}

// SaveSessionSummary 保存会话摘要
func (m *MemoryStore) SaveSessionSummary(ctx context.Context, summary *builtin.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 设置时间戳
	now := time.Now()
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = now
	}
	summary.UpdatedAt = now

	key := m.generateKey(summary.SessionID, summary.UserID)
	m.sessionSummaries[key] = summary
	return nil
}

// GetSessionSummary 获取会话摘要
func (m *MemoryStore) GetSessionSummary(ctx context.Context, sessionID string, userID string) (*builtin.SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.generateKey(sessionID, userID)
	summary, exists := m.sessionSummaries[key]
	if !exists {
		return nil, nil // 没找到返回nil，不是错误
	}

	return summary, nil
}

// UpdateSessionSummary 更新会话摘要
func (m *MemoryStore) UpdateSessionSummary(ctx context.Context, summary *builtin.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.generateKey(summary.SessionID, summary.UserID)
	existing, exists := m.sessionSummaries[key]
	if !exists {
		return errors.New("会话摘要不存在")
	}

	// 保持原有创建时间
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = existing.CreatedAt
	}
	summary.UpdatedAt = time.Now()

	m.sessionSummaries[key] = summary
	return nil
}

// DeleteSessionSummary 删除会话摘要
func (m *MemoryStore) DeleteSessionSummary(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.generateKey(sessionID, userID)
	delete(m.sessionSummaries, key)
	return nil
}

// SaveMessage 保存对话消息
func (m *MemoryStore) SaveMessage(ctx context.Context, message *builtin.ConversationMessage) error {
	if message == nil {
		return errors.New("消息对象不能为空")
	}
	if message.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if message.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果没有ID，生成一个
	if message.ID == "" {
		message.ID = utils.GetULID()
	}

	// 设置时间戳
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	key := m.generateKey(message.SessionID, message.UserID)
	m.messages[key] = append(m.messages[key], message)
	return nil
}

// GetMessages 获取会话的消息历史
func (m *MemoryStore) GetMessages(ctx context.Context, sessionID string, userID string, limit int) ([]*builtin.ConversationMessage, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.generateKey(sessionID, userID)
	msgs, exists := m.messages[key]
	if !exists {
		return []*builtin.ConversationMessage{}, nil
	}

	messages := make([]*builtin.ConversationMessage, len(msgs))
	copy(messages, msgs)

	// 按时间排序（最新的在后面）
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	// 应用限制（如果指定了limit，返回最后的limit条消息）
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	return messages, nil
}

// GetMessagesAfter 获取游标之后的会话消息历史。
func (m *MemoryStore) GetMessagesAfter(ctx context.Context, sessionID string, userID string, afterMessageID string, afterTime time.Time, limit int) ([]*builtin.ConversationMessage, error) {
	messages, err := m.GetMessages(ctx, sessionID, userID, 0)
	if err != nil {
		return nil, err
	}

	filtered := make([]*builtin.ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if !afterTime.IsZero() {
			if msg.CreatedAt.After(afterTime) {
				filtered = append(filtered, msg)
				continue
			}
			if msg.CreatedAt.Before(afterTime) {
				continue
			}
		}
		if afterMessageID == "" && afterTime.IsZero() {
			filtered = append(filtered, msg)
			continue
		}
		if afterMessageID != "" && msg.ID > afterMessageID {
			filtered = append(filtered, msg)
		}
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
}

// GetMessageCountAfter 获取游标之后的会话消息数量。
func (m *MemoryStore) GetMessageCountAfter(ctx context.Context, sessionID string, userID string, afterMessageID string, afterTime time.Time) (int, error) {
	messages, err := m.GetMessages(ctx, sessionID, userID, 0)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, msg := range messages {
		if !afterTime.IsZero() {
			if msg.CreatedAt.After(afterTime) {
				count++
				continue
			}
			if msg.CreatedAt.Before(afterTime) {
				continue
			}
		}
		if afterMessageID == "" && afterTime.IsZero() {
			count++
			continue
		}
		if afterMessageID != "" && msg.ID > afterMessageID {
			count++
		}
	}
	return count, nil
}

// DeleteMessages 删除会话的消息历史
func (m *MemoryStore) DeleteMessages(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.generateKey(sessionID, userID)
	delete(m.messages, key)
	return nil
}

// Close 关闭存储连接（内存存储无需关闭）
func (m *MemoryStore) Close() error {
	return nil
}

// CleanupOldMessages 清理指定时间之前的消息
func (m *MemoryStore) CleanupOldMessages(ctx context.Context, userID string, before time.Time) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cleanedCount := 0
	for sessionKey, messages := range m.messages {
		// 检查是否属于指定用户
		if !strings.HasSuffix(sessionKey, ":"+userID) {
			continue
		}

		// 过滤掉旧消息
		var validMessages []*builtin.ConversationMessage
		for _, msg := range messages {
			if msg.CreatedAt.After(before) {
				validMessages = append(validMessages, msg)
			} else {
				cleanedCount++
			}
		}

		if len(validMessages) == 0 {
			// 如果没有有效消息了，删除整个条目
			delete(m.messages, sessionKey)
		} else {
			m.messages[sessionKey] = validMessages
		}
	}

	if cleanedCount > 0 {
		slog.Infof("清理了 %d 条旧消息，用户: %s", cleanedCount, userID)
	}
	return nil
}

// CleanupMessagesByLimit 按数量限制清理消息，保留最新的N条
func (m *MemoryStore) CleanupMessagesByLimit(ctx context.Context, userID, sessionID string, keepLimit int) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if keepLimit <= 0 {
		return errors.New("保留数量必须大于0")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.generateKey(sessionID, userID)
	messages, exists := m.messages[key]
	if !exists {
		return nil // 没有消息需要清理
	}

	// 按时间排序（最新的在后面）
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	if len(messages) <= keepLimit {
		return nil // 消息数量在限制内，无需清理
	}

	// 保留最新的N条消息
	validMessages := messages[len(messages)-keepLimit:]
	cleanedCount := len(messages) - keepLimit
	m.messages[key] = validMessages

	slog.Infof("按限制清理了 %d 条旧消息，会话: %s, 用户: %s", cleanedCount, sessionID, userID)
	return nil
}

// GetMessageCount 获取消息总数
func (m *MemoryStore) GetMessageCount(ctx context.Context, userID, sessionID string) (int, error) {
	if userID == "" {
		return 0, errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return 0, errors.New("会话ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.generateKey(sessionID, userID)
	messages, exists := m.messages[key]
	if !exists {
		return 0, nil
	}

	return len(messages), nil
}
