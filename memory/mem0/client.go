package mem0

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a small HTTP client for mem0-compatible APIs.
type Client struct {
	baseURL    string
	httpClient *http.Client
	config     *ProviderConfig
}

// NewClient creates a client from a normalized provider config.
func NewClient(config *ProviderConfig) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	if httpClient.Timeout <= 0 {
		httpClient.Timeout = defaultTimeout
	}

	return &Client{
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		httpClient: httpClient,
		config:     config,
	}
}

// Add stores one memory turn in the configured backend.
func (c *Client) Add(ctx context.Context, req AddRequest) error {
	var body any
	switch c.config.Mode {
	case ModeOSS:
		body = ossAddRequest{
			Messages:  req.Messages,
			UserID:    req.UserID,
			RunID:     req.RunID,
			AgentID:   req.AgentID,
			AppID:     req.AppID,
			OrgID:     req.OrgID,
			ProjectID: req.ProjectID,
			Metadata:  req.Metadata,
		}
	default:
		body = hostedAddRequest{
			Messages:  req.Messages,
			UserID:    req.UserID,
			RunID:     req.RunID,
			AgentID:   req.AgentID,
			AppID:     req.AppID,
			OrgID:     req.OrgID,
			ProjectID: req.ProjectID,
			Metadata:  req.Metadata,
			Version:   c.config.Version,
		}
	}

	_, err := c.postJSON(ctx, c.config.AddPath, body)
	return err
}

// Search retrieves relevant memories from the configured backend.
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]SearchItem, error) {
	var body any
	switch c.config.Mode {
	case ModeOSS:
		body = ossSearchRequest{
			Query:     req.Query,
			UserID:    req.UserID,
			RunID:     req.RunID,
			AgentID:   req.AgentID,
			AppID:     req.AppID,
			OrgID:     req.OrgID,
			ProjectID: req.ProjectID,
		}
	default:
		body = hostedSearchRequest{
			Query:   req.Query,
			Filters: buildHostedFilters(req),
			TopK:    req.Limit,
			Version: c.config.Version,
		}
	}

	payload, err := c.postJSON(ctx, c.config.SearchPath, body)
	if err != nil {
		return nil, err
	}
	return normalizeSearchItems(payload), nil
}

func (c *Client) postJSON(ctx context.Context, path string, requestBody any) (any, error) {
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", path, err)
	}

	url := c.baseURL + normalizePath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", path, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send %s request: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			return nil, fmt.Errorf("%s returned status %s", path, resp.Status)
		}
		return nil, fmt.Errorf("%s returned status %s: %s", path, resp.Status, msg)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", path, err)
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	return payload, nil
}

func (c *Client) applyHeaders(req *http.Request) {
	for k, v := range c.config.ExtraHeaders {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	if strings.TrimSpace(c.config.APIKey) == "" {
		return
	}

	header := strings.TrimSpace(c.config.AuthHeader)
	scheme := strings.TrimSpace(c.config.AuthScheme)
	if header == "" {
		switch c.config.Mode {
		case ModeOSS:
			header = "X-API-Key"
		default:
			header = "Authorization"
			if scheme == "" {
				scheme = "Token"
			}
		}
	}

	if header == "" {
		return
	}
	if scheme != "" {
		req.Header.Set(header, scheme+" "+c.config.APIKey)
		return
	}
	req.Header.Set(header, c.config.APIKey)
}

func buildHostedFilters(req SearchRequest) map[string]any {
	clauses := make([]any, 0, 7)
	if strings.TrimSpace(req.UserID) != "" {
		clauses = append(clauses, map[string]any{"user_id": req.UserID})
	}
	if strings.TrimSpace(req.RunID) != "" {
		clauses = append(clauses, map[string]any{"run_id": req.RunID})
	}
	if strings.TrimSpace(req.AgentID) != "" {
		clauses = append(clauses, map[string]any{"agent_id": req.AgentID})
	}
	if strings.TrimSpace(req.AppID) != "" {
		clauses = append(clauses, map[string]any{"app_id": req.AppID})
	}
	if strings.TrimSpace(req.OrgID) != "" {
		clauses = append(clauses, map[string]any{"org_id": req.OrgID})
	}
	if strings.TrimSpace(req.ProjectID) != "" {
		clauses = append(clauses, map[string]any{"project_id": req.ProjectID})
	}
	if len(req.Filters) > 0 {
		clauses = append(clauses, cloneAnyMap(req.Filters))
	}

	switch len(clauses) {
	case 0:
		return nil
	case 1:
		if single, ok := clauses[0].(map[string]any); ok {
			return single
		}
	}
	return map[string]any{"AND": clauses}
}

func normalizeSearchItems(payload any) []SearchItem {
	objects := unwrapSearchObjects(payload)
	items := make([]SearchItem, 0, len(objects))
	for _, obj := range objects {
		if item, ok := normalizeSearchItem(obj); ok {
			items = append(items, item)
		}
	}
	return items
}

func unwrapSearchObjects(payload any) []map[string]any {
	switch v := payload.(type) {
	case nil:
		return nil
	case []any:
		return collectObjects(v)
	case map[string]any:
		for _, key := range []string{"results", "items", "memories", "data"} {
			if raw, ok := v[key]; ok {
				if arr, ok := raw.([]any); ok {
					return collectObjects(arr)
				}
			}
		}
		if looksLikeMemoryObject(v) {
			return []map[string]any{v}
		}
	}
	return nil
}

func collectObjects(values []any) []map[string]any {
	items := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if obj, ok := value.(map[string]any); ok {
			items = append(items, obj)
		}
	}
	return items
}

func normalizeSearchItem(obj map[string]any) (SearchItem, bool) {
	text := firstString(
		obj["memory"],
		obj["text"],
		obj["content"],
		obj["summary"],
	)
	if strings.TrimSpace(text) == "" {
		if nested, ok := obj["memory"].(map[string]any); ok {
			text = firstString(nested["text"], nested["content"], nested["memory"], nested["summary"])
		}
	}
	if strings.TrimSpace(text) == "" {
		return SearchItem{}, false
	}

	item := SearchItem{
		ID:         firstString(obj["id"], obj["memory_id"]),
		Memory:     strings.TrimSpace(text),
		Score:      firstFloat(obj["score"], obj["similarity"]),
		Categories: firstStringSlice(obj["categories"], obj["category"]),
		Metadata:   firstMap(obj["metadata"]),
		CreatedAt:  firstString(obj["created_at"], obj["createdAt"]),
		UpdatedAt:  firstString(obj["updated_at"], obj["updatedAt"]),
	}

	if len(item.Categories) == 0 && len(item.Metadata) > 0 {
		item.Categories = firstStringSlice(item.Metadata["categories"], item.Metadata["category"])
	}

	return item, true
}

func looksLikeMemoryObject(obj map[string]any) bool {
	return strings.TrimSpace(firstString(obj["memory"], obj["text"], obj["content"], obj["summary"])) != ""
}

func firstString(values ...any) string {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func firstFloat(values ...any) float64 {
	for _, value := range values {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case json.Number:
			if parsed, err := v.Float64(); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstStringSlice(values ...any) []string {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return []string{strings.TrimSpace(v)}
			}
		case []string:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if strings.TrimSpace(item) != "" {
					result = append(result, strings.TrimSpace(item))
				}
			}
			if len(result) > 0 {
				return result
			}
		case []any:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					result = append(result, strings.TrimSpace(s))
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}
	return nil
}

func firstMap(values ...any) map[string]any {
	for _, value := range values {
		if v, ok := value.(map[string]any); ok && len(v) > 0 {
			return cloneAnyMap(v)
		}
	}
	return nil
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
