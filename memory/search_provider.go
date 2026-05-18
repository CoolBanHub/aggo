package memory

import (
	"context"

	"github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/CoolBanHub/aggo/memory/memoryevent"
)

type SearchableProvider interface {
	MemoryProvider
	SearchMessages(ctx context.Context, q *search.SearchQuery) ([]*search.SearchHit, error)
}

// UserMemoryEventSearcher 让 search_user_memory 工具可以独立于具体 provider 实现，
// 任何能够检索用户事件级记忆的 provider 都可以实现该接口并被工具识别。
//
// 参数与返回值使用中性的 memoryevent.Event / memoryevent.Query；
// memory/builtin 通过 type alias 保持后向兼容。
type UserMemoryEventSearcher interface {
	SearchUserMemoryEvents(ctx context.Context, query *memoryevent.Query) ([]*memoryevent.Event, error)
	ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*memoryevent.Event, error)
}
