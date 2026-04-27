package memory

import (
	"context"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
)

type SearchableProvider interface {
	MemoryProvider
	SearchMessages(ctx context.Context, q *builtinsearch.SearchQuery) ([]*builtinsearch.SearchHit, error)
}
