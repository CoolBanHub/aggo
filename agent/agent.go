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
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Agent struct {
	// adk.Agent实例 - 核心agent
	agent adk.Agent

	// 内存管理器
	memoryManager *memory.MemoryManager

	// 基本信息（用于创建agent时使用）
	name         string
	description  string
	systemPrompt string
	cm           model.ToolCallingChatModel
	tools        []tool.BaseTool
	maxStep      int

	// 子agents（用于多agent场景）
	subAgents []adk.Agent
}

func NewAgent(ctx context.Context, cm model.ToolCallingChatModel, opts ...Option) (*Agent, error) {
	if cm == nil {
		return nil, fmt.Errorf("chat model不能为空")
	}

	this := &Agent{
		cm: cm,
	}

	// 应用选项配置
	for _, opt := range opts {
		if opt != nil {
			opt(this)
		}
	}

	name := this.name
	description := this.description
	if name == "" {
		name = "adk agent"
	}

	if description == "" {
		description = "adk agent"
	}

	// 创建adk.Agent实例
	mainAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: description,
		Instruction: this.systemPrompt,
		Model:       cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: this.tools,
			},
		},
		MaxIterations: this.maxStep,
	})
	if err != nil {
		return nil, err
	}

	this.agent = mainAgent

	// 如果有子agents，设置它们
	if len(this.subAgents) > 0 {
		this.agent, err = adk.SetSubAgents(ctx, mainAgent, this.subAgents)
		if err != nil {
			return nil, err
		}
	}

	return this, nil
}

// NewAgentFromADK 从已存在的adk.Agent创建Agent实例
// 适用于已经通过adk包创建的Agent
func NewAgentFromADK(adkAgent adk.Agent, opts ...Option) (*Agent, error) {
	if adkAgent == nil {
		return nil, fmt.Errorf("adk agent不能为空")
	}

	this := &Agent{
		agent: adkAgent,
	}

	// 应用选项配置（主要用于配置内存管理器等增强功能）
	for _, opt := range opts {
		if opt != nil {
			opt(this)
		}
	}

	return this, nil
}

func (this *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.Message, error) {

	// 创建 AgentInput
	agentInput := &adk.AgentInput{
		Messages:        input,
		EnableStreaming: false,
	}
	// 调用 Run 方法
	iter := this.Run(ctx, agentInput, WithChatOptions(opts))

	var response *schema.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, event.Err
		}

		// 处理输出事件
		if event.Output != nil && event.Output.MessageOutput != nil {
			mv := event.Output.MessageOutput
			if mv.Role == schema.Assistant && mv.Message != nil {
				response = mv.Message
			}
		}

		// 处理退出事件
		if event.Action != nil && event.Action.Exit {
			break
		}
	}

	return response, nil
}

func (this *Agent) Name(ctx context.Context) string {
	return this.agent.Name(ctx)
}

func (this *Agent) Description(ctx context.Context) string {
	return this.agent.Description(ctx)
}

func (this *Agent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	chatOpts := &chatOptions{}
	chatOpts = adk.GetImplSpecificOptions(chatOpts, options...)

	// 预处理输入消息
	ctx, processedInput, chatOpts, err := this.chatPreHandler(ctx, input.Messages, chatOpts)
	if err != nil {
		iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
		generator.Send(&adk.AgentEvent{Err: err})
		generator.Close()
		return iterator
	}

	// 创建新的 AgentInput 使用处理后的消息
	processedAgentInput := &adk.AgentInput{
		Messages:        processedInput,
		EnableStreaming: input.EnableStreaming,
	}

	// 调用底层 adk.Agent 的 Run 方法
	iter := this.agent.Run(ctx, processedAgentInput, options...)

	// 包装迭代器以处理消息存储
	wrappedIterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer generator.Close()

		var fullContent strings.Builder

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			// 转发事件
			generator.Send(event)

			if event.Err != nil {
				continue
			}

			// 收集助手消息内容用于存储
			if event.Output != nil && event.Output.MessageOutput != nil {
				mv := event.Output.MessageOutput
				if mv.Role == schema.Assistant {
					if mv.IsStreaming && mv.MessageStream != nil {
						// 流式输出：读取所有块
						for {
							chunk, streamErr := mv.MessageStream.Recv()
							if streamErr == io.EOF {
								break
							}
							if streamErr != nil {
								break
							}
							fullContent.WriteString(chunk.Content)
						}
					} else if mv.Message != nil {
						// 非流式输出
						fullContent.WriteString(mv.Message.Content)
					}
				}
			}

			// 检查是否退出
			if event.Action != nil && event.Action.Exit {
				break
			}
		}

		// 存储助手消息
		if this.hasMemoryManager() && fullContent.Len() > 0 {
			responseMsg := &schema.Message{
				Role:    schema.Assistant,
				Content: fullContent.String(),
			}
			this.handleMessageStorage(ctx, responseMsg)
		}
	}()

	// 需要保存 chatOpts 以便后续使用，这里先忽略未使用的变量警告
	_ = chatOpts

	return wrappedIterator
}

func (this *Agent) GetAdkAgent() adk.Agent {
	return this
}

func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {

	// 创建 AgentInput
	agentInput := &adk.AgentInput{
		Messages:        input,
		EnableStreaming: true,
	}

	// 调用 Run 方法
	iter := this.Run(ctx, agentInput, WithChatOptions(opts))

	// 创建流式读取器
	streamReader, streamWriter := schema.Pipe[*schema.Message](10)

	// 开启协程处理流式事件
	go func() {
		defer streamWriter.Close()

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				streamWriter.Send(&schema.Message{}, event.Err)
				return
			}

			// 处理流式输出事件
			if event.Output != nil && event.Output.MessageOutput != nil {
				mv := event.Output.MessageOutput
				if mv.Role == schema.Assistant {
					if mv.IsStreaming && mv.MessageStream != nil {
						// 处理流式数据
						for {
							chunk, streamErr := mv.MessageStream.Recv()
							if streamErr == io.EOF {
								break
							}
							if streamErr != nil {
								streamWriter.Send(&schema.Message{}, streamErr)
								return
							}

							// 发送流式数据块
							streamWriter.Send(chunk, nil)
						}
					} else if mv.Message != nil {
						// 非流式输出，直接发送
						streamWriter.Send(mv.Message, nil)
					}
				}
			}

			// 处理退出事件
			if event.Action != nil && event.Action.Exit {
				break
			}
		}
	}()

	return streamReader, nil
}

func (this *Agent) chatPreHandler(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) (context.Context, []*schema.Message, *chatOptions, error) {
	if chatOpts.sessionID == "" {
		chatOpts.sessionID = utils.GetULID()
	}
	if chatOpts.userID == "" {
		chatOpts.userID = chatOpts.sessionID
	}
	// 处理消息输入（如果有内存管理器则增强消息）
	processedInput := input
	if this.hasMemoryManager() {
		enhancedInput, err := this.inputMessageModifier(ctx, input, chatOpts)
		if err != nil {
			return nil, nil, nil, err
		}
		processedInput = enhancedInput

		// 存储用户消息（必须使用原始input，不是增强后的）
		if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
			return nil, nil, nil, err
		}
	}

	// 在最后添加用户消息后缀（不影响历史消息和存储）
	processedInput = this.applyUserMessageSuffix(processedInput, chatOpts)

	// 设置聊天上下文
	ctx = this.setupChatContext(ctx, processedInput, chatOpts)

	return ctx, processedInput, chatOpts, nil
}

func (this *Agent) inputMessageModifier(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) ([]*schema.Message, error) {

	_input := this.buildMessagesWithHistory(ctx, input, chatOpts)
	_input = this.addUserMemories(ctx, _input, chatOpts)
	_input = this.addSessionSummary(ctx, _input, chatOpts)
	return _input, nil
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

// applyUserMessageSuffix 将后缀添加到最后一条用户消息
func (this *Agent) applyUserMessageSuffix(input []*schema.Message, chatOpts *chatOptions) []*schema.Message {
	if chatOpts.userMessageSuffix == "" || len(input) == 0 {
		return input
	}

	// 找到最后一条用户消息
	lastUserMsgIdx := -1
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role == schema.User {
			lastUserMsgIdx = i
			break
		}
	}

	// 如果没有用户消息，直接返回
	if lastUserMsgIdx == -1 {
		return input
	}

	// 复制消息列表，避免修改原始输入
	result := make([]*schema.Message, len(input))
	copy(result, input)

	// 拼接后缀到最后一条用户消息 - 创建新的消息对象
	lastUserMsg := *result[lastUserMsgIdx]
	lastUserMsg.Content = lastUserMsg.Content + chatOpts.userMessageSuffix
	result[lastUserMsgIdx] = &lastUserMsg

	return result
}

// setupChatContext 设置聊天上下文状态
func (this *Agent) setupChatContext(ctx context.Context, _input []*schema.Message, chatOpts *chatOptions) context.Context {
	chatState := &state.ChatSate{
		Input:     _input,
		SessionID: chatOpts.sessionID,
		UserID:    chatOpts.userID,
	}
	if this.agent != nil && this.systemPrompt != "" {
		chatState.Input = _input[1:] // TODO 需要去掉？
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
