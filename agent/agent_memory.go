package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/state"
	"github.com/cloudwego/eino/schema"
)

// ============================================================================
// 记忆管理相关方法
// ============================================================================

// hasMemoryManager 检查是否有内存管理器
func (this *Agent) hasMemoryManager() bool {
	return this.memoryManager != nil
}

// inputMessageModifier 修改输入消息，添加历史消息、用户记忆和会话摘要
// 该方法使用并发方式同时获取三种类型的数据，然后按照以下顺序拼接消息：
// 1. 用户记忆（系统消息）
// 2. 会话摘要（系统消息）
// 3. 历史消息
// 4. 当前输入
func (this *Agent) inputMessageModifier(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) ([]*schema.Message, error) {
	// 防御性检查：确保 memoryManager 存在
	if this.memoryManager == nil {
		return input, nil
	}

	// 使用并发方式获取所有需要的数据
	type fetchResult struct {
		historyMessages   []*schema.Message
		userMemoryMsg     *schema.Message
		sessionSummaryMsg *schema.Message
	}

	result := &fetchResult{}
	var wg sync.WaitGroup

	// 并发获取历史消息
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.historyMessages = this.fetchHistoryMessages(ctx, chatOpts)
	}()

	// 并发获取用户记忆
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.userMemoryMsg = this.fetchUserMemoryMessage(ctx, chatOpts)
	}()

	// 并发获取会话摘要
	wg.Add(1)
	go func() {
		defer wg.Done()
		result.sessionSummaryMsg = this.fetchSessionSummaryMessage(ctx, chatOpts)
	}()

	// 等待所有并发任务完成
	wg.Wait()

	// 按照正确的顺序拼接消息
	enhancedMessages := make([]*schema.Message, 0, len(result.historyMessages)+len(input)+2)

	// 1. 添加用户记忆（如果有）
	if result.userMemoryMsg != nil {
		enhancedMessages = append(enhancedMessages, result.userMemoryMsg)
	}

	// 2. 添加会话摘要（如果有）
	if result.sessionSummaryMsg != nil {
		enhancedMessages = append(enhancedMessages, result.sessionSummaryMsg)
	}

	// 3. 添加历史消息
	if len(result.historyMessages) > 0 {
		enhancedMessages = append(enhancedMessages, result.historyMessages...)
	}

	// 4. 添加当前输入
	enhancedMessages = append(enhancedMessages, input...)

	return enhancedMessages, nil
}

// ============================================================================
// 消息存储相关方法
// ============================================================================

// storeUserMessage 存储用户消息
func (this *Agent) storeUserMessage(ctx context.Context, input []*schema.Message, chatOpts *chatOptions) error {
	if chatOpts == nil || chatOpts.sessionID == "" || !this.hasMemoryManager() || len(input) == 0 {
		return nil
	}
	return this.memoryManager.ProcessUserMessage(ctx, chatOpts.userID, chatOpts.sessionID, chatOpts.messageID, input[0].Content, input[0].UserInputMultiContent)
}

// storeCollectedContent 存储收集的助手消息内容
func (this *Agent) storeCollectedContent(ctx context.Context, fullContent *strings.Builder) {
	if this.hasMemoryManager() && fullContent.Len() > 0 {
		responseMsg := &schema.Message{
			Role:    schema.Assistant,
			Content: fullContent.String(),
		}
		this.handleMessageStorage(ctx, responseMsg)
	}
}

// storeCollectedContentAsync 异步存储收集的助手消息内容（不阻塞主流程）
func (this *Agent) storeCollectedContentAsync(ctx context.Context, content string) {
	if !this.hasMemoryManager() || content == "" {
		return
	}
	go func() {
		responseMsg := &schema.Message{
			Role:    schema.Assistant,
			Content: content,
		}
		this.handleMessageStorage(ctx, responseMsg)
	}()
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

	if err := this.memoryManager.ProcessAssistantMessage(ctx, chatState.UserID, chatState.SessionID, chatState.MessageID, response.Content); err != nil {
		log.Printf("storeAssistantMessage failed: %v", err)
	}
}

// ============================================================================
// 数据获取相关方法（用于并发调用）
// ============================================================================

// fetchHistoryMessages 获取历史消息（用于并发调用）
func (this *Agent) fetchHistoryMessages(ctx context.Context, chatOpts *chatOptions) []*schema.Message {
	if this.memoryManager == nil {
		return nil
	}

	historyMessages, err := this.memoryManager.GetMessages(ctx, chatOpts.sessionID, chatOpts.userID, this.memoryManager.GetConfig().MemoryLimit)
	if err != nil {
		log.Printf("获取历史消息失败: %v", err)
		return nil
	}

	return historyMessages
}

// fetchUserMemoryMessage 获取用户记忆消息（用于并发调用）
func (this *Agent) fetchUserMemoryMessage(ctx context.Context, chatOpts *chatOptions) *schema.Message {
	if this.memoryManager == nil {
		return nil
	}

	// 检查是否启用用户记忆功能
	if !this.memoryManager.GetConfig().EnableUserMemories {
		return nil
	}

	// 获取用户记忆
	userMemories, err := this.memoryManager.GetUserMemories(ctx, chatOpts.userID)
	if err != nil {
		log.Printf("获取用户记忆失败: %v", err)
		return nil
	}

	// 如果没有用户记忆，返回 nil
	if len(userMemories) == 0 {
		return nil
	}

	// 格式化用户记忆
	memoryContent := this.formatUserMemories(userMemories)
	if memoryContent == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.System,
		Content: memoryContent,
	}
}

// fetchSessionSummaryMessage 获取会话摘要消息（用于并发调用）
func (this *Agent) fetchSessionSummaryMessage(ctx context.Context, chatOpts *chatOptions) *schema.Message {
	if this.memoryManager == nil {
		return nil
	}

	// 检查是否启用会话摘要功能
	if !this.memoryManager.GetConfig().EnableSessionSummary {
		return nil
	}

	// 获取会话摘要
	sessionSummary, err := this.memoryManager.GetSessionSummary(ctx, chatOpts.sessionID, chatOpts.userID)
	if err != nil {
		log.Printf("获取会话摘要失败: %v", err)
		return nil
	}

	// 如果没有会话摘要或摘要为空，返回 nil
	if sessionSummary == nil || sessionSummary.Summary == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.System,
		Content: fmt.Sprintf("会话背景: %s", sessionSummary.Summary),
	}
}

// ============================================================================
// 辅助方法
// ============================================================================

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
