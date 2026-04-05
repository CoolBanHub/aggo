package mem0

import (
	"net/http"
	"time"
)

const MemorySystemPrefix = "[mem0-memory-context]"

const (
	defaultTimeout           = 30 * time.Second
	defaultSearchMsgLimit    = 6
	defaultSearchResultLimit = 5
	defaultOutputMemoryLimit = 5
	defaultQueryCharLimit    = 1200
	defaultSessionMetaKey    = "session_id"
	defaultHostedVersion     = "v2"
	defaultPerItemCharLimit  = 300
)

// Mode identifies which mem0-compatible API surface the provider should target.
type Mode string

const (
	ModeHosted Mode = "hosted"
	ModeOSS    Mode = "oss"
)

// ProviderConfig configures the mem0 provider and client behavior.
type ProviderConfig struct {
	BaseURL string

	Mode       Mode
	APIKey     string
	AuthHeader string
	AuthScheme string

	AddPath    string
	SearchPath string
	Version    string

	HTTPClient *http.Client
	Timeout    time.Duration

	SearchMsgLimit    int
	SearchResultLimit int
	OutputMemoryLimit int
	QueryCharLimit    int

	UseSessionAsRunID  bool
	SearchBySession    bool
	SessionMetadataKey string

	AgentID   string
	AppID     string
	OrgID     string
	ProjectID string

	ExtraHeaders map[string]string
	Metadata     map[string]any
}

// ClientMessage is the message format accepted by mem0-compatible APIs.
type ClientMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AddRequest is the normalized input for storing one memory turn.
type AddRequest struct {
	Messages  []ClientMessage
	UserID    string
	RunID     string
	AgentID   string
	AppID     string
	OrgID     string
	ProjectID string
	Metadata  map[string]any
}

// SearchRequest is the normalized input for memory retrieval.
type SearchRequest struct {
	Query     string
	UserID    string
	RunID     string
	AgentID   string
	AppID     string
	OrgID     string
	ProjectID string
	Limit     int
	Filters   map[string]any
}

// SearchItem is the normalized memory item returned from hosted or OSS APIs.
type SearchItem struct {
	ID         string
	Memory     string
	Score      float64
	Categories []string
	Metadata   map[string]any
	CreatedAt  string
	UpdatedAt  string
}

type hostedAddRequest struct {
	Messages  []ClientMessage `json:"messages"`
	UserID    string          `json:"user_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	AgentID   string          `json:"agent_id,omitempty"`
	AppID     string          `json:"app_id,omitempty"`
	OrgID     string          `json:"org_id,omitempty"`
	ProjectID string          `json:"project_id,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Version   string          `json:"version,omitempty"`
}

type hostedSearchRequest struct {
	Query   string         `json:"query"`
	Filters map[string]any `json:"filters,omitempty"`
	TopK    int            `json:"top_k,omitempty"`
	Version string         `json:"version,omitempty"`
}

type ossAddRequest struct {
	Messages  []ClientMessage `json:"messages"`
	UserID    string          `json:"user_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	AgentID   string          `json:"agent_id,omitempty"`
	AppID     string          `json:"app_id,omitempty"`
	OrgID     string          `json:"org_id,omitempty"`
	ProjectID string          `json:"project_id,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

type ossSearchRequest struct {
	Query     string `json:"query"`
	UserID    string `json:"user_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	AppID     string `json:"app_id,omitempty"`
	OrgID     string `json:"org_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}
