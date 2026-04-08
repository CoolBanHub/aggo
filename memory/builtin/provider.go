package builtin

import (
	"context"

	"github.com/cloudwego/eino/components/model"
)

type AsyncTaskContextBuilder func(taskType, userID, sessionID string) context.Context

// ProviderConfig is the config for the builtin memory provider.
type ProviderConfig struct {
	ChatModel    model.ToolCallingChatModel
	Storage      MemoryStorage
	MemoryConfig *MemoryConfig

	AsyncTaskContextBuilder AsyncTaskContextBuilder
}
