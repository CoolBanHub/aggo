// Package memoryevent defines provider-neutral types for the user-memory
// event search interface used by aggo's search_user_memory tool.
//
// Splitting these types out of memory/builtin lets any memory provider
// (builtin, mem0, memu, agent-memory, ...) implement
// memory.UserMemoryEventSearcher without dragging in builtin's storage
// implementation.
//
// memory/builtin re-exports Event and Query as UserMemoryEvent and
// UserMemoryEventQuery via type aliases, so existing callers compile
// unchanged.
package memoryevent

import "time"

// Event types.
const (
	// TypeMilestone 任务里程碑
	TypeMilestone = "milestone"
	// TypeEvent 事件记录
	TypeEvent = "event"
)

// Event 用户记忆事件
// 单条带时间属性的“任务里程碑/事件记录”条目，retrieve 时只取最近 N 条进
// 动态上下文，再通过 search_user_memory 工具按需检索更早内容。
type Event struct {
	// 主键 ID（ULID 单调递增；非内置 provider 可用任意稳定字符串）
	ID string `json:"id"`
	// 用户 ID
	UserID string `json:"userId"`
	// 事件类型 milestone / event
	Type string `json:"type"`
	// 事件发生日期（YYYY-MM-DD 起始的语义时间，不一定等于 CreatedAt）
	EventDate time.Time `json:"eventDate"`
	// 关键词（用于关键词检索）
	Keywords []string `json:"keywords,omitempty"`
	// 事件正文（精简事实陈述）
	Summary string `json:"summary"`
	// 入库时间
	CreatedAt time.Time `json:"createdAt"`
}

// Query 用户记忆事件检索条件
type Query struct {
	// 用户 ID（必填）
	UserID string
	// 事件类型 milestone / event，留空匹配全部
	Type string
	// 关键词列表
	Keywords []string
	// 关键词匹配方式 any/all
	Match string
	// 起止时间（基于 EventDate）
	Since *time.Time
	Until *time.Time
	// 返回条数上限，<=0 由调用方按默认处理
	Limit int
}
