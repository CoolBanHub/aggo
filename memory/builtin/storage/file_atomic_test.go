package storage

import (
	"context"
	"fmt"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"os"
	"sync"
	"testing"
	"time"
)

func TestFileStoreConcurrentWritesSurviveReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 300)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := fmt.Sprintf("user-%d", i)
			if err := store.UpsertUserMemory(ctx, &builtin.UserMemory{UserID: user, Memory: user}); err != nil {
				t.Error(err)
			}
			if err := store.SaveMessage(ctx, &builtin.ConversationMessage{UserID: "u", SessionID: "s", Content: user}); err != nil {
				t.Error(err)
			}
			if err := store.CleanupOldMessages(ctx, "u", time.Now().Add(-time.Hour)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	disk, err := NewFileStore(dir, 300)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		user := fmt.Sprintf("user-%d", i)
		got, err := disk.GetUserMemory(ctx, user)
		if err != nil || got == nil || got.Memory != user {
			t.Fatalf("lost memory %s: %+v %v", user, got, err)
		}
	}
	messages, err := disk.GetMessages(ctx, "s", "u", 0)
	if err != nil || len(messages) != 25 {
		t.Fatalf("lost messages: %d %v", len(messages), err)
	}
}

func TestFileStoreWriteFailureLeavesMemoryUnchanged(t *testing.T) {
	store, err := NewFileStore(t.TempDir(), 300)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.UpsertUserMemory(ctx, &builtin.UserMemory{UserID: "u", Memory: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.getFilePath("user_memories")+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertUserMemory(ctx, &builtin.UserMemory{UserID: "u", Memory: "new"}); err == nil {
		t.Fatal("expected update failure")
	}
	if err := store.ClearUserMemory(ctx, "u"); err == nil {
		t.Fatal("expected clear failure")
	}
	got, _ := store.GetUserMemory(ctx, "u")
	if got == nil || got.Memory != "old" {
		t.Fatalf("failed write changed memory: %+v", got)
	}
	if err := os.Mkdir(store.getFilePath("messages")+".tmp", 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessage(ctx, &builtin.ConversationMessage{UserID: "u", SessionID: "s", Content: "lost"}); err == nil {
		t.Fatal("expected message failure")
	}
	messages, _ := store.GetMessages(ctx, "s", "u", 0)
	if len(messages) != 0 {
		t.Fatal("failed message visible")
	}
}
