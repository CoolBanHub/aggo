package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/cloudwego/eino/schema"
)

func strPtr(s string) *string { return &s }

// 修改 Save 传入的对象不会改变存储内容（含 Parts 中的指针字段）。
func TestSaveInputIsCloned(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	url := "https://example.com/a.png"
	msg := &builtin.ConversationMessage{
		SessionID: "s1",
		UserID:    "u1",
		Role:      "user",
		Content:   "看图",
		Parts: []schema.MessageInputPart{
			{Type: schema.ChatMessagePartTypeText, Text: "看图"},
			{Type: schema.ChatMessagePartTypeImageURL, Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{URL: strPtr(url)},
			}},
		},
	}
	if err := store.SaveMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}

	// 保存后修改调用方对象（含嵌套指针）
	msg.Content = "篡改"
	msg.Parts[0].Text = "篡改"
	*msg.Parts[1].Image.URL = "https://evil.example.com/b.png"

	got, err := store.GetMessages(ctx, "s1", "u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("stored messages = %d, want 1", len(got))
	}
	if got[0].Content != "看图" {
		t.Fatalf("stored content mutated: %q", got[0].Content)
	}
	if got[0].Parts[0].Text != "看图" {
		t.Fatalf("stored part mutated: %q", got[0].Parts[0].Text)
	}
	if *got[0].Parts[1].Image.URL != url {
		t.Fatalf("stored image URL mutated: %q", *got[0].Parts[1].Image.URL)
	}
}

// 修改 Get 返回的对象不会改变下一次读取结果。
func TestGetReturnsClone(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
		SessionID: "s1", UserID: "u1", Role: "user", Content: "原始",
	}); err != nil {
		t.Fatal(err)
	}

	first, err := store.GetMessages(ctx, "s1", "u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Content = "篡改"

	second, err := store.GetMessages(ctx, "s1", "u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Content != "原始" {
		t.Fatalf("storage mutated via returned object: %q", second[0].Content)
	}
}

// 用户记忆与事件同样遵循出入库深拷贝语义。
func TestUserMemoryAndEventCloneSemantics(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	mem := &builtin.UserMemory{UserID: "u1", Memory: "喜欢脆苹果"}
	if err := store.UpsertUserMemory(ctx, mem); err != nil {
		t.Fatal(err)
	}
	mem.Memory = "篡改"
	gotMem, err := store.GetUserMemory(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if gotMem.Memory != "喜欢脆苹果" {
		t.Fatalf("user memory mutated after save: %q", gotMem.Memory)
	}
	gotMem.Memory = "再篡改"
	again, _ := store.GetUserMemory(ctx, "u1")
	if again.Memory != "喜欢脆苹果" {
		t.Fatalf("user memory mutated via returned object: %q", again.Memory)
	}

	evt := &builtin.UserMemoryEvent{UserID: "u1", Summary: "完成里程碑", Keywords: []string{"里程碑"}}
	if err := store.SaveUserMemoryEvent(ctx, evt); err != nil {
		t.Fatal(err)
	}
	evt.Keywords[0] = "篡改"
	evt.Summary = "篡改"
	recent, err := store.ListRecentUserMemoryEvents(ctx, "u1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Summary != "完成里程碑" || recent[0].Keywords[0] != "里程碑" {
		t.Fatalf("event mutated after save: %+v", recent[0])
	}
	recent[0].Keywords[0] = "再篡改"
	recent2, _ := store.ListRecentUserMemoryEvents(ctx, "u1", 0)
	if recent2[0].Keywords[0] != "里程碑" {
		t.Fatalf("event mutated via returned object: %+v", recent2[0])
	}
}

// 会话摘要出入库深拷贝。
func TestSessionSummaryCloneSemantics(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	summary := &builtin.SessionSummary{SessionID: "s1", UserID: "u1", Summary: "初始摘要"}
	if err := store.SaveSessionSummary(ctx, summary); err != nil {
		t.Fatal(err)
	}
	summary.Summary = "篡改"
	got, err := store.GetSessionSummary(ctx, "s1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "初始摘要" {
		t.Fatalf("summary mutated after save: %q", got.Summary)
	}

	got.Summary = "更新摘要"
	if err := store.UpdateSessionSummary(ctx, got); err != nil {
		t.Fatal(err)
	}
	got.Summary = "再篡改"
	final, _ := store.GetSessionSummary(ctx, "s1", "u1")
	if final.Summary != "更新摘要" {
		t.Fatalf("summary update not isolated: %q", final.Summary)
	}
}

// 外部修改、读取、摘要更新并发进行时无数据竞争（配合 go test -race）。
func TestConcurrentReadWriteNoRace(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.SaveSessionSummary(ctx, &builtin.SessionSummary{
		SessionID: "s1", UserID: "u1", Summary: "并发摘要",
	}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = store.SaveMessage(ctx, &builtin.ConversationMessage{
					SessionID: "s1", UserID: "u1", Role: "user", Content: "消息",
				})
				msgs, _ := store.GetMessages(ctx, "s1", "u1", 5)
				for _, m := range msgs {
					m.Content = "外部修改"
				}
				if s, _ := store.GetSessionSummary(ctx, "s1", "u1"); s != nil {
					s.Summary = "外部修改"
					_ = store.UpdateSessionSummary(ctx, s)
				}
			}
		}()
	}
	wg.Wait()

	final, _ := store.GetSessionSummary(ctx, "s1", "u1")
	if final == nil || final.Summary != "外部修改" {
		// 最后一次 Update 写入的就是 "外部修改"，但不能包含并发撕裂的内容
		t.Fatalf("unexpected final summary: %+v", final)
	}
}

// 复合键带长度前缀后，CleanupOldMessages 按 userID 精确匹配：
// 清理 "b" 不会误删 "a:b" 这类含冒号用户的消息。
func TestCleanupOldMessagesColonUserIDIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	old := time.Now().Add(-2 * time.Hour)
	newer := time.Now()

	// 用户 "a:b"：一条旧消息 + 一条新消息
	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
		SessionID: "s1", UserID: "a:b", Role: "user", Content: "旧", CreatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
		SessionID: "s1", UserID: "a:b", Role: "user", Content: "新", CreatedAt: newer,
	}); err != nil {
		t.Fatal(err)
	}
	// 用户 "b"：一条旧消息
	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{
		SessionID: "s2", UserID: "b", Role: "user", Content: "旧", CreatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.CleanupOldMessages(ctx, "b", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	msgsAB, err := store.GetMessages(ctx, "s1", "a:b", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgsAB) != 2 {
		t.Fatalf("user a:b must keep both messages, got %d", len(msgsAB))
	}
	msgsB, err := store.GetMessages(ctx, "s2", "b", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgsB) != 0 {
		t.Fatalf("user b's old message should be cleaned, got %d", len(msgsB))
	}
}

// 复合键解析：含冒号的 sessionID/userID 不碰撞，非法键返回 ok=false。
func TestParseCompositeKey(t *testing.T) {
	store := NewMemoryStore()
	cases := []struct{ sessionID, userID string }{
		{"s1", "u1"},
		{"with:colon", "u1"},
		{"s1", "with:colon"},
		{"with:colon", "also:colon"},
		{"", "u1"},
	}
	for _, tc := range cases {
		key := store.generateKey(tc.sessionID, tc.userID)
		gotSession, gotUser, ok := parseCompositeKey(key)
		if !ok || gotSession != tc.sessionID || gotUser != tc.userID {
			t.Fatalf("parseCompositeKey(%q) = (%q,%q,%v), want (%q,%q)", key, gotSession, gotUser, ok, tc.sessionID, tc.userID)
		}
	}
	if _, _, ok := parseCompositeKey("not-a-composite-key"); ok {
		t.Fatal("malformed key should not parse")
	}
}
