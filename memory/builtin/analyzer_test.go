package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type captureAgenticModel struct {
	input    []*schema.AgenticMessage
	response string
}

func (m *captureAgenticModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.input = input
	if m.response == "" {
		m.response = `{"op":"noop"}`
	}
	return agmsg.AssistantMessage(m.response), nil
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

func TestParseEventSearchAnalyzerResponse_Noop(t *testing.T) {
	result, err := parseEventSearchAnalyzerResponse(`{"op":"noop"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NeedUpdate {
		t.Fatalf("expected noop, got need_update=true")
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %d", len(result.Events))
	}
}

func TestParseEventSearchAnalyzerResponse_Update(t *testing.T) {
	raw := `{"op":"update","memory":"# 用户记忆\n\n### 核心约定\n- 不要使用emoji","events":[
        {"type":"milestone","date":"2026-04-15","summary":"删除10个主体","keywords":["酷企","1418"]},
        {"type":"event","date":"2026/04/16","summary":"充值2811.32元","keywords":["海豚","377"]},
        {"type":"任务里程碑","date":"bad-date","summary":"无效日期事件"}
    ]}`

	result, err := parseEventSearchAnalyzerResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.NeedUpdate {
		t.Fatalf("expected need_update=true")
	}
	if got := result.Memory; got == "" {
		t.Fatalf("expected non-empty memory")
	}
	if got := len(result.Events); got != 3 {
		t.Fatalf("expected 3 events, got %d", got)
	}

	if result.Events[0].Type != UserMemoryEventTypeMilestone {
		t.Errorf("event[0] type want milestone, got %s", result.Events[0].Type)
	}
	wantDate := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	if !result.Events[0].EventDate.Equal(wantDate) {
		t.Errorf("event[0] date want %v, got %v", wantDate, result.Events[0].EventDate)
	}

	if result.Events[1].Type != UserMemoryEventTypeEvent {
		t.Errorf("event[1] type want event, got %s", result.Events[1].Type)
	}
	if result.Events[1].EventDate.IsZero() {
		t.Errorf("event[1] date should be parsed from 2026/04/16")
	}

	// 第3条：类型用中文 + 无效日期，应该退化为 milestone + 当前时间
	if result.Events[2].Type != UserMemoryEventTypeMilestone {
		t.Errorf("event[2] type want milestone, got %s", result.Events[2].Type)
	}
	if result.Events[2].EventDate.IsZero() {
		t.Errorf("event[2] date should fall back to now")
	}
}

func TestParseLegacyAnalyzerResponse_BackwardsCompat(t *testing.T) {
	raw := `{"op":"update","memory":"# 用户记忆\n\n## 核心约定\n- 不要使用emoji"}`
	result, err := parseLegacyAnalyzerResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.NeedUpdate {
		t.Fatalf("expected need_update=true")
	}
	if result.Memory == "" {
		t.Fatalf("expected memory")
	}
	if len(result.Events) != 0 {
		t.Fatalf("legacy parser should not emit events")
	}
}

func TestAnalyzerPutsDynamicContextInUserMessage(t *testing.T) {
	cm := &captureAgenticModel{response: `{"op":"noop"}`}
	analyzer := NewUserMemoryAnalyzer(cm)
	useEvent := true
	_, err := analyzer.AnalyzeOnce(context.Background(), AnalyzeRequest{
		ExistingMemory: &UserMemory{
			Memory: "# 用户记忆\n\n### 基础信息\n- 喜欢脆苹果",
		},
		RecentEvents: []*UserMemoryEvent{
			{Type: UserMemoryEventTypeEvent, EventDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Summary: "测试事件"},
		},
		HistoryMessages: []*ConversationMessage{
			{Role: "user", Content: "明天提醒我买苹果"},
		},
		UseEventSearch: &useEvent,
	})
	if err != nil {
		t.Fatalf("AnalyzeOnce: %v", err)
	}
	assertDynamicContextOnlyInUser(t, cm.input, "喜欢脆苹果", "测试事件", "明天提醒我买苹果")
}

func TestSummaryPutsDynamicContextInUserMessage(t *testing.T) {
	cm := &captureAgenticModel{response: "摘要结果"}
	generator := NewSessionSummaryGenerator(cm)
	got, err := generator.GenerateSummary(context.Background(), []*ConversationMessage{
		{Role: "user", Content: "我喜欢脆苹果"},
	}, "现有摘要内容")
	if err != nil {
		t.Fatalf("GenerateSummary: %v", err)
	}
	if got != "摘要结果" {
		t.Fatalf("summary = %q, want 摘要结果", got)
	}
	assertDynamicContextOnlyInUser(t, cm.input, "现有摘要内容", "我喜欢脆苹果")
}

func TestIncrementalSummaryPutsDynamicContextInUserMessage(t *testing.T) {
	cm := &captureAgenticModel{response: "新版摘要"}
	generator := NewSessionSummaryGenerator(cm)
	got, err := generator.GenerateIncrementalSummary(context.Background(), []*ConversationMessage{
		{Role: "assistant", Content: "已记录"},
		{Role: "user", Content: "刚完成订单处理"},
	}, "旧摘要内容")
	if err != nil {
		t.Fatalf("GenerateIncrementalSummary: %v", err)
	}
	if got != "新版摘要" {
		t.Fatalf("summary = %q, want 新版摘要", got)
	}
	assertDynamicContextOnlyInUser(t, cm.input, "旧摘要内容", "刚完成订单处理")
}

func assertDynamicContextOnlyInUser(t *testing.T, messages []*schema.AgenticMessage, dynamicNeedles ...string) {
	t.Helper()
	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2: %#v", len(messages), messages)
	}
	if messages[0].Role != schema.AgenticRoleTypeSystem {
		t.Fatalf("message[0].Role = %s, want system", messages[0].Role)
	}
	if messages[1].Role != schema.AgenticRoleTypeUser {
		t.Fatalf("message[1].Role = %s, want user", messages[1].Role)
	}
	systemText := agmsg.Text(messages[0])
	userText := agmsg.Text(messages[1])
	if strings.Contains(systemText, "<current_time>") {
		t.Fatalf("system contains current_time: %q", systemText)
	}
	if !strings.Contains(userText, "<current_time>") {
		t.Fatalf("user missing current_time: %q", userText)
	}
	for _, needle := range dynamicNeedles {
		if strings.Contains(systemText, needle) {
			t.Fatalf("system contains dynamic content %q: %q", needle, systemText)
		}
		if !strings.Contains(userText, needle) {
			t.Fatalf("user missing dynamic content %q: %q", needle, userText)
		}
	}
}
