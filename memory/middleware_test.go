package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type fakeMemoryProvider struct {
	result         *RetrieveResult
	memorizeCalled chan *MemorizeRequest
	retrieveErr    error
	retrieveCalls  int
	closeCalls     int
	memorizeErr    error
}

func (p *fakeMemoryProvider) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error) {
	p.retrieveCalls++
	return p.result, p.retrieveErr
}

func (p *fakeMemoryProvider) Memorize(ctx context.Context, req *MemorizeRequest) error {
	if p.memorizeCalled != nil {
		select {
		case p.memorizeCalled <- req:
		default:
		}
	}
	return p.memorizeErr
}

func (p *fakeMemoryProvider) Close() error {
	p.closeCalls++
	return nil
}

type captureAgenticModel struct {
	input []*schema.AgenticMessage
}

func (m *captureAgenticModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.input = input
	return agmsg.AssistantMessage("done"), nil
}

func (m *captureAgenticModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
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

func TestMemoryMiddlewareAppendsRuntimeContextToCurrentUserMessage(t *testing.T) {
	ctx := context.Background()
	cm := &captureAgenticModel{}
	provider := &fakeMemoryProvider{result: &RetrieveResult{
		ContextMessages: []*schema.AgenticMessage{
			schema.UserAgenticMessage("<user_memory>\n喜欢脆苹果\n</user_memory>"),
		},
		SystemMessages: []*schema.AgenticMessage{
			schema.SystemAgenticMessage("<legacy_context>\n旧 provider 动态上下文\n</legacy_context>"),
		},
		HistoryMessages: []*schema.AgenticMessage{
			schema.UserAgenticMessage("历史问题"),
		},
	}, memorizeCalled: make(chan *MemorizeRequest, 1)}

	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "test",
		Description: "test",
		Instruction: "稳定系统提示",
		Model:       cm,
		Handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			NewMemoryMiddleware(provider),
		},
	})
	if err != nil {
		t.Fatalf("NewTypedChatModelAgent: %v", err)
	}

	runner := adk.NewTypedRunner[*schema.AgenticMessage](adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent})
	iter := runner.Query(ctx, "当前问题", adk.WithSessionValues(map[string]any{
		"userID":    "user-1",
		"sessionID": "session-1",
	}))
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			t.Fatalf("runner event error: %v", event.Err)
		}
	}

	if len(cm.input) != 3 {
		t.Fatalf("len(model input) = %d, want 3: %#v", len(cm.input), cm.input)
	}
	if cm.input[0].Role != schema.AgenticRoleTypeSystem {
		t.Fatalf("message[0].Role = %s, want system", cm.input[0].Role)
	}
	if cm.input[1].Role != schema.AgenticRoleTypeUser || agmsg.Text(cm.input[1]) != "历史问题" {
		t.Fatalf("message[1] = role %s content %q, want history user", cm.input[1].Role, agmsg.Text(cm.input[1]))
	}
	if cm.input[2].Role != schema.AgenticRoleTypeUser {
		t.Fatalf("message[2].Role = %s, want current user", cm.input[2].Role)
	}

	systemText := agmsg.Text(cm.input[0])
	currentUserText := agmsg.Text(cm.input[2])
	if systemText != "稳定系统提示" {
		t.Fatalf("system prompt was modified: %q", systemText)
	}
	if !strings.HasPrefix(currentUserText, "当前问题\n\n-----\n") {
		t.Fatalf("current user message was not preserved before context: %q", currentUserText)
	}
	if !strings.Contains(currentUserText, "<current_time>") ||
		!strings.Contains(currentUserText, "<user_memory>") ||
		!strings.Contains(currentUserText, "<legacy_context>") {
		t.Fatalf("runtime context was not appended to current user message: %q", currentUserText)
	}

	select {
	case req := <-provider.memorizeCalled:
		if len(req.Messages) < 1 || agmsg.Text(req.Messages[0]) != "当前问题" {
			t.Fatalf("memorized user message = %#v, want original current user", req.Messages)
		}
	case <-time.After(time.Second):
		t.Fatalf("Memorize was not called")
	}
}

func TestMemoryMiddlewareReusesExistingRuntimeContextSection(t *testing.T) {
	ctx := context.Background()
	cm := &captureAgenticModel{}
	provider := &fakeMemoryProvider{result: &RetrieveResult{
		ContextMessages: []*schema.AgenticMessage{
			schema.UserAgenticMessage("<user_memory>\n喜欢脆苹果\n</user_memory>"),
		},
	}}

	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "test",
		Description: "test",
		Instruction: "稳定系统提示",
		Model:       cm,
		Handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			NewMemoryMiddleware(provider),
		},
	})
	if err != nil {
		t.Fatalf("NewTypedChatModelAgent: %v", err)
	}

	currentUser := strings.Join([]string{
		"当前问题",
		"",
		"-----",
		"<current_time>2026-06-23 14:00:00 +08:00</current_time>",
		"",
		"[发言人]",
		"张三",
	}, "\n")
	runner := adk.NewTypedRunner[*schema.AgenticMessage](adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent})
	iter := runner.Run(ctx, []*schema.AgenticMessage{schema.UserAgenticMessage(currentUser)}, adk.WithSessionValues(map[string]any{
		"userID":    "user-1",
		"sessionID": "session-1",
	}))
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			t.Fatalf("runner event error: %v", event.Err)
		}
	}

	if len(cm.input) != 2 {
		t.Fatalf("len(model input) = %d, want 2: %#v", len(cm.input), cm.input)
	}
	currentUserText := agmsg.Text(cm.input[1])
	if strings.Count(currentUserText, "-----") != 1 {
		t.Fatalf("expected one runtime context divider: %q", currentUserText)
	}
	if strings.Count(currentUserText, "<current_time>") != 1 {
		t.Fatalf("expected current_time to be reused, not duplicated: %q", currentUserText)
	}
	if !strings.Contains(currentUserText, "[发言人]\n张三\n<user_memory>") {
		t.Fatalf("expected memory appended inside existing runtime context section: %q", currentUserText)
	}
}

// asyncMemorizeProvider 记录 EnqueueMemorize 调用，验证受管异步写入路径。
type asyncMemorizeProvider struct {
	*fakeMemoryProvider
	enqueued chan *MemorizeRequest
	closed   bool
}

func newAsyncMemorizeProvider(result *RetrieveResult) *asyncMemorizeProvider {
	return &asyncMemorizeProvider{
		fakeMemoryProvider: &fakeMemoryProvider{result: result},
		enqueued:           make(chan *MemorizeRequest, 4),
	}
}

func (p *asyncMemorizeProvider) EnqueueMemorize(ctx context.Context, req *MemorizeRequest) error {
	if p.closed {
		return fmt.Errorf("provider closed")
	}
	p.enqueued <- req
	return nil
}

func runOnce(t *testing.T, handler adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], session map[string]any) error {
	t.Helper()
	ctx := context.Background()
	cm := &captureAgenticModel{}
	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        "test",
		Description: "test",
		Instruction: "稳定系统提示",
		Model:       cm,
		Handlers:    []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{handler},
	})
	if err != nil {
		t.Fatalf("NewTypedChatModelAgent: %v", err)
	}
	runner := adk.NewTypedRunner[*schema.AgenticMessage](adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent})
	var opts []adk.AgentRunOption
	if session != nil {
		opts = append(opts, adk.WithSessionValues(session))
	}
	iter := runner.Query(ctx, "当前问题", opts...)
	var runErr error
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			runErr = event.Err
		}
	}
	return runErr
}

// 自定义身份键：应用可以用 owner/conversation 等业务键接入记忆。
func TestMemoryMiddlewareCustomIdentityKeys(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		UserIDKey:       "owner",
		SessionIDKey:    "conversation",
		MissingIdentity: MissingIdentityError,
	})

	if err := runOnce(t, mw, map[string]any{"owner": "u-9", "conversation": "c-9"}); err != nil {
		t.Fatalf("run with custom keys: %v", err)
	}

	select {
	case req := <-provider.enqueued:
		if req.UserID != "u-9" || req.SessionID != "c-9" {
			t.Fatalf("memorized as (%s,%s), want (u-9,c-9)", req.UserID, req.SessionID)
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
}

// 身份缺失时按 MissingIdentityError 策略中断请求，而不是静默跳过。
func TestMemoryMiddlewareMissingIdentityError(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		MissingIdentity: MissingIdentityError,
	})

	err := runOnce(t, mw, nil)
	if err == nil {
		t.Fatal("expected error when identity is missing")
	}
	if !strings.Contains(err.Error(), "missing identity") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Log 策略：不中断，只是记录日志
	mw = NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		MissingIdentity: MissingIdentityLog,
	})
	if err := runOnce(t, mw, nil); err != nil {
		t.Fatalf("Log policy must not abort the run: %v", err)
	}
	select {
	case req := <-provider.enqueued:
		t.Fatalf("memorize must be skipped when identity is missing, got %#v", req)
	default:
	}
}

// IdentityResolver 覆盖默认解析逻辑（如按 actor+conversation 组合归属）。
func TestMemoryMiddlewareIdentityResolver(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		IdentityResolver: func(ctx context.Context) (string, string, bool) {
			return "actor-1", "conv-1", true
		},
	})

	if err := runOnce(t, mw, map[string]any{"anything": "else"}); err != nil {
		t.Fatalf("run with resolver: %v", err)
	}
	select {
	case req := <-provider.enqueued:
		if req.UserID != "actor-1" || req.SessionID != "conv-1" {
			t.Fatalf("memorized as (%s,%s), want (actor-1,conv-1)", req.UserID, req.SessionID)
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
}

// Normalizer 在写入前生效：清洗后的内容进入 Memorize 请求，
// 且原会话消息不被修改。
func TestMemoryMiddlewareAppliesNormalizers(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		Normalizers: []MemorizeNormalizer{
			func(ctx context.Context, req *MemorizeRequest) (*MemorizeRequest, error) {
				cloned := *req
				cloned.Messages = append([]*schema.AgenticMessage(nil), req.Messages...)
				for i, msg := range cloned.Messages {
					if msg.Role == schema.AgenticRoleTypeUser {
						cleaned := agmsg.Clone(msg)
						agmsg.AppendUserText(cleaned, "\n[发言人] 张三")
						cloned.Messages[i] = cleaned
					}
				}
				return &cloned, nil
			},
			func(ctx context.Context, req *MemorizeRequest) (*MemorizeRequest, error) {
				cloned := *req
				cloned.Messages = append([]*schema.AgenticMessage(nil), req.Messages...)
				return &cloned, nil
			},
		},
	})

	if err := runOnce(t, mw, map[string]any{"userID": "user-1", "sessionID": "session-1"}); err != nil {
		t.Fatalf("run with normalizers: %v", err)
	}

	select {
	case req := <-provider.enqueued:
		if len(req.Messages) == 0 {
			t.Fatal("no messages memorized")
		}
		if !strings.Contains(agmsg.Text(req.Messages[0]), "[发言人] 张三") {
			t.Fatalf("normalizer output missing: %q", agmsg.Text(req.Messages[0]))
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
}

// provider 实现 AsyncMemorizer 时走受管异步路径，
// 不再调用同步 Memorize（由 provider 的 EnqueueMemorize 负责实际写入）。
func TestMemoryMiddlewarePrefersAsyncMemorizer(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddleware(provider)

	if err := runOnce(t, mw, map[string]any{"userID": "user-1", "sessionID": "session-1"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	select {
	case req := <-provider.enqueued:
		if len(req.Messages) < 2 {
			t.Fatalf("expected user+assistant messages, got %#v", req.Messages)
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
	// 同步 Memorize 不应被 middleware 直接调用
	select {
	case req := <-provider.memorizeCalled:
		_ = req
		t.Fatal("provider.Memorize must not be called directly when AsyncMemorizer is implemented")
	default:
	}
}

func TestMemoryMiddlewareRetrieveFailureOncePerRun(t *testing.T) {
	provider := &fakeMemoryProvider{retrieveErr: errors.New("backend unavailable")}
	model := &sequentialAgenticModelForMemory{responses: []*schema.AgenticMessage{
		memoryToolCall("echo", "call-1"),
		agmsg.AssistantMessage("done"),
	}}
	echoTool := &echoToolForMemory{}
	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](context.Background(), &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name: "test", Description: "test", Model: model,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{echoTool}}},
		Handlers:    []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{NewMemoryMiddleware(provider)},
	})
	if err != nil {
		t.Fatal(err)
	}
	iter := adk.NewTypedRunner[*schema.AgenticMessage](adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent}).Query(context.Background(), "question", adk.WithSessionValues(map[string]any{"userID": "u", "sessionID": "s"}))
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			t.Fatal(event.Err)
		}
	}
	if provider.retrieveCalls != 1 {
		t.Fatalf("retrieve calls = %d, want 1", provider.retrieveCalls)
	}
}

func TestMemoryMiddlewareRetrieveFailurePolicyReturn(t *testing.T) {
	provider := &fakeMemoryProvider{retrieveErr: errors.New("backend unavailable")}
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		RetrieveErrorPolicy: RetrieveErrorReturn,
	})
	err := runOnce(t, mw, map[string]any{"userID": "u", "sessionID": "s"})
	if err == nil || !strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("run error = %v, want backend failure", err)
	}
	if provider.retrieveCalls != 1 {
		t.Fatalf("retrieve calls = %d, want 1", provider.retrieveCalls)
	}
}

type scopeAwareAsyncProvider struct {
	*asyncMemorizeProvider
}

func (*scopeAwareAsyncProvider) ValidateScope(scope MemoryScope) error {
	if scope.ProjectID == "" || scope.UserID != "" || scope.SessionID != "" {
		return fmt.Errorf("expected project-only scope")
	}
	return nil
}

func TestMemoryMiddlewareSupportsValidatedProjectScope(t *testing.T) {
	provider := &scopeAwareAsyncProvider{asyncMemorizeProvider: newAsyncMemorizeProvider(nil)}
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		ScopeResolver: func(context.Context) (MemoryScope, bool) {
			return MemoryScope{ProjectID: "aggo"}, true
		},
	})
	if err := runOnce(t, mw, nil); err != nil {
		t.Fatalf("run with project scope: %v", err)
	}
	select {
	case req := <-provider.enqueued:
		if req.Scope.ProjectID != "aggo" || req.UserID != "" || req.SessionID != "" {
			t.Fatalf("memorize scope = %#v, want project-only scope", req.Scope)
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
}

func TestMemoryMiddlewareRejectsUnvalidatedProjectScope(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		ScopeResolver: func(context.Context) (MemoryScope, bool) {
			return MemoryScope{ProjectID: "aggo"}, true
		},
	})
	err := runOnce(t, mw, nil)
	if err == nil || !strings.Contains(err.Error(), "ScopeValidator") {
		t.Fatalf("run error = %v, want unvalidated scope error", err)
	}
}

func TestMemoryMiddlewareAllowsLegacyScopeFromScopeResolver(t *testing.T) {
	provider := newAsyncMemorizeProvider(nil)
	mw := NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{
		ScopeResolver: func(context.Context) (MemoryScope, bool) {
			return MemoryScope{UserID: "user-1", SessionID: "session-1"}, true
		},
	})
	if err := runOnce(t, mw, nil); err != nil {
		t.Fatalf("run with user/session scope: %v", err)
	}
	select {
	case req := <-provider.enqueued:
		if req.UserID != "user-1" || req.SessionID != "session-1" {
			t.Fatalf("legacy scope = %#v", req.Scope)
		}
	default:
		t.Fatal("EnqueueMemorize not called")
	}
}

type slowMemorizeProvider struct {
	fakeMemoryProvider
	started chan struct{}
	release chan struct{}
}

func (p *slowMemorizeProvider) Memorize(context.Context, *MemorizeRequest) error {
	close(p.started)
	<-p.release
	return nil
}

func TestMemoryMiddlewareShutdownDrainsFallbackWrites(t *testing.T) {
	provider := &slowMemorizeProvider{started: make(chan struct{}), release: make(chan struct{})}
	mw := NewMemoryMiddleware(provider)
	if err := runOnce(t, mw, map[string]any{"userID": "u", "sessionID": "s"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("memorize did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := mw.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Shutdown error = %v, want deadline exceeded", err)
	}
	close(provider.release)
	if err := mw.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	if provider.closeCalls != 1 {
		t.Fatalf("provider close calls = %d, want 1", provider.closeCalls)
	}
}

type sequentialAgenticModelForMemory struct {
	responses []*schema.AgenticMessage
	idx       int
}

func (m *sequentialAgenticModelForMemory) Generate(context.Context, []*schema.AgenticMessage, ...einomodel.Option) (*schema.AgenticMessage, error) {
	if m.idx >= len(m.responses) {
		return nil, fmt.Errorf("no more responses")
	}
	msg := m.responses[m.idx]
	m.idx++
	return msg, nil
}

func (m *sequentialAgenticModelForMemory) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	r, w := schema.Pipe[*schema.AgenticMessage](1)
	go func() { defer w.Close(); w.Send(msg, nil) }()
	return r, nil
}

func memoryToolCall(name, callID string) *schema.AgenticMessage {
	return &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{Name: name, CallID: callID, Arguments: `{}`})}}
}

type echoToolForMemory struct{}

func (*echoToolForMemory) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "echo", Desc: "echo"}, nil
}

func (*echoToolForMemory) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "ok", nil
}
