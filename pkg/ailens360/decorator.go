package ailens360

import (
	"net/http"
	"strings"
	"sync/atomic"
)

// Decorator wraps an OpenAI-compatible chat-model BaseURL to route through an
// AILens360 proxy and installs an HTTP transport that stamps telemetry headers
// from request context.
type Decorator struct {
	proxyPrefix string
	projectKey  string
}

// NewDecorator builds a decorator. Both proxyPrefix and projectKey must be
// non-empty for the decorator to be functional; otherwise nil is returned and
// callers should skip decoration.
func NewDecorator(proxyPrefix, projectKey string) *Decorator {
	proxyPrefix = strings.TrimRight(strings.TrimSpace(proxyPrefix), "/")
	projectKey = strings.TrimSpace(projectKey)
	if proxyPrefix == "" || projectKey == "" {
		return nil
	}
	return &Decorator{proxyPrefix: proxyPrefix, projectKey: projectKey}
}

// DecorateBaseURL prepends the proxy prefix to the upstream BaseURL.
// upstream is expected to be a fully-qualified URL like
// "https://api.openai.com/v1".
func (d *Decorator) DecorateBaseURL(upstream string) string {
	if d == nil {
		return upstream
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return upstream
	}
	return d.proxyPrefix + "/" + upstream
}

// HTTPClient returns an *http.Client whose Transport injects telemetry headers.
// If base is non-nil its Transport is preserved as the inner round-tripper.
func (d *Decorator) HTTPClient(base *http.Client) *http.Client {
	if d == nil {
		return base
	}
	var inner http.RoundTripper
	if base != nil {
		inner = base.Transport
	}
	client := &http.Client{
		Transport: &telemetryHeaders{projectKey: d.projectKey, base: inner},
	}
	if base != nil {
		client.Timeout = base.Timeout
		client.CheckRedirect = base.CheckRedirect
		client.Jar = base.Jar
	}
	return client
}

// --- global registry --------------------------------------------------------

var globalDecorator atomic.Pointer[Decorator]

// SetGlobal installs a process-wide decorator. Pass nil to disable.
func SetGlobal(d *Decorator) {
	if d == nil {
		globalDecorator.Store(nil)
		return
	}
	globalDecorator.Store(d)
}

// Global returns the process-wide decorator, or nil if none is installed.
func Global() *Decorator {
	return globalDecorator.Load()
}
