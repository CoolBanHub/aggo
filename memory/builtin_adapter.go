package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/memory/builtin"
	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
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

	var sessionSummary *builtin.SessionSummary

	// Fetch user memory as system message
	if cfg.EnableUserMemories {
		userMemory, err := p.MemoryManager.GetUserMemory(ctx, req.UserID)
		if err == nil && userMemory != nil && userMemory.Memory != "" {
			result.SystemMessages = append(result.SystemMessages, &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("<user_memory>\n%s\n</user_memory>", userMemory.Memory),
			})
		}
	}

	// Fetch session summary as system message
	if cfg.EnableSessionSummary {
		summary, err := p.MemoryManager.GetSessionSummary(ctx, req.SessionID, req.UserID)
		if err == nil && summary != nil && summary.Summary != "" {
			sessionSummary = summary
			result.SystemMessages = append(result.SystemMessages, &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("<session_context>\n%s\n</session_context>", summary.Summary),
			})
		}
	}

	// Fetch conversation history
	limit := req.Limit
	if limit <= 0 {
		limit = cfg.MemoryLimit
	}
	if cfg.EnableSessionSummary && sessionSummary != nil {
		history, err := p.MemoryManager.GetMessagesAfterSummary(ctx, req.SessionID, req.UserID, limit)
		if err == nil && len(history) > 0 {
			result.HistoryMessages = decorateHistoryMessages(history)
		}
	} else {
		history, err := p.MemoryManager.GetMessages(ctx, req.SessionID, req.UserID, limit)
		if err == nil && len(history) > 0 {
			result.HistoryMessages = decorateHistoryMessages(history)
		}
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
			content := msg.Content
			if content == "" && len(msg.UserInputMultiContent) > 0 {
				content = extractTextFromParts(msg.UserInputMultiContent)
			}
			if err := p.MemoryManager.ProcessUserMessage(ctx, req.UserID, req.SessionID, content, msg.UserInputMultiContent); err != nil {
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

func (p *builtinProvider) SearchMessages(ctx context.Context, q *builtinsearch.SearchQuery) ([]*builtinsearch.SearchHit, error) {
	return p.MemoryManager.SearchMessages(ctx, q)
}

// extractTextFromParts 从多部分内容中提取纯文本，拼接为一个字符串
func extractTextFromParts(parts []schema.MessageInputPart) string {
	var texts []string
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func decorateHistoryMessages(history []*schema.Message) []*schema.Message {
	decorated := make([]*schema.Message, 0, len(history))
	for _, msg := range history {
		decorated = append(decorated, builtin.PrefixHistoryTimestamp(msg))
	}
	return decorated
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
			mgr.SetAsyncTaskContextBuilder(cfg.AsyncTaskContextBuilder)
			return &builtinProvider{MemoryManager: mgr}, nil
		},
	})
}
