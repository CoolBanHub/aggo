package memory

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeMemoryProvider struct {
	result         *RetrieveResult
	memorizeCalled chan *MemorizeRequest
}

func (p *fakeMemoryProvider) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error) {
	return p.result, nil
}

func (p *fakeMemoryProvider) Memorize(ctx context.Context, req *MemorizeRequest) error {
	if p.memorizeCalled != nil {
		p.memorizeCalled <- req
	}
	return nil
}

func (p *fakeMemoryProvider) Close() error {
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
