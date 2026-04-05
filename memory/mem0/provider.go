package mem0

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

var _ memory.MemoryProvider = (*Provider)(nil)

// Provider implements memory.MemoryProvider backed by a mem0-compatible API.
type Provider struct {
	client *Client
	config *ProviderConfig
}

// NewProvider creates a mem0 provider with normalized config defaults.
func NewProvider(config *ProviderConfig) (*Provider, error) {
	cfg, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	return &Provider{
		client: NewClient(cfg),
		config: cfg,
	}, nil
}

// Retrieve fetches relevant mem0 context before a model call.
func (p *Provider) Retrieve(ctx context.Context, req *memory.RetrieveRequest) (*memory.RetrieveResult, error) {
	query := buildSearchQuery(req.Messages, p.config.SearchMsgLimit, p.config.QueryCharLimit)
	if strings.TrimSpace(query) == "" {
		return &memory.RetrieveResult{}, nil
	}

	searchReq := SearchRequest{
		Query:     query,
		UserID:    req.UserID,
		AgentID:   p.config.AgentID,
		AppID:     p.config.AppID,
		OrgID:     p.config.OrgID,
		ProjectID: p.config.ProjectID,
		Limit:     p.config.SearchResultLimit,
	}
	if p.config.UseSessionAsRunID && p.config.SearchBySession && strings.TrimSpace(req.SessionID) != "" {
		searchReq.RunID = req.SessionID
	}

	items, err := p.client.Search(ctx, searchReq)
	if err != nil {
		log.Printf("mem0: Retrieve failed: %v", err)
		return &memory.RetrieveResult{}, nil
	}

	memoryContext := FormatMemoryContext(items, p.config.OutputMemoryLimit)
	result := &memory.RetrieveResult{
		Metadata: map[string]any{
			"query":           query,
			"retrieved_count": len(items),
		},
	}
	if strings.TrimSpace(memoryContext) != "" {
		result.SystemMessages = []*schema.Message{
			schema.SystemMessage(memoryContext),
		}
	}

	return result, nil
}

// Memorize persists the latest user + assistant turn into mem0.
func (p *Provider) Memorize(ctx context.Context, req *memory.MemorizeRequest) error {
	var userText, assistantText string
	for _, msg := range req.Messages {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		switch msg.Role {
		case schema.User:
			userText = strings.TrimSpace(msg.Content)
		case schema.Assistant:
			assistantText = strings.TrimSpace(msg.Content)
		}
	}

	if userText == "" || assistantText == "" {
		return nil
	}

	metadata := cloneAnyMap(p.config.Metadata)
	if key := strings.TrimSpace(p.config.SessionMetadataKey); key != "" && strings.TrimSpace(req.SessionID) != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata[key] = req.SessionID
	}

	addReq := AddRequest{
		Messages: []ClientMessage{
			{Role: "user", Content: userText},
			{Role: "assistant", Content: assistantText},
		},
		UserID:    req.UserID,
		AgentID:   p.config.AgentID,
		AppID:     p.config.AppID,
		OrgID:     p.config.OrgID,
		ProjectID: p.config.ProjectID,
		Metadata:  metadata,
	}
	if p.config.UseSessionAsRunID && strings.TrimSpace(req.SessionID) != "" {
		addReq.RunID = req.SessionID
	}

	if err := p.client.Add(ctx, addReq); err != nil {
		log.Printf("mem0: Memorize failed: %v", err)
		return err
	}
	return nil
}

// Close releases resources held by the provider. Currently a no-op.
func (p *Provider) Close() error {
	return nil
}

// FormatMemoryContext turns search results into a system message payload.
func FormatMemoryContext(items []SearchItem, limit int) string {
	if limit <= 0 {
		limit = defaultOutputMemoryLimit
	}

	var lines []string
	for _, item := range items {
		text := truncateRunes(strings.TrimSpace(item.Memory), defaultPerItemCharLimit)
		if text == "" {
			continue
		}

		line := "- "
		if len(item.Categories) > 0 {
			line += "[" + strings.Join(item.Categories, ", ") + "] "
		}
		line += text
		if item.Score > 0 {
			line += fmt.Sprintf(" [score: %.2f]", item.Score)
		}
		if recordedAt := strings.TrimSpace(item.CreatedAt); recordedAt != "" {
			line += " [recorded: " + recordedAt + "]"
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			break
		}
	}

	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(MemorySystemPrefix)
	b.WriteString("\nLong-term memory from mem0. Use it as supporting context only. If it conflicts with the current user request, follow the current request.\n")
	b.WriteString("\nRelevant memory items:\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

func normalizeConfig(config *ProviderConfig) (*ProviderConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("mem0: config is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("mem0: BaseURL is required")
	}

	cfg := *config
	if cfg.Mode == "" {
		cfg.Mode = ModeHosted
	}
	if cfg.Mode != ModeHosted && cfg.Mode != ModeOSS {
		return nil, fmt.Errorf("mem0: unsupported mode %q", cfg.Mode)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.SearchMsgLimit <= 0 {
		cfg.SearchMsgLimit = defaultSearchMsgLimit
	}
	if cfg.SearchResultLimit <= 0 {
		cfg.SearchResultLimit = defaultSearchResultLimit
	}
	if cfg.OutputMemoryLimit <= 0 {
		cfg.OutputMemoryLimit = defaultOutputMemoryLimit
	}
	if cfg.QueryCharLimit <= 0 {
		cfg.QueryCharLimit = defaultQueryCharLimit
	}
	if strings.TrimSpace(cfg.SessionMetadataKey) == "" {
		cfg.SessionMetadataKey = defaultSessionMetaKey
	}
	if strings.TrimSpace(cfg.AddPath) == "" {
		cfg.AddPath = defaultAddPath(cfg.Mode)
	}
	if strings.TrimSpace(cfg.SearchPath) == "" {
		cfg.SearchPath = defaultSearchPath(cfg.Mode)
	}
	if strings.TrimSpace(cfg.Version) == "" && cfg.Mode == ModeHosted {
		cfg.Version = defaultHostedVersion
	}
	cfg.ExtraHeaders = cloneStringMap(cfg.ExtraHeaders)
	cfg.Metadata = cloneAnyMap(cfg.Metadata)
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}

	return &cfg, nil
}

func defaultAddPath(mode Mode) string {
	switch mode {
	case ModeOSS:
		return "/memories"
	default:
		return "/v1/memories/"
	}
}

func defaultSearchPath(mode Mode) string {
	switch mode {
	case ModeOSS:
		return "/search"
	default:
		return "/v1/memories/search/"
	}
}

func buildSearchQuery(messages []*schema.Message, limit, maxChars int) string {
	recent := recentConversationMessages(messages, limit)
	if len(recent) == 0 {
		return ""
	}

	for i := len(recent) - 1; i >= 0; i-- {
		if recent[i].Role != schema.User {
			continue
		}

		query := recent[i].Content
		if runeCount(query) < 80 && i > 0 {
			query = recent[i-1].Content + "\n" + query
		}
		return truncateRunes(strings.TrimSpace(query), maxChars)
	}

	start := len(recent) - 2
	if start < 0 {
		start = 0
	}
	parts := make([]string, 0, len(recent[start:]))
	for _, msg := range recent[start:] {
		parts = append(parts, msg.Content)
	}
	return truncateRunes(strings.Join(parts, "\n"), maxChars)
}

func recentConversationMessages(messages []*schema.Message, limit int) []*schema.Message {
	filtered := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		if msg.Role != schema.User && msg.Role != schema.Assistant {
			continue
		}
		filtered = append(filtered, &schema.Message{
			Role:    msg.Role,
			Content: strings.TrimSpace(msg.Content),
		})
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

func truncateRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || runeCount(value) <= max {
		return value
	}

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func init() {
	memory.MustRegisterPlugin(&memory.Plugin{
		ID: "mem0",
		Factory: func(config any) (memory.MemoryProvider, error) {
			cfg, ok := config.(*ProviderConfig)
			if !ok {
				return nil, fmt.Errorf("mem0: expected *ProviderConfig, got %T", config)
			}
			return NewProvider(cfg)
		},
	})
}
