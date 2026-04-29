package builtin

import (
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/cloudwego/eino/schema"
)

const (
	MessageExtraIDKey        = "aggo_message_id"
	MessageExtraSessionIDKey = "aggo_session_id"
	MessageExtraUserIDKey    = "aggo_user_id"
	MessageExtraRoleKey      = "aggo_role"
	MessageExtraCreatedAtKey = "aggo_created_at"
)

// ptrTo returns a pointer to the given value.
func ptrTo[T any](v T) *T {
	return &v
}

// ToSchemaMessage 将 ConversationMessage 转换为 schema.Message
// 统一转换逻辑，避免在多处重复实现
func (m *ConversationMessage) ToSchemaMessage() *schema.Message {
	msg := &schema.Message{
		Role: schema.RoleType(m.Role),
	}
	msg.Content = m.Content
	if len(m.Parts) > 0 {
		multiContent := make([]schema.MessageInputPart, len(m.Parts))
		copy(multiContent, m.Parts)
		msg.UserInputMultiContent = multiContent
		msg.Content = ""
	}
	msg.Extra = map[string]any{
		MessageExtraIDKey:        m.ID,
		MessageExtraSessionIDKey: m.SessionID,
		MessageExtraUserIDKey:    m.UserID,
		MessageExtraRoleKey:      m.Role,
	}
	if !m.CreatedAt.IsZero() {
		msg.Extra[MessageExtraCreatedAtKey] = m.CreatedAt.Format(time.RFC3339)
	}
	return msg
}

// UserMemory 用户记忆结构
// 每个用户一条记录，使用Markdown格式存储所有记忆内容
type UserMemory struct {
	// 用户ID（主键）
	UserID string `json:"userId"`
	// 记忆内容（Markdown格式）
	Memory string `json:"memory"`
	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// 最后更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// SessionSummary 会话摘要结构
// 存储对话会话的智能摘要
type SessionSummary struct {
	// 会话ID
	SessionID string `json:"sessionId"`
	// 用户ID
	UserID string `json:"userId"`
	// 摘要内容
	Summary string `json:"summary"`
	// 上次已纳入摘要的最后一条消息ID
	LastSummarizedMessageID string `json:"lastSummarizedMessageId,omitempty"`
	// 上次已纳入摘要的最后一条消息时间
	LastSummarizedMessageAt time.Time `json:"lastSummarizedMessageAt,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// 最后更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// ConversationMessage 对话消息结构
// 存储完整的对话历史
type ConversationMessage struct {
	// 消息ID
	ID string `json:"id"`
	// 会话ID
	SessionID string `json:"sessionId"`
	// 用户ID
	UserID string `json:"userId"`
	// 角色 (user/assistant/system)
	Role string `json:"role"`
	// 消息内容（简单文本消息）
	Content string `json:"content,omitempty"`
	// 多部分内容，支持文本、图片、音频、视频、文件等
	Parts []schema.MessageInputPart `json:"parts,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
}

// MemoryRetrieval 记忆检索方式
type MemoryRetrieval string

const (
	// RetrievalLastN 检索最近的N条记忆
	RetrievalLastN MemoryRetrieval = "last_n"
	// RetrievalFirstN 检索最早的N条记忆
	RetrievalFirstN MemoryRetrieval = "first_n"
	// RetrievalSemantic 语义检索（基于相似性）
	RetrievalSemantic MemoryRetrieval = "semantic"
)

// MemoryConfig 记忆配置
type MemoryConfig struct {
	// 是否启用用户记忆
	EnableUserMemories bool `json:"enableUserMemories"`
	// 是否启用会话摘要
	EnableSessionSummary bool `json:"enableSessionSummary"`
	// 用户记忆检索方式 EnableUserMemories开启采生效
	Retrieval MemoryRetrieval `json:"retrieval"`
	// 记忆数量限制
	MemoryLimit int `json:"memoryLimit"`
	// 启用会话摘要时，除摘要游标之后的消息外，额外保留最近N条原始消息作为短期上下文。
	// 默认为0，表示保持仅注入摘要游标之后消息的旧行为。
	SummaryRecentMessageLimit int `json:"summaryRecentMessageLimit,omitempty"`
	// 异步处理的goroutine池大小
	AsyncWorkerPoolSize int `json:"asyncWorkerPoolSize"`
	// 记忆任务聚合窗口（秒），同一用户+会话在该窗口内的多次请求只执行一次记忆分析
	// 默认30秒，设为0则每次回复后立即执行（向后兼容）
	DebounceWindowSeconds *int `json:"debounceWindowSeconds,omitempty"`

	// 摘要触发配置
	SummaryTrigger SummaryTriggerConfig `json:"summaryTrigger"`

	// 会话摘要缓存配置
	SummaryCache SummaryCacheConfig `json:"summaryCache"`

	// 清理配置
	Cleanup CleanupConfig `json:"cleanup"`

	// 搜索配置。nil 时按 keyword 默认行为初始化。
	Search *SearchConfig `json:"search,omitempty"`
}

// CleanupConfig 清理相关配置
type CleanupConfig struct {
	// 会话状态清理间隔（小时），默认24小时
	SessionCleanupInterval int `json:"sessionCleanupInterval"`
	// 会话状态保留时间（小时），默认168小时（7天）
	SessionRetentionTime int `json:"sessionRetentionTime"`
	// 消息历史保留数量限制，默认1000条
	MessageHistoryLimit int `json:"messageHistoryLimit"`
	// 定期清理间隔（小时），默认12小时
	CleanupInterval int `json:"cleanupInterval"`
}

// SummaryCacheConfig 会话摘要缓存配置
type SummaryCacheConfig struct {
	// TTLSeconds 表示单条摘要缓存 TTL，单位秒
	TTLSeconds int `json:"ttlSeconds"`
	// MaxEntries 表示缓存最多保留多少条会话摘要
	MaxEntries int `json:"maxEntries"`
}

// DefaultMemoryConfig 返回完整的默认配置
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		EnableUserMemories:    true,
		EnableSessionSummary:  false,
		Retrieval:             RetrievalLastN,
		MemoryLimit:           20,
		AsyncWorkerPoolSize:   5,
		DebounceWindowSeconds: ptrTo(30),
		SummaryTrigger: SummaryTriggerConfig{
			Strategy:         TriggerSmart,
			MessageThreshold: 10,
			MinInterval:      600, // 600秒最小间隔
		},
		SummaryCache: SummaryCacheConfig{
			TTLSeconds: int(defaultSessionSummaryCacheTTL / time.Second),
			MaxEntries: defaultSessionSummaryCacheMaxEntries,
		},
		Cleanup: CleanupConfig{
			SessionCleanupInterval: 24,   // 24小时
			SessionRetentionTime:   168,  // 7天
			MessageHistoryLimit:    1000, // 1000条
			CleanupInterval:        12,   // 12小时
		},
		Search: &SearchConfig{
			Mode: builtinsearch.ModeKeyword,
		},
	}
}

// SummaryTriggerConfig 摘要触发配置
type SummaryTriggerConfig struct {
	// 触发策略类型
	Strategy SummaryTriggerStrategy `json:"strategy"`
	// 基于消息数量触发的阈值
	MessageThreshold int `json:"messageThreshold"`
	// 最小触发间隔（秒）
	MinInterval int `json:"minInterval"`
}

// SummaryTriggerStrategy 摘要触发策略
type SummaryTriggerStrategy string

const (
	// TriggerAlways 每次都触发（原有行为）
	TriggerAlways SummaryTriggerStrategy = "always"
	// TriggerByMessages 基于消息数量触发
	TriggerByMessages SummaryTriggerStrategy = "by_messages"
	// TriggerByTime 基于时间间隔触发
	TriggerByTime SummaryTriggerStrategy = "by_time"
	// TriggerSmart 智能触发（综合考虑多种因素）
	TriggerSmart SummaryTriggerStrategy = "smart"
)

// UserMemoryAnalyzerParam 用户记忆更新参数
type UserMemoryAnalyzerParam struct {
	// 操作类型: update(更新记忆)、noop(无需更新)
	Op string `json:"op"`
	// 记忆内容（完整Markdown文档，op为update时有效）
	Memory string `json:"memory"`
}

// 用户记忆操作类型
const (
	// UserMemoryOpUpdate 更新记忆
	UserMemoryOpUpdate = "update"
	// UserMemoryOpNoop 无需更新
	UserMemoryOpNoop = "noop"
)

// TaskQueueStats 异步任务队列统计
type TaskQueueStats struct {
	// 队列大小
	QueueSize int `json:"queueSize"`
	// 队列容量
	QueueCapacity int `json:"queueCapacity"`
	// 已处理任务数
	ProcessedTasks int64 `json:"processedTasks"`
	// 丢弃任务数
	DroppedTasks int64 `json:"droppedTasks"`
	// 当前工作goroutine数
	ActiveWorkers int `json:"activeWorkers"`
	// 队列使用率
	QueueUtilization float64 `json:"queueUtilization"`
}
