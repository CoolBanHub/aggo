package langfuse

import (
	"context"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type Config struct {
	ClientConfig

	Name   string
	Clock  func() time.Time
	Logger *log.Logger
}

type Handler struct {
	client *Client
	cfg    Config
	clock  func() time.Time
	logger *log.Logger
}

type stateKey struct{}

type state struct {
	TraceID       string
	ObservationID string
	Trace         *traceState
	Kind          string
	StartTime     time.Time
	Name          string
	Component     string
}

func NewHandler(cfg Config) (*Handler, func(), error) {
	client, err := NewClient(cfg.ClientConfig)
	if err != nil {
		return nil, nil, err
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	h := &Handler{
		client: client,
		cfg:    cfg,
		clock:  clock,
		logger: cfg.Logger,
	}
	return h, client.Close, nil
}

func (h *Handler) Needed(ctx context.Context, info *callbacks.RunInfo, timing callbacks.CallbackTiming) bool {
	if h == nil || h.client == nil || info == nil {
		return false
	}
	if !shouldRecord(info) {
		return false
	}
	if info.Component == components.ComponentOfTool && !hasTraceState(ctx) {
		return false
	}
	switch timing {
	case callbacks.TimingOnStart, callbacks.TimingOnEnd, callbacks.TimingOnError, callbacks.TimingOnEndWithStreamOutput:
		return true
	case callbacks.TimingOnStartWithStreamInput:
		return false
	default:
		return false
	}
}

func (h *Handler) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	if h == nil || h.client == nil || info == nil {
		return ctx
	}
	if !shouldRecord(info) {
		return ctx
	}
	name := getName(info.Name, info.Type, string(info.Component))
	trace, ok := h.traceForStart(ctx, info, name)
	if !ok {
		return ctx
	}
	traceID := trace.Body.ID

	observationID := uuid.NewString()
	st := &state{
		TraceID:       traceID,
		ObservationID: observationID,
		Trace:         trace,
		Kind:          eventKind(info),
		StartTime:     h.clock(),
		Name:          name,
		Component:     string(info.Component),
	}

	body := observationBody{
		ID:                  observationID,
		TraceID:             traceID,
		Name:                name,
		StartTime:           formatTime(st.StartTime),
		ParentObservationID: parentObservationID(ctx),
		Environment:         firstNonEmpty(h.cfg.Environment, defaultEnvironment),
		Version:             h.cfg.Version,
		Metadata: mergeMetadata(
			map[string]any{"component": string(info.Component), "type": info.Type},
			componentMetadata(info, input),
		),
	}

	if info.Component == components.ComponentOfChatModel {
		if in := model.ConvCallbackInput(input); in != nil {
			body.Input = chatModelInput(in)
			body.ModelParameters = modelParameters(in.Config, in.ToolChoice)
			if in.Config != nil {
				body.Model = in.Config.Model
			}
			body.Metadata = mergeMetadata(
				body.Metadata.(map[string]any),
				in.Extra,
				map[string]any{
					"tools":       toolDefinitionsMetadata(in.Tools),
					"tool_choice": toolChoiceValue(in.ToolChoice),
				},
			)
		} else {
			body.Input = marshalAny(input)
		}
		h.client.Enqueue(eventTypeGenerationCreate, body)
		h.updateTraceInput(st, body.Input)
		return context.WithValue(ctx, stateKey{}, st)
	}

	body.Input = normalizeInput(input)
	if st.Kind == "generation" {
		h.client.Enqueue(eventTypeGenerationCreate, body)
	} else {
		h.client.Enqueue(eventTypeSpanCreate, body)
	}
	return context.WithValue(ctx, stateKey{}, st)
}

func (h *Handler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	if h == nil || h.client == nil || info == nil {
		return ctx
	}
	if !shouldRecord(info) {
		return ctx
	}
	st, ok := ctx.Value(stateKey{}).(*state)
	if !ok || st == nil {
		return ctx
	}

	body := observationBody{
		ID:          st.ObservationID,
		TraceID:     st.TraceID,
		EndTime:     formatTime(h.clock()),
		Environment: firstNonEmpty(h.cfg.Environment, defaultEnvironment),
	}

	if info.Component == components.ComponentOfChatModel {
		if out := model.ConvCallbackOutput(output); out != nil {
			body.Output = convertMessage(out.Message)
			body.CompletionStartTime = formatTime(st.StartTime)
			body.Metadata = mergeMetadata(out.Extra, map[string]any{
				"tool_calls": toolCallsMetadata(out.Message),
			})
			body.Usage, body.UsageDetails = usageFromModel(out.TokenUsage)
			if out.Config != nil {
				body.Model = out.Config.Model
				body.ModelParameters = modelParameters(out.Config, nil)
			}
		} else {
			body.Output = normalizeOutput(output)
		}
		h.client.Enqueue(eventTypeGenerationUpdate, body)
		h.updateTraceOutput(st, body.Output)
		return ctx
	}

	body.Output = normalizeOutput(output)
	if st.Kind == "generation" {
		body.CompletionStartTime = formatTime(st.StartTime)
		h.client.Enqueue(eventTypeGenerationUpdate, body)
	} else {
		h.client.Enqueue(eventTypeSpanUpdate, body)
	}
	return ctx
}

func (h *Handler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	if h == nil || h.client == nil || info == nil {
		return ctx
	}
	if !shouldRecord(info) {
		return ctx
	}
	st, ok := ctx.Value(stateKey{}).(*state)
	if !ok || st == nil {
		return ctx
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	body := observationBody{
		ID:            st.ObservationID,
		TraceID:       st.TraceID,
		EndTime:       formatTime(h.clock()),
		Level:         "ERROR",
		StatusMessage: msg,
		Output:        msg,
		Environment:   firstNonEmpty(h.cfg.Environment, defaultEnvironment),
	}
	if st.Kind == "generation" {
		body.CompletionStartTime = formatTime(st.StartTime)
		h.client.Enqueue(eventTypeGenerationUpdate, body)
	} else {
		h.client.Enqueue(eventTypeSpanUpdate, body)
	}
	return ctx
}

func (h *Handler) OnStartWithStreamInput(ctx context.Context, _ *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	if input != nil {
		input.Close()
	}
	return ctx
}

func (h *Handler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	if h == nil || h.client == nil || info == nil || output == nil {
		return ctx
	}
	if !shouldRecord(info) {
		output.Close()
		return ctx
	}
	st, ok := ctx.Value(stateKey{}).(*state)
	if !ok || st == nil {
		output.Close()
		return ctx
	}

	go h.consumeStreamOutput(st, info, output)
	return ctx
}

func (h *Handler) ensureTrace(ctx context.Context, defaultName string) (*traceState, bool) {
	if st, ok := ctx.Value(stateKey{}).(*state); ok && st != nil && st.Trace != nil && st.Trace.Body.ID != "" {
		return st.Trace, true
	}
	return h.initTrace(ctx, defaultName)
}

func (h *Handler) traceForStart(ctx context.Context, info *callbacks.RunInfo, defaultName string) (*traceState, bool) {
	if info != nil && info.Component == components.ComponentOfTool {
		st, ok := ctx.Value(stateKey{}).(*state)
		if !ok || st == nil || st.Trace == nil || st.Trace.Body.ID == "" {
			return nil, false
		}
		return st.Trace, true
	}
	return h.ensureTrace(ctx, defaultName)
}

func hasTraceState(ctx context.Context) bool {
	st, ok := ctx.Value(stateKey{}).(*state)
	return ok && st != nil && st.Trace != nil && st.Trace.Body.ID != ""
}

func (h *Handler) updateTraceInput(st *state, input any) {
	if st == nil || st.Trace == nil || st.Trace.Body.ID == "" || input == nil {
		return
	}
	if st.Trace.InputExplicit || st.Trace.AutoInputSet {
		return
	}

	body := st.Trace.Body
	body.Timestamp = formatTime(h.clock())
	body.Input = input
	h.client.Enqueue(eventTypeTraceCreate, body)

	st.Trace.Body.Input = input
	st.Trace.AutoInputSet = true
}

func (h *Handler) updateTraceOutput(st *state, output any) {
	if st == nil || st.Trace == nil || st.Trace.Body.ID == "" || output == nil {
		return
	}
	if st.Trace.OutputExplicit {
		return
	}

	body := st.Trace.Body
	body.Timestamp = formatTime(h.clock())
	body.Output = output
	h.client.Enqueue(eventTypeTraceCreate, body)

	st.Trace.Body.Output = output
}

func (h *Handler) consumeStreamOutput(st *state, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) {
	defer func() {
		if r := recover(); r != nil {
			h.logf("langfuse stream callback panic: %v stack=%s", r, string(debug.Stack()))
		}
		output.Close()
	}()

	var raw []callbacks.CallbackOutput
	for {
		chunk, err := output.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			h.finishStreamError(st, err)
			return
		}
		raw = append(raw, chunk)
	}

	body := observationBody{
		ID:          st.ObservationID,
		TraceID:     st.TraceID,
		EndTime:     formatTime(h.clock()),
		Environment: firstNonEmpty(h.cfg.Environment, defaultEnvironment),
	}

	if info.Component == components.ComponentOfChatModel {
		usage, msg, extra := concatModelOutput(raw)
		body.Output = convertMessage(msg)
		body.Metadata = mergeMetadata(extra, map[string]any{"tool_calls": toolCallsMetadata(msg)})
		body.CompletionStartTime = formatTime(st.StartTime)
		body.Usage, body.UsageDetails = usageFromModel(usage)
		h.client.Enqueue(eventTypeGenerationUpdate, body)
		h.updateTraceOutput(st, body.Output)
		return
	}

	body.Output = normalizeOutput(raw)
	if st.Kind == "generation" {
		body.CompletionStartTime = formatTime(st.StartTime)
		h.client.Enqueue(eventTypeGenerationUpdate, body)
	} else {
		h.client.Enqueue(eventTypeSpanUpdate, body)
	}
}

func (h *Handler) finishStreamError(st *state, err error) {
	msg := err.Error()
	body := observationBody{
		ID:            st.ObservationID,
		TraceID:       st.TraceID,
		EndTime:       formatTime(h.clock()),
		Level:         "ERROR",
		StatusMessage: msg,
		Output:        msg,
		Environment:   firstNonEmpty(h.cfg.Environment, defaultEnvironment),
	}
	if st.Kind == "generation" {
		body.CompletionStartTime = formatTime(st.StartTime)
		h.client.Enqueue(eventTypeGenerationUpdate, body)
	} else {
		h.client.Enqueue(eventTypeSpanUpdate, body)
	}
	if st.Component == string(components.ComponentOfChatModel) {
		h.updateTraceOutput(st, msg)
	}
}

func (h *Handler) logf(format string, args ...any) {
	if h.logger != nil {
		h.logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func eventKind(info *callbacks.RunInfo) string {
	if info == nil {
		return "span"
	}
	switch info.Component {
	case components.ComponentOfChatModel:
		return "generation"
	case components.ComponentOfTool:
		return "generation"
	case adk.ComponentOfAgent:
		return "generation"
	default:
		return "span"
	}
}

func shouldRecord(info *callbacks.RunInfo) bool {
	if info == nil {
		return false
	}
	switch info.Component {
	case components.ComponentOfChatModel, components.ComponentOfTool:
		return true
	default:
		return false
	}
}

func parentObservationID(ctx context.Context) string {
	if st, ok := ctx.Value(stateKey{}).(*state); ok && st != nil {
		return st.ObservationID
	}
	return ""
}

func componentMetadata(info *callbacks.RunInfo, input callbacks.CallbackInput) map[string]any {
	if info == nil {
		return nil
	}
	metadata := map[string]any{}
	switch info.Component {
	case components.ComponentOfTool:
		metadata["observation_type"] = "TOOL"
	case adk.ComponentOfAgent:
		metadata["observation_type"] = "AGENT"
	}
	if ti, ok := input.(interface {
		GetName() string
	}); ok {
		metadata["input_name"] = ti.GetName()
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func normalizeInput(input callbacks.CallbackInput) any {
	switch v := input.(type) {
	case *schema.ToolArgument:
		return v
	case []*schema.Message:
		return convertMessages(v)
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return jsonRaw(v)
	}
}

func normalizeOutput(output callbacks.CallbackOutput) any {
	switch v := output.(type) {
	case *schema.Message:
		return convertMessage(v)
	case *schema.ToolResult:
		return v
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return jsonRaw(v)
	}
}

func concatModelOutput(raw []callbacks.CallbackOutput) (*model.TokenUsage, *schema.Message, map[string]any) {
	var (
		usage    *model.TokenUsage
		messages []*schema.Message
		extra    map[string]any
	)
	for _, item := range raw {
		out := model.ConvCallbackOutput(item)
		if out == nil {
			continue
		}
		if out.TokenUsage != nil {
			usage = out.TokenUsage
		}
		if out.Message != nil {
			messages = append(messages, out.Message)
		}
		if len(out.Extra) > 0 {
			extra = mergeMetadata(extra, out.Extra)
		}
	}
	if len(messages) == 0 {
		return usage, nil, extra
	}
	msg, err := schema.ConcatMessages(messages)
	if err != nil {
		return usage, &schema.Message{Role: schema.Assistant, Content: strings.TrimSpace(safeJSON(messages))}, extra
	}
	return usage, msg, extra
}
