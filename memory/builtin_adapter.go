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
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
)

// Ensure MemoryManager still satisfies MemoryProvider after wrapping.
// The adapter methods are defined here to avoid import cycles between
// memory and memory/builtin.

// builtinProvider wraps a *builtin.MemoryManager to implement MemoryProvider.
type builtinProvider struct {
	*builtin.MemoryManager
}

// Retrieve implements MemoryProvider.
//
// 检索子步骤失败不中断整体检索（best-effort 兼容旧行为），但会记录到
// result.Warnings 并输出日志；配置了 ContextBudget 时按优先级裁剪注入内容，
// 裁剪同样记录 warning 与 Metadata[“truncated”]。
func (p *builtinProvider) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error) {
	if req == nil {
		return nil, fmt.Errorf("retrieve request is nil")
	}

	cfg := p.MemoryManager.GetConfig()
	budget := cfg.ContextBudget

	result := &RetrieveResult{
		Metadata: make(map[string]any),
	}
	addWarning := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		result.Warnings = append(result.Warnings, msg)
		slog.Warnf("memory: retrieve degraded: %s", msg)
	}
	markTruncated := func() {
		result.Metadata["truncated"] = true
	}

	var sessionSummary *builtin.SessionSummary
	var memoryBlock, eventsBlock, summaryBlock string

	// Fetch user memory as dynamic context messages. Keeping it out of the
	// system prompt preserves provider-side prompt caching for the stable
	// instruction.
	if cfg.EnableUserMemories {
		userMemory, err := p.MemoryManager.GetUserMemory(ctx, req.UserID)
		switch {
		case err != nil:
			addWarning("get_user_memory_failed: %v", err)
		case userMemory != nil && userMemory.Memory != "":
			text := userMemory.Memory
			if budget.MaxMemoryRunes > 0 {
				var truncated bool
				text, truncated = truncateRunes(text, budget.MaxMemoryRunes)
				if truncated {
					markTruncated()
					addWarning("user_memory_truncated_to_%d_runes", budget.MaxMemoryRunes)
				}
			}
			memoryBlock = fmt.Sprintf("<user_memory>\n%s\n</user_memory>", text)
		}

		// 事件检索模式：再追加“最近 N 条事件”块，更早的事件由 search_user_memory 工具按需检索。
		if cfg.EnableEventSearch && cfg.RecentEventLimit > 0 {
			events, evtErr := p.MemoryManager.ListRecentUserMemoryEvents(ctx, req.UserID, cfg.RecentEventLimit)
			switch {
			case evtErr != nil:
				addWarning("list_recent_events_failed: %v", evtErr)
			case len(events) > 0:
				if budget.MaxEventItems > 0 && len(events) > budget.MaxEventItems {
					events = events[:budget.MaxEventItems]
					markTruncated()
					addWarning("recent_events_truncated_to_%d_items", budget.MaxEventItems)
				}
				eventsBlock = formatRecentEventsBlock(events)
			}
		}
	}

	// Fetch session summary as dynamic context.
	if cfg.EnableSessionSummary {
		summary, err := p.MemoryManager.GetSessionSummary(ctx, req.SessionID, req.UserID)
		switch {
		case err != nil:
			addWarning("get_session_summary_failed: %v", err)
		case summary != nil && summary.Summary != "":
			sessionSummary = summary
			text := summary.Summary
			if budget.MaxSummaryRunes > 0 {
				var truncated bool
				text, truncated = truncateRunes(text, budget.MaxSummaryRunes)
				if truncated {
					markTruncated()
					addWarning("session_summary_truncated_to_%d_runes", budget.MaxSummaryRunes)
				}
			}
			summaryBlock = fmt.Sprintf("<session_context>\n%s\n</session_context>", text)
		}
	}

	// Fetch conversation history
	limit := req.Limit
	if limit <= 0 {
		limit = cfg.MemoryLimit
	}
	var history []*schema.AgenticMessage
	if cfg.EnableSessionSummary && sessionSummary != nil {
		h, err := p.MemoryManager.GetMessagesAfterSummary(ctx, req.SessionID, req.UserID, limit)
		if err != nil {
			addWarning("get_messages_after_summary_failed: %v", err)
		}
		history = h
		if cfg.SummaryRecentMessageLimit > 0 {
			recent, recentErr := p.MemoryManager.GetMessages(ctx, req.SessionID, req.UserID, cfg.SummaryRecentMessageLimit)
			if recentErr != nil {
				addWarning("get_recent_messages_failed: %v", recentErr)
			} else if len(recent) > 0 {
				history = mergeHistoryMessages(maxInt(limit, cfg.SummaryRecentMessageLimit), recent, history)
			}
		}
	} else {
		h, err := p.MemoryManager.GetMessages(ctx, req.SessionID, req.UserID, limit)
		if err != nil {
			addWarning("get_messages_failed: %v", err)
		}
		history = h
	}

	// 预算裁剪：先限制历史条数，再按总字符预算从低优先级端丢弃。
	// 优先级（保留从高到低）：最近未摘要消息 > 会话摘要 > 最近事件 > 长期记忆。
	if budget.MaxHistoryItems > 0 && len(history) > budget.MaxHistoryItems {
		history = history[len(history)-budget.MaxHistoryItems:]
		markTruncated()
		addWarning("history_truncated_to_%d_items", budget.MaxHistoryItems)
	}
	if budget.MaxRunes > 0 {
		total := runeLen(memoryBlock) + runeLen(eventsBlock) + runeLen(summaryBlock) + agenticMessagesRunes(history)
		dropped := false
		for total > budget.MaxRunes {
			switch {
			case memoryBlock != "":
				total -= runeLen(memoryBlock)
				memoryBlock = ""
				dropped = true
			case eventsBlock != "":
				total -= runeLen(eventsBlock)
				eventsBlock = ""
				dropped = true
			case summaryBlock != "":
				total -= runeLen(summaryBlock)
				summaryBlock = ""
				dropped = true
			case len(history) > 1: // 至少保留最近一条历史消息
				droppedRunes := agmsg.Text(history[0])
				history = history[1:]
				total -= runeLen(droppedRunes)
				dropped = true
			default:
				// Keep the newest message, but trim its text to the remaining
				// budget instead of pretending the budget is satisfied.
				remaining := budget.MaxRunes - runeLen(memoryBlock) - runeLen(eventsBlock) - runeLen(summaryBlock)
				if remaining < 0 {
					remaining = 0
				}
				if len(history) == 1 {
					trimmed, didTrim := truncateAgenticMessage(history[0], remaining)
					history[0] = trimmed
					dropped = dropped || didTrim
				}
				total = runeLen(memoryBlock) + runeLen(eventsBlock) + runeLen(summaryBlock) + agenticMessagesRunes(history)
				if total >= budget.MaxRunes {
					break
				}
			}
		}
		if dropped {
			markTruncated()
			addWarning("context_budget_exceeded_max_%d_runes_dropped_lower_priority_context", budget.MaxRunes)
		}
	}

	// 保持原有注入顺序：长期记忆 → 最近事件 → 会话摘要
	for _, block := range []string{memoryBlock, eventsBlock, summaryBlock} {
		if block == "" {
			continue
		}
		result.ContextMessages = append(result.ContextMessages, schema.UserAgenticMessage(block))
	}
	if len(history) > 0 {
		result.HistoryMessages = decorateHistoryMessages(history)
	}

	return result, nil
}

// truncateRunes 在 rune 边界裁剪文本，保证结果不超过 max 个 rune。
// 发生裁剪时返回 true；截断处以省略号标记。
func truncateRunes(s string, max int) (string, bool) {
	if max <= 0 {
		return s, false
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	const marker = "…"
	markerLen := len([]rune(marker))
	cut := max - markerLen
	if cut <= 0 {
		// 预算太小放不下标记，直接硬裁剪
		return string(runes[:max]), true
	}
	return string(runes[:cut]) + marker, true
}

func runeLen(s string) int {
	return len([]rune(s))
}

func agenticMessagesRunes(messages []*schema.AgenticMessage) int {
	total := 0
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		total += runeLen(agmsg.Text(msg))
	}
	return total
}

func truncateAgenticMessage(message *schema.AgenticMessage, max int) (*schema.AgenticMessage, bool) {
	if message == nil {
		return nil, false
	}
	text := agmsg.Text(message)
	trimmed, changed := truncateRunes(text, max)
	if max <= 0 && text != "" {
		trimmed, changed = "", true
	}
	if !changed {
		return message, false
	}
	copyMessage := &schema.AgenticMessage{Role: message.Role}
	if message.Extra != nil {
		copyMessage.Extra = make(map[string]any, len(message.Extra))
		for key, value := range message.Extra {
			copyMessage.Extra[key] = value
		}
	}
	if message.Role == schema.AgenticRoleTypeAssistant {
		copyMessage.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: trimmed})}
	} else {
		copyMessage.ContentBlocks = []*schema.ContentBlock{schema.NewContentBlock(&schema.UserInputText{Text: trimmed})}
	}
	return copyMessage, true
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

// EnqueueMemorize implements memory.AsyncMemorizer：把整轮 Memorize 交给
// MemoryManager 的受管工作池执行，写入随 Flush/Shutdown 收敛，
// 替代 middleware 侧无生命周期的游离 goroutine。
func (p *builtinProvider) EnqueueMemorize(ctx context.Context, req *MemorizeRequest) error {
	if req == nil {
		return fmt.Errorf("memorize request is nil")
	}

	// 深拷贝请求，调用方后续修改入参不影响实际写入
	cloned := &MemorizeRequest{
		UserID:    req.UserID,
		SessionID: req.SessionID,
		Messages:  make([]*schema.AgenticMessage, 0, len(req.Messages)),
	}
	for _, msg := range req.Messages {
		if msg == nil {
			continue
		}
		clonedMsg, err := cloneAgenticMessage(msg)
		if err != nil {
			return err
		}
		cloned.Messages = append(cloned.Messages, clonedMsg)
	}

	userID, sessionID := req.UserID, req.SessionID
	return p.MemoryManager.EnqueueTask("memorize", userID, sessionID, func(taskCtx context.Context) {
		if err := p.Memorize(taskCtx, cloned); err != nil {
			slog.Errorf("memory: async memorize failed: userID=%s sessionID=%s err=%v", userID, sessionID, err)
		}
	})
}

func cloneAgenticMessage(message *schema.AgenticMessage) (*schema.AgenticMessage, error) {
	if message == nil {
		return nil, nil
	}
	data, err := sonic.Marshal(message)
	if err != nil {
		return nil, err
	}
	cloned := new(schema.AgenticMessage)
	if err := sonic.Unmarshal(data, cloned); err != nil {
		return nil, err
	}
	return cloned, nil
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

// formatRecentEventsBlock 把最近事件渲染为上下文块，控制每条字数避免冲爆 prompt。
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
