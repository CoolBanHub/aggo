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
	ctx, _input, agentOpts, err := this.chatPreHandler(ctx, input, opts...)
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

	this.handleMessageStorage(ctx, response)
	return response, nil
}

func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {
	ctx, _input, agentOpts, err := this.chatPreHandler(ctx, input, opts...)
	if err != nil {
		return nil, err
	}

	var response *schema.StreamReader[*schema.Message]
	if this.multiAgent != nil {
		response, err = this.multiAgent.Stream(ctx, _input, agentOpts)
	} else {
		response, err = this.agent.Stream(ctx, _input, agentOpts)
	}
	if err != nil {
		return nil, err
	}

	if this.hasMemoryManager() {
		go this.handleStreamMessageStorage(ctx, response)
	}

	return response, nil
}

func (this *Agent) chatPreHandler(ctx context.Context, input []*schema.Message, opts ...ChatOption) (context.Context, []*schema.Message, agent.AgentOption, error) {
	chatOpts := this.buildChatOptions(opts...)
	agentOpts := agent.WithComposeOptions(chatOpts.composeOptions...)

	_input, err := this.inputMessageModifier(ctx, input, chatOpts)
	if err != nil {
		return nil, nil, agentOpts, err
	}

	if !this.hasMemoryManager() {
		return ctx, _input, agentOpts, nil
	}

	ctx = this.setupChatContext(ctx, _input, chatOpts)

	// 存储用户消息,必须是最原始的input，不是_input
	if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
		return nil, nil, agentOpts, err
	}

	return ctx, _input, agentOpts, nil
}

func (this *Agent) inputMessageModifier(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) ([]*schema.Message, error) {
	_input := this.buildBaseMessages(ctx, input, chatOpts)

	// 添加系统提示词到最前面
	this.addSystemPrompt(&_input)

	return _input, nil
}

// buildBaseMessages 构建基础消息序列
func (this *Agent) buildBaseMessages(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) []*schema.Message {
	if !this.hasMemoryManager() {
		return input
	}

	_input := this.buildMessagesWithHistory(ctx, input, chatOpts)
	_input = this.addUserMemories(ctx, _input, chatOpts)
	_input = this.addSessionSummary(ctx, _input, chatOpts)
	return _input
}

// buildMessagesWithHistory 构建包含历史消息的序列
func (this *Agent) buildMessagesWithHistory(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) []*schema.Message {
	historyMessages, err := this.memoryManager.GetMessages(ctx, chatOpts.sessionID, chatOpts.userID, this.memoryManager.GetConfig().MemoryLimit)
	if err != nil {
		log.Printf("获取历史消息失败: %v", err)
		return input
	}
	return append(historyMessages, input...)
}

// addUserMemories 添加用户记忆
func (this *Agent) addUserMemories(ctx context.Context, _input []*schema.Message, chatOpts *chatOptions) []*schema.Message {
	if !this.memoryManager.GetConfig().EnableUserMemories {
		return _input
	}

	userMemories, err := this.memoryManager.GetUserMemories(ctx, chatOpts.userID)
	if err != nil {
		log.Printf("获取用户记忆失败: %v", err)
		return _input
	}

	if len(userMemories) == 0 {
		return _input
	}

	memoryContent := this.formatUserMemories(userMemories)
	memoryMessage := &schema.Message{
		Role:    schema.System,
		Content: memoryContent,
	}
	return append([]*schema.Message{memoryMessage}, _input...)
}

// addSessionSummary 添加会话摘要
func (this *Agent) addSessionSummary(ctx context.Context, _input []*schema.Message, chatOpts *chatOptions) []*schema.Message {
	if !this.memoryManager.GetConfig().EnableSessionSummary {
		return _input
	}

	sessionSummary, err := this.memoryManager.GetSessionSummary(ctx, chatOpts.sessionID, chatOpts.userID)
	if err != nil {
		log.Printf("获取会话摘要失败: %v", err)
		return _input
	}

	if sessionSummary == nil || sessionSummary.Summary == "" {
		return _input
	}

	summaryMessage := &schema.Message{
		Role:    schema.System,
		Content: fmt.Sprintf("会话背景: %s", sessionSummary.Summary),
	}
	return append([]*schema.Message{summaryMessage}, _input...)
}

// addSystemPrompt 添加系统提示词
func (this *Agent) addSystemPrompt(_input *[]*schema.Message) {
	if this.agent != nil && this.systemPrompt != "" {
		systemMessage := &schema.Message{
			Role:    schema.System,
			Content: this.systemPrompt,
		}
		*_input = append([]*schema.Message{systemMessage}, *_input...)
	}
}

// buildChatOptions 构建聊天选项
func (this *Agent) buildChatOptions(opts ...ChatOption) *chatOptions {
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
	return chatOpts
}

// setupChatContext 设置聊天上下文状态
func (this *Agent) setupChatContext(ctx context.Context, _input []*schema.Message, chatOpts *chatOptions) context.Context {
	chatState := &state.ChatSate{
		Input:     _input,
		SessionID: chatOpts.sessionID,
		UserID:    chatOpts.userID,
	}
	if this.agent != nil && this.systemPrompt != "" {
		chatState.Input = _input[1:]
	}
	return state.SetChatChatSate(ctx, chatState)
}

// storeUserMessage 存储用户消息
func (this *Agent) storeUserMessage(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) error {
	if chatOpts == nil || chatOpts.sessionID == "" || !this.hasMemoryManager() || len(input) == 0 {
		return nil
	}
	return this.memoryManager.ProcessUserMessage(ctx, chatOpts.userID, chatOpts.sessionID, input[0].Content)
}

// hasMemoryManager 检查是否有内存管理器
func (this *Agent) hasMemoryManager() bool {
	return this.memoryManager != nil
}

// handleMessageStorage 处理消息存储的统一逻辑
func (this *Agent) handleMessageStorage(ctx context.Context, response *schema.Message) {
	if !this.hasMemoryManager() || response == nil {
		return
	}
	this.storeAssistantMessage(ctx, response)
}

// handleStreamMessageStorage 处理流式消息存储
func (this *Agent) handleStreamMessageStorage(ctx context.Context, response *schema.StreamReader[*schema.Message]) {
	var content strings.Builder
	for {
		chunk, err := response.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("读取流式消息失败: %v", err)
			return
		}
		content.WriteString(chunk.Content)
	}

	if content.Len() > 0 {
		this.storeAssistantMessage(ctx, &schema.Message{Role: schema.Assistant, Content: content.String()})
	}
}

// storeAssistantMessage 存储助手消息,在callback统一处理
func (this *Agent) storeAssistantMessage(ctx context.Context, response *schema.Message) {
	if !this.hasMemoryManager() || response == nil {
		return
	}

	chatState := state.GetChatChatSate(ctx)
	if chatState == nil {
		log.Println("storeAssistantMessage: chatState is nil")
		return
	}

	if err := this.memoryManager.ProcessAssistantMessage(ctx, chatState.UserID, chatState.SessionID, response.Content); err != nil {
		log.Printf("storeAssistantMessage failed: %v", err)
	}
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
