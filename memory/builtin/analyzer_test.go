package builtin

import (
	"testing"
	"time"
)

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
