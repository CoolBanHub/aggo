# Memory Plugin System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the monolithic memory package into a plugin-based architecture with a `MemoryProvider` interface, supporting builtin (current logic), memu, and mem0 backends.

**Architecture:** Define a `MemoryProvider` interface at the top-level `memory` package with `Retrieve`/`Memorize`/`Close` methods. The existing `MemoryManager` becomes the "builtin" provider implementation under `memory/builtin/`. A plugin registry allows registering and creating providers by name. The `MemoryMiddleware` depends only on `MemoryProvider`, not on specific implementations. A new `memu` provider integrates the memu HTTP service.

**Tech Stack:** Go 1.24, cloudwego/eino (adk, schema), net/http (for memu client)

---

### Task 1: Create the MemoryProvider Interface

**Files:**
- Create: `memory/provider.go`

**Step 1: Define the core interface and data structures**

```go
package memory

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// MemoryProvider is the core interface that all memory backends must implement.
// It abstracts memory retrieval (before model call) and memorization (after model call).
type MemoryProvider interface {
	// Retrieve fetches relevant memory context before a model call.
	// Returns enriched messages to inject into the conversation.
	Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error)

	// Memorize persists a conversation turn after a model call.
	Memorize(ctx context.Context, req *MemorizeRequest) error

	// Close releases resources held by the provider.
	Close() error
}

// RetrieveRequest is the input for memory retrieval.
type RetrieveRequest struct {
	UserID    string
	SessionID string
	Messages  []*schema.Message // recent conversation messages for context
	Limit     int               // max items to return, 0 = use provider default
}

// RetrieveResult is the output of memory retrieval.
type RetrieveResult struct {
	// SystemMessages are injected before the conversation as system context.
	SystemMessages []*schema.Message
	// HistoryMessages are injected as conversation history.
	HistoryMessages []*schema.Message
	// Metadata contains provider-specific data.
	Metadata map[string]any
}

// MemorizeRequest is the input for memorizing a conversation turn.
type MemorizeRequest struct {
	UserID    string
	SessionID string
	Messages  []*schema.Message // the conversation turn(s) to store
}

// HookableProvider is an optional interface for providers that support lifecycle hooks.
type HookableProvider interface {
	MemoryProvider
	RegisterHook(event HookEvent, handler HookHandler)
}

// HookEvent represents a lifecycle event in the memory system.
type HookEvent string

const (
	HookBeforeRetrieve HookEvent = "before_retrieve"
	HookAfterRetrieve  HookEvent = "after_retrieve"
	HookBeforeMemorize HookEvent = "before_memorize"
	HookAfterMemorize  HookEvent = "after_memorize"
)

// HookHandler is a function that handles a lifecycle event.
type HookHandler func(ctx context.Context, event HookEvent, data any) error
```

**Step 2: Commit**

```bash
git add memory/provider.go
git commit -m "feat(memory): add MemoryProvider interface and data structures"
```

---

### Task 2: Create the Plugin Registry

**Files:**
- Create: `memory/registry.go`

**Step 1: Implement the plugin registry**

```go
package memory

import (
	"fmt"
	"sync"
)

// ProviderFactory creates a new MemoryProvider from config.
type ProviderFactory func(config any) (MemoryProvider, error)

// Plugin represents a registered memory backend plugin.
type Plugin struct {
	// ID is the unique identifier (e.g. "builtin", "memu", "mem0").
	ID string
	// Factory creates a new MemoryProvider instance.
	Factory ProviderFactory
}

// Registry manages available memory plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
}

// Global registry instance.
var globalRegistry = NewRegistry()

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Plugin),
	}
}

// GlobalRegistry returns the global plugin registry.
func GlobalRegistry() *Registry {
	return globalRegistry
}

// Register adds a plugin to the registry.
func (r *Registry) Register(plugin *Plugin) error {
	if plugin.ID == "" {
		return fmt.Errorf("plugin ID cannot be empty")
	}
	if plugin.Factory == nil {
		return fmt.Errorf("plugin factory cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[plugin.ID]; exists {
		return fmt.Errorf("plugin %q already registered", plugin.ID)
	}

	r.plugins[plugin.ID] = plugin
	return nil
}

// MustRegister registers a plugin or panics.
func (r *Registry) MustRegister(plugin *Plugin) {
	if err := r.Register(plugin); err != nil {
		panic(err)
	}
}

// Get returns a plugin by ID.
func (r *Registry) Get(id string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// CreateProvider creates a MemoryProvider from a registered plugin.
func (r *Registry) CreateProvider(pluginID string, config any) (MemoryProvider, error) {
	r.mu.RLock()
	plugin, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("memory plugin %q not found", pluginID)
	}
	return plugin.Factory(config)
}

// ListPlugins returns all registered plugin IDs.
func (r *Registry) ListPlugins() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.plugins))
	for id := range r.plugins {
		ids = append(ids, id)
	}
	return ids
}

// RegisterPlugin is a convenience function to register on the global registry.
func RegisterPlugin(plugin *Plugin) error {
	return globalRegistry.Register(plugin)
}

// MustRegisterPlugin registers on the global registry or panics.
func MustRegisterPlugin(plugin *Plugin) {
	globalRegistry.MustRegister(plugin)
}
```

**Step 2: Commit**

```bash
git add memory/registry.go
git commit -m "feat(memory): add plugin registry for memory backends"
```

---

### Task 3: Create builtin Provider (Wrap Current MemoryManager)

**Files:**
- Create: `memory/builtin/provider.go`
- Create: `memory/builtin/types.go` (re-exports from current types)
- Create: `memory/builtin/analyzer.go` (move user_memory_analyzer.go)
- Create: `memory/builtin/summary.go` (move session_summary_generator.go)
- Create: `memory/builtin/trigger.go` (move summary_trigger_manager.go)
- Create: `memory/builtin/prompt.go` (move prompt.go)
- Create: `memory/builtin/storage.go` (move storage.go)
- Create: `memory/builtin/manager.go` (move manager.go, adapt to implement MemoryProvider)
- Move: `memory/storage/*` -> `memory/builtin/storage/`
- Modify: `memory/builtin/manager.go` — add Retrieve/Memorize methods

This task is the biggest — it moves the existing code into `memory/builtin/` and makes `MemoryManager` implement the `MemoryProvider` interface.

**Step 1: Create the builtin package structure**

Move files into `memory/builtin/` with the package name `builtin`:

```
memory/builtin/
├── provider.go           # New: MemoryManager as MemoryProvider
├── manager.go            # Moved from memory/manager.go (package -> builtin)
├── types.go              # Moved from memory/types.go
├── analyzer.go           # Moved from memory/user_memory_analyzer.go
├── summary.go            # Moved from memory/session_summary_generator.go
├── trigger.go            # Moved from memory/summary_trigger_manager.go
├── prompt.go             # Moved from memory/prompt.go
├── storage.go            # Moved from memory/storage.go (MemoryStorage interface)
└── storage/
    ├── memory.go         # Moved from memory/storage/memory.go
    ├── file.go           # Moved from memory/storage/file.go
    ├── sql.go            # Moved from memory/storage/sql.go
    ├── sql_models.go     # Moved from memory/storage/sql_models.go
    ├── sql_session.go    # Moved from memory/storage/sql_session.go
    ├── sql_summary.go    # Moved from memory/storage/sql_summary.go
    ├── sql_user_memory.go # Moved from memory/storage/sql_user_memory.go
    └── table_name_pre.go # Moved from memory/storage/table_name_pre.go
```

**Step 2: Implement MemoryProvider on MemoryManager**

Create `memory/builtin/provider.go`:

```go
package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

// Ensure MemoryManager implements MemoryProvider.
var _ memory.MemoryProvider = (*MemoryManager)(nil)

// Retrieve implements memory.MemoryProvider.
// Fetches user memory, session summary, and conversation history.
func (m *MemoryManager) Retrieve(ctx context.Context, req *memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	if req == nil {
		return nil, fmt.Errorf("retrieve request is nil")
	}

	result := &memory.RetrieveResult{
		Metadata: make(map[string]any),
	}

	// Fetch user memory as system message
	if m.config.EnableUserMemories {
		if sysMsg := m.fetchUserMemoryMessage(ctx, req.UserID); sysMsg != nil {
			result.SystemMessages = append(result.SystemMessages, sysMsg)
		}
	}

	// Fetch session summary as system message
	if m.config.EnableSessionSummary {
		if sysMsg := m.fetchSessionSummaryMessage(ctx, req.SessionID, req.UserID); sysMsg != nil {
			result.SystemMessages = append(result.SystemMessages, sysMsg)
		}
	}

	// Fetch conversation history
	limit := req.Limit
	if limit <= 0 {
		limit = m.config.MemoryLimit
	}
	history, err := m.GetMessages(ctx, req.SessionID, req.UserID, limit)
	if err == nil && len(history) > 0 {
		result.HistoryMessages = history
	}

	return result, nil
}

// Memorize implements memory.MemoryProvider.
// Saves user and assistant messages, triggers async analysis.
func (m *MemoryManager) Memorize(ctx context.Context, req *memory.MemorizeRequest) error {
	if req == nil {
		return fmt.Errorf("memorize request is nil")
	}

	// Extract and save user message
	for _, msg := range req.Messages {
		if msg.Role == schema.User {
			if err := m.ProcessUserMessage(ctx, req.UserID, req.SessionID, msg.Content, msg.UserInputMultiContent); err != nil {
				return fmt.Errorf("save user message: %w", err)
			}
		}
	}

	// Extract and save assistant message
	for _, msg := range req.Messages {
		if msg.Role == schema.Assistant && msg.Content != "" {
			if err := m.ProcessAssistantMessage(ctx, req.UserID, req.SessionID, msg.Content); err != nil {
				return fmt.Errorf("save assistant message: %w", err)
			}
		}
	}

	return nil
}

// fetchUserMemoryMessage fetches user memory formatted as a system message.
func (m *MemoryManager) fetchUserMemoryMessage(ctx context.Context, userID string) *schema.Message {
	userMemory, err := m.storage.GetUserMemory(ctx, userID)
	if err != nil || userMemory == nil || userMemory.Memory == "" {
		return nil
	}

	var builder strings.Builder
	builder.WriteString("用户个人信息记忆（请在回复中考虑这些信息，提供个性化的响应）:\n")
	builder.WriteString(userMemory.Memory)

	return &schema.Message{
		Role:    schema.System,
		Content: builder.String(),
	}
}

// fetchSessionSummaryMessage fetches session summary formatted as a system message.
func (m *MemoryManager) fetchSessionSummaryMessage(ctx context.Context, sessionID, userID string) *schema.Message {
	summary, err := m.storage.GetSessionSummary(ctx, sessionID, userID)
	if err != nil || summary == nil || summary.Summary == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.System,
		Content: fmt.Sprintf("会话背景: %s", summary.Summary),
	}
}
```

**Step 3: Register builtin plugin**

Add an `init()` function or explicit registration in `memory/builtin/provider.go`:

```go
func init() {
	memory.MustRegisterPlugin(&memory.Plugin{
		ID: "builtin",
		Factory: func(config any) (memory.MemoryProvider, error) {
			cfg, ok := config.(*ProviderConfig)
			if !ok {
				return nil, fmt.Errorf("builtin: expected *ProviderConfig, got %T", config)
			}
			return NewMemoryManager(cfg.ChatModel, cfg.Storage, cfg.MemoryConfig)
		},
	})
}

// ProviderConfig is the config for the builtin memory provider.
type ProviderConfig struct {
	ChatModel    model.ToolCallingChatModel
	Storage      MemoryStorage
	MemoryConfig *MemoryConfig
}
```

**Step 4: Update all package declarations and imports**

In all moved files, change `package memory` to `package builtin`. Update internal cross-references accordingly.

**Step 5: Commit**

```bash
git add memory/builtin/
git commit -m "feat(memory): create builtin provider wrapping existing MemoryManager"
```

---

### Task 4: Refactor MemoryMiddleware to Use MemoryProvider

**Files:**
- Modify: `memory/middleware.go`

**Step 1: Rewrite middleware to depend on MemoryProvider**

Replace the current middleware that depends on `*MemoryManager` with one that depends on `MemoryProvider`:

```go
package memory

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// MemoryMiddleware implements adk.ChatModelAgentMiddleware.
// It delegates to a MemoryProvider for retrieval and memorization.
type MemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware

	provider MemoryProvider
}

// NewMemoryMiddleware creates a MemoryMiddleware with a MemoryProvider.
func NewMemoryMiddleware(provider MemoryProvider) *MemoryMiddleware {
	return &MemoryMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		provider:                     provider,
	}
}

// BeforeAgent is called before the agent runs.
func (m *MemoryMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return ctx, runCtx, nil
}

// BeforeModelRewriteState injects memory context before a model call.
func (m *MemoryMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m.provider == nil {
		return ctx, state, nil
	}

	sessionID, _ := adk.GetSessionValue(ctx, "sessionID")
	userID, _ := adk.GetSessionValue(ctx, "userID")
	sid, _ := sessionID.(string)
	uid, _ := userID.(string)
	if sid == "" || uid == "" {
		return ctx, state, nil
	}

	if prepared, ok := adk.GetSessionValue(ctx, m.beforeModelRewriteStateKey()); ok {
		if done, ok := prepared.(bool); ok && done {
			return ctx, state, nil
		}
	}

	// Call provider to retrieve context
	result, err := m.provider.Retrieve(ctx, &RetrieveRequest{
		UserID:    uid,
		SessionID: sid,
		Messages:  state.Messages,
	})
	if err != nil {
		log.Printf("MemoryMiddleware: Retrieve failed: %v", err)
		return ctx, state, nil
	}

	// Assemble enhanced messages
	enhanced := make([]*schema.Message, 0)

	if result != nil {
		if len(result.SystemMessages) > 0 {
			enhanced = append(enhanced, result.SystemMessages...)
		}
		if len(result.HistoryMessages) > 0 {
			enhanced = append(enhanced, result.HistoryMessages...)
		}
	}
	enhanced = append(enhanced, state.Messages...)
	state.Messages = enhanced

	// Store user message
	m.storeUserMessage(ctx, state.Messages, uid, sid)
	adk.AddSessionValue(ctx, m.beforeModelRewriteStateKey(), true)

	return ctx, state, nil
}

// AfterModelRewriteState stores assistant response after a model call.
func (m *MemoryMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m.provider == nil {
		return ctx, state, nil
	}

	sessionID, _ := adk.GetSessionValue(ctx, "sessionID")
	userID, _ := adk.GetSessionValue(ctx, "userID")
	sid, _ := sessionID.(string)
	uid, _ := userID.(string)
	if sid == "" || uid == "" {
		return ctx, state, nil
	}

	// Find the last user message and last assistant message
	var userMsg, assistantMsg *schema.Message
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if assistantMsg == nil && state.Messages[i].Role == schema.Assistant && state.Messages[i].Content != "" {
			assistantMsg = state.Messages[i]
		}
		if userMsg == nil && state.Messages[i].Role == schema.User {
			userMsg = state.Messages[i]
		}
		if userMsg != nil && assistantMsg != nil {
			break
		}
	}

	var messagesToMemorize []*schema.Message
	if userMsg != nil {
		messagesToMemorize = append(messagesToMemorize, userMsg)
	}
	if assistantMsg != nil {
		messagesToMemorize = append(messagesToMemorize, assistantMsg)
	}

	if len(messagesToMemorize) > 0 {
		go func() {
			bgCtx := context.Background()
			if err := m.provider.Memorize(bgCtx, &MemorizeRequest{
				UserID:    uid,
				SessionID: sid,
				Messages:  messagesToMemorize,
			}); err != nil {
				log.Printf("MemoryMiddleware: Memorize failed: %v", err)
			}
		}()
	}

	return ctx, state, nil
}

func (m *MemoryMiddleware) storeUserMessage(ctx context.Context, messages []*schema.Message, userID, sessionID string) {
	if len(messages) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			go func(msg *schema.Message) {
				bgCtx := context.Background()
				if err := m.provider.Memorize(bgCtx, &MemorizeRequest{
					UserID:    userID,
					SessionID: sessionID,
					Messages:  []*schema.Message{msg},
				}); err != nil {
					log.Printf("MemoryMiddleware: store user message failed: %v", err)
				}
			}(messages[i])
			return
		}
	}
}

func (m *MemoryMiddleware) beforeModelRewriteStateKey() string {
	return fmt.Sprintf("__aggo_memory_middleware_prepared_%p", m)
}
```

**Step 2: Commit**

```bash
git add memory/middleware.go
git commit -m "refactor(memory): rewrite MemoryMiddleware to use MemoryProvider interface"
```

---

### Task 5: Create memu Provider

**Files:**
- Create: `memory/memu/provider.go`
- Create: `memory/memu/client.go`
- Create: `memory/memu/types.go`

**Step 1: Create memu types**

`memory/memu/types.go`:

```go
package memu

import (
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

const MemorySystemPrefix = "[memu-memory-context]"

// HTTP request/response types for the memu API.
type QueryContent struct {
	Text string `json:"text"`
}

type Query struct {
	Role    string       `json:"role"`
	Content QueryContent `json:"content"`
}

type RetrieveRequest struct {
	Queries []Query        `json:"queries"`
	Where   map[string]any `json:"where,omitempty"`
}

type RetrievedCategory struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	Score       float64 `json:"score,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

type RetrievedItem struct {
	ID         string  `json:"id,omitempty"`
	MemoryType string  `json:"memory_type,omitempty"`
	Summary    string  `json:"summary,omitempty"`
	Score      float64 `json:"score,omitempty"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
	HappenedAt string  `json:"happened_at,omitempty"`
}

type RetrievedResource struct {
	ID       string  `json:"id,omitempty"`
	URL      string  `json:"url,omitempty"`
	Modality string  `json:"modality,omitempty"`
	Caption  string  `json:"caption,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

type RetrieveResponse struct {
	NeedsRetrieval bool                `json:"needs_retrieval,omitempty"`
	OriginalQuery  string              `json:"original_query,omitempty"`
	RewrittenQuery string              `json:"rewritten_query,omitempty"`
	NextStepQuery  string              `json:"next_step_query,omitempty"`
	Categories     []RetrievedCategory `json:"categories,omitempty"`
	Items          []RetrievedItem     `json:"items,omitempty"`
	Resources      []RetrievedResource `json:"resources,omitempty"`
}

type MemorizeRequest struct {
	Content  string         `json:"content"`
	Modality string         `json:"modality"`
	User     map[string]any `json:"user,omitempty"`
}

type MemorizeResponse map[string]any

// ProviderConfig is the config for the memu provider.
type ProviderConfig struct {
	BaseURL      string
	UserID       string
	HistoryLimit int // number of recent messages for retrieval context (default 6)
	MaxItems     int // max memory items to return (default 3)
}

// BuildRetrieveRequest builds a memu RetrieveRequest from conversation messages.
func BuildRetrieveRequest(messages []*schema.Message, userID string, limit int) RetrieveRequest {
	filtered := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role != schema.User && msg.Role != schema.Assistant {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		filtered = append(filtered, msg)
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	req := RetrieveRequest{
		Queries: make([]Query, 0, len(filtered)),
	}
	if userID != "" {
		req.Where = map[string]any{"user_id": userID}
	}

	for _, msg := range filtered {
		req.Queries = append(req.Queries, Query{
			Role: string(msg.Role),
			Content: QueryContent{
				Text: strings.TrimSpace(msg.Content),
			},
		})
	}

	return req
}

// BuildConversationTurn formats a user-assistant exchange for memorization.
func BuildConversationTurn(userText, assistantText string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return "[time: " + now + "]\nuser: " + strings.TrimSpace(userText) + "\nassistant: " + strings.TrimSpace(assistantText) + "\n"
}

// FormatMemoryContext formats a RetrieveResponse into a system message string.
func FormatMemoryContext(resp RetrieveResponse, maxItems int) string {
	hasMemory := len(resp.Categories) > 0 || len(resp.Items) > 0 || len(resp.Resources) > 0
	if !hasMemory {
		return ""
	}

	if maxItems <= 0 {
		maxItems = 3
	}

	var b strings.Builder
	b.WriteString(MemorySystemPrefix)
	b.WriteString("\nLong-term memory from memU. Use it as supporting context only. If it conflicts with the current user request, follow the current request.\n")

	if strings.TrimSpace(resp.RewrittenQuery) != "" && resp.RewrittenQuery != resp.OriginalQuery {
		b.WriteString("\nMemory search focus: ")
		b.WriteString(strings.TrimSpace(resp.RewrittenQuery))
		b.WriteString("\n")
	}

	if len(resp.Items) > 0 {
		b.WriteString("\nRelevant memory items:\n")
		for _, item := range resp.Items[:min(maxItems, len(resp.Items))] {
			summary := strings.TrimSpace(item.Summary)
			if summary == "" {
				continue
			}
			if item.MemoryType != "" {
				b.WriteString("- (")
				b.WriteString(item.MemoryType)
				b.WriteString(") ")
			} else {
				b.WriteString("- ")
			}
			b.WriteString(summary)
			if ts := strings.TrimSpace(item.CreatedAt); ts != "" {
				b.WriteString(" [recorded: ")
				b.WriteString(ts)
				b.WriteString("]")
			}
			b.WriteString("\n")
		}
	}

	if len(resp.Categories) > 0 {
		b.WriteString("\nRelevant categories:\n")
		for _, category := range resp.Categories[:min(maxItems, len(resp.Categories))] {
			b.WriteString("- ")
			name := strings.TrimSpace(category.Name)
			if name == "" {
				name = "uncategorized"
			}
			b.WriteString(name)
			summary := strings.TrimSpace(category.Summary)
			if summary != "" {
				b.WriteString(": ")
				b.WriteString(summary)
			}
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

**Step 2: Create memu HTTP client**

`memory/memu/client.go`:

```go
package memu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the memu service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new memu HTTP client.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Retrieve calls the memu /retrieve endpoint.
func (c *Client) Retrieve(ctx context.Context, req RetrieveRequest) (RetrieveResponse, error) {
	var resp RetrieveResponse
	if err := c.postJSON(ctx, "/retrieve", req, &resp); err != nil {
		return RetrieveResponse{}, err
	}
	return resp, nil
}

// Memorize calls the memu /memorize endpoint.
func (c *Client) Memorize(ctx context.Context, req MemorizeRequest) (MemorizeResponse, error) {
	var resp MemorizeResponse
	if err := c.postJSON(ctx, "/memorize", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) postJSON(ctx context.Context, path string, requestBody any, responseBody any) error {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send %s request: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("%s returned status %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
}
```

**Step 3: Create memu MemoryProvider implementation**

`memory/memu/provider.go`:

```go
package memu

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

// Ensure Provider implements MemoryProvider.
var _ memory.MemoryProvider = (*Provider)(nil)

// Provider implements memory.MemoryProvider using the memu HTTP service.
type Provider struct {
	client *Client
	config *ProviderConfig
}

// NewProvider creates a memu-backed MemoryProvider.
func NewProvider(config *ProviderConfig) (*Provider, error) {
	if config == nil || config.BaseURL == "" {
		return nil, fmt.Errorf("memu: BaseURL is required")
	}
	if config.HistoryLimit <= 0 {
		config.HistoryLimit = 6
	}
	if config.MaxItems <= 0 {
		config.MaxItems = 3
	}

	return &Provider{
		client: NewClient(config.BaseURL, nil),
		config: config,
	}, nil
}

// Retrieve calls memu /retrieve and formats the result as system messages.
func (p *Provider) Retrieve(ctx context.Context, req *memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	memuReq := BuildRetrieveRequest(req.Messages, req.UserID, p.config.HistoryLimit)
	if len(memuReq.Queries) == 0 {
		return &memory.RetrieveResult{}, nil
	}

	resp, err := p.client.Retrieve(ctx, memuReq)
	if err != nil {
		log.Printf("memu: Retrieve failed: %v", err)
		return &memory.RetrieveResult{}, nil // graceful degradation
	}

	memoryContext := FormatMemoryContext(resp, p.config.MaxItems)
	result := &memory.RetrieveResult{
		Metadata: map[string]any{
			"rewritten_query": resp.RewrittenQuery,
		},
	}

	if strings.TrimSpace(memoryContext) != "" {
		result.SystemMessages = []*schema.Message{
			schema.SystemMessage(memoryContext),
		}
	}

	return result, nil
}

// Memorize calls memu /memorize with the conversation turn.
func (p *Provider) Memorize(ctx context.Context, req *memory.MemorizeRequest) error {
	var userText, assistantText string
	for _, msg := range req.Messages {
		if msg.Role == schema.User {
			userText = msg.Content
		}
		if msg.Role == schema.Assistant {
			assistantText = msg.Content
		}
	}

	// Only memorize if we have both sides of a conversation turn
	if userText == "" || assistantText == "" {
		return nil
	}

	content := BuildConversationTurn(userText, assistantText)
	memReq := MemorizeRequest{
		Content:  content,
		Modality: "conversation",
	}
	if req.UserID != "" {
		memReq.User = map[string]any{"user_id": req.UserID}
	}

	_, err := p.client.Memorize(ctx, memReq)
	if err != nil {
		log.Printf("memu: Memorize failed: %v", err)
	}
	return err
}

// Close is a no-op for the memu provider (HTTP client has no persistent resources).
func (p *Provider) Close() error {
	return nil
}

func init() {
	memory.MustRegisterPlugin(&memory.Plugin{
		ID: "memu",
		Factory: func(config any) (memory.MemoryProvider, error) {
			cfg, ok := config.(*ProviderConfig)
			if !ok {
				return nil, fmt.Errorf("memu: expected *ProviderConfig, got %T", config)
			}
			return NewProvider(cfg)
		},
	})
}
```

**Step 4: Commit**

```bash
git add memory/memu/
git commit -m "feat(memory): add memu provider implementation"
```

---

### Task 6: Update Agent Builder

**Files:**
- Modify: `agent/builder.go`

**Step 1: Update WithMemoryMiddleware to accept MemoryProvider**

Change the builder to accept `memory.MemoryProvider` instead of `*memory.MemoryMiddleware`:

```go
// WithMemory adds a memory provider and creates the middleware automatically.
func (b *AgentBuilder) WithMemory(provider memory.MemoryProvider) *AgentBuilder {
	b.middlewares = append(b.middlewares, memory.NewMemoryMiddleware(provider))
	return b
}

// WithMemoryMiddleware adds a pre-built MemoryMiddleware (for advanced usage).
func (b *AgentBuilder) WithMemoryMiddleware(mm *memory.MemoryMiddleware) *AgentBuilder {
	b.middlewares = append(b.middlewares, mm)
	return b
}
```

**Step 2: Commit**

```bash
git add agent/builder.go
git commit -m "refactor(agent): add WithMemory method accepting MemoryProvider"
```

---

### Task 7: Update Cron Agent

**Files:**
- Modify: `cron/agent.go`

**Step 1: Replace MemoryManager with MemoryProvider**

Change `WithCronMemoryManager(mm *memory.MemoryManager)` to `WithCronMemory(provider memory.MemoryProvider)`:

```go
// WithCronMemory sets a memory provider for the cron agent.
func WithCronMemory(provider memory.MemoryProvider) CronAgentOption {
	return func(c *cronConfig) {
		c.memoryProvider = provider
	}
}
```

Update the handler construction:
```go
if cfg.memoryProvider != nil {
	handlers = append(handlers, memory.NewMemoryMiddleware(cfg.memoryProvider))
}
```

**Step 2: Commit**

```bash
git add cron/agent.go
git commit -m "refactor(cron): use MemoryProvider instead of MemoryManager"
```

---

### Task 8: Update Examples

**Files:**
- Modify: `example/mem_agent_test/main.go`
- Modify: `example/sse/main.go`
- Modify: `example/knowledge_agent_tool_test/main.go`
- Modify: `example/claw/main.go`

**Step 1: Update all examples to use the new API**

Each example needs to:
1. Import `github.com/CoolBanHub/aggo/memory/builtin` instead of direct `memory.NewMemoryManager`
2. Use `agent.WithMemory(memoryManager)` instead of `agent.WithMemoryMiddleware(memory.NewMemoryMiddleware(memoryManager))`

Example pattern:
```go
import (
    "github.com/CoolBanHub/aggo/memory"
    "github.com/CoolBanHub/aggo/memory/builtin"
    "github.com/CoolBanHub/aggo/memory/builtin/storage"
)

// Create storage
s := storage.NewMemoryStore()

// Create builtin provider
memoryManager, err := builtin.NewMemoryManager(cm, s, &builtin.MemoryConfig{...})

// Use with agent
ag, err := agent.NewAgentBuilder(cm).
    WithMemory(memoryManager).  // MemoryManager implements MemoryProvider
    Build(ctx)
```

**Step 2: Commit**

```bash
git add example/
git commit -m "refactor(example): update all examples to use new memory API"
```

---

### Task 9: Add Compatibility Layer (Optional)

**Files:**
- Create: `memory/compat.go`

To ease migration, add type aliases and re-exports so existing code doesn't break:

```go
package memory

// Re-export builtin types for backward compatibility.
// Deprecated: import github.com/CoolBanHub/aggo/memory/builtin directly.
type (
	MemoryConfig             = builtin.MemoryConfig
	MemoryStorage            = builtin.MemoryStorage
	UserMemory               = builtin.UserMemory
	SessionSummary           = builtin.SessionSummary
	ConversationMessage      = builtin.ConversationMessage
	MemoryManager            = builtin.MemoryManager
	MemoryRetrieval          = builtin.MemoryRetrieval
	SummaryTriggerConfig     = builtin.SummaryTriggerConfig
	SummaryTriggerStrategy   = builtin.SummaryTriggerStrategy
	CleanupConfig            = builtin.CleanupConfig
)
```

And re-export functions:
```go
var (
	NewMemoryManager = builtin.NewMemoryManager
	DefaultMemoryConfig = builtin.DefaultMemoryConfig
)
```

**Step 2: Commit**

```bash
git add memory/compat.go
git commit -m "feat(memory): add backward compatibility re-exports"
```

---

### Task 10: Verify Build and Run Tests

**Step 1: Run go build**

```bash
cd /Users/dachang/Workspace/go/src/github.com/CoolBanHub/aggo
go build ./...
```

Expected: Clean build with no errors.

**Step 2: Run tests**

```bash
go test ./...
```

Expected: All existing tests pass (may need path updates for moved files).

**Step 3: Commit final state**

```bash
git add -A
git commit -m "chore: fix compilation and test issues after memory plugin refactor"
```
