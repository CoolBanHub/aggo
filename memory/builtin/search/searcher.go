package search

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type SearchMode string

const (
	ModeKeyword SearchMode = "keyword"
	ModeVector  SearchMode = "vector"
	ModeHybrid  SearchMode = "hybrid"
)

const (
	MatchAny = "any"
	MatchAll = "all"
)

const (
	HybridStrategyRRF      = "rrf"
	HybridStrategyWeighted = "weighted"
)

type Message struct {
	ID        string
	SessionID string
	UserID    string
	Role      string
	Content   string
	Parts     []schema.MessageInputPart
	CreatedAt time.Time
}

type SearchQuery struct {
	SessionID string
	UserID    string

	Since *time.Time
	Until *time.Time
	Role  string
	Limit int

	Keywords []string
	Match    string

	Query string
}

type SearchHit struct {
	Message *Message
	Score   float64
	Snippet string
}

type Searcher interface {
	Search(ctx context.Context, q *SearchQuery) ([]*SearchHit, error)
	Index(ctx context.Context, msg *Message) error
	Reindex(ctx context.Context, sessionID, userID string) error
}

type MessageSource interface {
	ListMessages(ctx context.Context, sessionID, userID string) ([]*Message, error)
}

type KeywordStore interface {
	MessageSource
	SearchMessagesByKeywords(ctx context.Context, q *SearchQuery) ([]*Message, error)
}

type VectorStore interface {
	Upsert(ctx context.Context, msg *Message, vector []float64) error
	Search(ctx context.Context, q *SearchQuery, vector []float64, limit int) ([]*SearchHit, error)
}

type HybridConfig struct {
	Strategy string
	RRFK     int
	Weights  HybridWeights
}

type HybridWeights struct {
	Keyword float64
	Vector  float64
}

func CloneMessage(msg *Message) *Message {
	if msg == nil {
		return nil
	}
	cloned := *msg
	if len(msg.Parts) > 0 {
		cloned.Parts = append([]schema.MessageInputPart(nil), msg.Parts...)
	}
	return &cloned
}

func NormalizeMatch(match string) string {
	match = strings.TrimSpace(strings.ToLower(match))
	if match == MatchAll {
		return MatchAll
	}
	return MatchAny
}

func InferKeywords(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	splitter := strings.NewReplacer(
		"\n", " ",
		"\t", " ",
		"，", " ",
		"。", " ",
		"、", " ",
		"；", " ",
		";", " ",
		",", " ",
		"|", " ",
	)
	fields := strings.Fields(splitter.Replace(text))
	if len(fields) == 0 {
		return []string{text}
	}

	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func SearchText(msg *Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Content) != "" {
		return strings.TrimSpace(msg.Content)
	}

	texts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.Type == schema.ChatMessagePartTypeText && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(texts, "\n")
}

func MatchesKeywords(text string, keywords []string, match string) (int, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0, false
	}

	filtered := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			filtered = append(filtered, keyword)
		}
	}
	if len(filtered) == 0 {
		return 0, false
	}

	matched := 0
	for _, keyword := range filtered {
		if strings.Contains(text, keyword) {
			matched++
			continue
		}
		if NormalizeMatch(match) == MatchAll {
			return matched, false
		}
	}

	if NormalizeMatch(match) == MatchAll {
		return matched, matched == len(filtered)
	}
	return matched, matched > 0
}
