package cron

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// recordingModel 记录 Generate 调用，允许测试断言模型是否被任务触发调用。
type recordingModel struct {
	mu    sync.Mutex
	calls int
}

func (m *recordingModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return agmsg.AssistantMessage("提醒：该喝水了"), nil
}

func (m *recordingModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
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

func (m *recordingModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// lifecycleTool 是一个真实可调用的工具，同时绑定 CronService 并暴露
// JobLifecycle，用于验证 NewCronAgent 会在绑定的 service 上安装处理器
// 并合并工具层配置的回调。
type lifecycleTool struct {
	tool.InvokableTool
	service *CronService
	lc      *JobLifecycle
}

func (t *lifecycleTool) CronService() *CronService { return t.service }

func (t *lifecycleTool) CronLifecycle() *JobLifecycle { return t.lc }

func newLifecycleTool(service *CronService, lc *JobLifecycle) tool.BaseTool {
	base, _ := utils.InferTool("lifecycle_probe", "probe", func(ctx context.Context, params struct{}) (any, error) {
		return nil, nil
	})
	return &lifecycleTool{InvokableTool: base, service: service, lc: lc}
}

func triggerJob(t *testing.T, cs *CronService, userID string) *CronJob {
	t.Helper()
	every := int64(3600)
	job, err := cs.AddJob("job", CronSchedule{Kind: "every", EveryMS: &every}, "msg", userID)
	if err != nil {
		t.Fatal(err)
	}
	// 直接驱动内部执行路径，绕过调度器等待
	cs.executeJob(job.ID)
	return job
}

// 四种回调组合下，模型处理都执行、两个回调按固定顺序触发。
func TestNewCronAgentCallbackCombinations(t *testing.T) {
	cases := []struct {
		name              string
		withTriggered     bool
		withProcessed     bool
		wantModelCalls    int
		wantTriggered     bool
		wantProcessed     bool
		disableProcessing bool
	}{
		{name: "no callbacks", wantModelCalls: 1},
		{name: "triggered only", withTriggered: true, wantModelCalls: 1, wantTriggered: true},
		{name: "processed only", withProcessed: true, wantModelCalls: 1, wantProcessed: true},
		{name: "both callbacks", withTriggered: true, withProcessed: true, wantModelCalls: 1, wantTriggered: true, wantProcessed: true},
		{name: "both callbacks, processing disabled", withTriggered: true, withProcessed: true, disableProcessing: true, wantModelCalls: 0, wantTriggered: true, wantProcessed: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			cm := &recordingModel{}

			store, err := NewFileStoreWithError(filepath.Join(t.TempDir(), "jobs.json"))
			if err != nil {
				t.Fatal(err)
			}
			service := NewCronService(store, nil)

			var mu sync.Mutex
			var events []string
			opts := []CronAgentOption{}
			if tc.withTriggered {
				opts = append(opts, WithOnJobTriggered(func(job *CronJob) {
					mu.Lock()
					events = append(events, "triggered")
					mu.Unlock()
				}))
			}
			if tc.withProcessed {
				opts = append(opts, WithOnJobProcessed(func(job *CronJob, response string, err error) {
					mu.Lock()
					events = append(events, fmt.Sprintf("processed(%s,%v)", response, err))
					mu.Unlock()
				}))
			}
			if tc.disableProcessing {
				opts = append(opts, WithDisableJobProcessing())
			}

			result, err := NewCronAgent(ctx, cm, []tool.BaseTool{newLifecycleTool(service, nil)}, opts...)
			if err != nil {
				t.Fatal(err)
			}
			_ = result // Agent 本身不在本测试中运行

			triggerJob(t, service, "userA")

			if got := cm.callCount(); got != tc.wantModelCalls {
				t.Fatalf("model calls = %d, want %d", got, tc.wantModelCalls)
			}

			mu.Lock()
			defer mu.Unlock()
			hasTriggered := contains(events, "triggered")
			hasProcessed := containsPrefix(events, "processed(")
			if hasTriggered != tc.wantTriggered {
				t.Fatalf("triggered called = %v, want %v (events=%v)", hasTriggered, tc.wantTriggered, events)
			}
			if hasProcessed != tc.wantProcessed {
				t.Fatalf("processed called = %v, want %v (events=%v)", hasProcessed, tc.wantProcessed, events)
			}
			// 顺序：triggered 必须发生在 processed 之前
			if tc.wantTriggered && tc.wantProcessed {
				ti, pi := indexOf(events, "triggered"), lastIndexOfPrefix(events, "processed(")
				if ti < 0 || pi < 0 || ti > pi {
					t.Fatalf("callback order wrong: events=%v", events)
				}
			}
		})
	}
}

func contains(list []string, s string) bool       { return indexOf(list, s) >= 0 }
func containsPrefix(list []string, p string) bool { return lastIndexOfPrefix(list, p) >= 0 }

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func lastIndexOfPrefix(list []string, p string) int {
	idx := -1
	for i, v := range list {
		if len(v) >= len(p) && v[:len(p)] == p {
			idx = i
		}
	}
	return idx
}

// 工具层与 Agent 层的回调都要执行，且模型处理不会被工具层回调拦截。
func TestNewCronAgentMergesToolLifecycle(t *testing.T) {
	ctx := context.Background()
	cm := &recordingModel{}

	store, err := NewFileStoreWithError(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewCronService(store, nil)

	var mu sync.Mutex
	var events []string
	toolLC := &JobLifecycle{
		OnTriggered: func(ctx context.Context, job *CronJob) {
			mu.Lock()
			events = append(events, "tool-triggered")
			mu.Unlock()
		},
	}
	agentLC := &JobLifecycle{
		OnTriggered: func(ctx context.Context, job *CronJob) {
			mu.Lock()
			events = append(events, "agent-triggered")
			mu.Unlock()
		},
		OnProcessed: func(ctx context.Context, job *CronJob, response string, err error) {
			mu.Lock()
			events = append(events, "agent-processed")
			mu.Unlock()
		},
	}

	probe := newLifecycleTool(service, toolLC)
	if _, err := NewCronAgent(ctx, cm, []tool.BaseTool{probe},
		WithOnJobTriggeredContext(agentLC.OnTriggered),
		WithOnJobProcessedContext(agentLC.OnProcessed),
	); err != nil {
		t.Fatal(err)
	}

	triggerJob(t, service, "userA")

	if got := cm.callCount(); got != 1 {
		t.Fatalf("model calls = %d, want 1 (tool callbacks must not suppress model processing)", got)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"tool-triggered", "agent-triggered", "agent-processed"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

// Agent 层 JobContextBuilder 构建的 context 会传给任务回调。
func TestNewCronAgentJobContextBuilder(t *testing.T) {
	ctx := context.Background()
	cm := &recordingModel{}

	store, err := NewFileStoreWithError(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewCronService(store, nil)

	type traceKey struct{}
	got := make(chan context.Context, 1)
	if _, err := NewCronAgent(ctx, cm, []tool.BaseTool{newLifecycleTool(service, nil)},
		WithJobContextBuilder(func(ctx context.Context, job *CronJob) context.Context {
			return context.WithValue(ctx, traceKey{}, "trace-1")
		}),
		WithOnJobTriggeredContext(func(jctx context.Context, job *CronJob) {
			got <- jctx
		}),
	); err != nil {
		t.Fatal(err)
	}

	triggerJob(t, service, "userA")

	select {
	case jctx := <-got:
		if v, ok := jctx.Value(traceKey{}).(string); !ok || v != "trace-1" {
			t.Fatalf("job context missing builder value: %v", v)
		}
		if owner, ok := JobOwnerIDFromContext(jctx); !ok || owner != "userA" {
			t.Fatalf("JobOwnerIDFromContext = %q, want userA", owner)
		}
	default:
		t.Fatal("triggered callback not invoked with job context")
	}
}

// NewCronAgent 会覆盖 service 上已存在的旧式处理器，
// 保证模型处理与回调语义由 Agent 统一安装。
func TestNewCronAgentOverridesServiceHandler(t *testing.T) {
	ctx := context.Background()
	cm := &recordingModel{}

	store, err := NewFileStoreWithError(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewCronService(store, nil)
	service.SetOnJob(func(job *CronJob) (string, error) {
		return "manual", nil
	})

	if _, err := NewCronAgent(ctx, cm, []tool.BaseTool{newLifecycleTool(service, nil)}); err != nil {
		t.Fatal(err)
	}

	triggerJob(t, service, "userA")
	if got := cm.callCount(); got != 1 {
		t.Fatalf("model calls = %d, want 1", got)
	}
}
