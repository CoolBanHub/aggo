package ailens360

import (
	"context"
	"strings"
)

type ctxKey int

const (
	keyUser ctxKey = iota
	keySession
	keyTag
	keyTraceID
	keyTraceName
)

type TraceConfig struct {
	ID        string
	Name      string
	UserID    string
	SessionID string
	Tag       string
}

// TraceContext exposes the ailens360 telemetry values currently stamped on a
// context. Empty fields mean "not set".
type TraceContext struct {
	ID        string
	Name      string
	UserID    string
	SessionID string
	Tag       string
}

func CurrentTrace(ctx context.Context) TraceContext {
	if ctx == nil {
		return TraceContext{}
	}
	return TraceContext{
		ID:        ctxString(ctx, keyTraceID),
		Name:      ctxString(ctx, keyTraceName),
		UserID:    ctxString(ctx, keyUser),
		SessionID: ctxString(ctx, keySession),
		Tag:       ctxString(ctx, keyTag),
	}
}

func WithUser(ctx context.Context, v string) context.Context {
	if v = strings.TrimSpace(v); v == "" {
		return ctx
	}
	return context.WithValue(ctx, keyUser, v)
}

func WithSession(ctx context.Context, v string) context.Context {
	if v = strings.TrimSpace(v); v == "" {
		return ctx
	}
	return context.WithValue(ctx, keySession, v)
}

func WithTag(ctx context.Context, v string) context.Context {
	if v = strings.TrimSpace(v); v == "" {
		return ctx
	}
	return context.WithValue(ctx, keyTag, v)
}

func WithTraceID(ctx context.Context, v string) context.Context {
	if v = strings.TrimSpace(v); v == "" {
		return ctx
	}
	return context.WithValue(ctx, keyTraceID, v)
}

func WithTraceName(ctx context.Context, v string) context.Context {
	if v = strings.TrimSpace(v); v == "" {
		return ctx
	}
	return context.WithValue(ctx, keyTraceName, v)
}

func WithTrace(ctx context.Context, cfg TraceConfig) context.Context {
	ctx = WithTraceID(ctx, cfg.ID)
	ctx = WithTraceName(ctx, cfg.Name)
	ctx = WithUser(ctx, cfg.UserID)
	ctx = WithSession(ctx, cfg.SessionID)
	ctx = WithTag(ctx, cfg.Tag)
	return ctx
}

func ctxString(ctx context.Context, k ctxKey) string {
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}
