package builtin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
)

type lifecycleStorage struct{ MemoryStorage }

func (*lifecycleStorage) AutoMigrate() error { return nil }
func (*lifecycleStorage) Close() error       { return nil }

func TestUpdateConfigCannotRestartCleanupAfterShutdown(t *testing.T) {
	for i := 0; i < 30; i++ {
		cfg := DefaultMemoryConfig()
		mgr, err := NewMemoryManager(nil, &lifecycleStorage{}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				mgr.UpdateConfig(cfg)
			}
		}()
		go func() {
			defer wg.Done()
			if err := mgr.Shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()
		mgr.UpdateConfig(cfg)
		if mgr.cleanupCtx.Err() == nil {
			t.Fatal("cleanup context active after Shutdown")
		}
	}
}

func TestShutdownDeadlineWhileUpdateConfigWaitsForCleanup(t *testing.T) {
	cfg := DefaultMemoryConfig()
	mgr, err := NewMemoryManager(nil, &lifecycleStorage{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Restart cleanup after installing the callback, so the goroutine sees the
	// callback through normal goroutine-start synchronization.
	mgr.cleanupCancel()
	mgr.cleanupWg.Wait()
	entered, release := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		if err := mgr.Close(); err != nil {
			t.Error(err)
		}
	}()
	mgr.CleanupOldMessagesFunc = func(context.Context) error {
		enterOnce.Do(func() { close(entered) })
		<-release // Simulate a callback that cannot finish immediately on cancel.
		return nil
	}
	mgr.cleanupCtx, mgr.cleanupCancel = context.WithCancel(context.Background())
	oldCleanupCtx := mgr.cleanupCtx
	mgr.startPeriodicCleanup()
	mgr.cleanupTicker.Reset(time.Millisecond)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("cleanup callback did not start")
	}
	mgr.cleanupTicker.Stop()

	updated := make(chan struct{})
	go func() {
		mgr.UpdateConfig(cfg)
		close(updated)
	}()
	select {
	case <-oldCleanupCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("UpdateConfig did not cancel cleanup")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- mgr.Shutdown(ctx) }()
	select {
	case err := <-shutdownResult:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown waited for cleanup instead of respecting its deadline")
	}
	if err := mgr.EnqueueTask("new", "u", "s", func(context.Context) {}); !errors.Is(err, ErrMemoryClosed) {
		t.Fatalf("Shutdown should reject new work while cleanup is blocked: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("UpdateConfig did not finish after cleanup returned")
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
	if mgr.cleanupCtx.Err() == nil {
		t.Fatal("UpdateConfig restarted cleanup after shutdown began")
	}
}

func TestFlushDeadlineWithFullQueueAndPendingTimer(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.EnableUserMemories = true
	cfg.EnableSessionSummary = false
	cfg.AsyncWorkerPoolSize = 1
	seconds := 60
	cfg.DebounceWindowSeconds = &seconds
	mgr, err := NewMemoryManager(nil, &lifecycleStorage{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The timer remains pending while both the worker and channel are occupied.
	entered, release := make(chan struct{}), make(chan struct{})
	if err := mgr.EnqueueTask("block", "u", "s", func(context.Context) { close(entered); <-release }); err != nil {
		t.Fatal(err)
	}
	<-entered
	defer func() { close(release); mgr.Close() }()
	for i := 0; i < cap(mgr.taskChannel); i++ {
		if err := mgr.EnqueueTask("queued", "u", "s", func(context.Context) {}); err != nil {
			t.Fatal(err)
		}
	}
	// Use a timer without model work so draining the test doesn't need storage.
	key := memoryTimerKey{userID: "u", sessionID: "s"}
	pending := &pendingMemoryTimer{timer: time.NewTimer(time.Hour)}
	mgr.inFlight.Add(1)
	mgr.memoryTimers.Store(key, pending)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := mgr.Flush(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Flush: %v", err)
	}
	// Replace the counted timer with a harmless custom task for teardown.
	mgr.submitMu.Lock()
	pending.timer.Stop()
	mgr.memoryTimers.Delete(key)
	mgr.inFlight.Add(-1)
	mgr.submitMu.Unlock()
}

type indexingLifecycleStorage struct{ lifecycleStorage }

func (*indexingLifecycleStorage) SaveMessage(context.Context, *ConversationMessage) error { return nil }

type countingLifecycleSearcher struct{ count atomic.Int64 }

func (s *countingLifecycleSearcher) Index(ctx context.Context, msg *builtinsearch.Message) error {
	if msg.ID == "" || msg.CreatedAt.IsZero() {
		return errors.New("missing index metadata")
	}
	s.count.Add(1)
	return nil
}
func (*countingLifecycleSearcher) Search(context.Context, *builtinsearch.SearchQuery) ([]*builtinsearch.SearchHit, error) {
	return nil, nil
}
func (*countingLifecycleSearcher) Reindex(context.Context, string, string) error { return nil }

func TestShutdownDrainsEveryIndexChild(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AsyncWorkerPoolSize = 1
	cfg.Search.AsyncIndex = true
	mgr, err := NewMemoryManager(nil, &indexingLifecycleStorage{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	searcher := &countingLifecycleSearcher{}
	mgr.searcher = searcher
	entered, release := make(chan struct{}), make(chan struct{})
	if err := mgr.EnqueueTask("block", "u", "s", func(context.Context) { close(entered); <-release }); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := mgr.EnqueueTask("memorize", "u", "s", func(ctx context.Context) {
		for _, text := range []string{"question", "answer"} {
			if err := mgr.SaveMessage(ctx, &ConversationMessage{UserID: "u", SessionID: "s", Content: text}); err != nil {
				t.Error(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_ = mgr.Shutdown(ctx)
	cancel()
	close(release)
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
	if got := searcher.count.Load(); got != 2 {
		t.Fatalf("indexed %d messages; both child tasks must survive", got)
	}
}
