package agent

import (
	"context"
	"fmt"
	"io"
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

// Generate 生成完整响应（非流式）
// 性能优化：预分配内存，异步存储消息
func (this *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.Message, error) {
	// 预处理并获取迭代器
	ctx, iter, err := this.runAgentWithPreprocess(ctx, input, false, WithChatOptions(opts))
	if err != nil {
		return nil, err
	}

	var response *schema.Message
	// 预分配 Builder 容量，减少内存分配
	var fullContent strings.Builder
	fullContent.Grow(1024)

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, event.Err
		}

		// 优先处理退出事件
		if event.Action != nil && event.Action.Exit {
			break
		}

		// 处理输出事件
		if event.Output != nil && event.Output.MessageOutput != nil {
			mv := event.Output.MessageOutput
			if mv.Role == schema.Assistant && mv.Message != nil {
				response = mv.Message
			}
			// 收集内容
			this.collectAssistantContent(&fullContent, mv)
		}
	}

	if response == nil {
		return nil, fmt.Errorf("generate response is nil")
	}

	// 异步存储助手消息，不阻塞返回
	if fullContent.Len() > 0 {
		this.storeCollectedContentAsync(ctx, fullContent.String())
	}

	return response, nil
}

func (this *Agent) Name(ctx context.Context) string {
	return this.agent.Name(ctx)
}

func (this *Agent) Description(ctx context.Context) string {
	return this.agent.Description(ctx)
}

// Run adk的Run入口
// 性能优化：预分配内存，异步存储消息
func (this *Agent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	// 预处理并获取迭代器
	ctx, iter, err := this.runAgentWithPreprocess(ctx, input.Messages, input.EnableStreaming, options...)
	if err != nil {
		iterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
		generator.Send(&adk.AgentEvent{Err: err})
		generator.Close()
		return iterator
	}

	// 包装迭代器以处理消息存储
	wrappedIterator, generator := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer generator.Close()

		// 预分配 Builder 容量，减少内存分配
		var fullContent strings.Builder
		fullContent.Grow(1024)

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			// 转发事件（优先发送，减少延迟）
			generator.Send(event)

			// 处理错误事件
			if event.Err != nil {
				continue
			}

			// 检查是否退出
			if event.Action != nil && event.Action.Exit {
				break
			}

			// 收集助手消息内容用于存储
			if event.Output != nil && event.Output.MessageOutput != nil {
				this.collectAssistantContent(&fullContent, event.Output.MessageOutput)
			}
		}

		// 异步存储助手消息，不阻塞迭代器关闭
		if fullContent.Len() > 0 {
			this.storeCollectedContentAsync(ctx, fullContent.String())
		}
	}()

	return wrappedIterator
}

// Stream 流式输出，如果需要输出具体的agent流转详情等，建议使用Run方法
// 性能优化：
// 1. 增大 Pipe 缓冲区减少阻塞
// 2. 异步存储消息不阻塞流式输出
// 3. 预分配内存减少 GC 压力
func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {
	// 预处理并获取迭代器
	ctx, iter, err := this.runAgentWithPreprocess(ctx, input, true, WithChatOptions(opts))
	if err != nil {
		return nil, err
	}

	// 创建流式读取器，增大缓冲区以减少阻塞
	streamReader, streamWriter := schema.Pipe[*schema.Message](64)

	// 开启协程处理流式事件
	go func() {
		defer streamWriter.Close()

		// 预分配 Builder 容量，减少内存分配
		var fullContent strings.Builder
		fullContent.Grow(1024)

		for {
			event, ok := iter.Next()
			if !ok {
				break
			}

			// 优先处理错误和退出事件
			if event.Err != nil {
				streamWriter.Send(&schema.Message{}, event.Err)
				return
			}

			if event.Action != nil && event.Action.Exit {
				break
			}

			// 跳过非输出事件
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}

			mv := event.Output.MessageOutput
			if mv.Role != schema.Assistant {
				continue
			}

			// 处理流式输出
			if mv.IsStreaming && mv.MessageStream != nil {
				for {
					chunk, streamErr := mv.MessageStream.Recv()
					if streamErr == io.EOF {
						break
					}
					if streamErr != nil {
						streamWriter.Send(&schema.Message{}, streamErr)
						return
					}

					// 收集内容用于存储
					fullContent.WriteString(chunk.Content)

					// 发送流式数据块
					streamWriter.Send(chunk, nil)
				}
			} else if mv.Message != nil {
				// 非流式输出，直接发送
				fullContent.WriteString(mv.Message.Content)
				streamWriter.Send(mv.Message, nil)
			}
		}

		// 异步存储助手消息，不阻塞流式输出完成
		if fullContent.Len() > 0 {
			this.storeCollectedContentAsync(ctx, fullContent.String())
		}
	}()

	return streamReader, nil
}

func (this *Agent) GetAdkAgent() adk.Agent {
	return this
}

// collectAssistantContent 从事件中收集助手消息内容
func (this *Agent) collectAssistantContent(fullContent *strings.Builder, mv *adk.MessageVariant) {
	if mv.Role != schema.Assistant {
		return
	}

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

// runAgentWithPreprocess 预处理输入并调用底层agent，返回迭代器和处理后的上下文
func (this *Agent) runAgentWithPreprocess(ctx context.Context, input []*schema.Message, enableStreaming bool, options ...adk.AgentRunOption) (context.Context, *adk.AsyncIterator[*adk.AgentEvent], error) {
	chatOpts := &chatOptions{}
	chatOpts = adk.GetImplSpecificOptions(chatOpts, options...)

	// 预处理输入消息
	ctx, processedInput, chatOpts, err := this.chatPreHandler(ctx, input, chatOpts)
	if err != nil {
		return ctx, nil, err
	}

	// 创建新的 AgentInput 使用处理后的消息
	processedAgentInput := &adk.AgentInput{
		Messages:        processedInput,
		EnableStreaming: enableStreaming,
	}

	// 默认去掉TransferMessages
	options = append(options, adk.WithSkipTransferMessages())

	// 调用底层 adk.Agent 的 Run 方法
	iter := this.agent.Run(ctx, processedAgentInput, options...)

	return ctx, iter, nil
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
	return state.SetChatChatSate(ctx, chatState)
}
