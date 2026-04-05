package memu

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

var _ memory.MemoryProvider = (*Provider)(nil)

// Provider implements memory.MemoryProvider backed by the memu HTTP service.
type Provider struct {
	client *Client
	config *ProviderConfig
}

// NewProvider creates a new memu Provider. It returns an error if config is nil
// or BaseURL is empty. Default values are applied for HistoryLimit (6) and
// MaxItems (3) when not set.
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

// Retrieve fetches relevant memory context from the memu service before a model call.
// It gracefully degrades on error, returning an empty result.
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

// Memorize persists a conversation turn to the memu service after a model call.
// It extracts the user and assistant messages and sends them as a conversation turn.
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

// Close releases any resources held by the provider. Currently a no-op.
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
