package langfuse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	PromptTypeText = "text"
	PromptTypeChat = "chat"
)

type PromptOption func(*promptOptions)

type promptOptions struct {
	version *int
	label   string
	resolve *bool
}

type Prompt struct {
	ID              string          `json:"id,omitempty"`
	CreatedAt       time.Time       `json:"createdAt,omitempty"`
	UpdatedAt       time.Time       `json:"updatedAt,omitempty"`
	ProjectID       string          `json:"projectId,omitempty"`
	CreatedBy       string          `json:"createdBy,omitempty"`
	Name            string          `json:"name"`
	Version         int             `json:"version"`
	Type            string          `json:"type"`
	Text            string          `json:"-"`
	Messages        []PromptMessage `json:"-"`
	Config          any             `json:"config"`
	Labels          []string        `json:"labels"`
	Tags            []string        `json:"tags"`
	CommitMessage   *string         `json:"commitMessage,omitempty"`
	ResolutionGraph any             `json:"resolutionGraph,omitempty"`
	RawPrompt       json.RawMessage `json:"prompt,omitempty"`
}

type PromptMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
}

func WithPromptVersion(version int) PromptOption {
	return func(o *promptOptions) { o.version = &version }
}

func WithPromptLabel(label string) PromptOption {
	return func(o *promptOptions) { o.label = strings.TrimSpace(label) }
}

func WithPromptResolve(resolve bool) PromptOption {
	return func(o *promptOptions) { o.resolve = &resolve }
}

func (c *Client) GetPrompt(ctx context.Context, name string, opts ...PromptOption) (*Prompt, error) {
	if c == nil {
		return nil, errors.New("langfuse client is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("prompt name is required")
	}

	options := &promptOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	if options.version != nil && options.label != "" {
		return nil, errors.New("prompt version and label are mutually exclusive")
	}

	endpoint, err := c.promptEndpoint(name, options)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create prompt request: %w", err)
	}
	req.Header.Set("authorization", "Basic "+basicAuth(c.cfg.PublicKey, c.cfg.SecretKey))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get prompt %q: %w", name, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get prompt %q status=%d body=%s", name, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	prompt, err := decodePrompt(body)
	if err != nil {
		return nil, fmt.Errorf("decode prompt %q: %w", name, err)
	}
	return prompt, nil
}

func (c *Client) GetTextPrompt(ctx context.Context, name string, opts ...PromptOption) (string, error) {
	prompt, err := c.GetPrompt(ctx, name, opts...)
	if err != nil {
		return "", err
	}
	if prompt.Type != "" && prompt.Type != PromptTypeText {
		return "", fmt.Errorf("prompt %q is %s, not %s", name, prompt.Type, PromptTypeText)
	}
	return prompt.Text, nil
}

func (c *Client) promptEndpoint(name string, opts *promptOptions) (string, error) {
	u, err := url.Parse(strings.TrimSpace(c.cfg.Host))
	if err != nil {
		return "", fmt.Errorf("parse langfuse host: %w", err)
	}
	basePath := strings.TrimRight(u.Path, "/") + "/api/public/v2/prompts/"
	u.Path = basePath + name
	u.RawPath = basePath + url.PathEscape(name)
	u.RawQuery = ""
	u.Fragment = ""

	q := u.Query()
	if opts != nil {
		if opts.version != nil {
			q.Set("version", strconv.Itoa(*opts.version))
		}
		if opts.label != "" {
			q.Set("label", opts.label)
		}
		if opts.resolve != nil {
			q.Set("resolve", strconv.FormatBool(*opts.resolve))
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func decodePrompt(body []byte) (*Prompt, error) {
	var raw struct {
		ID              string          `json:"id"`
		CreatedAt       time.Time       `json:"createdAt"`
		UpdatedAt       time.Time       `json:"updatedAt"`
		ProjectID       string          `json:"projectId"`
		CreatedBy       string          `json:"createdBy"`
		Name            string          `json:"name"`
		Version         int             `json:"version"`
		Type            string          `json:"type"`
		Prompt          json.RawMessage `json:"prompt"`
		Config          any             `json:"config"`
		Labels          []string        `json:"labels"`
		Tags            []string        `json:"tags"`
		CommitMessage   *string         `json:"commitMessage"`
		ResolutionGraph any             `json:"resolutionGraph"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	prompt := &Prompt{
		ID:              raw.ID,
		CreatedAt:       raw.CreatedAt,
		UpdatedAt:       raw.UpdatedAt,
		ProjectID:       raw.ProjectID,
		CreatedBy:       raw.CreatedBy,
		Name:            raw.Name,
		Version:         raw.Version,
		Type:            raw.Type,
		Config:          raw.Config,
		Labels:          raw.Labels,
		Tags:            raw.Tags,
		CommitMessage:   raw.CommitMessage,
		ResolutionGraph: raw.ResolutionGraph,
		RawPrompt:       raw.Prompt,
	}

	switch raw.Type {
	case PromptTypeChat:
		if len(raw.Prompt) > 0 {
			if err := json.Unmarshal(raw.Prompt, &prompt.Messages); err != nil {
				return nil, err
			}
		}
	default:
		if len(raw.Prompt) > 0 {
			if err := json.Unmarshal(raw.Prompt, &prompt.Text); err != nil {
				return nil, err
			}
		}
	}
	return prompt, nil
}
