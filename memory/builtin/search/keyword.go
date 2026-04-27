package search

import (
	"context"
	"errors"
	"strings"
)

type KeywordSearcher struct {
	store KeywordStore
}

func NewKeywordSearcher(store KeywordStore) *KeywordSearcher {
	return &KeywordSearcher{store: store}
}

func (s *KeywordSearcher) Search(ctx context.Context, q *SearchQuery) ([]*SearchHit, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("keyword searcher is not configured")
	}
	if q == nil {
		return nil, errors.New("search query is nil")
	}

	query := *q
	if query.Limit <= 0 {
		query.Limit = 5
	}
	query.Match = NormalizeMatch(query.Match)
	if len(query.Keywords) == 0 {
		query.Keywords = InferKeywords(query.Query)
	}
	if len(query.Keywords) == 0 {
		return nil, errors.New("keywords are required")
	}

	msgs, err := s.store.SearchMessagesByKeywords(ctx, &query)
	if err != nil {
		return nil, err
	}

	hits := make([]*SearchHit, 0, len(msgs))
	for _, msg := range msgs {
		text := SearchText(msg)
		matched, ok := MatchesKeywords(text, query.Keywords, query.Match)
		if !ok {
			continue
		}
		hits = append(hits, &SearchHit{
			Message: CloneMessage(msg),
			Score:   float64(matched),
			Snippet: buildSnippet(text, query.Keywords),
		})
	}
	return hits, nil
}

func (s *KeywordSearcher) Index(context.Context, *Message) error {
	return nil
}

func (s *KeywordSearcher) Reindex(context.Context, string, string) error {
	return nil
}

func buildSnippet(text string, keywords []string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	runes := []rune(text)
	lowerRunes := []rune(strings.ToLower(text))
	start := 0
	found := false

	for _, keyword := range keywords {
		keyword = strings.TrimSpace(strings.ToLower(keyword))
		if keyword == "" {
			continue
		}
		target := []rune(keyword)
		for i := 0; i <= len(lowerRunes)-len(target); i++ {
			if string(lowerRunes[i:i+len(target)]) == string(target) {
				start = i
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		if len(runes) <= 120 {
			return text
		}
		return string(runes[:120]) + "..."
	}

	begin := start - 40
	if begin < 0 {
		begin = 0
	}
	end := start + 80
	if end > len(runes) {
		end = len(runes)
	}

	snippet := string(runes[begin:end])
	if begin > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}
	return snippet
}
