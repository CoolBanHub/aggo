package memory

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// MemoryMiddleware 实现 adk.ChatModelAgentMiddleware 接口
// 通过 Eino 生命周期钩子自动处理：历史消息注入、用户记忆/会话摘要增强、消息存储
type MemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware

	manager *MemoryManager
}

// NewMemoryMiddleware 创建 MemoryMiddleware 实例
func NewMemoryMiddleware(manager *MemoryManager) *MemoryMiddleware {
	return &MemoryMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		manager:                      manager,
	}
}

// BeforeAgent 在 Agent 运行前初始化 session/user 上下文到 SessionValues
func (m *MemoryMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return ctx, runCtx, nil
}

// BeforeModelRewriteState 在模型调用前注入历史消息、用户记忆和会话摘要
func (m *MemoryMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m.manager == nil {
		return ctx, state, nil
	}

	// 从 SessionValues 获取 sessionID 和 userID
	sessionID, _ := adk.GetSessionValue(ctx, "sessionID")
	userID, _ := adk.GetSessionValue(ctx, "userID")
	sid, _ := sessionID.(string)
	uid, _ := userID.(string)
	if sid == "" || uid == "" {
		return ctx, state, nil
	}
	if prepared, ok := adk.GetSessionValue(ctx, m.beforeModelRewriteStateKey()); ok {
		if done, ok := prepared.(bool); ok && done {
			return ctx, state, nil
		}
	}

	// 并发获取三种数据
	type fetchResult struct {
		historyMessages   []*schema.Message
		userMemoryMsg     *schema.Message
		sessionSummaryMsg *schema.Message
	}

	result := &fetchResult{}
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		result.historyMessages = m.fetchHistoryMessages(ctx, sid, uid)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		result.userMemoryMsg = m.fetchUserMemoryMessage(ctx, uid)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		result.sessionSummaryMsg = m.fetchSessionSummaryMessage(ctx, sid, uid)
	}()

	wg.Wait()

	// 按顺序拼接消息
	enhanced := make([]*schema.Message, 0, len(result.historyMessages)+len(state.Messages)+2)

	if result.userMemoryMsg != nil {
		enhanced = append(enhanced, result.userMemoryMsg)
	}
	if result.sessionSummaryMsg != nil {
		enhanced = append(enhanced, result.sessionSummaryMsg)
	}
	if len(result.historyMessages) > 0 {
		enhanced = append(enhanced, result.historyMessages...)
	}
	enhanced = append(enhanced, state.Messages...)

	state.Messages = enhanced

	// 存储用户消息（取最后一条 user 消息）
	m.storeUserMessage(ctx, state.Messages, uid, sid)
	adk.AddSessionValue(ctx, m.beforeModelRewriteStateKey(), true)

	return ctx, state, nil
}

// AfterModelRewriteState 在模型调用后存储助手消息
func (m *MemoryMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if m.manager == nil {
		return ctx, state, nil
	}

	// 从 SessionValues 获取 sessionID 和 userID
	sessionID, _ := adk.GetSessionValue(ctx, "sessionID")
	userID, _ := adk.GetSessionValue(ctx, "userID")
	sid, _ := sessionID.(string)
	uid, _ := userID.(string)
	if sid == "" || uid == "" {
		return ctx, state, nil
	}

	// 找到最后的 assistant 消息并异步存储
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Role == schema.Assistant && state.Messages[i].Content != "" {
			go func(content string) {
				bgCtx := context.Background()
				if err := m.manager.ProcessAssistantMessage(bgCtx, uid, sid, content); err != nil {
					log.Printf("MemoryMiddleware: 存储助手消息失败: %v", err)
				}
			}(state.Messages[i].Content)
			break
		}
	}

	return ctx, state, nil
}

// fetchHistoryMessages 获取历史消息
func (m *MemoryMiddleware) fetchHistoryMessages(ctx context.Context, sessionID, userID string) []*schema.Message {
	messages, err := m.manager.GetMessages(ctx, sessionID, userID, m.manager.GetConfig().MemoryLimit)
	if err != nil {
		log.Printf("MemoryMiddleware: 获取历史消息失败: %v", err)
		return nil
	}
	return messages
}

// fetchUserMemoryMessage 获取用户记忆作为 System Message
func (m *MemoryMiddleware) fetchUserMemoryMessage(ctx context.Context, userID string) *schema.Message {
	if !m.manager.GetConfig().EnableUserMemories {
		return nil
	}

	userMemory, err := m.manager.GetUserMemory(ctx, userID)
	if err != nil {
		log.Printf("MemoryMiddleware: 获取用户记忆失败: %v", err)
		return nil
	}
	if userMemory == nil || userMemory.Memory == "" {
		return nil
	}

	var builder strings.Builder
	builder.WriteString("用户个人信息记忆（请在回复中考虑这些信息，提供个性化的响应）:\n")
	builder.WriteString(userMemory.Memory)

	return &schema.Message{
		Role:    schema.System,
		Content: builder.String(),
	}
}

// fetchSessionSummaryMessage 获取会话摘要作为 System Message
func (m *MemoryMiddleware) fetchSessionSummaryMessage(ctx context.Context, sessionID, userID string) *schema.Message {
	if !m.manager.GetConfig().EnableSessionSummary {
		return nil
	}

	summary, err := m.manager.GetSessionSummary(ctx, sessionID, userID)
	if err != nil {
		log.Printf("MemoryMiddleware: 获取会话摘要失败: %v", err)
		return nil
	}
	if summary == nil || summary.Summary == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.System,
		Content: fmt.Sprintf("会话背景: %s", summary.Summary),
	}
}

// storeUserMessage 存储用户消息
func (m *MemoryMiddleware) storeUserMessage(ctx context.Context, messages []*schema.Message, userID, sessionID string) {
	if len(messages) == 0 {
		return
	}
	// 找到最后一条 user 消息
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			if err := m.manager.ProcessUserMessage(ctx, userID, sessionID, messages[i].Content, messages[i].UserInputMultiContent); err != nil {
				log.Printf("MemoryMiddleware: 存储用户消息失败: %v", err)
			}
			return
		}
	}
}

func (m *MemoryMiddleware) beforeModelRewriteStateKey() string {
	return fmt.Sprintf("__aggo_memory_middleware_prepared_%p", m)
}
