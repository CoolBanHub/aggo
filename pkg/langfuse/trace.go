package langfuse

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type traceOptionsKey struct{}

type TraceOption func(*traceOptions)

type traceOptions struct {
	ID          string
	Name        string
	UserID      string
	SessionID   string
	Release     string
	Version     string
	Environment string
	Metadata    map[string]string
	Tags        []string
	Public      *bool
	Input       any
	Output      any
}

type traceState struct {
	Body           traceBody
	InputExplicit  bool
	OutputExplicit bool
	AutoInputSet   bool
}

func SetTrace(ctx context.Context, opts ...TraceOption) context.Context {
	options := &traceOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	return context.WithValue(ctx, traceOptionsKey{}, options)
}

func WithID(id string) TraceOption {
	return func(o *traceOptions) { o.ID = strings.TrimSpace(id) }
}

func WithName(name string) TraceOption {
	return func(o *traceOptions) { o.Name = strings.TrimSpace(name) }
}

func WithUserID(userID string) TraceOption {
	return func(o *traceOptions) { o.UserID = strings.TrimSpace(userID) }
}

func WithSessionID(sessionID string) TraceOption {
	return func(o *traceOptions) { o.SessionID = strings.TrimSpace(sessionID) }
}

func WithRelease(release string) TraceOption {
	return func(o *traceOptions) { o.Release = strings.TrimSpace(release) }
}

func WithVersion(version string) TraceOption {
	return func(o *traceOptions) { o.Version = strings.TrimSpace(version) }
}

func WithEnvironment(environment string) TraceOption {
	return func(o *traceOptions) { o.Environment = strings.TrimSpace(environment) }
}

func WithMetadata(metadata map[string]string) TraceOption {
	return func(o *traceOptions) { o.Metadata = metadata }
}

func WithTags(tags ...string) TraceOption {
	return func(o *traceOptions) { o.Tags = cleanStrings(tags) }
}

func WithPublic(public bool) TraceOption {
	return func(o *traceOptions) { o.Public = &public }
}

func WithInput(input any) TraceOption {
	return func(o *traceOptions) { o.Input = input }
}

func WithOutput(output any) TraceOption {
	return func(o *traceOptions) { o.Output = output }
}

func (h *Handler) initTrace(ctx context.Context, defaultName string) (*traceState, bool) {
	opts, _ := ctx.Value(traceOptionsKey{}).(*traceOptions)
	if opts == nil {
		opts = &traceOptions{}
	}

	traceID := strings.TrimSpace(opts.ID)
	if traceID == "" {
		traceID = uuid.NewString()
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = strings.TrimSpace(h.cfg.Name)
	}
	if name == "" {
		name = defaultName
	}

	release := firstNonEmpty(opts.Release, h.cfg.Release)
	version := firstNonEmpty(opts.Version, h.cfg.Version)
	environment := firstNonEmpty(opts.Environment, h.cfg.Environment, defaultEnvironment)

	body := traceBody{
		ID:          traceID,
		Timestamp:   formatTime(h.clock()),
		Name:        name,
		UserID:      opts.UserID,
		Input:       opts.Input,
		Output:      opts.Output,
		SessionID:   opts.SessionID,
		Release:     release,
		Version:     version,
		Metadata:    opts.Metadata,
		Tags:        opts.Tags,
		Environment: environment,
		Public:      opts.Public,
	}
	h.client.Enqueue(eventTypeTraceCreate, body)

	return &traceState{
		Body:           body,
		InputExplicit:  opts.Input != nil,
		OutputExplicit: opts.Output != nil,
	}, true
}
