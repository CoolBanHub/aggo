package builtin_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// noopModel 返回合法的分析器结果（op=noop），使记忆分析任务能完整跑完。
type noopModel struct{}

func (m *noopModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	return agmsg.AssistantMessage(`{"op":"noop"}`), nil
}

func (m *noopModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	r, w := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer w.Close()
		w.Send(msg, nil)
	}()
	return r, nil
}

func intPtr(v int) *int { return &v }

func newLifecycleManager(t *testing.T, mutate func(*builtin.MemoryConfig)) *builtin.MemoryManager {
	t.Helper()
	cfg := &builtin.MemoryConfig{
		EnableUserMemories:   false,
		EnableSessionSummary: false,
		AsyncWorkerPoolSize:  2,
		DebounceWindowSeconds: intPtr(0),
		SummaryTrigger:       builtin.DefaultMemoryConfig().SummaryTrigger,
		SummaryCache:         builtin.DefaultMemoryConfig().SummaryCache,
		Cleanup:              builtin.DefaultMemoryConfig().Cleanup,
		Search:               builtin.DefaultMemoryConfig().Search,
	}
	if mutate != nil {
		mutate(cfg)
	}
	mgr, err := builtin.NewMemoryManager(&noopModel{}, storage.NewMemoryStore(), cfg)
	if err != nil {
		t.Fatalf("NewMemoryManager: %v", err)
	}
	return mgr
}

// 并发 Close 与重复 Close 都安全且幂等。
func TestCloseIsIdempotentAndConcurrentSafe(t *testing.T) {
	mgr := newLifecycleManager(t, nil)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = mgr.Close()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Close %d: %v", i, err)
		}
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// 关闭后提交任务返回 ErrMemoryClosed，不会向已关闭的通道发送。
func TestEnqueueTaskAfterClose(t *testing.T) {
	mgr := newLifecycleManager(t, nil)
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}

	err := mgr.EnqueueTask("probe", "u", "s", func(ctx context.Context) {})
	if !errors.Is(err, builtin.ErrMemoryClosed) {
		t.Fatalf("EnqueueTask after Close = %v, want ErrMemoryClosed", err)
	}
}

// Flush 等待队列中的任务和正在执行的任务全部结束。
func TestFlushWaitsForQueuedTasks(t *testing.T) {
	mgr := newLifecycleManager(t, nil)

	gate := make(chan struct{})
	var ran int64
	for i := 0; i < 4; i++ {
		if err := mgr.EnqueueTask("probe", "u", "s", func(ctx context.Context) {
			<-gate
			atomic.AddInt64(&ran, 1)
		}); err != nil {
			t.Fatalf("EnqueueTask: %v", err)
		}
	}

	flushDone := make(chan error, 1)
	go func() { flushDone <- mgr.Flush(context.Background()) }()

	select {
	case err := <-flushDone:
		t.Fatalf("Flush returned while tasks are blocked: %v", err)
	case <-time.After(80 * time.Millisecond):
		// 仍在等待，符合预期
	}

	close(gate)
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("Flush error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Flush did not return after tasks finished")
	}
	if got := atomic.LoadInt64(&ran); got != 4 {
		t.Fatalf("executed tasks = %d, want 4", got)
	}
}

// Close 返回前，已接受的任务必须全部执行完（排空语义）。
func TestCloseDrainsAcceptedTasks(t *testing.T) {
	mgr := newLifecycleManager(t, nil)

	started := make(chan struct{})
	release := make(chan struct{})
	var done int64
	if err := mgr.EnqueueTask("probe", "u", "s", func(ctx context.Context) {
		close(started)
		<-release
		atomic.AddInt64(&done, 1)
	}); err != nil {
		t.Fatalf("EnqueueTask: %v", err)
	}
	<-started

	closed := make(chan error, 1)
	go func() { closed <- mgr.Close() }()

	select {
	case err := <-closed:
		t.Fatalf("Close returned before accepted task drained: %v", err)
	case <-time.After(80 * time.Millisecond):
		// 仍在排空，符合预期
	}

	close(release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after task finished")
	}
	if got := atomic.LoadInt64(&done); got != 1 {
		t.Fatalf("accepted task executed %d times, want 1", got)
	}
}

// 提交与关闭并发时不 panic（配合 go test -race）。
func TestConcurrentSubmitAndClose(t *testing.T) {
	mgr := newLifecycleManager(t, func(cfg *builtin.MemoryConfig) {
		cfg.AsyncWorkerPoolSize = 2
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = mgr.EnqueueTask("spam", "u", "s", func(ctx context.Context) {
					time.Sleep(time.Millisecond)
				})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		if err := mgr.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	wg.Wait()

	stats := mgr.GetTaskQueueStats()
	if stats.RejectedTasks == 0 {
		t.Log("note: no tasks were rejected; close may have happened after all submits")
	}
}

// Flush 会触发未到期的 debounce 定时器并等待其分析完成。
func TestFlushFiresPendingDebounceTimers(t *testing.T) {
	mgr := newLifecycleManager(t, func(cfg *builtin.MemoryConfig) {
		cfg.EnableUserMemories = true
		cfg.DebounceWindowSeconds = intPtr(60) // 窗口内自然不会触发
	})
	defer mgr.Close()

	ctx := context.Background()
	if err := mgr.ProcessUserMessage(ctx, "u1", "s1", "你好", nil); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ProcessAssistantMessage(ctx, "u1", "s1", "你好，有什么可以帮你？"); err != nil {
		t.Fatal(err)
	}

	before := mgr.GetTaskQueueStats().ProcessedTasks
	if err := mgr.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	after := mgr.GetTaskQueueStats().ProcessedTasks
	if after <= before {
		t.Fatalf("Flush did not fire pending debounce task: processed %d -> %d", before, after)
	}
}

// debounce 定时器与 Close 并发（定时器触发时正在关闭）不产生
// send on closed channel 或 panic。
func TestDebounceRaceWithClose(t *testing.T) {
	for i := 0; i < 20; i++ {
		mgr := newLifecycleManager(t, func(cfg *builtin.MemoryConfig) {
			cfg.EnableUserMemories = true
			cfg.DebounceWindowSeconds = intPtr(1) // 短窗口，增大与 Close 撞车的概率
		})
		ctx := context.Background()
		_ = mgr.ProcessUserMessage(ctx, "u1", "s1", "你好", nil)
		_ = mgr.ProcessAssistantMessage(ctx, "u1", "s1", "收到")
		if err := mgr.Close(); err != nil {
			t.Fatalf("round %d Close: %v", i, err)
		}
	}
}
