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

	// 创建adk.Agent实例
	mainAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        this.name,
		Description: this.description,
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

func (this *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.Message, error) {
	ctx, _input, chatOpts, err := this.chatPreHandler(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	_ = chatOpts

	// 创建Runner，禁用流式输出
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		EnableStreaming: false,
		Agent:           this.agent,
	})

	// 运行Agent

	adkOpts := []adk.AgentRunOption{
		adk.WithSkipTransferMessages(),
	}

	iter := runner.Run(ctx, _input, adkOpts...)

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
			if mv.Role == schema.Assistant {
				// 获取完整消息（对于非流式输出，这里直接返回Message）
				msg, err := mv.GetMessage()
				if err != nil {
					return nil, err
				}
				response = msg
			}
		}

		// 处理退出事件
		if event.Action != nil && event.Action.Exit {
			break
		}
	}

	// 存储响应消息
	this.handleMessageStorage(ctx, response)
	return response, nil
}

func (this *Agent) Name(ctx context.Context) string {
	return this.name
}

func (this *Agent) Description(ctx context.Context) string {
	return this.description
}

func (this *Agent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return this.agent.Run(ctx, input, options...)
}

func (this *Agent) GetAdkAgent() adk.Agent {
	return this
}

func (this *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...ChatOption) (*schema.StreamReader[*schema.Message], error) {
	ctx, _input, chatOpts, err := this.chatPreHandler(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	_ = chatOpts
	// 创建Runner，启用流式输出
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		EnableStreaming: true,
		Agent:           this.agent,
	})

	adkOpts := []adk.AgentRunOption{
		adk.WithSkipTransferMessages(),
	}

	// 运行Agent并获取事件迭代器
	iter := runner.Run(ctx, _input, adkOpts...)

	// 创建流式读取器
	streamReader, streamWriter := schema.Pipe[*schema.Message](10)

	// 开启协程处理流式事件
	go func() {
		defer streamWriter.Close()

		var fullContent strings.Builder

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
				if mv.Role == schema.Assistant && mv.IsStreaming {
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
						fullContent.WriteString(chunk.Content)
					}
				} else if mv.Role == schema.Assistant && !mv.IsStreaming {
					// 非流式输出，直接发送
					streamWriter.Send(mv.Message, nil)
					fullContent.WriteString(mv.Message.Content)
				}
			}

			// 处理退出事件
			if event.Action != nil && event.Action.Exit {
				break
			}
		}

		// 存储完整的响应消息
		if this.hasMemoryManager() && fullContent.Len() > 0 {
			responseMsg := &schema.Message{
				Role:    schema.Assistant,
				Content: fullContent.String(),
			}
			this.handleMessageStorage(ctx, responseMsg)
		}
	}()

	return streamReader, nil
}

func (this *Agent) chatPreHandler(ctx context.Context, input []*schema.Message, opts ...ChatOption) (context.Context, []*schema.Message, *chatOptions, error) {
	chatOpts := this.buildChatOptions(opts...)

	_input, err := this.inputMessageModifier(ctx, input, chatOpts)
	if err != nil {
		return nil, nil, nil, err
	}

	if !this.hasMemoryManager() {
		return ctx, _input, chatOpts, nil
	}

	ctx = this.setupChatContext(ctx, _input, chatOpts)

	// 存储用户消息,必须是最原始的input，不是_input
	if err := this.storeUserMessage(ctx, input, chatOpts); err != nil {
		return nil, nil, nil, err
	}

	return ctx, _input, chatOpts, nil
}

func (this *Agent) inputMessageModifier(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) ([]*schema.Message, error) {
	_input := this.buildBaseMessages(ctx, input, chatOpts)

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

//func (this *Agent) NewSpecialist() *host.Specialist {
//	return &host.Specialist{
//		AgentMeta: host.AgentMeta{
//			Name:        this.name,
//			IntendedUse: this.description,
//		},
//		Invokable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.Message, err error) {
//			// 直接使用底层adk.Agent，不使用内存管理
//			runner := adk.NewRunner(ctx, adk.RunnerConfig{
//				EnableStreaming: false,
//				Agent:           this.agent,
//			})
//
//			iter := runner.Run(ctx, input)
//			var response *schema.Message
//
//			for {
//				event, ok := iter.Next()
//				if !ok {
//					break
//				}
//				if event.Err != nil {
//					return nil, event.Err
//				}
//
//				if event.Output != nil && event.Output.MessageOutput != nil {
//					mv := event.Output.MessageOutput
//					if mv.Role == schema.Assistant {
//						msg, err := mv.GetMessage()
//						if err != nil {
//							return nil, err
//						}
//						response = msg
//					}
//				}
//
//				if event.Action != nil && event.Action.Exit {
//					break
//				}
//			}
//
//			return response, nil
//		},
//		Streamable: func(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (output *schema.StreamReader[*schema.Message], err error) {
//			// 直接使用底层adk.Agent进行流式处理
//			runner := adk.NewRunner(ctx, adk.RunnerConfig{
//				EnableStreaming: true,
//				Agent:           this.agent,
//			})
//
//			iter := runner.Run(ctx, input)
//			streamReader, streamWriter := schema.Pipe[*schema.Message](10)
//
//			go func() {
//				defer streamWriter.Close()
//
//				for {
//					event, ok := iter.Next()
//					if !ok {
//						break
//					}
//					if event.Err != nil {
//						streamWriter.Send(&schema.Message{}, event.Err)
//						return
//					}
//
//					if event.Output != nil && event.Output.MessageOutput != nil {
//						mv := event.Output.MessageOutput
//						if mv.Role == schema.Assistant && mv.IsStreaming {
//							for {
//								chunk, streamErr := mv.MessageStream.Recv()
//								if streamErr == io.EOF {
//									break
//								}
//								if streamErr != nil {
//									streamWriter.Send(&schema.Message{}, streamErr)
//									return
//								}
//								streamWriter.Send(chunk, nil)
//							}
//						} else if mv.Role == schema.Assistant && !mv.IsStreaming {
//							streamWriter.Send(mv.Message, nil)
//						}
//					}
//
//					if event.Action != nil && event.Action.Exit {
//						break
//					}
//				}
//			}()
//
//			return streamReader, nil
//		},
//	}
//}
