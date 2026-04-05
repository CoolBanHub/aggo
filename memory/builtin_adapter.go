package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/cloudwego/eino/schema"
)

// Ensure MemoryManager still satisfies MemoryProvider after wrapping.
// The adapter methods are defined here to avoid import cycles between
// memory and memory/builtin.

// builtinProvider wraps a *builtin.MemoryManager to implement MemoryProvider.
type builtinProvider struct {
	*builtin.MemoryManager
}

// Retrieve implements MemoryProvider.
func (p *builtinProvider) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error) {
	if req == nil {
		return nil, fmt.Errorf("retrieve request is nil")
	}

	cfg := p.MemoryManager.GetConfig()

	result := &RetrieveResult{
		Metadata: make(map[string]any),
	}

	// Fetch user memory as system message
	if cfg.EnableUserMemories {
		userMemory, err := p.MemoryManager.GetUserMemory(ctx, req.UserID)
		if err == nil && userMemory != nil && userMemory.Memory != "" {
			var builder strings.Builder
			builder.WriteString("用户个人信息记忆（请在回复中考虑这些信息，提供个性化的响应）:\n")
			builder.WriteString(userMemory.Memory)
			result.SystemMessages = append(result.SystemMessages, &schema.Message{
				Role:    schema.System,
				Content: builder.String(),
			})
		}
	}

	// Fetch session summary as system message
	if cfg.EnableSessionSummary {
		summary, err := p.MemoryManager.GetSessionSummary(ctx, req.SessionID, req.UserID)
		if err == nil && summary != nil && summary.Summary != "" {
			result.SystemMessages = append(result.SystemMessages, &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("会话背景: %s", summary.Summary),
			})
		}
	}

	// Fetch conversation history
	limit := req.Limit
	if limit <= 0 {
		limit = cfg.MemoryLimit
	}
	history, err := p.MemoryManager.GetMessages(ctx, req.SessionID, req.UserID, limit)
	if err == nil && len(history) > 0 {
		result.HistoryMessages = history
	}

	return result, nil
}

// Memorize implements MemoryProvider.
func (p *builtinProvider) Memorize(ctx context.Context, req *MemorizeRequest) error {
	if req == nil {
		return fmt.Errorf("memorize request is nil")
	}

	for _, msg := range req.Messages {
		if msg.Role == schema.User {
			if err := p.MemoryManager.ProcessUserMessage(ctx, req.UserID, req.SessionID, msg.Content, msg.UserInputMultiContent); err != nil {
				return fmt.Errorf("save user message: %w", err)
			}
		}
	}

	for _, msg := range req.Messages {
		if msg.Role == schema.Assistant && msg.Content != "" {
			if err := p.MemoryManager.ProcessAssistantMessage(ctx, req.UserID, req.SessionID, msg.Content); err != nil {
				return fmt.Errorf("save assistant message: %w", err)
			}
		}
	}

	return nil
}

// Close delegates to the underlying MemoryManager.
func (p *builtinProvider) Close() error {
	return p.MemoryManager.Close()
}

func init() {
	MustRegisterPlugin(&Plugin{
		ID: "builtin",
		Factory: func(config any) (MemoryProvider, error) {
			cfg, ok := config.(*builtin.ProviderConfig)
			if !ok {
				return nil, fmt.Errorf("builtin: expected *ProviderConfig, got %T", config)
			}
			mgr, err := builtin.NewMemoryManager(cfg.ChatModel, cfg.Storage, cfg.MemoryConfig)
			if err != nil {
				return nil, err
			}
			return &builtinProvider{MemoryManager: mgr}, nil
		},
	})
}
