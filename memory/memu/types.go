package memu

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

const MemorySystemPrefix = "[memu-memory-context]"

// QueryContent represents the text content of a query message.
type QueryContent struct {
	Text string `json:"text"`
}

// Query represents a single message in a retrieve request.
type Query struct {
	Role    string       `json:"role"`
	Content QueryContent `json:"content"`
}

// RetrieveRequest is the request body for the /retrieve endpoint.
type RetrieveRequest struct {
	Queries []Query        `json:"queries"`
	Where   map[string]any `json:"where,omitempty"`
}

// RetrieveResponse is the response from the /retrieve endpoint.
type RetrieveResponse struct {
	NeedsRetrieval bool                `json:"needs_retrieval,omitempty"`
	OriginalQuery  string              `json:"original_query,omitempty"`
	RewrittenQuery string              `json:"rewritten_query,omitempty"`
	NextStepQuery  string              `json:"next_step_query,omitempty"`
	Categories     []RetrievedCategory `json:"categories,omitempty"`
	Items          []RetrievedItem     `json:"items,omitempty"`
	Resources      []RetrievedResource `json:"resources,omitempty"`
}

// RetrievedCategory represents a memory category returned by retrieval.
type RetrievedCategory struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	Score       float64 `json:"score,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

// RetrievedItem represents a memory item returned by retrieval.
type RetrievedItem struct {
	ID         string  `json:"id,omitempty"`
	MemoryType string  `json:"memory_type,omitempty"`
	Summary    string  `json:"summary,omitempty"`
	Score      float64 `json:"score,omitempty"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
	HappenedAt string  `json:"happened_at,omitempty"`
}

// RetrievedResource represents a resource returned by retrieval.
type RetrievedResource struct {
	ID       string  `json:"id,omitempty"`
	URL      string  `json:"url,omitempty"`
	Modality string  `json:"modality,omitempty"`
	Caption  string  `json:"caption,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// MemorizeRequest is the request body for the /memorize endpoint.
type MemorizeRequest struct {
	Content  string         `json:"content"`
	Modality string         `json:"modality"`
	User     map[string]any `json:"user,omitempty"`
}

// MemorizeResponse is the response from the /memorize endpoint.
type MemorizeResponse map[string]any

// ProviderConfig holds the configuration for the memu provider.
type ProviderConfig struct {
	BaseURL      string
	UserID       string
	HistoryLimit int
	MaxItems     int
}

// BuildRetrieveRequest constructs a RetrieveRequest from a list of messages.
// It filters to only user/assistant messages, trims whitespace, and limits
// to the most recent `limit` messages.
func BuildRetrieveRequest(messages []*schema.Message, userID string, limit int) RetrieveRequest {
	filtered := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Role != schema.User && msg.Role != schema.Assistant {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		filtered = append(filtered, msg)
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	req := RetrieveRequest{
		Queries: make([]Query, 0, len(filtered)),
	}
	if userID != "" {
		req.Where = map[string]any{"user_id": userID}
	}

	for _, msg := range filtered {
		req.Queries = append(req.Queries, Query{
			Role: string(msg.Role),
			Content: QueryContent{
				Text: strings.TrimSpace(msg.Content),
			},
		})
	}

	return req
}

// BuildConversationTurn formats a user/assistant exchange into a single
// conversation turn string suitable for memorization.
func BuildConversationTurn(userText, assistantText string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("[time: %s]\nuser: %s\nassistant: %s\n", now, strings.TrimSpace(userText), strings.TrimSpace(assistantText))
}

// FormatMemoryContext formats a RetrieveResponse into a human-readable string
// suitable for injection as a system message. It limits output to maxItems per
// category.
func FormatMemoryContext(resp RetrieveResponse, maxItems int) string {
	hasMemory := len(resp.Categories) > 0 || len(resp.Items) > 0 || len(resp.Resources) > 0
	if !hasMemory {
		return ""
	}

	if maxItems <= 0 {
		maxItems = 3
	}

	var b strings.Builder
	b.WriteString(MemorySystemPrefix)
	b.WriteString("\nLong-term memory from memU. Use it as supporting context only. If it conflicts with the current user request, follow the current request.\n")

	if strings.TrimSpace(resp.RewrittenQuery) != "" && resp.RewrittenQuery != resp.OriginalQuery {
		b.WriteString("\nMemory search focus: ")
		b.WriteString(strings.TrimSpace(resp.RewrittenQuery))
		b.WriteString("\n")
	}

	if len(resp.Items) > 0 {
		b.WriteString("\nRelevant memory items:\n")
		for _, item := range resp.Items[:minInt(maxItems, len(resp.Items))] {
			summary := strings.TrimSpace(item.Summary)
			if summary == "" {
				continue
			}
			if item.MemoryType != "" {
				b.WriteString("- (")
				b.WriteString(item.MemoryType)
				b.WriteString(") ")
			} else {
				b.WriteString("- ")
			}
			b.WriteString(summary)
			if ts := strings.TrimSpace(item.CreatedAt); ts != "" {
				b.WriteString(" [recorded: ")
				b.WriteString(ts)
				b.WriteString("]")
			}
			b.WriteString("\n")
		}
	}

	if len(resp.Categories) > 0 {
		b.WriteString("\nRelevant categories:\n")
		for _, category := range resp.Categories[:minInt(maxItems, len(resp.Categories))] {
			b.WriteString("- ")
			name := strings.TrimSpace(category.Name)
			if name == "" {
				name = "uncategorized"
			}
			b.WriteString(name)
			summary := strings.TrimSpace(category.Summary)
			if summary != "" {
				b.WriteString(": ")
				b.WriteString(summary)
			}
			if ts := strings.TrimSpace(category.CreatedAt); ts != "" {
				b.WriteString(" [recorded: ")
				b.WriteString(ts)
				b.WriteString("]")
			}
			b.WriteString("\n")
		}
	}

	if len(resp.Resources) > 0 {
		b.WriteString("\nRelated resources:\n")
		for _, resource := range resp.Resources[:minInt(maxItems, len(resp.Resources))] {
			label := strings.TrimSpace(resource.Caption)
			if label == "" {
				label = strings.TrimSpace(resource.URL)
			}
			if label == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString("\n")
		}
	}

	return strings.TrimSpace(b.String())
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
