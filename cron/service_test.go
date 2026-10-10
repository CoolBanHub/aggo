package cron

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeStore 内存 Store 实现，可注入各类错误。
type fakeStore struct {
	mu        sync.Mutex
	jobs      map[string]*CronJob
	listErr   error
	getErr    error
	saveErr   error
	deleteErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{jobs: make(map[string]*CronJob)}
}

func (f *fakeStore) List() ([]CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]CronJob, 0, len(f.jobs))
	for _, job := range f.jobs {
		out = append(out, *job)
	}
	return out, nil
}

func (f *fakeStore) Get(id string) (*CronJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	job, ok := f.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	cloned := cloneJob(job)
	return cloned, nil
}

func (f *fakeStore) Save(job *CronJob) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.jobs[job.ID] = cloneJob(job)
	return nil
}

func (f *fakeStore) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.jobs[id]; !ok {
		return fmt.Errorf("job %s not found", id)
	}
	delete(f.jobs, id)
	return nil
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func addJobForUser(t *testing.T, cs *CronService, userID, name string) *CronJob {
	t.Helper()
	every := int64(3600)
	job, err := cs.AddJob(name, CronSchedule{Kind: "every", EveryMS: &every}, "msg-"+name, userID)
	if err != nil {
		t.Fatalf("AddJob(%s): %v", userID, err)
	}
	return job
}

// 用户 A 不能列出用户 B 的任务；无身份时直接报错而不是退化为全量。
func TestListJobsByUserIsolation(t *testing.T) {
	cs := NewCronService(newFakeStore(), nil)

	jobA1 := addJobForUser(t, cs, "userA", "a1")
	addJobForUser(t, cs, "userB", "b1")

	got, err := cs.ListJobsByUser("userA", true)
	if err != nil {
		t.Fatalf("ListJobsByUser: %v", err)
	}
	if len(got) != 1 || got[0].ID != jobA1.ID {
		t.Fatalf("ListJobsByUser(userA) = %+v, want only %s", got, jobA1.ID)
	}

	// 禁用过滤
	if cs.EnableJob(jobA1.ID, false) == nil {
		t.Fatal("EnableJob returned nil")
	}
	enabledOnly, err := cs.ListJobsByUser("userA", false)
	if err != nil {
		t.Fatalf("ListJobsByUser: %v", err)
	}
	if len(enabledOnly) != 0 {
		t.Fatalf("ListJobsByUser(includeDisabled=false) = %d jobs, want 0", len(enabledOnly))
	}

	if _, err := cs.ListJobsByUser("", true); err == nil {
		t.Fatal("ListJobsByUser(\"\") should fail, not return all jobs")
	}
}

// 用户 A 不能删除/启停用户 B 的任务，且“不存在”与“无权”错误一致。
func TestMutationsRejectForeignOwner(t *testing.T) {
	cs := NewCronService(newFakeStore(), nil)

	jobA := addJobForUser(t, cs, "userA", "a1")

	if err := cs.RemoveJobForUser(jobA.ID, "userB"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("RemoveJobForUser(userB) err = %v, want ErrJobNotFound", err)
	}
	if cs.Status()["totalJobs"] != 1 {
		t.Fatal("job of userA must survive denied remove")
	}

	if _, err := cs.EnableJobForUser(jobA.ID, "userB", false); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("EnableJobForUser(userB) err = %v, want ErrJobNotFound", err)
	}

	// 空身份一律拒绝
	if err := cs.RemoveJobForUser(jobA.ID, ""); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("RemoveJobForUser(\"\") err = %v, want ErrJobNotFound", err)
	}
	if _, err := cs.EnableJobForUser(jobA.ID, "", true); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("EnableJobForUser(\"\") err = %v, want ErrJobNotFound", err)
	}

	// 不存在的任务与无权任务返回同一个错误，避免泄漏任务存在性
	if err := cs.RemoveJobForUser("missing", "userA"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("RemoveJobForUser(missing) err = %v, want ErrJobNotFound", err)
	}

	// 本人操作成功
	if err := cs.RemoveJobForUser(jobA.ID, "userA"); err != nil {
		t.Fatalf("RemoveJobForUser(userA): %v", err)
	}
	if cs.Status()["totalJobs"] != 0 {
		t.Fatal("job should be removed")
	}
}

// 持久化失败必须返回错误，不能静默返回“成功”。
func TestMutationsPropagateStoreErrors(t *testing.T) {
	store := newFakeStore()
	cs := NewCronService(store, nil)

	job := addJobForUser(t, cs, "userA", "a1")

	store.saveErr = errors.New("disk full")
	if _, err := cs.EnableJobForUser(job.ID, "userA", false); err == nil {
		t.Fatal("EnableJobForUser should return store save error")
	}

	store.saveErr = nil
	store.deleteErr = errors.New("permission denied")
	if err := cs.RemoveJobForUser(job.ID, "userA"); err == nil {
		t.Fatal("RemoveJobForUser should return store delete error")
	}
}

// captureLocker 记录 Unlock 收到的 context，用于验证解锁不复用锁获取 context。
type captureLocker struct {
	mu          sync.Mutex
	unlockCtxs  []context.Context
	blockSignal chan struct{}
}

func newCaptureLocker() *captureLocker {
	return &captureLocker{blockSignal: make(chan struct{})}
}

func (l *captureLocker) Lock(ctx context.Context, key string) error { return nil }

func (l *captureLocker) Unlock(ctx context.Context, key string) error {
	l.mu.Lock()
	l.unlockCtxs = append(l.unlockCtxs, ctx)
	l.mu.Unlock()
	// 模拟长任务：等待测试放行，确保任务耗时超过锁超时
	<-l.blockSignal
	return nil
}

func (l *captureLocker) unlockContexts() []context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]context.Context(nil), l.unlockCtxs...)
}

// 任务执行超过锁超时后，Unlock 仍必须拿到未取消的 context。
func TestUnlockUsesIndependentContext(t *testing.T) {
	store := newFakeStore()
	cs := NewCronService(store, nil)
	every := int64(3600)
	job, err := cs.AddJob("long", CronSchedule{Kind: "every", EveryMS: &every}, "hello", "userA")
	if err != nil {
		t.Fatal(err)
	}

	locker := newCaptureLocker()
	cs.SetLocker(locker)
	// 锁超时 50ms，任务执行 500ms：旧实现会复用已过期的锁 context 解锁
	cs.SetLockTimeouts(50*time.Millisecond, 5*time.Second)

	handlerDone := make(chan struct{})
	cs.SetOnJobContext(func(ctx context.Context, j *CronJob) (string, error) {
		defer close(handlerDone)
		time.Sleep(500 * time.Millisecond)
		return "ok", nil
	})

	done := make(chan struct{})
	go func() {
		cs.executeJob(job.ID)
		close(done)
	}()

	<-handlerDone
	// 任务已跑完（耗时 > lockTimeout），等待解锁被调用并检查其 context。
	// 用轮询消除“handler 结束 → deferred unlock 执行”之间的窗口。
	deadline := time.Now().Add(2 * time.Second)
	var unlockCtx context.Context
	for {
		cts := locker.unlockContexts()
		if len(cts) > 0 {
			unlockCtx = cts[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("unlock was not called")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := unlockCtx.Err(); err != nil {
		t.Fatalf("unlock context already canceled: %v", err)
	}
	close(locker.blockSignal)
	<-done
}

// 任务回调收到的 context 注入了 OwnerID/JobID，并经过 JobContextBuilder。
func TestExecuteJobInjectsJobContext(t *testing.T) {
	store := newFakeStore()
	cs := NewCronService(store, nil)
	job := addJobForUser(t, cs, "userA", "a1")

	type builderKey struct{}
	captured := make(chan context.Context, 1)
	cs.SetOnJobContext(func(ctx context.Context, j *CronJob) (string, error) {
		captured <- ctx
		return "ok", nil
	})

	cs.SetJobContextBuilder(func(ctx context.Context, j *CronJob) context.Context {
		return context.WithValue(ctx, builderKey{}, "builder-seen")
	})

	cs.executeJob(job.ID)

	select {
	case ctx := <-captured:
		if v, ok := ctx.Value(builderKey{}).(string); !ok || v != "builder-seen" {
			t.Fatalf("builder value missing from job context: %v", v)
		}
		if owner, ok := JobOwnerIDFromContext(ctx); !ok || owner != "userA" {
			t.Fatalf("JobOwnerIDFromContext = %q %v, want userA", owner, ok)
		}
		if id, ok := JobIDFromContext(ctx); !ok || id != job.ID {
			t.Fatalf("JobIDFromContext = %q %v, want %s", id, ok, job.ID)
		}
	default:
		t.Fatal("job handler not called")
	}
}

// StopContext 等待正在执行的任务结束；ctx 先到期则返回超时错误。
func TestStopContextWaitsForRunningJobs(t *testing.T) {
	store := newFakeStore()
	cs := NewCronService(store, nil)
	job := addJobForUser(t, cs, "userA", "a1")

	entered := make(chan struct{})
	release := make(chan struct{})
	cs.SetOnJobContext(func(ctx context.Context, j *CronJob) (string, error) {
		close(entered)
		<-release
		return "ok", nil
	})

	done := make(chan struct{})
	go func() {
		cs.executeJob(job.ID)
		close(done)
	}()
	<-entered

	// 等待窗口很短 → 返回超时错误，任务仍在后台执行
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer shortCancel()
	if err := cs.StopContext(shortCtx); err == nil {
		t.Fatal("StopContext with short ctx should time out while a job is running")
	}

	// 足够长的窗口 → 阻塞直到任务结束
	stopped := make(chan error, 1)
	go func() { stopped <- cs.StopContext(context.Background()) }()

	select {
	case err := <-stopped:
		t.Fatalf("StopContext returned before job finished: %v", err)
	case <-time.After(150 * time.Millisecond):
		// 仍在等待，符合预期
	}

	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("StopContext error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StopContext did not return after job finished")
	}
	<-done
}

// Start 时注册失败的任务应被标记为不可用；标记本身持久化失败时启动报错。
func TestStartMarksUnregisterableJob(t *testing.T) {
	store := newFakeStore()
	bad := &CronJob{
		ID:       "bad1",
		UserID:   "userA",
		Name:     "bad",
		Enabled:  true,
		Schedule: CronSchedule{Kind: "bogus"},
	}
	if err := store.Save(bad); err != nil {
		t.Fatal(err)
	}

	cs := NewCronService(store, nil)
	if err := cs.Start(); err != nil {
		t.Fatalf("Start should succeed when the bad job can be persisted as disabled: %v", err)
	}
	defer cs.Stop()

	got, err := store.Get("bad1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("bad job should be persisted as disabled")
	}

	// 持久化禁用状态失败 → Start 必须报错
	store2 := newFakeStore()
	if err := store2.Save(bad); err != nil {
		t.Fatal(err)
	}
	store2.saveErr = errors.New("disk full")
	cs2 := NewCronService(store2, nil)
	if err := cs2.Start(); err == nil {
		t.Fatal("Start must fail when persisting the disabled state fails")
	}
}

// SetOnJob 之后 SetOnJobContext 覆盖前者，SetOnJob(nil) 清空处理器。
func TestHandlerSwapSemantics(t *testing.T) {
	cs := NewCronService(newFakeStore(), nil)

	var calls []string
	var mu sync.Mutex
	cs.SetOnJob(func(job *CronJob) (string, error) {
		mu.Lock()
		calls = append(calls, "legacy")
		mu.Unlock()
		return "legacy", nil
	})
	cs.SetOnJobContext(func(ctx context.Context, job *CronJob) (string, error) {
		mu.Lock()
		calls = append(calls, "ctx")
		mu.Unlock()
		return "ctx", nil
	})

	job := addJobForUser(t, cs, "userA", "a1")
	cs.executeJob(job.ID)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0] != "ctx" {
		t.Fatalf("handler calls = %v, want single ctx handler", calls)
	}
}
