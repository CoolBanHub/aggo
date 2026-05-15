package builtin

import (
	"context"
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"gorm.io/gorm"
)

// MemoryStorage 记忆存储接口
// 定义了记忆存储的基本操作，可以有多种实现（内存、SQL、NoSQL等）
type MemoryStorage interface {
	AutoMigrate() error

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

// SearchMessageStorage is an optional extension for stores that can perform
// keyword search efficiently in the storage layer.
type SearchMessageStorage interface {
	SearchMessagesByKeywords(ctx context.Context, q *builtinsearch.SearchQuery) ([]*ConversationMessage, error)
}

// UserMemoryEventStorage 是可选扩展接口，提供按用户拆分的事件级记忆存储与检索。
// 启用 MemoryConfig.EnableEventSearch 时，底层 MemoryStorage 必须实现该接口；
// 不实现时 Provider 会退化为兼容模式（全量注入 UserMemory.Memory）。
type UserMemoryEventStorage interface {
	// SaveUserMemoryEvent 新增一条用户记忆事件。事件 ID 未填写时由实现侧生成（建议 ULID 单调递增）。
	SaveUserMemoryEvent(ctx context.Context, event *UserMemoryEvent) error

	// ListRecentUserMemoryEvents 返回该用户最近的 N 条事件，按 EventDate 倒序、同日按 CreatedAt 倒序。
	// limit <= 0 时由实现侧选取一个安全的默认值（建议返回空切片以避免误注入大量数据）。
	ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*UserMemoryEvent, error)

	// SearchUserMemoryEvents 按 UserMemoryEventQuery 过滤事件，返回按 EventDate 倒序的命中列表。
	SearchUserMemoryEvents(ctx context.Context, query *UserMemoryEventQuery) ([]*UserMemoryEvent, error)

	// DeleteUserMemoryEvent 删除指定事件。eventID 为空时返回错误。
	DeleteUserMemoryEvent(ctx context.Context, userID, eventID string) error

	// ClearUserMemoryEvents 清空用户的所有事件。迁移脚本和单元测试需要。
	ClearUserMemoryEvents(ctx context.Context, userID string) error
}

// GormConversationStorage exposes the underlying gorm DB and message table
// so builtin search can construct the default vector store without depending
// on concrete storage implementations.
type GormConversationStorage interface {
	ConversationDB() *gorm.DB
	ConversationMessageTableName() string
}
