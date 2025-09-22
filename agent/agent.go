package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/state"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
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

	memoryManager *memory.MemoryManager

	//作为子agent的时候必须传
	name        string
	description string

	tools   []tool.BaseTool //只支持单agent的模式下使用
	agent   *react.Agent
	maxStep int

	//多agent的时候 使用
	multiAgent *host.MultiAgent
	specialist []*host.Specialist

	retriever        retriever.Retriever
	retrieverOptions []retriever.Option
}

func NewAgent(ctx context.Context, cm model.ToolCallingChatModel, opts ...Option) (*Agent, error) {
	if cm == nil {
		return nil, fmt.Errorf("chat model不能为空")
	}

	this := &Agent{
		cm: cm,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(this)
		}
	}

	if len(this.specialist) > 0 {
		h := &host.Host{
			ToolCallingModel: cm,
			SystemPrompt:     this.systemPrompt,
		}
		multiAgent, err := host.NewMultiAgent(ctx, &host.MultiAgentConfig{
			Name:        this.name,
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

	var _input []*schema.Message
	var err error
	var agentOpts agent.AgentOption
	ctx, _input, agentOpts, err = this.chatPreHandler(ctx, input, opts...)
	if err != nil {
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
	this.storeAssistantMessage(ctx, response)
	return response, nil
}

func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {

	var _input []*schema.Message
	var err error
	var agentOpts agent.AgentOption
	ctx, _input, agentOpts, err = this.chatPreHandler(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	var response *schema.StreamReader[*schema.Message]
	if this.multiAgent != nil {
		response, err = this.multiAgent.Stream(ctx, _input, agentOpts)
	} else {
		response, err = this.agent.Stream(ctx, _input, agentOpts)
	}

	go func() {
		var outs []callbacks.CallbackOutput
		content := ""
		for {
			chunk, err := response.Recv()
			if err == io.EOF {
				break
			}
			outs = append(outs, chunk)
			content += chunk.Content
		}
		this.storeAssistantMessage(ctx, &schema.Message{Content: content})
	}()

	return response, err
}

func (this *Agent) chatPreHandler(ctx context.Context, input []*schema.Message, opts ...ChatOption) (context.Context, []*schema.Message, agent.AgentOption, error) {
	chatOpts := &chatOptions{}
	for _, opt := range opts {
		opt(chatOpts)
	}
	if chatOpts.sessionID == "" {
		chatOpts.sessionID = utils.GetUUIDNoDash()
	}
	if chatOpts.userID == "" {
		chatOpts.userID = chatOpts.sessionID
	}
	agentOpts := agent.WithComposeOptions(chatOpts.composeOptions...)

	_input, err := this.inputMessageModifier(ctx, input, chatOpts)
	if err != nil {
		return nil, nil, agentOpts, err
	}
	if this.memoryManager == nil {
		return ctx, _input, agentOpts, nil
	}
	chatState := &state.ChatSate{
		Input:     _input,
		SessionID: chatOpts.sessionID,
		UserID:    chatOpts.userID,
	}
	if this.agent != nil && this.systemPrompt != "" {
		chatState.Input = _input[1:]
	}
	ctx = state.SetChatChatSate(ctx, chatState)

	// 存储用户消息,必须是最原始的input，不是_input
	if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
		return nil, nil, agentOpts, err
	}

	return ctx, _input, agentOpts, nil
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
	if this.retriever != nil && len(input) > 0 {
		userInput := input[len(input)-1].Content // 获取最新的用户输入
		if userInput == "" {
			// 如果用户输入为空，跳过知识库查询
			return _input, nil
		}

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
				if knowledgeContent != "" {
					knowledgeMessage := &schema.Message{
						Role:    schema.System,
						Content: knowledgeContent,
					}
					// 将知识库信息插入到对话消息之前，但在摘要和记忆之后
					_input = append([]*schema.Message{knowledgeMessage}, _input...)
				}
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
	if chatOpts == nil || chatOpts.sessionID == "" || this.memoryManager == nil || len(input) == 0 {
		return nil
	}
	return this.memoryManager.ProcessUserMessage(ctx, chatOpts.userID, chatOpts.sessionID, input[0].Content)
}

// storeAssistantMessage 存储助手消息,在callback统一处理
func (this *Agent) storeAssistantMessage(ctx context.Context, response *schema.Message) {
	if this.memoryManager != nil && response != nil {
		chatState := state.GetChatChatSate(ctx)
		if chatState == nil {
			log.Println("storeAssistantMessage: chatState is nil")
			return
		}
		err := this.memoryManager.ProcessAssistantMessage(ctx, chatState.UserID, chatState.SessionID, response.Content)
		if err != nil {
			log.Println("storeAssistantMessage is err:", err)
		}
		return
	}
	return
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
	if this.retriever == nil {
		return false
	}

	return true
}

// queryKnowledge 查询知识库并返回相关文档
func (this *Agent) queryKnowledge(ctx context.Context, query string) ([]*schema.Document, error) {
	if this.retriever == nil {
		return nil, nil
	}

	results, err := this.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("知识库搜索失败: %w", err)
	}

	return results, nil
}

// formatKnowledgeResults 格式化知识库搜索结果为上下文信息
func (this *Agent) formatKnowledgeResults(results []*schema.Document) string {
	if len(results) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("相关知识库信息（请基于以下信息回答用户问题）:\n\n")

	for i, result := range results {
		builder.WriteString(fmt.Sprintf("【知识%d】(相似度: %.2f)\n", i+1, result.Score()))
		builder.WriteString(fmt.Sprintf("内容: %s\n", result.Content))

		// 如果有元数据，添加一些有用的信息
		if len(result.MetaData) > 0 {
			if title, ok := result.MetaData["title"]; ok {
				builder.WriteString(fmt.Sprintf("标题: %v\n", title))
			}
			if source, ok := result.MetaData["source"]; ok {
				builder.WriteString(fmt.Sprintf("来源: %v\n", source))
			}
		}
		builder.WriteString("\n")
	}

	builder.WriteString("请结合以上知识库信息，为用户提供准确、有帮助的回答。\n")
	return builder.String()
}

func (this *Agent) NewSpecialist() *host.Specialist {
	// 创建一个专门用于specialist的agent副本，避免修改原始实例
	specialistAgent := &Agent{
		systemPrompt:  this.systemPrompt,
		cm:            this.cm,
		memoryManager: nil, // 作为子agent时不使用memory manager
		name:          this.name,
		description:   this.description,
		tools:         this.tools,
		agent:         this.agent,
		maxStep:       this.maxStep,
		multiAgent:    this.multiAgent,
		specialist:    this.specialist,
		retriever:     this.retriever,
	}

	return &host.Specialist{
		AgentMeta: host.AgentMeta{
			Name:        specialistAgent.name,
			IntendedUse: specialistAgent.description,
		},
		Invokable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.Message, err error) {
			return specialistAgent.Generate(ctx, input)
		},
		Streamable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.StreamReader[*schema.Message], err error) {
			return specialistAgent.Stream(ctx, input)
		},
	}
}
