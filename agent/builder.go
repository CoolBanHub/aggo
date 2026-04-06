package agent

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// AgentBuilder 辅助构建 adk.Agent，不重新实现任何执行方法，只做配置透传
type AgentBuilder struct {
	name         string
	description  string
	instruction  string
	cm           model.ToolCallingChatModel
	tools        []tool.BaseTool
	middlewares  []adk.ChatModelAgentMiddleware
	maxStep      int
	subAgents    []adk.Agent
	subAgentMode string
}

// NewAgentBuilder 创建 AgentBuilder
func NewAgentBuilder(cm model.ToolCallingChatModel) *AgentBuilder {
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
func (b *AgentBuilder) WithMemory(provider memory.MemoryProvider) *AgentBuilder {
	b.middlewares = append(b.middlewares, memory.NewMemoryMiddleware(provider))
	return b
}

// WithMiddlewares 添加自定义 Middleware
func (b *AgentBuilder) WithMiddlewares(mw ...adk.ChatModelAgentMiddleware) *AgentBuilder {
	b.middlewares = append(b.middlewares, mw...)
	return b
}

// WithMaxStep 设置最大迭代次数
func (b *AgentBuilder) WithMaxStep(maxStep int) *AgentBuilder {
	b.maxStep = maxStep
	return b
}

// WithSubAgents 设置子 Agents
func (b *AgentBuilder) WithSubAgents(mode string, agents ...adk.Agent) *AgentBuilder {
	b.subAgentMode = mode
	b.subAgents = append(b.subAgents, agents...)
	return b
}

// Build 构建 adk.Agent
func (b *AgentBuilder) Build(ctx context.Context) (adk.Agent, error) {
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
	// framework-injected sections (sub-agent transfer, skills) with XML tags.
	handlers := make([]adk.ChatModelAgentMiddleware, len(b.middlewares), len(b.middlewares)+1)
	copy(handlers, b.middlewares)
	handlers = append(handlers, &instructionFormatter{})

	mainAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
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
	if err != nil {
		return nil, err
	}

	// 如果有子 agents，设置它们
	if len(b.subAgents) > 0 {
		subAgents := b.subAgents
		if b.subAgentMode == SubAgentModeSupervisor {
			subAgents = make([]adk.Agent, 0, len(b.subAgents))
			supervisorName := mainAgent.Name(ctx)
			for _, subAgent := range b.subAgents {
				subAgents = append(subAgents, adk.AgentWithDeterministicTransferTo(ctx, &adk.DeterministicTransferConfig{
					Agent:        subAgent,
					ToAgentNames: []string{supervisorName},
				}))
			}
		}
		return adk.SetSubAgents(ctx, mainAgent, subAgents)
	}

	return mainAgent, nil
}
