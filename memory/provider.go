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
	Messages []*schema.Message

	// Limit is the maximum number of memory items to return.
	// A value of 0 means the provider should use its own default.
	Limit int
}

// RetrieveResult is the output of the Retrieve operation.
type RetrieveResult struct {
	// SystemMessages are injected as system context before the conversation.
	SystemMessages []*schema.Message

	// HistoryMessages are injected as conversation history.
	HistoryMessages []*schema.Message

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
	Messages []*schema.Message
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
