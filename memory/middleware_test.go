package memory

import (
	"context"
	"io"
	"strings"
	"testing"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeMemoryProvider struct {
	result *RetrieveResult
}

func (p *fakeMemoryProvider) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResult, error) {
	return p.result, nil
}

func (p *fakeMemoryProvider) Memorize(ctx context.Context, req *MemorizeRequest) error {
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

func TestMemoryMiddlewareInjectsDynamicContextAsUserMessage(t *testing.T) {
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

	if len(cm.input) != 4 {
		t.Fatalf("len(model input) = %d, want 4: %#v", len(cm.input), cm.input)
	}
	if cm.input[0].Role != schema.AgenticRoleTypeSystem {
		t.Fatalf("message[0].Role = %s, want system", cm.input[0].Role)
	}
	if cm.input[1].Role != schema.AgenticRoleTypeUser {
		t.Fatalf("message[1].Role = %s, want user", cm.input[1].Role)
	}
	if cm.input[2].Role != schema.AgenticRoleTypeUser || agmsg.Text(cm.input[2]) != "历史问题" {
		t.Fatalf("message[2] = role %s content %q, want history user", cm.input[2].Role, agmsg.Text(cm.input[2]))
	}
	if cm.input[3].Role != schema.AgenticRoleTypeUser || agmsg.Text(cm.input[3]) != "当前问题" {
		t.Fatalf("message[3] = role %s content %q, want current user", cm.input[3].Role, agmsg.Text(cm.input[3]))
	}

	systemText := agmsg.Text(cm.input[0])
	contextText := agmsg.Text(cm.input[1])
	if systemText != "稳定系统提示" {
		t.Fatalf("system prompt was modified: %q", systemText)
	}
	if !strings.Contains(contextText, "<user_memory>") || !strings.Contains(contextText, "<legacy_context>") {
		t.Fatalf("dynamic context was not merged into user message: %q", contextText)
	}
}
