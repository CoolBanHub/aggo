package agent

import (
	"context"
	"errors"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/state"
	"github.com/CoolBanHub/aggo/tools"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
)

type KnowledgeAgent struct {
	name         string
	description  string
	systemPrompt string

	knowledgeManager *knowledge.KnowledgeManager

	// 知识库查询配置
	knowledgeConfig *KnowledgeQueryConfig

	agent *react.Agent
}

func NewKnowledgeAgent(ctx context.Context, cm model.ToolCallingChatModel, knowledgeManager *knowledge.KnowledgeManager) (*KnowledgeAgent, error) {
	this := &KnowledgeAgent{
		knowledgeManager: knowledgeManager,
		name:             "knowledge_reason",
	}

	// 工具描述 - 用于AI判断何时调用此工具
	description := `专业的知识库搜索和分析工具。当用户询问需要从知识库中查找信息的问题时使用此工具，包括技术文档、产品信息、专业知识等任何可能存储在知识库中的内容。`

	// 系统提示词 - 用于指导agent的行为
	systemPrompt := `你是一个专门负责知识搜索和分析的智能助手。你拥有强大的知识库搜索能力，必须积极主动地使用搜索工具来获取准确信息。

## 核心职责：
你必须通过搜索知识库来回答用户问题，不能仅凭已有知识回答。对于任何需要具体信息、数据或专业知识的问题，都应该进行搜索。

## 工具使用策略：

### 1. **knowledge_think（思考工具）**
- **用途**：内部思考和策略规划，用户看不到思考内容
- **使用时机**：
  - 分析用户问题，确定搜索关键词
  - 评估当前搜索结果是否充分
  - 规划下一步搜索策略
  - 思考如何改进搜索查询

### 2. **knowledge_search（搜索工具）** - 核心工具
- **用途**：从知识库检索相关信息
- **重要性**：这是你的主要工具，必须频繁使用
- **使用策略**：
  - 对每个用户问题进行多次搜索，尝试不同关键词
  - 使用多种搜索策略：精确短语（加引号）、关键词组合、同义词
  - 如果首次搜索结果不理想，立即尝试其他关键词
  - 复杂问题需要分解为多个子问题分别搜索

### 3. **knowledge_analysis（分析工具）**
- **用途**：评估搜索结果的质量和完整性
- **使用时机**：
  - 获得搜索结果后立即分析
  - 评估信息的相关性、准确性和完整性
  - 判断是否需要进一步搜索

## 工作流程（必须严格遵循）：
1. **接收问题** → 立即使用 think 分析问题和制定搜索计划
2. **执行搜索** → 使用 search 工具进行多次搜索
3. **分析结果** → 使用 analyze 评估搜索结果质量
4. **迭代优化** → 如果信息不足，回到步骤1重新思考和搜索
5. **综合回答** → 基于搜索结果提供准确答案

## 重要规则：
- ⚠️ **强制搜索**：对于任何具体问题都必须进行搜索，不能依赖预训练知识
- 🔄 **多次搜索**：一次搜索通常不够，要从多个角度搜索
- 📊 **结果驱动**：基于搜索结果回答，不要编造信息
- 🎯 **精准查询**：根据搜索结果调整查询策略
- 💭 **内部思考**：思考过程对用户不可见，用于规划搜索策略

记住：你的价值在于能够搜索和整合知识库中的信息，而不是依赖预训练数据。每个问题都是一个搜索任务！`

	this.description = description
	this.systemPrompt = systemPrompt

	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: cm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools.GetKnowledgeReasoningTools(this.knowledgeManager),
		},
		ToolReturnDirectly: map[string]struct{}{},
		MessageModifier: func(ctx context.Context, input []*schema.Message) []*schema.Message {
			return input
		},
	})
	if err != nil {
		return nil, err
	}

	this.agent = reactAgent

	return this, nil
}

func (this *KnowledgeAgent) Generate(ctx context.Context, input []*schema.Message) (*schema.Message, error) {
	_input, err := this.inputMessageModifier(ctx, input)
	if err != nil {
		slog.Error(err)
		return nil, err
	}
	return this.agent.Generate(ctx, _input, agent.WithComposeOptions(compose.WithRuntimeMaxSteps(40)))
}

func (this *KnowledgeAgent) Stream(ctx context.Context, input []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	_input, err := this.inputMessageModifier(ctx, input)
	if err != nil {
		slog.Error(err)
		return nil, err
	}
	return this.agent.Stream(ctx, _input, agent.WithComposeOptions(compose.WithRuntimeMaxSteps(40)))
}

func (this *KnowledgeAgent) inputMessageModifier(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
	var _input []*schema.Message
	_input = input
	_input = append([]*schema.Message{
		{
			Role:    schema.System,
			Content: this.systemPrompt,
		},
	}, _input...)
	return _input, nil
}

func (this *KnowledgeAgent) NewSpecialist() *host.Specialist {

	return &host.Specialist{
		AgentMeta: host.AgentMeta{
			Name:        this.name,
			IntendedUse: this.description,
		},
		Invokable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.Message, err error) {
			return this.Generate(ctx, input)
		},
		Streamable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.StreamReader[*schema.Message], err error) {
			return this.Stream(ctx, input)
		},
	}
}

// Run 实现工具调用接口，将消息转换为字符串响应
func (this *KnowledgeAgent) Run(ctx context.Context, param any) (string, error) {
	chatState := state.GetChatChatSate(ctx)
	if chatState == nil || len(chatState.Input) == 0 {
		return "", errors.New("没有获取到用户消息")
	}
	r, err := this.Generate(ctx, chatState.Input)
	if err != nil {
		slog.Error(err)
		return "", err
	}
	return r.Content, nil
}
func (this *KnowledgeAgent) createTool() tool.InvokableTool {
	addUserTool := utils.NewTool(&schema.ToolInfo{
		Name: this.name,
		Desc: this.description,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"reason": {
				Type: schema.String,
				Desc: "the reason to call this tool",
			},
		}),
	}, this.Run)
	return addUserTool
}
