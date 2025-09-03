package agent

import (
	"context"
	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
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

func WithSpecialists(specialist []*host.Specialist) Option {
	return func(agent *Agent) {
		agent.specialist = specialist
	}
}

type chatOptions struct {
	composeOptions []compose.Option
	userID         string
	sessionID      string
}

type ChatOption func(*chatOptions)

func WithChatTools(tools []tool.BaseTool) ChatOption {
	return func(co *chatOptions) {
		toolInfos, _ := genToolInfos(context.Background(), tools)
		co.composeOptions = append(co.composeOptions,
			compose.WithToolsNodeOption(compose.WithToolList(tools...)),
			compose.WithChatModelOption(model.WithTools(toolInfos)),
		)
	}
}

func WithChatUserID(userID string) ChatOption {
	return func(co *chatOptions) {
		co.userID = userID
	}
}

func WithChatSessionID(sessionID string) ChatOption {
	return func(co *chatOptions) {
		co.sessionID = sessionID
	}
}
