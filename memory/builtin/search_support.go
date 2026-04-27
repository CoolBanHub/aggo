package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
)

type storageSearchAdapter struct {
	storage MemoryStorage
}

func (a *storageSearchAdapter) SearchMessagesByKeywords(ctx context.Context, q *builtinsearch.SearchQuery) ([]*builtinsearch.Message, error) {
	if searchStore, ok := a.storage.(SearchMessageStorage); ok {
		msgs, err := searchStore.SearchMessagesByKeywords(ctx, q)
		if err != nil {
			return nil, err
		}
		return toSearchMessages(msgs), nil
	}

	msgs, err := a.ListMessages(ctx, q.SessionID, q.UserID)
	if err != nil {
		return nil, err
	}

	keywords := q.Keywords
	if len(keywords) == 0 {
		keywords = builtinsearch.InferKeywords(q.Query)
	}
	if len(keywords) == 0 {
		return []*builtinsearch.Message{}, nil
	}

	filtered := make([]*builtinsearch.Message, 0, len(msgs))
	for _, msg := range msgs {
		if role := strings.TrimSpace(q.Role); role != "" && msg.Role != role {
			continue
		}
		if q.Since != nil && msg.CreatedAt.Before(*q.Since) {
			continue
		}
		if q.Until != nil && msg.CreatedAt.After(*q.Until) {
			continue
		}
		if _, ok := builtinsearch.MatchesKeywords(builtinsearch.SearchText(msg), keywords, q.Match); !ok {
			continue
		}
		filtered = append(filtered, msg)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 5
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (a *storageSearchAdapter) ListMessages(ctx context.Context, sessionID, userID string) ([]*builtinsearch.Message, error) {
	msgs, err := a.storage.GetMessages(ctx, sessionID, userID, 0)
	if err != nil {
		return nil, err
	}
	return toSearchMessages(msgs), nil
}

func normalizeSearchConfig(cfg *SearchConfig) *SearchConfig {
	if cfg == nil {
		return &SearchConfig{Mode: builtinsearch.ModeKeyword}
	}
	if strings.TrimSpace(string(cfg.Mode)) == "" {
		cfg.Mode = builtinsearch.ModeKeyword
	}
	return cfg
}

func newSearcher(storage MemoryStorage, cfg *SearchConfig) (builtinsearch.Searcher, error) {
	cfg = normalizeSearchConfig(cfg)
	adapter := &storageSearchAdapter{storage: storage}
	keywordSearcher := builtinsearch.NewKeywordSearcher(adapter)

	switch cfg.Mode {
	case builtinsearch.ModeKeyword:
		return keywordSearcher, nil
	case builtinsearch.ModeVector:
		return newVectorSearcher(storage, cfg, adapter)
	case builtinsearch.ModeHybrid:
		vectorSearcher, err := newVectorSearcher(storage, cfg, adapter)
		if err != nil {
			return nil, err
		}
		return builtinsearch.NewHybridSearcher(keywordSearcher, vectorSearcher, cfg.Hybrid), nil
	default:
		return nil, fmt.Errorf("unsupported search mode: %s", cfg.Mode)
	}
}

func newVectorSearcher(storage MemoryStorage, cfg *SearchConfig, source builtinsearch.MessageSource) (builtinsearch.Searcher, error) {
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("search embedder is required for mode %s", cfg.Mode)
	}

	store := cfg.VectorStore
	if store == nil {
		gormStore, ok := storage.(GormConversationStorage)
		if !ok {
			return nil, fmt.Errorf("default gorm vector store requires gorm-backed storage")
		}
		defaultStore, err := builtinsearch.NewGormVectorStore(gormStore.ConversationDB(), gormStore.ConversationMessageTableName())
		if err != nil {
			return nil, err
		}
		store = defaultStore
	}

	return builtinsearch.NewVectorSearcher(cfg.Embedder, store, source)
}

func toSearchMessage(msg *ConversationMessage) *builtinsearch.Message {
	if msg == nil {
		return nil
	}
	return &builtinsearch.Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		UserID:    msg.UserID,
		Role:      msg.Role,
		Content:   msg.Content,
		Parts:     append([]schema.MessageInputPart(nil), msg.Parts...),
		CreatedAt: msg.CreatedAt,
	}
}

func toSearchMessages(msgs []*ConversationMessage) []*builtinsearch.Message {
	out := make([]*builtinsearch.Message, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, toSearchMessage(msg))
	}
	return out
}

func cloneConversationMessage(msg *ConversationMessage) *ConversationMessage {
	if msg == nil {
		return nil
	}
	cloned := *msg
	if len(msg.Parts) > 0 {
		cloned.Parts = append([]schema.MessageInputPart(nil), msg.Parts...)
	}
	return &cloned
}

func (m *MemoryManager) SearchMessages(ctx context.Context, q *builtinsearch.SearchQuery) ([]*builtinsearch.SearchHit, error) {
	if m.searcher == nil {
		return nil, fmt.Errorf("memory searcher is not configured")
	}
	if q == nil {
		return nil, fmt.Errorf("search query is nil")
	}
	return m.searcher.Search(ctx, q)
}

func (m *MemoryManager) ReindexConversation(ctx context.Context, sessionID, userID string) error {
	if m.searcher == nil {
		return nil
	}
	return m.searcher.Reindex(ctx, sessionID, userID)
}

func formatHistoryMessageTime(raw any) string {
	createdAt, ok := raw.(string)
	if !ok || strings.TrimSpace(createdAt) == "" {
		return ""
	}
	ts, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	return ts.Format("2006-01-02 15:04")
}

func PrefixHistoryTimestamp(msg *schema.Message) *schema.Message {
	if msg == nil || strings.TrimSpace(msg.Content) == "" {
		return msg
	}
	if msg.Extra == nil {
		return msg
	}
	formatted := formatHistoryMessageTime(msg.Extra[MessageExtraCreatedAtKey])
	if formatted == "" {
		return msg
	}
	cloned := *msg
	cloned.Content = fmt.Sprintf("[%s] %s", formatted, msg.Content)
	return &cloned
}

func enqueueIndexTask(manager *MemoryManager, msg *ConversationMessage) {
	if manager == nil || manager.searcher == nil {
		return
	}

	searchCfg := normalizeSearchConfig(manager.config.Search)
	if searchCfg.AsyncIndex {
		submitted := manager.submitAsyncTask(asyncTask{
			taskType:  "index",
			userID:    msg.UserID,
			sessionID: msg.SessionID,
			message:   cloneConversationMessage(msg),
		})
		if !submitted {
			slog.Errorf("警告: 搜索索引队列已满，跳过处理: sessionID=%s, userID=%s", msg.SessionID, msg.UserID)
		}
		return
	}

	if err := manager.searcher.Index(context.Background(), toSearchMessage(msg)); err != nil {
		slog.Errorf("同步建立搜索索引失败: sessionID=%s, userID=%s, err=%v", msg.SessionID, msg.UserID, err)
	}
}
