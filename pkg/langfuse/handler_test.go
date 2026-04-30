package langfuse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestHandlerRecordsModelToolsToolCallsAndUsage(t *testing.T) {
	var requests []ingestionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/ingestion" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req ingestionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, req)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	handler, flush, err := NewHandler(Config{
		ClientConfig: ClientConfig{
			Host:       server.URL,
			PublicKey:  "pk",
			SecretKey:  "sk",
			FlushAt:    100,
			HTTPClient: server.Client(),
		},
		Clock: fixedClock(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	ctx := SetTrace(context.Background(), WithID("trace-1"), WithName("test-trace"))
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{Component: components.ComponentOfChatModel, Type: "OpenAI", Name: "chat"}, &model.CallbackInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
		Tools: []*schema.ToolInfo{{
			Name: "search",
			Desc: "Search docs",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {Type: schema.String, Desc: "query", Required: true},
			}),
		}},
		Config: &model.Config{Model: "gpt-test", MaxTokens: 128, Temperature: 0.7},
	})
	ctx = handler.OnEnd(ctx, &callbacks.RunInfo{Component: components.ComponentOfChatModel, Type: "OpenAI", Name: "chat"}, &model.CallbackOutput{
		Message: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "search",
					Arguments: `{"query":"hello"}`,
				},
			}},
		},
		TokenUsage: &model.TokenUsage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
	})
	flush()

	events := flattenEvents(requests)
	if len(events) != 5 {
		t.Fatalf("events len = %d, want 5: %#v", len(events), events)
	}
	if events[0].Type != eventTypeTraceCreate {
		t.Fatalf("first event type = %s", events[0].Type)
	}
	if events[1].Type != eventTypeGenerationCreate {
		t.Fatalf("second event type = %s", events[1].Type)
	}
	if events[2].Type != eventTypeTraceCreate {
		t.Fatalf("third event type = %s", events[2].Type)
	}
	if events[3].Type != eventTypeGenerationUpdate {
		t.Fatalf("fourth event type = %s", events[3].Type)
	}
	if events[4].Type != eventTypeTraceCreate {
		t.Fatalf("fifth event type = %s", events[4].Type)
	}

	createBody := eventBodyMap(t, events[1])
	input := createBody["input"].(map[string]any)
	if got := input["model"]; got != "gpt-test" {
		t.Fatalf("input.model = %v", got)
	}
	if got := input["max_tokens"]; got != float64(128) {
		t.Fatalf("input.max_tokens = %v", got)
	}
	if _, ok := input["messages"].([]any); !ok {
		t.Fatalf("missing input.messages: %#v", input)
	}
	tools := input["tools"].([]any)
	if got := tools[0].(map[string]any)["function"].(map[string]any)["name"]; got != "search" {
		t.Fatalf("tool name = %v", got)
	}

	traceInputBody := eventBodyMap(t, events[2])
	if got := traceInputBody["id"]; got != "trace-1" {
		t.Fatalf("trace input id = %v", got)
	}
	traceInput := traceInputBody["input"].(map[string]any)
	if got := traceInput["model"]; got != "gpt-test" {
		t.Fatalf("trace input.model = %v", got)
	}

	updateBody := eventBodyMap(t, events[3])
	output := updateBody["output"].(map[string]any)
	outputToolCalls := output["tool_calls"].([]any)
	if got := outputToolCalls[0].(map[string]any)["function"].(map[string]any)["name"]; got != "search" {
		t.Fatalf("tool call name = %v", got)
	}
	usageDetails := updateBody["usageDetails"].(map[string]any)
	if got := usageDetails["input"]; got != float64(10) {
		t.Fatalf("usageDetails.input = %v", got)
	}

	traceOutputBody := eventBodyMap(t, events[4])
	if got := traceOutputBody["id"]; got != "trace-1" {
		t.Fatalf("trace output id = %v", got)
	}
	traceOutput := traceOutputBody["output"].(map[string]any)
	traceOutputToolCalls := traceOutput["tool_calls"].([]any)
	if got := traceOutputToolCalls[0].(map[string]any)["function"].(map[string]any)["name"]; got != "search" {
		t.Fatalf("trace output tool call name = %v", got)
	}
}

func TestHandlerSkipsTopLevelToolInvocation(t *testing.T) {
	var requests []ingestionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ingestionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, req)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	handler, flush, err := NewHandler(Config{
		ClientConfig: ClientConfig{
			Host:       server.URL,
			PublicKey:  "pk",
			SecretKey:  "sk",
			FlushAt:    100,
			HTTPClient: server.Client(),
		},
		Clock: fixedClock(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	ctx := SetTrace(context.Background(), WithID("trace-2"), WithName("tool-trace"))
	if handler.Needed(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, callbacks.TimingOnStart) {
		t.Fatal("top-level tool should not be needed")
	}
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, `{"query":"hello"}`)
	ctx = handler.OnEnd(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, "result text")
	flush()

	if events := flattenEvents(requests); len(events) != 0 {
		t.Fatalf("events len = %d, want 0: %#v", len(events), events)
	}
}

func TestHandlerRecordsNestedToolInvocation(t *testing.T) {
	var requests []ingestionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ingestionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, req)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	handler, flush, err := NewHandler(Config{
		ClientConfig: ClientConfig{
			Host:       server.URL,
			PublicKey:  "pk",
			SecretKey:  "sk",
			FlushAt:    100,
			HTTPClient: server.Client(),
		},
		Clock: fixedClock(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	ctx := SetTrace(context.Background(), WithID("trace-2"), WithName("tool-trace"))
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{Component: components.ComponentOfChatModel, Type: "OpenAI", Name: "chat"}, &model.CallbackInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
		Config:   &model.Config{Model: "gpt-test"},
	})
	if !handler.Needed(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, callbacks.TimingOnStart) {
		t.Fatal("nested tool should be needed")
	}
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, `{"query":"hello"}`)
	ctx = handler.OnEnd(ctx, &callbacks.RunInfo{Component: components.ComponentOfTool, Type: "InferTool", Name: "search"}, "result text")
	flush()

	events := flattenEvents(requests)
	if len(events) != 5 {
		t.Fatalf("events len = %d, want 5", len(events))
	}
	createBody := eventBodyMap(t, events[3])
	if got := createBody["input"]; got != `{"query":"hello"}` {
		t.Fatalf("tool input = %#v", got)
	}
	metadata := createBody["metadata"].(map[string]any)
	if got := metadata["observation_type"]; got != "TOOL" {
		t.Fatalf("observation_type = %v", got)
	}
	updateBody := eventBodyMap(t, events[4])
	if got := updateBody["output"]; got != "result text" {
		t.Fatalf("tool output = %#v", got)
	}
}

func TestHandlerSkipsWorkflowAndLambda(t *testing.T) {
	var requests []ingestionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ingestionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, req)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	handler, flush, err := NewHandler(Config{
		ClientConfig: ClientConfig{
			Host:       server.URL,
			PublicKey:  "pk",
			SecretKey:  "sk",
			FlushAt:    100,
			HTTPClient: server.Client(),
		},
		Clock: fixedClock(),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	workflow := &callbacks.RunInfo{Component: "Workflow", Type: "Workflow", Name: "customer_agent_turn"}
	if handler.Needed(context.Background(), workflow, callbacks.TimingOnStart) {
		t.Fatal("workflow should not be needed")
	}
	ctx := SetTrace(context.Background(), WithID("trace-3"), WithName("workflow-trace"))
	ctx = handler.OnStart(ctx, workflow, map[string]any{"input": "value"})
	ctx = handler.OnEnd(ctx, workflow, map[string]any{"output": "value"})

	lambda := &callbacks.RunInfo{Component: "Lambda", Type: "Lambda", Name: "prepare_messages"}
	if handler.Needed(ctx, lambda, callbacks.TimingOnStart) {
		t.Fatal("lambda should not be needed")
	}
	ctx = handler.OnStart(ctx, lambda, "input")
	ctx = handler.OnEnd(ctx, lambda, "output")
	flush()

	if events := flattenEvents(requests); len(events) != 0 {
		t.Fatalf("events len = %d, want 0: %#v", len(events), events)
	}
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func flattenEvents(requests []ingestionRequest) []ingestionEvent {
	var events []ingestionEvent
	for _, req := range requests {
		events = append(events, req.Batch...)
	}
	return events
}

func eventBodyMap(t *testing.T, event ingestionEvent) map[string]any {
	t.Helper()
	raw, err := json.Marshal(event.Body)
	if err != nil {
		t.Fatalf("marshal event body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal event body: %v", err)
	}
	return body
}
