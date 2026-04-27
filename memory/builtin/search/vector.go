package search

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
)

type VectorSearcher struct {
	embedder embedding.Embedder
	store    VectorStore
	source   MessageSource
}

func NewVectorSearcher(embedder embedding.Embedder, store VectorStore, source MessageSource) (*VectorSearcher, error) {
	if embedder == nil {
		return nil, errors.New("embedder is required")
	}
	if store == nil {
		return nil, errors.New("vector store is required")
	}
	if source == nil {
		return nil, errors.New("message source is required")
	}
	return &VectorSearcher{
		embedder: embedder,
		store:    store,
		source:   source,
	}, nil
}

func (s *VectorSearcher) Search(ctx context.Context, q *SearchQuery) ([]*SearchHit, error) {
	if s == nil {
		return nil, errors.New("vector searcher is nil")
	}
	if q == nil {
		return nil, errors.New("search query is nil")
	}

	queryText := strings.TrimSpace(q.Query)
	if queryText == "" {
		queryText = strings.Join(q.Keywords, " ")
	}
	if queryText == "" {
		return nil, errors.New("query is required for vector search")
	}

	vectors, err := s.embedder.EmbedStrings(ctx, []string{queryText})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, nil
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 5
	}
	return s.store.Search(ctx, q, vectors[0], limit)
}

func (s *VectorSearcher) Index(ctx context.Context, msg *Message) error {
	if s == nil {
		return errors.New("vector searcher is nil")
	}
	if msg == nil {
		return errors.New("message is nil")
	}

	text := SearchText(msg)
	if text == "" {
		return nil
	}

	vectors, err := s.embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		return err
	}
	if len(vectors) == 0 {
		return nil
	}
	return s.store.Upsert(ctx, msg, vectors[0])
}

func (s *VectorSearcher) Reindex(ctx context.Context, sessionID, userID string) error {
	if s == nil {
		return errors.New("vector searcher is nil")
	}

	msgs, err := s.source.ListMessages(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if err := s.Index(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
