package builtin

import (
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/CoolBanHub/aggo/memory/memoryevent"
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

// ToAgenticMessage 将 ConversationMessage 转换为 schema.AgenticMessage
// 统一转换逻辑，避免在多处重复实现
func (m *ConversationMessage) ToAgenticMessage() *schema.AgenticMessage {
	msg := &schema.AgenticMessage{
		Role: schema.AgenticRoleType(m.Role),
	}
	if len(m.Parts) > 0 {
		msg.ContentBlocks = messageInputPartsToContentBlocks(m.Parts)
	} else if m.Content != "" {
		switch msg.Role {
		case schema.AgenticRoleTypeAssistant:
			msg.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: m.Content})}
		default:
			msg.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: m.Content})}
		}
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

func messageInputPartsToContentBlocks(parts []schema.MessageInputPart) []*schema.ContentBlock {
	blocks := make([]*schema.ContentBlock, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			blocks = append(blocks, schema.NewContentBlock(&schema.UserInputText{Text: part.Text}))
		case schema.ChatMessagePartTypeImageURL:
			if part.Image != nil {
				blocks = append(blocks, schema.NewContentBlock(&schema.UserInputImage{
					URL:        valueOrEmpty(part.Image.URL),
					Base64Data: valueOrEmpty(part.Image.Base64Data),
					MIMEType:   part.Image.MIMEType,
					Detail:     part.Image.Detail,
				}))
			}
		case schema.ChatMessagePartTypeAudioURL:
			if part.Audio != nil {
				blocks = append(blocks, schema.NewContentBlock(&schema.UserInputAudio{
					URL:        valueOrEmpty(part.Audio.URL),
					Base64Data: valueOrEmpty(part.Audio.Base64Data),
					MIMEType:   part.Audio.MIMEType,
				}))
			}
		case schema.ChatMessagePartTypeVideoURL:
			if part.Video != nil {
				blocks = append(blocks, schema.NewContentBlock(&schema.UserInputVideo{
					URL:        valueOrEmpty(part.Video.URL),
					Base64Data: valueOrEmpty(part.Video.Base64Data),
					MIMEType:   part.Video.MIMEType,
				}))
			}
		case schema.ChatMessagePartTypeFileURL:
			if part.File != nil {
				blocks = append(blocks, schema.NewContentBlock(&schema.UserInputFile{
					URL:        valueOrEmpty(part.File.URL),
					Name:       part.File.Name,
					Base64Data: valueOrEmpty(part.File.Base64Data),
					MIMEType:   part.File.MIMEType,
				}))
			}
		default:
			if part.Text != "" {
				blocks = append(blocks, schema.NewContentBlock(&schema.UserInputText{Text: part.Text}))
			}
		}
	}
	return blocks
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// UserMemory 用户记忆结构
// 每个用户一条记录，使用Markdown格式存储“常驻短文档”（核心约定 + 基础信息）。
// 累积型条目（任务里程碑、事件记录）请改用 UserMemoryEvent，避免短文档无限膨胀。
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

// 用户记忆事件类型（兼容别名，新代码请直接用 memoryevent.TypeMilestone/TypeEvent）
const (
	// UserMemoryEventTypeMilestone 任务里程碑
	UserMemoryEventTypeMilestone = memoryevent.TypeMilestone
	// UserMemoryEventTypeEvent 事件记录
	UserMemoryEventTypeEvent = memoryevent.TypeEvent
)

// UserMemoryEvent 是 memoryevent.Event 的别名，便于其他 provider 复用同一份
// 中性类型实现 memory.UserMemoryEventSearcher 接口。
type UserMemoryEvent = memoryevent.Event

// UserMemoryEventQuery 是 memoryevent.Query 的别名。
type UserMemoryEventQuery = memoryevent.Query

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
	// 是否启用“事件检索”模式：
	//   true  - 用户记忆拆为“常驻短文档（核心约定+基础信息）”和“事件检索表”，
	//           Retrieve 仅注入短文档 + 最近 RecentEventLimit 条事件；
	//           更早的事件需通过 search_user_memory 工具按关键词/时间检索。
	//   false - 兼容旧行为，整篇 UserMemory.Memory 全量注入 system。
	// 仅当 EnableUserMemories=true 时生效。
	EnableEventSearch bool `json:"enableEventSearch"`
	// 常驻注入的最近事件条数，默认 20，仅在 EnableEventSearch=true 时生效。
	// 设为 0 表示不注入任何事件，全部交给检索工具。
	RecentEventLimit int `json:"recentEventLimit,omitempty"`
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
	// 异步任务执行超时时间（秒），用于用户记忆分析、会话摘要和索引任务。
	// 默认120秒；设为0或负数时使用默认值。
	AsyncTaskTimeoutSeconds int `json:"asyncTaskTimeoutSeconds,omitempty"`

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
		EnableUserMemories:      true,
		EnableSessionSummary:    false,
		EnableEventSearch:       false,
		RecentEventLimit:        20,
		Retrieval:               RetrievalLastN,
		MemoryLimit:             20,
		AsyncWorkerPoolSize:     5,
		DebounceWindowSeconds:   ptrTo(30),
		AsyncTaskTimeoutSeconds: 120,
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
