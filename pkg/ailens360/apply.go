package ailens360

import (
	openai "github.com/cloudwego/eino-ext/components/model/openai"
)

// Apply mutates an openai.ChatModelConfig in place so that requests flow
// through the AILens360 proxy. The BaseURL is rewritten to
// "<proxy_prefix>/<original_base_url>" and the HTTPClient is wrapped with a
// RoundTripper that stamps the project key plus per-call user/session/trace
// headers pulled from request context.
//
// If d is nil (e.g. when AILens360 is not configured), Apply is a no-op so
// callers can blindly invoke it.
func (d *Decorator) Apply(cfg *openai.ChatModelConfig) {
	if d == nil || cfg == nil {
		return
	}
	cfg.BaseURL = d.DecorateBaseURL(cfg.BaseURL)
	cfg.HTTPClient = d.HTTPClient(cfg.HTTPClient)
}

// ApplyGlobal is a convenience wrapper that pulls the process-wide Decorator
// installed via SetGlobal. Returns true when the global decorator is active.
func ApplyGlobal(cfg *openai.ChatModelConfig) bool {
	dec := Global()
	if dec == nil {
		return false
	}
	dec.Apply(cfg)
	return true
}
