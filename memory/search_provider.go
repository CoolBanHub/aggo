package memory

import (
	"context"

	"github.com/CoolBanHub/aggo/memory/builtin"
	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
)

type SearchableProvider interface {
	MemoryProvider
	SearchMessages(ctx context.Context, q *builtinsearch.SearchQuery) ([]*builtinsearch.SearchHit, error)
}

// UserMemoryEventSearcher 让 search_user_memory 工具可以独立于具体 provider 实现，
// 任何能够检索用户事件级记忆的 provider 都可以实现该接口并被工具识别。
type UserMemoryEventSearcher interface {
	SearchUserMemoryEvents(ctx context.Context, query *builtin.UserMemoryEventQuery) ([]*builtin.UserMemoryEvent, error)
	ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*builtin.UserMemoryEvent, error)
}
