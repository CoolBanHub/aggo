package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
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

	if len(result.SystemMessages) != 1 || !strings.Contains(result.SystemMessages[0].Content, "已摘要到最新一轮") {
		t.Fatalf("summary was not injected: %#v", result.SystemMessages)
	}
	if len(result.HistoryMessages) != 2 {
		t.Fatalf("len(HistoryMessages) = %d, want 2: %#v", len(result.HistoryMessages), result.HistoryMessages)
	}
	if !strings.Contains(result.HistoryMessages[0].Content, "project-19313590114") {
		t.Fatalf("recent user message missing: %q", result.HistoryMessages[0].Content)
	}
	if !strings.Contains(result.HistoryMessages[1].Content, "未找到对应记录") {
		t.Fatalf("recent assistant message missing: %q", result.HistoryMessages[1].Content)
	}
}

func intPtr(v int) *int {
	return &v
}
