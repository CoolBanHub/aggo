package ailens360

import "net/http"

// telemetryHeaders stamps AILens360 headers on every outbound request:
//   - X-AILens-Project-Key is fixed (per-app), set at construction
//   - X-AILens-User / Session / Tag / Trace-* come from req.Context()
type telemetryHeaders struct {
	projectKey string
	base       http.RoundTripper
}

func (t *telemetryHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.projectKey != "" {
		req.Header.Set("X-AILens-Project-Key", t.projectKey)
	}
	ctx := req.Context()
	setFromCtx := func(h string, k ctxKey) {
		if v := ctxString(ctx, k); v != "" {
			req.Header.Set(h, v)
		}
	}
	setFromCtx("X-AILens-User", keyUser)
	setFromCtx("X-AILens-Session", keySession)
	setFromCtx("X-AILens-Tag", keyTag)
	setFromCtx("X-AILens-Trace-Id", keyTraceID)
	setFromCtx("X-AILens-Trace-Name", keyTraceName)

	rt := t.base
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(req)
}
