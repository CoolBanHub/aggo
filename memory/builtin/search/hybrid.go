package search

import (
	"context"
	"errors"
	"sort"
	"strings"
)

type HybridSearcher struct {
	keyword Searcher
	vector  Searcher
	config  HybridConfig
}

func NewHybridSearcher(keyword Searcher, vector Searcher, cfg HybridConfig) *HybridSearcher {
	if strings.TrimSpace(cfg.Strategy) == "" {
		cfg.Strategy = HybridStrategyRRF
	}
	if cfg.RRFK <= 0 {
		cfg.RRFK = 60
	}
	if cfg.Weights.Keyword <= 0 {
		cfg.Weights.Keyword = 1
	}
	if cfg.Weights.Vector <= 0 {
		cfg.Weights.Vector = 1
	}
	return &HybridSearcher{
		keyword: keyword,
		vector:  vector,
		config:  cfg,
	}
}

func (s *HybridSearcher) Search(ctx context.Context, q *SearchQuery) ([]*SearchHit, error) {
	if s == nil {
		return nil, errors.New("hybrid searcher is nil")
	}
	if q == nil {
		return nil, errors.New("search query is nil")
	}

	keywordQuery := *q
	if len(keywordQuery.Keywords) == 0 {
		keywordQuery.Keywords = InferKeywords(keywordQuery.Query)
	}

	keywordHits, err := s.keyword.Search(ctx, &keywordQuery)
	if err != nil {
		return nil, err
	}
	vectorHits, err := s.vector.Search(ctx, q)
	if err != nil {
		return nil, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 5
	}
	switch strings.ToLower(strings.TrimSpace(s.config.Strategy)) {
	case HybridStrategyWeighted:
		return weightedFuse(keywordHits, vectorHits, s.config, limit), nil
	default:
		return rrfFuse(keywordHits, vectorHits, s.config.RRFK, limit), nil
	}
}

func (s *HybridSearcher) Index(ctx context.Context, msg *Message) error {
	if s == nil || s.vector == nil {
		return errors.New("hybrid searcher has no vector indexer")
	}
	return s.vector.Index(ctx, msg)
}

func (s *HybridSearcher) Reindex(ctx context.Context, sessionID, userID string) error {
	if s == nil || s.vector == nil {
		return errors.New("hybrid searcher has no vector indexer")
	}
	return s.vector.Reindex(ctx, sessionID, userID)
}

func rrfFuse(keywordHits, vectorHits []*SearchHit, rrfK, limit int) []*SearchHit {
	if rrfK <= 0 {
		rrfK = 60
	}
	merged := make(map[string]*SearchHit)
	scoreByID := make(map[string]float64)
	mergeRankedHits(merged, scoreByID, keywordHits, func(rank int) float64 {
		return 1.0 / float64(rrfK+rank)
	})
	mergeRankedHits(merged, scoreByID, vectorHits, func(rank int) float64 {
		return 1.0 / float64(rrfK+rank)
	})
	return topHits(merged, scoreByID, limit)
}

func weightedFuse(keywordHits, vectorHits []*SearchHit, cfg HybridConfig, limit int) []*SearchHit {
	merged := make(map[string]*SearchHit)
	scoreByID := make(map[string]float64)
	mergeRankedHits(merged, scoreByID, keywordHits, func(rank int) float64 {
		return cfg.Weights.Keyword / float64(rank)
	})
	mergeRankedHits(merged, scoreByID, vectorHits, func(rank int) float64 {
		return cfg.Weights.Vector / float64(rank)
	})
	return topHits(merged, scoreByID, limit)
}

func mergeRankedHits(dst map[string]*SearchHit, scoreByID map[string]float64, hits []*SearchHit, scoreFn func(rank int) float64) {
	for i, hit := range hits {
		if hit == nil || hit.Message == nil || hit.Message.ID == "" {
			continue
		}
		id := hit.Message.ID
		if _, ok := dst[id]; !ok {
			dst[id] = &SearchHit{
				Message: CloneMessage(hit.Message),
				Snippet: hit.Snippet,
			}
		} else if dst[id].Snippet == "" && hit.Snippet != "" {
			dst[id].Snippet = hit.Snippet
		}
		scoreByID[id] += scoreFn(i + 1)
	}
}

func topHits(merged map[string]*SearchHit, scoreByID map[string]float64, limit int) []*SearchHit {
	out := make([]*SearchHit, 0, len(merged))
	for id, hit := range merged {
		hit.Score = scoreByID[id]
		out = append(out, hit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			if out[i].Message == nil || out[j].Message == nil {
				return out[i].Score > out[j].Score
			}
			return out[i].Message.CreatedAt.After(out[j].Message.CreatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
