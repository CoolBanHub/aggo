package storage

import (
	"context"
	"testing"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
)

func TestMemoryStore_UserMemoryEvent_CRUD(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	events := []*builtin.UserMemoryEvent{
		{UserID: "u1", Type: builtin.UserMemoryEventTypeMilestone, EventDate: base, Summary: "酷企1418主体删除完成", Keywords: []string{"酷企", "1418"}},
		{UserID: "u1", Type: builtin.UserMemoryEventTypeEvent, EventDate: base.Add(24 * time.Hour), Summary: "海豚377充值入账异常", Keywords: []string{"海豚", "377"}},
		{UserID: "u1", Type: builtin.UserMemoryEventTypeEvent, EventDate: base.Add(48 * time.Hour), Summary: "西皮士1473扩容完成", Keywords: []string{"西皮士", "1473"}},
	}
	for _, evt := range events {
		if err := store.SaveUserMemoryEvent(ctx, evt); err != nil {
			t.Fatalf("save event err: %v", err)
		}
		if evt.ID == "" {
			t.Fatalf("ID should be assigned")
		}
	}

	recent, err := store.ListRecentUserMemoryEvents(ctx, "u1", 0)
	if err != nil {
		t.Fatalf("list err: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("want 3 events, got %d", len(recent))
	}
	if recent[0].Summary != "西皮士1473扩容完成" {
		t.Fatalf("expected most recent first, got %s", recent[0].Summary)
	}

	hits, err := store.SearchUserMemoryEvents(ctx, &builtin.UserMemoryEventQuery{
		UserID:   "u1",
		Keywords: []string{"酷企"},
	})
	if err != nil {
		t.Fatalf("search err: %v", err)
	}
	if len(hits) != 1 || hits[0].Summary != "酷企1418主体删除完成" {
		t.Fatalf("unexpected search result: %+v", hits)
	}

	// 关键词命中 keywords 字段
	hits, err = store.SearchUserMemoryEvents(ctx, &builtin.UserMemoryEventQuery{
		UserID:   "u1",
		Keywords: []string{"1473"},
	})
	if err != nil {
		t.Fatalf("search err: %v", err)
	}
	if len(hits) != 1 || hits[0].Summary != "西皮士1473扩容完成" {
		t.Fatalf("unexpected keyword search: %+v", hits)
	}

	// 类型过滤
	hits, err = store.SearchUserMemoryEvents(ctx, &builtin.UserMemoryEventQuery{
		UserID: "u1",
		Type:   builtin.UserMemoryEventTypeMilestone,
	})
	if err != nil {
		t.Fatalf("search err: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("type filter expected 1, got %d", len(hits))
	}

	// 时间窗
	since := base.Add(12 * time.Hour)
	until := base.Add(36 * time.Hour)
	hits, err = store.SearchUserMemoryEvents(ctx, &builtin.UserMemoryEventQuery{
		UserID: "u1",
		Since:  &since,
		Until:  &until,
	})
	if err != nil {
		t.Fatalf("search err: %v", err)
	}
	if len(hits) != 1 || hits[0].Summary != "海豚377充值入账异常" {
		t.Fatalf("time window filter unexpected: %+v", hits)
	}

	// 删除单条 + 全清
	if err := store.DeleteUserMemoryEvent(ctx, "u1", recent[0].ID); err != nil {
		t.Fatalf("delete err: %v", err)
	}
	left, _ := store.ListRecentUserMemoryEvents(ctx, "u1", 0)
	if len(left) != 2 {
		t.Fatalf("after delete one, want 2 left, got %d", len(left))
	}
	if err := store.ClearUserMemoryEvents(ctx, "u1"); err != nil {
		t.Fatalf("clear err: %v", err)
	}
	left, _ = store.ListRecentUserMemoryEvents(ctx, "u1", 0)
	if len(left) != 0 {
		t.Fatalf("after clear, want 0, got %d", len(left))
	}
}
