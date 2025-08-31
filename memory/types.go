package memory

import (
	"time"
)

// UserMemory 用户记忆结构
// 存储关于用户的个人化信息，如偏好、兴趣、个人事实等
type UserMemory struct {
	// 记忆的唯一标识符
	ID string `json:"id"`
	// 用户ID
	UserID string `json:"userId"`
	// 记忆内容
	Memory string `json:"memory"`
	// 触发该记忆的原始用户输入
	Input string `json:"input,omitempty"`
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
	// 消息内容
	Content string `json:"content"`
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
	// 记忆检索方式
	Retrieval MemoryRetrieval `json:"retrieval"`
	// 记忆数量限制
	MemoryLimit int `json:"memoryLimit"`
	// 是否异步处理记忆分析和会话摘要
	AsyncProcessing bool `json:"asyncProcessing"`
	// 异步处理的goroutine池大小
	AsyncWorkerPoolSize int `json:"asyncWorkerPoolSize"`

	// 摘要触发配置
	SummaryTrigger SummaryTriggerConfig `json:"summaryTrigger"`
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

// MemoryClassifierParam 用户记忆分类参数
type UserMemoryAnalyzerParam struct {
	Op     string `json:"op"`
	Id     string `json:"id"`
	Memory string `json:"memory"`
}

// 用户记忆分类操作
const (
	//UserMemoryAnalyzerOpCreate 创建
	UserMemoryAnalyzerOpCreate = "create"
	//UserMemoryAnalyzerOpUpdate 更新
	UserMemoryAnalyzerOpUpdate = "update"
	//UserMemoryAnalyzerOpDelete 删除
	UserMemoryAnalyzerOpDelete = "del"
)
