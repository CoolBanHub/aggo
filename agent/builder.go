package agent

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/memory"
	memorytool "github.com/CoolBanHub/aggo/tools/memory"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// AgentBuilder 辅助构建 adk.Agent，不重新实现任何执行方法，只做配置透传
type AgentBuilder struct {
	name        string
	description string
	instruction string
	cm          model.AgenticModel
	tools       []tool.BaseTool
	middlewares []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]
	maxStep     int
}

// NewAgentBuilder 创建 AgentBuilder
func NewAgentBuilder(cm model.AgenticModel) *AgentBuilder {
	return &AgentBuilder{
		cm: cm,
	}
}

// WithName 设置 Agent 名称
func (b *AgentBuilder) WithName(name string) *AgentBuilder {
	b.name = name
	return b
}

// WithDescription 设置 Agent 描述
func (b *AgentBuilder) WithDescription(desc string) *AgentBuilder {
	b.description = desc
	return b
}

// WithInstruction 设置系统提示词
func (b *AgentBuilder) WithInstruction(instruction string) *AgentBuilder {
	b.instruction = instruction
	return b
}

// WithTools 设置工具列表
func (b *AgentBuilder) WithTools(tools ...tool.BaseTool) *AgentBuilder {
	b.tools = append(b.tools, tools...)
	return b
}

// WithMemoryMiddleware 添加 MemoryMiddleware（同时注册 MemoryManager）
func (b *AgentBuilder) WithMemoryMiddleware(mm *memory.MemoryMiddleware) *AgentBuilder {
	b.middlewares = append(b.middlewares, mm)
	return b
}

// WithMemory adds a memory provider and creates the middleware automatically.
// 如果 provider 实现了 memory.UserMemoryEventSearcher（事件检索模式），
// 同时会自动注入 search_user_memory 工具，让 Agent 主动检索更早的事件记忆。
func (b *AgentBuilder) WithMemory(provider memory.MemoryProvider) *AgentBuilder {
	b.middlewares = append(b.middlewares, memory.NewMemoryMiddleware(provider))
	if searcher, ok := provider.(memory.UserMemoryEventSearcher); ok {
		if t, err := memorytool.SearchUserMemoryTool(searcher); err == nil && t != nil {
			b.tools = append(b.tools, t)
		}
	}
	return b
}

// WithMiddlewares 添加自定义 Middleware
func (b *AgentBuilder) WithMiddlewares(mw ...adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]) *AgentBuilder {
	b.middlewares = append(b.middlewares, mw...)
	return b
}

// WithMaxStep 设置最大迭代次数
func (b *AgentBuilder) WithMaxStep(maxStep int) *AgentBuilder {
	b.maxStep = maxStep
	return b
}

// Build 构建 adk.Agent
func (b *AgentBuilder) Build(ctx context.Context) (adk.TypedAgent[*schema.AgenticMessage], error) {
	if b.cm == nil {
		return nil, fmt.Errorf("chat model 不能为空")
	}

	name := b.name
	if name == "" {
		name = "adk agent"
	}
	description := b.description
	if description == "" {
		description = "adk agent"
	}

	// Append instruction formatter as the last handler to restructure
	// framework-injected skill sections with XML tags.
	handlers := make([]adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], len(b.middlewares), len(b.middlewares)+1)
	copy(handlers, b.middlewares)
	handlers = append(handlers, &instructionFormatter{})

	return adk.NewTypedChatModelAgent[*schema.AgenticMessage](ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:        name,
		Description: description,
		Instruction: b.instruction,
		Model:       b.cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: b.tools,
			},
		},
		MaxIterations: b.maxStep,
		Handlers:      handlers,
	})
}
