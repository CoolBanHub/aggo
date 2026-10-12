package builtin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type debounceAnalysisStorage struct{ MemoryStorage }

func (*debounceAnalysisStorage) AutoMigrate() error { return nil }
func (*debounceAnalysisStorage) Close() error       { return nil }
func (*debounceAnalysisStorage) GetUserMemory(context.Context, string) (*UserMemory, error) {
	return nil, nil
}
func (*debounceAnalysisStorage) GetMessages(context.Context, string, string, int) ([]*ConversationMessage, error) {
	return []*ConversationMessage{{Role: "user", Content: "question"}, {Role: "assistant", Content: "answer"}}, nil
}

type debounceAnalysisModel struct {
	calls   atomic.Int64
	active  atomic.Int64
	peak    atomic.Int64
	entered chan struct{}
	release <-chan struct{}
}

func (*debounceAnalysisModel) Generate(context.Context, []*schema.AgenticMessage, ...model.Option) (*schema.AgenticMessage, error) {
	return agmsg.AssistantMessage(`{"op":"noop"}`), nil
}

func (m *debounceAnalysisModel) Stream(ctx context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	m.calls.Add(1)
	active := m.active.Add(1)
	for peak := m.peak.Load(); active > peak; peak = m.peak.Load() {
		if m.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	m.entered <- struct{}{}
	r, w := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer w.Close()
		select {
		case <-m.release:
			m.active.Add(-1)
			w.Send(agmsg.AssistantMessage(`{"op":"noop"}`), nil)
		case <-ctx.Done():
			m.active.Add(-1)
			w.Send(nil, ctx.Err())
		}
	}()
	return r, nil
}

func newDebounceRegressionManager(t *testing.T, window time.Duration, cm *debounceAnalysisModel) *MemoryManager {
	t.Helper()
	cfg := DefaultMemoryConfig()
	cfg.AsyncWorkerPoolSize = 1
	cfg.EnableSessionSummary = false
	mgr, err := NewMemoryManager(cm, &debounceAnalysisStorage{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	mgr.configMu.Lock()
	mgr.debounceWindow = window
	mgr.configMu.Unlock()
	t.Cleanup(func() {
		if err := mgr.Close(); err != nil {
			t.Error(err)
		}
	})
	return mgr
}

func occupyDebounceWorkerAndQueue(t *testing.T, mgr *MemoryManager) func() {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if err := mgr.EnqueueTask("block", "u", "s", func(context.Context) { close(entered); <-release }); err != nil {
		t.Fatal(err)
	}
	<-entered
	for i := 0; i < cap(mgr.taskChannel); i++ {
		if err := mgr.EnqueueTask("queued", "u", "s", func(context.Context) {}); err != nil {
			t.Fatal(err)
		}
	}
	return unblock
}

func TestTimedOutFlushKeepsDebounceAnalysisScheduled(t *testing.T) {
	releaseModel := make(chan struct{})
	close(releaseModel)
	cm := &debounceAnalysisModel{entered: make(chan struct{}, 1), release: releaseModel}
	mgr := newDebounceRegressionManager(t, time.Hour, cm)
	unblock := occupyDebounceWorkerAndQueue(t, mgr)
	mgr.scheduleMemoryTask(context.Background(), "u", "s")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := mgr.Flush(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Flush = %v, want deadline exceeded", err)
	}

	unblock()
	// No second Flush or Shutdown may be needed to revive the accepted analysis.
	select {
	case <-cm.entered:
	case <-time.After(time.Second):
		t.Fatal("debounce analysis stopped after Flush timed out")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := mgr.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if calls := cm.calls.Load(); calls != 1 {
		t.Fatalf("analysis calls = %d, want exactly 1", calls)
	}
}

func TestExpiredDebounceUsesWorkerPoolWhenQueueIsFull(t *testing.T) {
	const windows = 25
	releaseModel := make(chan struct{})
	cm := &debounceAnalysisModel{entered: make(chan struct{}, windows), release: releaseModel}
	mgr := newDebounceRegressionManager(t, time.Millisecond, cm)
	// Register before the worker unblock cleanup so failures can drain safely.
	var once sync.Once
	unblockModel := func() { once.Do(func() { close(releaseModel) }) }
	t.Cleanup(unblockModel)
	unblock := occupyDebounceWorkerAndQueue(t, mgr)
	for i := 0; i < windows; i++ {
		mgr.scheduleMemoryTask(context.Background(), "u", fmt.Sprintf("s%d", i))
	}

	select {
	case <-cm.entered:
		t.Fatal("timer ran analysis while the sole worker was occupied")
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case <-cm.entered:
	case <-time.After(time.Second):
		t.Fatal("expired debounce windows did not resume after capacity became available")
	}
	select {
	case <-cm.entered:
		t.Fatal("debounce analysis exceeded AsyncWorkerPoolSize=1")
	case <-time.After(30 * time.Millisecond):
	}

	unblockModel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if calls := cm.calls.Load(); calls != windows {
		t.Fatalf("analysis calls = %d, want %d", calls, windows)
	}
	if peak := cm.peak.Load(); peak != 1 {
		t.Fatalf("peak concurrent model calls = %d, want 1", peak)
	}
}
