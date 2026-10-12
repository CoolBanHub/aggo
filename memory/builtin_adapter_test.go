package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"github.com/cloudwego/eino/schema"
)

func TestBuiltinRetrieveKeepsRecentMessagesWithSessionSummary(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := builtin.NewMemoryManager(nil, store, &builtin.MemoryConfig{
		EnableUserMemories:        false,
		EnableSessionSummary:      true,
		MemoryLimit:               20,
		SummaryRecentMessageLimit: 2,
		AsyncWorkerPoolSize:       1,
		DebounceWindowSeconds:     intPtr(0),
		SummaryTrigger:            builtin.DefaultMemoryConfig().SummaryTrigger,
		SummaryCache:              builtin.DefaultMemoryConfig().SummaryCache,
		Cleanup:                   builtin.DefaultMemoryConfig().Cleanup,
		Search:                    builtin.DefaultMemoryConfig().Search,
	})
	if err != nil {
		t.Fatalf("NewMemoryManager: %v", err)
	}
	defer manager.Close()

	base := time.Date(2026, 4, 29, 17, 38, 0, 0, time.FixedZone("CST", 8*60*60))
	messages := []*builtin.ConversationMessage{
		{ID: "01", SessionID: "session-1", UserID: "user-1", Role: "user", Content: "第一轮问题", CreatedAt: base},
		{ID: "02", SessionID: "session-1", UserID: "user-1", Role: "assistant", Content: "第一轮回答", CreatedAt: base.Add(time.Second)},
		{ID: "03", SessionID: "session-1", UserID: "user-1", Role: "user", Content: "检查 project-19313590114 下 subject-alpha 的 member-beta 是否有 group-delta", CreatedAt: base.Add(2 * time.Second)},
		{ID: "04", SessionID: "session-1", UserID: "user-1", Role: "assistant", Content: "未找到对应记录", CreatedAt: base.Add(3 * time.Second)},
	}
	for _, msg := range messages {
		if err := manager.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage(%s): %v", msg.ID, err)
		}
	}
	if err := store.SaveSessionSummary(ctx, &builtin.SessionSummary{
		SessionID:               "session-1",
		UserID:                  "user-1",
		Summary:                 "已摘要到最新一轮。",
		LastSummarizedMessageID: "04",
		LastSummarizedMessageAt: messages[3].CreatedAt,
		CreatedAt:               base,
		UpdatedAt:               base.Add(4 * time.Second),
	}); err != nil {
		t.Fatalf("SaveSessionSummary: %v", err)
	}

	provider := &builtinProvider{MemoryManager: manager}
	result, err := provider.Retrieve(ctx, &RetrieveRequest{
		UserID:    "user-1",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	if len(result.ContextMessages) != 1 || !strings.Contains(agmsg.Text(result.ContextMessages[0]), "已摘要到最新一轮") {
		t.Fatalf("summary was not injected as context: %#v", result.ContextMessages)
	}
	if len(result.HistoryMessages) != 2 {
		t.Fatalf("len(HistoryMessages) = %d, want 2: %#v", len(result.HistoryMessages), result.HistoryMessages)
	}
	if !strings.Contains(agmsg.Text(result.HistoryMessages[0]), "project-19313590114") {
		t.Fatalf("recent user message missing: %q", agmsg.Text(result.HistoryMessages[0]))
	}
	if !strings.Contains(agmsg.Text(result.HistoryMessages[1]), "未找到对应记录") {
		t.Fatalf("recent assistant message missing: %q", agmsg.Text(result.HistoryMessages[1]))
	}
}

func intPtr(v int) *int {
	return &v
}

// flakyStorage 让特定子查询失败，验证检索降级可观测。
type flakyStorage struct {
	*storage.MemoryStore
	failUserMemory bool
}

func (f *flakyStorage) GetUserMemory(ctx context.Context, userID string) (*builtin.UserMemory, error) {
	if f.failUserMemory {
		return nil, errors.New("backend down")
	}
	return f.MemoryStore.GetUserMemory(ctx, userID)
}

func newBudgetManager(t *testing.T, store builtin.MemoryStorage, mutate func(*builtin.MemoryConfig)) (*builtin.MemoryManager, *builtinProvider) {
	t.Helper()
	cfg := &builtin.MemoryConfig{
		EnableUserMemories:    true,
		EnableSessionSummary:  false,
		MemoryLimit:           20,
		AsyncWorkerPoolSize:   1,
		DebounceWindowSeconds: intPtr(0),
		SummaryTrigger:        builtin.DefaultMemoryConfig().SummaryTrigger,
		SummaryCache:          builtin.DefaultMemoryConfig().SummaryCache,
		Cleanup:               builtin.DefaultMemoryConfig().Cleanup,
		Search:                builtin.DefaultMemoryConfig().Search,
	}
	if mutate != nil {
		mutate(cfg)
	}
	manager, err := builtin.NewMemoryManager(nil, store, cfg)
	if err != nil {
		t.Fatalf("NewMemoryManager: %v", err)
	}
	return manager, &builtinProvider{MemoryManager: manager}
}

// 长期记忆超过 MaxMemoryRunes 时按 rune 边界裁剪，并记录 warning。
func TestRetrieveBudgetTruncatesUserMemory(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, provider := newBudgetManager(t, store, func(cfg *builtin.MemoryConfig) {
		cfg.ContextBudget.MaxMemoryRunes = 50
	})
	defer manager.Close()

	longMemory := strings.Repeat("忆", 300)
	if err := store.UpsertUserMemory(ctx, &builtin.UserMemory{UserID: "u1", Memory: longMemory}); err != nil {
		t.Fatal(err)
	}

	result, err := provider.Retrieve(ctx, &RetrieveRequest{UserID: "u1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.ContextMessages) != 1 {
		t.Fatalf("context messages = %d, want 1", len(result.ContextMessages))
	}
	text := agmsg.Text(result.ContextMessages[0])
	// 块包含 <user_memory> 包装标签，内容部分必须不超过预算
	inner := strings.TrimPrefix(strings.TrimSuffix(text, "\n</user_memory>"), "<user_memory>\n")
	if len([]rune(inner)) > 50 {
		t.Fatalf("memory block not truncated: %d runes", len([]rune(inner)))
	}
	// rune 边界裁剪：结果必须是合法 UTF-8 且不含半个字符
	if !utf8.ValidString(text) {
		t.Fatalf("truncated text is not valid UTF-8")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected truncation warning")
	}
	if result.Metadata["truncated"] != true {
		t.Fatal("expected truncated metadata")
	}
}

// 总预算 MaxRunes 不足时，先丢弃优先级最低的长期记忆，再丢弃历史。
func TestRetrieveBudgetDropsLowestPriorityFirst(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, provider := newBudgetManager(t, store, func(cfg *builtin.MemoryConfig) {
		cfg.ContextBudget.MaxRunes = 60
	})
	defer manager.Close()

	if err := store.UpsertUserMemory(ctx, &builtin.UserMemory{UserID: "u1", Memory: strings.Repeat("长", 200)}); err != nil {
		t.Fatal(err)
	}
	// 历史消息文本较短，应该被保留
	for i := 0; i < 3; i++ {
		if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
			SessionID: "s1", UserID: "u1", Role: "user", Content: fmt.Sprintf("第%d条", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := provider.Retrieve(ctx, &RetrieveRequest{UserID: "u1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	for _, msg := range result.ContextMessages {
		if strings.Contains(agmsg.Text(msg), "user_memory") {
			t.Fatal("low-priority user_memory should be dropped under tight budget")
		}
	}
	if len(result.HistoryMessages) == 0 {
		t.Fatal("recent history should be kept under budget")
	}
	if result.Metadata["truncated"] != true {
		t.Fatal("expected truncated metadata")
	}
}

// MaxHistoryItems 限制注入的历史条数，保留最新消息。
func TestRetrieveBudgetCapsHistoryItems(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, provider := newBudgetManager(t, store, func(cfg *builtin.MemoryConfig) {
		cfg.ContextBudget.MaxHistoryItems = 2
	})
	defer manager.Close()

	for i := 0; i < 5; i++ {
		if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
			SessionID: "s1", UserID: "u1", Role: "user", Content: fmt.Sprintf("消息%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := provider.Retrieve(ctx, &RetrieveRequest{UserID: "u1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(result.HistoryMessages) != 2 {
		t.Fatalf("history items = %d, want 2", len(result.HistoryMessages))
	}
	if !strings.Contains(agmsg.Text(result.HistoryMessages[1]), "消息4") {
		t.Fatalf("latest history message must be kept: %v", agmsg.Text(result.HistoryMessages[1]))
	}
}

// 检索子步骤失败不中断整体检索，但写入 Warnings 供调用方观测。
func TestRetrieveRecordsSubQueryFailures(t *testing.T) {
	ctx := context.Background()
	store := &flakyStorage{MemoryStore: storage.NewMemoryStore(), failUserMemory: true}
	manager, provider := newBudgetManager(t, store, nil)
	defer manager.Close()

	result, err := provider.Retrieve(ctx, &RetrieveRequest{UserID: "u1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("Retrieve should be best-effort: %v", err)
	}
	found := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "get_user_memory_failed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected get_user_memory_failed warning, got %v", result.Warnings)
	}
}

// builtinProvider 实现 AsyncMemorizer：写入经由 MemoryManager 的受管工作池，
// Flush 后落库；关闭后提交返回 ErrMemoryClosed。
func TestBuiltinProviderEnqueueMemorize(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := builtin.NewMemoryManager(nil, store, &builtin.MemoryConfig{
		EnableUserMemories:    false,
		EnableSessionSummary:  false,
		AsyncWorkerPoolSize:   2,
		DebounceWindowSeconds: intPtr(0),
		SummaryTrigger:        builtin.DefaultMemoryConfig().SummaryTrigger,
		SummaryCache:          builtin.DefaultMemoryConfig().SummaryCache,
		Cleanup:               builtin.DefaultMemoryConfig().Cleanup,
		Search:                builtin.DefaultMemoryConfig().Search,
	})
	if err != nil {
		t.Fatalf("NewMemoryManager: %v", err)
	}
	provider := &builtinProvider{MemoryManager: manager}

	req := &MemorizeRequest{
		UserID:    "user-1",
		SessionID: "session-1",
		Messages: []*schema.AgenticMessage{
			schema.UserAgenticMessage("第一个问题"),
			agmsg.AssistantMessage("第一个回答"),
		},
	}
	if err := provider.EnqueueMemorize(ctx, req); err != nil {
		t.Fatalf("EnqueueMemorize: %v", err)
	}

	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := manager.Flush(flushCtx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	messages, err := manager.GetMessages(ctx, "session-1", "user-1", 0)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("stored messages = %d, want 2", len(messages))
	}

	// 关闭后提交被拒绝且可识别
	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err = provider.EnqueueMemorize(ctx, req)
	if !errors.Is(err, builtin.ErrMemoryClosed) {
		t.Fatalf("EnqueueMemorize after Close = %v, want ErrMemoryClosed", err)
	}
}

func TestRetrieveBudgetIncludesTimestamp(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, provider := newBudgetManager(t, store, func(cfg *builtin.MemoryConfig) { cfg.ContextBudget.MaxRunes = 20 })
	defer manager.Close()
	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{SessionID: "s1", UserID: "u1", Role: "user", Content: strings.Repeat("长", 100)}); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Retrieve(ctx, &RetrieveRequest{SessionID: "s1", UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	total := agenticMessagesRunes(result.ContextMessages) + agenticMessagesRunes(result.HistoryMessages)
	if total > 20 {
		t.Fatalf("final injected text = %d runes, budget 20", total)
	}
}
