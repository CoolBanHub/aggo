package memory

import "context"

// MemoryStorage 记忆存储接口
// 定义了记忆存储的基本操作，可以有多种实现（内存、SQL、NoSQL等）
type MemoryStorage interface {
	AutoMigrate() error

	//SetTablePrefix 设置表前缀
	SetTablePrefix(prefix string)

	// 用户记忆操作

	// SaveUserMemory 保存用户记忆
	SaveUserMemory(ctx context.Context, memory *UserMemory) error

	// GetUserMemories 获取用户的记忆列表
	// userID: 用户ID
	// limit: 限制返回数量，0表示不限制
	// retrieval: 检索方式
	GetUserMemories(ctx context.Context, userID string, limit int, retrieval MemoryRetrieval) ([]*UserMemory, error)

	// UpdateUserMemory 更新用户记忆
	UpdateUserMemory(ctx context.Context, memory *UserMemory) error

	// DeleteUserMemory 删除用户记忆
	DeleteUserMemory(ctx context.Context, memoryID string) error

	// DeleteUserMemoriesByIds 批量删除用户记忆
	DeleteUserMemoriesByIds(ctx context.Context, userID string, memoryIDs []string) error

	// ClearUserMemories 清空用户的所有记忆
	ClearUserMemories(ctx context.Context, userID string) error

	// SearchUserMemories 搜索用户记忆（支持语义搜索）
	SearchUserMemories(ctx context.Context, userID string, query string, limit int) ([]*UserMemory, error)

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
}

// MemoryQuery 记忆查询条件
type MemoryQuery struct {
	// 用户ID
	UserID string
	// 会话ID (可选)
	SessionID string
	// 时间范围 (可选)
	CreatedAfter  *int64 // Unix时间戳
	CreatedBefore *int64 // Unix时间戳
	// 内容关键词 (可选)
	Keywords []string
	// 分页
	Offset int
	Limit  int
}
