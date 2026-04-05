package builtin

import "github.com/cloudwego/eino/components/model"

// ProviderConfig is the config for the builtin memory provider.
type ProviderConfig struct {
	ChatModel    model.ToolCallingChatModel
	Storage      MemoryStorage
	MemoryConfig *MemoryConfig
}
