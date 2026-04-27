package search

import (
	"context"
	"testing"
	"time"
)

type fakeKeywordStore struct {
	messages []*Message
}

func (s *fakeKeywordStore) SearchMessagesByKeywords(_ context.Context, q *SearchQuery) ([]*Message, error) {
	out := make([]*Message, 0, len(s.messages))
	for _, msg := range s.messages {
		if _, ok := MatchesKeywords(SearchText(msg), q.Keywords, q.Match); ok {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (s *fakeKeywordStore) ListMessages(context.Context, string, string) ([]*Message, error) {
	return s.messages, nil
}

func TestKeywordSearcherSearchBuildsSnippetAndScore(t *testing.T) {
	now := time.Now()
	searcher := NewKeywordSearcher(&fakeKeywordStore{
		messages: []*Message{
			{
				ID:        "m1",
				SessionID: "s1",
				UserID:    "u1",
				Role:      "user",
				Content:   "用户反馈登录接口报错 500，需要排查日志。",
				CreatedAt: now,
			},
			{
				ID:        "m2",
				SessionID: "s1",
				UserID:    "u1",
				Role:      "assistant",
				Content:   "我们已经修复了支付超时问题。",
				CreatedAt: now.Add(-time.Minute),
			},
		},
	})

	hits, err := searcher.Search(context.Background(), &SearchQuery{
		SessionID: "s1",
		UserID:    "u1",
		Keywords:  []string{"登录", "报错"},
		Match:     MatchAll,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if hits[0].Message.ID != "m1" {
		t.Fatalf("unexpected hit id: %s", hits[0].Message.ID)
	}
	if hits[0].Score != 2 {
		t.Fatalf("score = %v, want 2", hits[0].Score)
	}
	if hits[0].Snippet == "" {
		t.Fatalf("snippet should not be empty")
	}
}
