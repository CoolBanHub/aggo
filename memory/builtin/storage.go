package builtin

import (
	"context"
	"time"
)

// MemoryStorage 记忆存储接口
// 定义了记忆存储的基本操作，可以有多种实现（内存、SQL、NoSQL等）
type MemoryStorage interface {
	AutoMigrate() error

	//SetTablePrefix 设置表前缀
	SetTablePrefix(prefix string)

	// 用户记忆操作

	// UpsertUserMemory 创建或更新用户记忆（每个用户一条记录）
	UpsertUserMemory(ctx context.Context, memory *UserMemory) error

	// GetUserMemory 获取用户的记忆
	GetUserMemory(ctx context.Context, userID string) (*UserMemory, error)

	// ClearUserMemory 清空用户记忆
	ClearUserMemory(ctx context.Context, userID string) error

	// 会话摘要操作

	// SaveSessionSummary 保存会话摘要
	SaveSessionSummary(ctx context.Context, summary *SessionSummary) error

	// GetSessionSummary 获取会话摘要
	GetSessionSummary(ctx context.Context, sessionID string, userID string) (*SessionSummary, error)

	// UpdateSessionSummary 更新会话摘要
	UpdateSessionSummary(ctx context.Context, summary *SessionSummary) error

	// DeleteSessionSummary 删除会话摘要
	DeleteSessionSummary(ctx context.Context, sessionID string, userID string) error

	// 对话消息操作

	// SaveMessage 保存对话消息
	SaveMessage(ctx context.Context, message *ConversationMessage) error

	// GetMessages 获取会话的消息历史
	// sessionID: 会话ID
	// userID: 用户ID
	// limit: 限制返回数量，0表示不限制
	GetMessages(ctx context.Context, sessionID string, userID string, limit int) ([]*ConversationMessage, error)

	// DeleteMessages 删除会话的消息历史
	DeleteMessages(ctx context.Context, sessionID string, userID string) error

	// 通用操作

	// Close 关闭存储连接
	Close() error

	// Health 检查存储健康状态
	Health(ctx context.Context) error

	// 清理操作

	// CleanupOldMessages 清理指定时间之前的消息
	CleanupOldMessages(ctx context.Context, userID string, before time.Time) error

	// CleanupMessagesByLimit 按数量限制清理消息，保留最新的N条
	CleanupMessagesByLimit(ctx context.Context, userID, sessionID string, keepLimit int) error

	// GetMessageCount 获取消息总数
	GetMessageCount(ctx context.Context, userID, sessionID string) (int, error)
}

// CursorMessageStorage is an optional extension for stores that can query messages
// directly by a persisted summary cursor without loading the full session history.
type CursorMessageStorage interface {
	// GetMessagesAfter 获取游标之后的会话消息。
	// afterMessageID/afterTime 同时存在时，先按时间筛，再按消息ID打破同时间戳顺序。
	// 仅 afterTime 存在时，返回 created_at 晚于 afterTime 的消息。
	// 仅 afterMessageID 存在时，返回 ID 晚于 afterMessageID 的消息。
	// 两者都为空时，等价于 GetMessages(..., limit)。
	GetMessagesAfter(ctx context.Context, sessionID string, userID string, afterMessageID string, afterTime time.Time, limit int) ([]*ConversationMessage, error)

	// GetMessageCountAfter 获取游标之后的会话消息数量（避免加载完整消息列表）。
	// 语义与 GetMessagesAfter 相同，但只返回数量。
	GetMessageCountAfter(ctx context.Context, sessionID string, userID string, afterMessageID string, afterTime time.Time) (int, error)
}
