package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
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
			result.SystemMessages = append(result.SystemMessages, schema.SystemAgenticMessage(fmt.Sprintf("<user_memory>\n%s\n</user_memory>", userMemory.Memory)))
		}

		// 事件检索模式：再追加“最近 N 条事件”块，更早的事件由 search_user_memory 工具按需检索。
		if cfg.EnableEventSearch && cfg.RecentEventLimit > 0 {
			events, evtErr := p.MemoryManager.ListRecentUserMemoryEvents(ctx, req.UserID, cfg.RecentEventLimit)
			if evtErr == nil && len(events) > 0 {
				result.SystemMessages = append(result.SystemMessages, schema.SystemAgenticMessage(formatRecentEventsBlock(events)))
			}
		}
	}

	// Fetch session summary as system message
	if cfg.EnableSessionSummary {
		summary, err := p.MemoryManager.GetSessionSummary(ctx, req.SessionID, req.UserID)
		if err == nil && summary != nil && summary.Summary != "" {
			sessionSummary = summary
			result.SystemMessages = append(result.SystemMessages, schema.SystemAgenticMessage(fmt.Sprintf("<session_context>\n%s\n</session_context>", summary.Summary)))
		}
	}

	// Fetch conversation history
	limit := req.Limit
	if limit <= 0 {
		limit = cfg.MemoryLimit
	}
	if cfg.EnableSessionSummary && sessionSummary != nil {
		history, err := p.MemoryManager.GetMessagesAfterSummary(ctx, req.SessionID, req.UserID, limit)
		if err != nil {
			history = nil
		}
		if cfg.SummaryRecentMessageLimit > 0 {
			recent, recentErr := p.MemoryManager.GetMessages(ctx, req.SessionID, req.UserID, cfg.SummaryRecentMessageLimit)
			if recentErr == nil && len(recent) > 0 {
				history = mergeHistoryMessages(maxInt(limit, cfg.SummaryRecentMessageLimit), recent, history)
			}
		}
		if len(history) > 0 {
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
		if msg.Role == schema.AgenticRoleTypeUser {
			content := agmsg.Text(msg)
			parts := agmsg.InputParts(msg)
			if content == "" && len(parts) > 0 {
				content = extractTextFromParts(parts)
			}
			if err := p.MemoryManager.ProcessUserMessage(ctx, req.UserID, req.SessionID, content, parts); err != nil {
				return fmt.Errorf("save user message: %w", err)
			}
		}
	}

	for _, msg := range req.Messages {
		if msg.Role == schema.AgenticRoleTypeAssistant {
			content := agmsg.Text(msg)
			if content == "" {
				continue
			}
			if err := p.MemoryManager.ProcessAssistantMessage(ctx, req.UserID, req.SessionID, content); err != nil {
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

// SearchUserMemoryEvents 让外部（如 search_user_memory 工具）按条件检索用户记忆事件。
func (p *builtinProvider) SearchUserMemoryEvents(ctx context.Context, query *builtin.UserMemoryEventQuery) ([]*builtin.UserMemoryEvent, error) {
	return p.MemoryManager.SearchUserMemoryEvents(ctx, query)
}

// ListRecentUserMemoryEvents 让外部读取最近事件，可用于调试或额外的工具适配。
func (p *builtinProvider) ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*builtin.UserMemoryEvent, error) {
	return p.MemoryManager.ListRecentUserMemoryEvents(ctx, userID, limit)
}

// formatRecentEventsBlock 把最近事件渲染为 system message 块，控制每条字数避免冲爆 prompt。
func formatRecentEventsBlock(events []*builtin.UserMemoryEvent) string {
	var b strings.Builder
	b.WriteString("<user_memory_recent_events>\n")
	b.WriteString("以下是该用户最近的任务里程碑/事件记录，按 EventDate 倒序。\n")
	b.WriteString("如需查找更早或更宽范围的事件，请调用 search_user_memory 工具检索。\n\n")
	for _, evt := range events {
		if evt == nil {
			continue
		}
		date := ""
		if !evt.EventDate.IsZero() {
			date = evt.EventDate.Format("2006-01-02")
		}
		summary := evt.Summary
		// 限制单条字数，防止短文档塞满异常长的事件
		const maxSummaryRunes = 180
		if r := []rune(summary); len(r) > maxSummaryRunes {
			summary = string(r[:maxSummaryRunes]) + "…"
		}
		b.WriteString(fmt.Sprintf("- [%s][%s] %s\n", date, evt.Type, summary))
	}
	b.WriteString("</user_memory_recent_events>")
	return b.String()
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

func decorateHistoryMessages(history []*schema.AgenticMessage) []*schema.AgenticMessage {
	decorated := make([]*schema.AgenticMessage, 0, len(history))
	for _, msg := range history {
		decorated = append(decorated, builtin.PrefixHistoryTimestamp(msg))
	}
	return decorated
}

func mergeHistoryMessages(limit int, histories ...[]*schema.AgenticMessage) []*schema.AgenticMessage {
	type item struct {
		msg     *schema.AgenticMessage
		order   int
		created time.Time
		hasTime bool
	}

	items := make([]item, 0)
	seen := make(map[string]struct{})
	order := 0
	for _, history := range histories {
		for _, msg := range history {
			if msg == nil {
				continue
			}
			if id := messageExtraString(msg, builtin.MessageExtraIDKey); id != "" {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
			}
			created, hasTime := messageCreatedAt(msg)
			items = append(items, item{
				msg:     msg,
				order:   order,
				created: created,
				hasTime: hasTime,
			})
			order++
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].hasTime && items[j].hasTime && !items[i].created.Equal(items[j].created) {
			return items[i].created.Before(items[j].created)
		}
		return items[i].order < items[j].order
	})

	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}

	merged := make([]*schema.AgenticMessage, 0, len(items))
	for _, item := range items {
		merged = append(merged, item.msg)
	}
	return merged
}

func messageExtraString(msg *schema.AgenticMessage, key string) string {
	if msg == nil || msg.Extra == nil {
		return ""
	}
	value, ok := msg.Extra[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func messageCreatedAt(msg *schema.AgenticMessage) (time.Time, bool) {
	raw := messageExtraString(msg, builtin.MessageExtraCreatedAtKey)
	if raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
