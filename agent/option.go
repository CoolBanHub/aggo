package agent

import (
	"context"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
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

func WithSubAgent(agents []adk.Agent) Option {
	return func(agent *Agent) {
		agent.subAgents = agents
	}
}

func WithMaxStep(maxStep int) Option {
	return func(agent *Agent) {
		agent.maxStep = maxStep
	}
}

type chatOptions struct {
	composeOptions     []compose.Option
	userID             string
	sessionID          string
	tools              []tool.BaseTool
	userMessageSuffix  string
	adkAgentRunOptions []adk.AgentRunOption
}

type ChatOption func(*chatOptions)

func WithChatTools(tools []tool.BaseTool) ChatOption {
	return func(co *chatOptions) {
		co.tools = tools
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

// WithUserMessageSuffix 添加用户消息后缀
func WithUserMessageSuffix(suffix string) ChatOption {
	return func(co *chatOptions) {
		co.userMessageSuffix = suffix
	}
}

func WithChatComposeOptions(composeOptions []compose.Option) ChatOption {
	return func(co *chatOptions) {
		co.composeOptions = append(co.composeOptions, composeOptions...)
	}
}

func WithChatOptions(chatOpts []ChatOption) adk.AgentRunOption {
	return adk.WrapImplSpecificOptFn(func(t *chatOptions) {
		for _, opt := range chatOpts {
			opt(t)
		}
		return
	})
}
