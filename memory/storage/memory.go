package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/utils"
)

// MemoryStore 内存存储实现
// 这是一个基于内存的记忆存储实现，适合测试和开发环境
type MemoryStore struct {
	// 读写锁，保证并发安全
	mu sync.RWMutex

	// 用户记忆存储 map[userID]map[memoryID]*UserMemory
	userMemories map[string]map[string]*memory.UserMemory

	// 会话摘要存储 map[sessionID+userID]*SessionSummary
	sessionSummaries map[string]*memory.SessionSummary

	// 对话消息存储 map[sessionID+userID][]*ConversationMessage
	messages map[string][]*memory.ConversationMessage
}

// NewMemoryStore 创建新的内存存储实例
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		userMemories:     make(map[string]map[string]*memory.UserMemory),
		sessionSummaries: make(map[string]*memory.SessionSummary),
		messages:         make(map[string][]*memory.ConversationMessage),
	}
}

func (m *MemoryStore) AutoMigrate() error {

	return nil
}

func (m *MemoryStore) SetTablePrefix(prefix string) {
}

// generateKey 生成会话相关的复合键
func (m *MemoryStore) generateKey(sessionID, userID string) string {
	return fmt.Sprintf("%s:%s", sessionID, userID)
}

// SaveUserMemory 保存用户记忆
func (m *MemoryStore) SaveUserMemory(ctx context.Context, userMemory *memory.UserMemory) error {
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

	// 如果用户的记忆映射不存在，创建一个
	if m.userMemories[userMemory.UserID] == nil {
		m.userMemories[userMemory.UserID] = make(map[string]*memory.UserMemory)
	}

	// 如果没有ID，生成一个
	if userMemory.ID == "" {
		userMemory.ID = utils.GetULID()
	}

	// 设置时间戳
	now := time.Now()
	if userMemory.CreatedAt.IsZero() {
		userMemory.CreatedAt = now
	}
	userMemory.UpdatedAt = now

	// 保存记忆
	m.userMemories[userMemory.UserID][userMemory.ID] = userMemory
	return nil
}

// GetUserMemories 获取用户的记忆列表
func (m *MemoryStore) GetUserMemories(ctx context.Context, userID string, limit int, retrieval memory.MemoryRetrieval) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	userMems, exists := m.userMemories[userID]
	if !exists {
		return []*memory.UserMemory{}, nil
	}

	// 将map转换为slice
	var memories []*memory.UserMemory
	for _, mem := range userMems {
		memories = append(memories, mem)
	}

	// 根据检索方式排序
	switch retrieval {
	case memory.RetrievalLastN:
		// 按更新时间降序排列（最新的在前）
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].UpdatedAt.After(memories[j].UpdatedAt)
		})
	case memory.RetrievalFirstN:
		// 按创建时间升序排列（最早的在前）
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].CreatedAt.Before(memories[j].CreatedAt)
		})
	default:
		// 默认按更新时间降序
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].UpdatedAt.After(memories[j].UpdatedAt)
		})
	}

	// 应用限制
	if limit > 0 && len(memories) > limit {
		memories = memories[:limit]
	}

	return memories, nil
}

// UpdateUserMemory 更新用户记忆
func (m *MemoryStore) UpdateUserMemory(ctx context.Context, memory *memory.UserMemory) error {
	if memory == nil {
		return errors.New("记忆对象不能为空")
	}
	if memory.ID == "" {
		return errors.New("记忆ID不能为空")
	}
	if memory.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userMems, exists := m.userMemories[memory.UserID]
	if !exists || userMems[memory.ID] == nil {
		return errors.New("记忆不存在")
	}

	// 更新时间戳
	memory.UpdatedAt = time.Now()

	// 保持原有的创建时间
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = userMems[memory.ID].CreatedAt
	}

	// 更新记忆
	userMems[memory.ID] = memory
	return nil
}

// DeleteUserMemory 删除用户记忆
func (m *MemoryStore) DeleteUserMemory(ctx context.Context, memoryID string) error {
	if memoryID == "" {
		return errors.New("记忆ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 遍历所有用户查找并删除记忆
	for userID, userMems := range m.userMemories {
		if _, exists := userMems[memoryID]; exists {
			delete(userMems, memoryID)
			// 如果用户没有记忆了，删除整个用户条目
			if len(userMems) == 0 {
				delete(m.userMemories, userID)
			}
			return nil
		}
	}

	return errors.New("记忆不存在")
}

func (m *MemoryStore) DeleteUserMemoriesByIds(ctx context.Context, userID string, memoryIDs []string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if len(memoryIDs) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userMems, ok := m.userMemories[userID]
	if !ok {
		return nil
	}

	deleted := false
	for _, memoryID := range memoryIDs {
		if _, exists := userMems[memoryID]; exists {
			delete(userMems, memoryID)
			deleted = true
		}
	}

	// 如果用户没有记忆了，删除整个用户条目
	if len(userMems) == 0 {
		delete(m.userMemories, userID)
	}

	if !deleted {
		return errors.New("记忆不存在")
	}

	return nil
}

// ClearUserMemories 清空用户的所有记忆
func (m *MemoryStore) ClearUserMemories(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.userMemories, userID)
	return nil
}

// SearchUserMemories 搜索用户记忆（简单的文本匹配实现）
func (m *MemoryStore) SearchUserMemories(ctx context.Context, userID string, query string, limit int) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	if query == "" {
		return []*memory.UserMemory{}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	userMems, exists := m.userMemories[userID]
	if !exists {
		return []*memory.UserMemory{}, nil
	}

	var results []*memory.UserMemory
	queryLower := strings.ToLower(query)

	for _, mem := range userMems {
		// 简单的文本匹配搜索
		if strings.Contains(strings.ToLower(mem.Memory), queryLower) ||
			strings.Contains(strings.ToLower(mem.Input), queryLower) {
			results = append(results, mem)
		}
	}

	// 按相关性排序（这里简单按更新时间排序）
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	// 应用限制
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SaveSessionSummary 保存会话摘要
func (m *MemoryStore) SaveSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
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
func (m *MemoryStore) GetSessionSummary(ctx context.Context, sessionID string, userID string) (*memory.SessionSummary, error) {
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
func (m *MemoryStore) UpdateSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
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
func (m *MemoryStore) SaveMessage(ctx context.Context, message *memory.ConversationMessage) error {
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
func (m *MemoryStore) GetMessages(ctx context.Context, sessionID string, userID string, limit int) ([]*memory.ConversationMessage, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.generateKey(sessionID, userID)
	messages, exists := m.messages[key]
	if !exists {
		return []*memory.ConversationMessage{}, nil
	}

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

// Health 检查存储健康状态
func (m *MemoryStore) Health(ctx context.Context) error {
	// 内存存储总是健康的
	return nil
}
