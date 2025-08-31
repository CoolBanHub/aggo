package agent

import (
	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/components/tool"
)

type Option func(*Agent)

func WithName(name string) Option {
	return func(agent *Agent) {
		agent.name = name
	}
}

func WithDescription(description string) Option {
	return func(agent *Agent) {
		agent.description = description
	}
}

func WithTools(tools []tool.BaseTool) Option {
	return func(agent *Agent) {
		agent.tools = tools
	}
}

func WithKnowledgeManager(knowledgeManager *knowledge.KnowledgeManager) Option {
	return func(agent *Agent) {
		agent.knowledgeManager = knowledgeManager
	}
}

func WithSessionID(sessionID string) Option {
	return func(agent *Agent) {
		agent.sessionID = sessionID
	}
}

func WithUserID(userID string) Option {
	return func(agent *Agent) {
		agent.userID = userID
	}
}

func WithMemoryManager(memoryManager *memory.MemoryManager) Option {
	return func(agent *Agent) {
		agent.memoryManager = memoryManager
	}
}

func WithSystemPrompt(systemPrompt string) Option {
	return func(agent *Agent) {
		agent.systemPrompt = systemPrompt
	}
}

func WithKnowledgeQueryConfig(config *KnowledgeQueryConfig) Option {
	return func(agent *Agent) {
		agent.knowledgeConfig = config
	}
}
