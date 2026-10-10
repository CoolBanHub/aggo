package memory

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// MemoryProvider defines the interface that all memory backends must implement.
// It provides the core operations for retrieving relevant context before a model
// call and persisting conversation turns after a model call.
type MemoryProvider interface {
	// Retrieve fetches relevant memory context before a model call.
	Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error)

	// Memorize persists a conversation turn after a model call.
	Memorize(ctx context.Context, req *MemorizeRequest) error

	// Close releases any resources held by the provider.
	Close() error
}

// RetrieveRequest is the input for the Retrieve operation.
type RetrieveRequest struct {
	// UserID identifies the user whose memory to search.
	UserID string

	// SessionID identifies the current conversation session.
	SessionID string

	// Messages are the current conversation messages used as context for retrieval.
	Messages []*schema.AgenticMessage

	// Limit is the maximum number of memory items to return.
	// A value of 0 means the provider should use its own default.
	Limit int
}

// RetrieveResult is the output of the Retrieve operation.
type RetrieveResult struct {
	// ContextMessages are appended to the current user message as runtime context.
	// Providers should use this for user/session-specific retrieved data so the
	// base system prompt and conversation history remain cacheable.
	ContextMessages []*schema.AgenticMessage

	// SystemMessages are legacy context messages.
	// Deprecated: use ContextMessages for dynamic memory or retrieval results.
	SystemMessages []*schema.AgenticMessage

	// HistoryMessages are injected as conversation history.
	HistoryMessages []*schema.AgenticMessage

	// Metadata holds provider-specific data.
	Metadata map[string]any
}

// MemorizeRequest is the input for the Memorize operation.
type MemorizeRequest struct {
	// UserID identifies the user whose conversation to store.
	UserID string

	// SessionID identifies the conversation session.
	SessionID string

	// Messages are the conversation turn(s) to store.
	Messages []*schema.AgenticMessage
}

// HookEvent represents a lifecycle event in the memory provider.
type HookEvent string

const (
	// HookBeforeRetrieve fires before the Retrieve operation.
	HookBeforeRetrieve HookEvent = "before_retrieve"

	// HookAfterRetrieve fires after the Retrieve operation.
	HookAfterRetrieve HookEvent = "after_retrieve"

	// HookBeforeMemorize fires before the Memorize operation.
	HookBeforeMemorize HookEvent = "before_memorize"

	// HookAfterMemorize fires after the Memorize operation.
	HookAfterMemorize HookEvent = "after_memorize"
)

// HookHandler is a callback function invoked when a lifecycle event fires.
type HookHandler func(ctx context.Context, event HookEvent, data any) error

// HookableProvider is an optional extension of MemoryProvider for providers
// that support lifecycle hooks.
type HookableProvider interface {
	MemoryProvider

	// RegisterHook registers a handler for the given lifecycle event.
	RegisterHook(event HookEvent, handler HookHandler)
}

// AsyncMemorizer is an optional extension of MemoryProvider for providers
// that own a managed async write path (e.g. builtin.MemoryManager's worker
// pool with Flush/Shutdown lifecycle). When implemented, MemoryMiddleware
// delegates memorization to EnqueueMemorize instead of spawning an
// untracked goroutine, so pending writes are drained on shutdown.
type AsyncMemorizer interface {
	// EnqueueMemorize accepts a memorize request for asynchronous execution.
	// It must be safe to reject the request after the provider is closed
	// (returning an error instead of panicking or blocking).
	EnqueueMemorize(ctx context.Context, req *MemorizeRequest) error
}

// MemorizeNormalizer 在消息写入记忆前对请求做清洗/裁剪（如移除图片索引、
// 发言人标记、runtime context 注入内容）。
//
// 约定：
//   - 入参请求视为不可变，返回新的请求对象，不得原地修改；
//   - 返回 nil, nil 表示丢弃本次写入；
//   - 返回错误表示清洗失败，MemoryMiddleware 会记录日志并跳过本次写入。
type MemorizeNormalizer func(ctx context.Context, req *MemorizeRequest) (*MemorizeRequest, error)

// MissingIdentityPolicy 决定 MemoryMiddleware 在会话缺少身份信息时的行为。
type MissingIdentityPolicy int

const (
	// MissingIdentitySkip 静默跳过记忆读写（旧默认行为）。
	MissingIdentitySkip MissingIdentityPolicy = iota
	// MissingIdentityLog 记录日志后跳过。作为新配置的默认值，
	// 避免现有业务在身份缺失时无声失效。
	MissingIdentityLog
	// MissingIdentityError 向上层返回错误，中断本轮请求。
	MissingIdentityError
)
