package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/multiagent/host"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	systemPrompt string
	cm           model.ToolCallingChatModel

	knowledgeManager *knowledge.KnowledgeManager
	// 知识库查询配置
	knowledgeConfig *KnowledgeQueryConfig

	memoryManager *memory.MemoryManager

	//作为子agent的时候必须传
	name        string
	description string

	tools []tool.BaseTool //只支持单agent的模式下使用
	agent *react.Agent

	//多agent的时候 使用
	multiAgent *host.MultiAgent
	specialist []*host.Specialist
}

// KnowledgeQueryConfig 知识库查询配置
type KnowledgeQueryConfig struct {

	// 是否总是查询（不使用关键词触发）
	AlwaysQuery bool
}

func NewAgent(ctx context.Context, cm model.ToolCallingChatModel, opts ...Option) (*Agent, error) {
	this := &Agent{
		cm: cm,
	}

	for _, opt := range opts {
		opt(this)
	}
	//if this.sessionID == "" {
	//	this.sessionID = utils.GetUUIDNoDash()
	//}
	//
	//if this.sessionID != "" && this.userID == "" {
	//	this.userID = this.sessionID
	//}
	if this.knowledgeManager != nil {
		//配置知识库的分析tool
		if this.knowledgeConfig == nil {
			// 默认知识库查询配置
			this.knowledgeConfig = &KnowledgeQueryConfig{
				AlwaysQuery: false,
			}
		}
		knowagent, err := NewKnowledgeAgent(ctx, cm, this.knowledgeManager)
		if err != nil {
			return nil, err
		}

		if !this.knowledgeConfig.AlwaysQuery {
			if len(this.specialist) > 0 {
				this.specialist = append(this.specialist, knowagent.NewSpecialist())
			} else {
				this.tools = append(this.tools, knowagent.createTool())
			}
		}
	}
	if len(this.specialist) > 0 {
		h := &host.Host{
			ToolCallingModel: cm,
			SystemPrompt:     this.systemPrompt,
		}
		multiAgent, err := host.NewMultiAgent(ctx, &host.MultiAgentConfig{
			Host:        *h,
			Specialists: this.specialist,
		})
		if err != nil {
			return nil, err
		}
		this.multiAgent = multiAgent
	} else {
		reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
			ToolCallingModel: cm,
			ToolsConfig: compose.ToolsNodeConfig{
				Tools: this.tools,
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
	}

	return this, nil
}

func (this *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.Message, error) {

	chatOpts := &chatOptions{}
	for _, opt := range opts {
		opt(chatOpts)
	}

	agentOpts := agent.WithComposeOptions(chatOpts.composeOptions...)

	_input, err := this.inputMessageModifier(ctx, input, chatOpts)
	if err != nil {
		return nil, err
	}
	if this.agent != nil && this.systemPrompt != "" {
		ctx = context.WithValue(ctx, "messages", _input[1:])
	} else {
		ctx = context.WithValue(ctx, "messages", _input)
	}
	// 存储用户消息
	if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
		return nil, err
	}

	var response *schema.Message
	if this.multiAgent != nil {
		response, err = this.multiAgent.Generate(ctx, _input, agentOpts)
	} else {
		response, err = this.agent.Generate(ctx, _input, agentOpts)
	}
	if err != nil {
		return nil, err
	}

	// 存储助手回复
	if err := this.storeAssistantMessage(ctx, response, chatOpts); err != nil {
		// 存储失败不应阻断回复，只记录错误
		fmt.Printf("存储助手消息失败: %v\n", err)
	}

	return response, nil
}

func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {

	chatOpts := &chatOptions{}
	for _, opt := range opts {
		opt(chatOpts)
	}

	agentOpts := agent.WithComposeOptions(chatOpts.composeOptions...)

	_input, err := this.inputMessageModifier(ctx, input, chatOpts)
	if err != nil {
		return nil, err
	}
	if this.agent != nil && this.systemPrompt != "" {
		ctx = context.WithValue(ctx, "messages", _input[1:])
	} else {
		ctx = context.WithValue(ctx, "messages", _input)
	}
	// 存储用户消息
	if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
		return nil, err
	}

	//todo 增加stream的callback
	var response *schema.StreamReader[*schema.Message]
	if this.multiAgent != nil {
		response, err = this.multiAgent.Stream(ctx, _input, agentOpts)
	} else {
		response, err = this.agent.Stream(ctx, _input, agentOpts)
	}
	return response, err
}

func (this *Agent) inputMessageModifier(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) ([]*schema.Message, error) {
	var _input []*schema.Message

	if this.memoryManager != nil {
		//input 肯定只有1条

		// 1. 获取历史消息（不包含当前消息）
		historyMessages, err := this.memoryManager.GetMessages(ctx, chatOpts.sessionID, chatOpts.userID, this.memoryManager.GetConfig().MemoryLimit)
		if err != nil {
			return nil, fmt.Errorf("获取历史消息失败: %v", err)
		}

		// 2. 构建消息序列：历史消息 + 当前输入
		_input = append(historyMessages, input...)

		// 3. 添加用户记忆到上下文
		if this.memoryManager.GetConfig().EnableUserMemories {
			userMemories, err := this.memoryManager.GetUserMemories(ctx, chatOpts.userID)
			if err != nil {
				return nil, fmt.Errorf("获取用户记忆失败: %v", err)
			}

			if len(userMemories) > 0 {
				memoryContent := this.formatUserMemories(userMemories)
				memoryMessage := &schema.Message{
					Role:    schema.System,
					Content: memoryContent,
				}
				// 将记忆消息插入到对话消息之前
				_input = append([]*schema.Message{memoryMessage}, _input...)
			}
		}

		// 4. 添加会话摘要到上下文
		if this.memoryManager.GetConfig().EnableSessionSummary {
			sessionSummary, err := this.memoryManager.GetSessionSummary(ctx, chatOpts.sessionID, chatOpts.userID)
			if err != nil {
				return nil, fmt.Errorf("获取会话摘要失败: %v", err)
			}

			if sessionSummary != nil && sessionSummary.Summary != "" {
				summaryMessage := &schema.Message{
					Role:    schema.System,
					Content: fmt.Sprintf("会话背景: %s", sessionSummary.Summary),
				}
				// 将摘要消息插入到最前面
				_input = append([]*schema.Message{summaryMessage}, _input...)
			}
		}
	} else {
		// 没有memory manager时，直接使用输入
		_input = input
	}

	// 6. 检查是否需要查询知识库
	if this.knowledgeManager != nil && len(input) > 0 {
		userInput := input[len(input)-1].Content // 获取最新的用户输入

		// 判断是否需要查询知识库
		if this.shouldQueryKnowledge() {
			// 查询知识库
			knowledgeResults, err := this.queryKnowledge(ctx, userInput)
			if err != nil {
				// 知识库查询失败不应阻断对话，只记录错误
				fmt.Printf("知识库查询失败: %v\n", err)
			} else if len(knowledgeResults) > 0 {
				// 格式化知识库结果
				knowledgeContent := this.formatKnowledgeResults(knowledgeResults)
				knowledgeMessage := &schema.Message{
					Role:    schema.System,
					Content: knowledgeContent,
				}
				// 将知识库信息插入到对话消息之前，但在摘要和记忆之后
				_input = append([]*schema.Message{knowledgeMessage}, _input...)
			}
		}
	}
	// 5. 添加系统提示词到最前面
	if this.agent != nil && this.systemPrompt != "" {
		//单agent的时候需要
		_input = append([]*schema.Message{
			{
				Role:    schema.System,
				Content: this.systemPrompt,
			},
		}, _input...)
	}

	return _input, nil
}

// storeUserMessage 存储用户消息
func (this *Agent) storeUserMessage(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) error {
	if chatOpts.sessionID != "" && this.memoryManager != nil && len(input) > 0 {
		return this.memoryManager.ProcessUserMessage(ctx, chatOpts.userID, chatOpts.sessionID, input[0].Content)
	}
	return nil
}

// storeAssistantMessage 存储助手消息
func (this *Agent) storeAssistantMessage(ctx context.Context, response *schema.Message, chatOpts *chatOptions) error {
	if chatOpts.sessionID != "" && this.memoryManager != nil && response != nil {
		return this.memoryManager.ProcessAssistantMessage(ctx, chatOpts.userID, chatOpts.sessionID, response.Content)
	}
	return nil
}

// formatUserMemories 格式化用户记忆为可读的上下文信息
func (this *Agent) formatUserMemories(memories []*memory.UserMemory) string {
	if len(memories) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("用户个人信息记忆（请在回复中考虑这些信息，提供个性化的响应）:\n")

	for _, mem := range memories {
		builder.WriteString(fmt.Sprintf("- %s\n", mem.Memory))
	}

	return builder.String()
}

// shouldQueryKnowledge 判断是否需要查询知识库
func (this *Agent) shouldQueryKnowledge() bool {
	if this.knowledgeManager == nil || this.knowledgeConfig == nil {
		return false
	}

	// 如果设置了总是查询，则直接返回true
	if this.knowledgeConfig.AlwaysQuery {
		return true
	}

	//让ai调用分析工具自己判断

	return false
}

// queryKnowledge 查询知识库并返回相关文档
func (this *Agent) queryKnowledge(ctx context.Context, query string) ([]*knowledge.SearchResult, error) {
	if this.knowledgeManager == nil {
		return nil, nil
	}

	searchOptions := knowledge.SearchOptions{
		Limit:     this.knowledgeManager.GetConfig().DefaultSearchOptions.Limit,
		Threshold: this.knowledgeManager.GetConfig().DefaultSearchOptions.Threshold,
	}

	results, err := this.knowledgeManager.Search(ctx, query, searchOptions)
	if err != nil {
		return nil, fmt.Errorf("知识库搜索失败: %w", err)
	}

	// 转换为指针类型以便返回
	resultPointers := make([]*knowledge.SearchResult, len(results))
	for i := range results {
		resultPointers[i] = &results[i]
	}

	return resultPointers, nil
}

// formatKnowledgeResults 格式化知识库搜索结果为上下文信息
func (this *Agent) formatKnowledgeResults(results []*knowledge.SearchResult) string {
	if len(results) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("相关知识库信息（请基于以下信息回答用户问题）:\n\n")

	for i, result := range results {
		builder.WriteString(fmt.Sprintf("【知识%d】(相似度: %.2f)\n", i+1, result.Score))
		builder.WriteString(fmt.Sprintf("内容: %s\n", result.Document.Content))

		// 如果有元数据，添加一些有用的信息
		if len(result.Document.Metadata) > 0 {
			if title, ok := result.Document.Metadata["title"]; ok {
				builder.WriteString(fmt.Sprintf("标题: %v\n", title))
			}
			if source, ok := result.Document.Metadata["source"]; ok {
				builder.WriteString(fmt.Sprintf("来源: %v\n", source))
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString("请结合以上知识库信息，为用户提供准确、有帮助的回答。\n")
	return builder.String()
}

func (this *Agent) NewSpecialist() *host.Specialist {
	//作为子agent的时候，不调用memory，这个由父级的agent去调用接管
	this.memoryManager = nil

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
