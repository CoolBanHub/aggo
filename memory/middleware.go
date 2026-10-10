package memory

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/pkg/sessionkeys"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMemorizeTimeout = 2 * time.Minute
	runtimeContextLayout   = "2006-01-02 15:04:05 -07:00"
	runtimeContextDivider  = "\n\n-----\n"
)

// MemoryMiddlewareConfig 配置 MemoryMiddleware 的身份解析与写入行为。
// 零值可用：所有字段都有与旧行为兼容的默认值。
type MemoryMiddlewareConfig struct {
	// UserIDKey 从会话中读取用户身份的键，默认 "userID"，
	// 找不到时回退读取 "actorID"。
	UserIDKey string
	// SessionIDKey 从会话中读取会话 ID 的键，默认 "sessionID"。
	SessionIDKey string
	// IdentityResolver 覆盖默认的身份解析逻辑
	//（例如从 actorID/conversationID 组合出记忆归属），设置后 UserIDKey/SessionIDKey 被忽略。
	IdentityResolver func(ctx context.Context) (userID, sessionID string, ok bool)
	// MissingIdentity 决定身份缺失时的行为，默认 MissingIdentityLog。
	MissingIdentity MissingIdentityPolicy
	// Normalizers 在写入前依次作用于 MemorizeRequest（深拷贝后执行）。
	Normalizers []MemorizeNormalizer
	// MemorizeTimeout 仅在 provider 未实现 AsyncMemorizer 时，
	// 用于兼容路径的后台写入超时，默认 2 分钟。
	MemorizeTimeout time.Duration
}

func (c *MemoryMiddlewareConfig) normalize() {
	if c.UserIDKey == "" {
		c.UserIDKey = sessionkeys.KeyUserID
	}
	if c.SessionIDKey == "" {
		c.SessionIDKey = sessionkeys.KeySessionID
	}
	if c.MemorizeTimeout <= 0 {
		c.MemorizeTimeout = defaultMemorizeTimeout
	}
}

// MemoryMiddleware implements adk.ChatModelAgentMiddleware.
// It delegates to a MemoryProvider for retrieval and memorization.
type MemoryMiddleware struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
	provider MemoryProvider
	config   MemoryMiddlewareConfig
}

// NewMemoryMiddleware creates a MemoryMiddleware with a MemoryProvider.
// 行为与旧版本兼容：身份缺失时记录一条日志后跳过（MissingIdentityLog）。
func NewMemoryMiddleware(provider MemoryProvider) *MemoryMiddleware {
	return NewMemoryMiddlewareWithConfig(provider, MemoryMiddlewareConfig{})
}

// NewMemoryMiddlewareWithConfig creates a MemoryMiddleware with explicit config.
func NewMemoryMiddlewareWithConfig(provider MemoryProvider, config MemoryMiddlewareConfig) *MemoryMiddleware {
	config.normalize()
	return &MemoryMiddleware{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]{},
		provider:                          provider,
		config:                            config,
	}
}

// resolveIdentity 从运行时会话解析记忆身份。
func (m *MemoryMiddleware) resolveIdentity(ctx context.Context) (string, string, bool) {
	if m.config.IdentityResolver != nil {
		uid, sid, ok := m.config.IdentityResolver(ctx)
		return uid, sid, ok && uid != "" && sid != ""
	}
	uid, uidOK := sessionkeys.String(ctx, m.config.UserIDKey)
	if !uidOK {
		// 回退到 actorID：大多数应用中操作者即记忆归属人
		uid, uidOK = sessionkeys.ActorID(ctx)
	}
	sid, sidOK := sessionkeys.String(ctx, m.config.SessionIDKey)
	if !uidOK || !sidOK {
		return "", "", false
	}
	return uid, sid, true
}

// handleMissingIdentity 按策略处理身份缺失，返回是否应中断本轮请求。
func (m *MemoryMiddleware) handleMissingIdentity(stage string) error {
	switch m.config.MissingIdentity {
	case MissingIdentityError:
		return fmt.Errorf("MemoryMiddleware: missing identity in session (userIDKey=%s, sessionIDKey=%s), %s aborted", m.config.UserIDKey, m.config.SessionIDKey, stage)
	case MissingIdentityLog:
		log.Printf("MemoryMiddleware: session missing identity (userIDKey=%s, sessionIDKey=%s), skipping %s", m.config.UserIDKey, m.config.SessionIDKey, stage)
	}
	return nil
}

// BeforeAgent is called before the agent runs.
func (m *MemoryMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return ctx, runCtx, nil
}

// BeforeModelRewriteState injects memory context before a model call.
func (m *MemoryMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.AgenticMessage], mc *adk.TypedModelContext[*schema.AgenticMessage]) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	if m.provider == nil {
		return ctx, state, nil
	}

	uid, sid, ok := m.resolveIdentity(ctx)
	if !ok {
		if err := m.handleMissingIdentity("retrieve"); err != nil {
			return ctx, state, err
		}
		return ctx, state, nil
	}

	if prepared, ok := adk.GetSessionValue(ctx, m.beforeModelRewriteStateKey()); ok {
		if done, ok := prepared.(bool); ok && done {
			return ctx, state, nil
		}
	}

	// Call provider to retrieve context
	result, err := m.provider.Retrieve(ctx, &RetrieveRequest{
		UserID:    uid,
		SessionID: sid,
		Messages:  state.Messages,
	})
	if err != nil {
		log.Printf("MemoryMiddleware: Retrieve failed: %v", err)
		return ctx, state, nil
	}

	if result == nil {
		adk.AddSessionValue(ctx, m.beforeModelRewriteStateKey(), true)
		return ctx, state, nil
	}

	// 检索成功但发生降级（子步骤失败/预算裁剪）时让调用方可观测
	if len(result.Warnings) > 0 {
		log.Printf("MemoryMiddleware: Retrieve degraded: %v", result.Warnings)
	}

	// Split state.Messages into: first system message + rest.
	var systemMsg *schema.AgenticMessage
	var restMessages []*schema.AgenticMessage
	for _, msg := range state.Messages {
		if msg != nil && systemMsg == nil && msg.Role == schema.AgenticRoleTypeSystem {
			systemMsg = msg
		} else {
			restMessages = append(restMessages, msg)
		}
	}

	runtimeContext := buildRuntimeContext(result, latestUserText(restMessages), time.Now())
	var fallbackContextMessages []*schema.AgenticMessage
	if runtimeContext != "" {
		restMessages = append([]*schema.AgenticMessage(nil), restMessages...)
		currentUserText := latestUserText(restMessages)
		if originalUserMsg, ok := appendRuntimeContextToLatestUser(restMessages, runtimeContextSuffix(currentUserText, runtimeContext)); ok {
			adk.AddSessionValue(ctx, m.originalUserMessageKey(), originalUserMsg)
		} else {
			fallbackContextMessages = []*schema.AgenticMessage{schema.UserAgenticMessage(runtimeContext)}
		}
	}

	// Reassemble: stable system prompt → history → current user with runtime context.
	enhanced := make([]*schema.AgenticMessage, 0, 1+len(result.HistoryMessages)+len(fallbackContextMessages)+len(restMessages))
	if systemMsg != nil {
		enhanced = append(enhanced, systemMsg)
	}
	enhanced = append(enhanced, result.HistoryMessages...)
	enhanced = append(enhanced, fallbackContextMessages...)
	enhanced = append(enhanced, restMessages...)
	state.Messages = enhanced

	adk.AddSessionValue(ctx, m.beforeModelRewriteStateKey(), true)

	return ctx, state, nil
}

// AfterModelRewriteState stores assistant response after a model call.
func (m *MemoryMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.AgenticMessage], mc *adk.TypedModelContext[*schema.AgenticMessage]) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	if m.provider == nil {
		return ctx, state, nil
	}

	uid, sid, ok := m.resolveIdentity(ctx)
	if !ok {
		if err := m.handleMissingIdentity("memorize"); err != nil {
			return ctx, state, err
		}
		return ctx, state, nil
	}

	if len(state.Messages) == 0 {
		return ctx, state, nil
	}

	latestMsg := state.Messages[len(state.Messages)-1]
	if latestMsg == nil || latestMsg.Role != schema.AgenticRoleTypeAssistant {
		return ctx, state, nil
	}

	// Only persist the final natural-language assistant reply for this turn.
	// Intermediate assistant messages that contain tool calls are not final
	// user-visible answers and should never be stored as memories, even if
	// providers/models also include explanatory text in the same message.
	if agmsg.HasFunctionToolCall(latestMsg) || strings.TrimSpace(agmsg.Text(latestMsg)) == "" {
		return ctx, state, nil
	}

	// Find the latest user message for the current turn. When runtime context
	// was appended before the model call, memorize the original user message
	// instead of the augmented model input.
	var userMsg *schema.AgenticMessage
	if original, ok := adk.GetSessionValue(ctx, m.originalUserMessageKey()); ok {
		if originalMsg, ok := original.(*schema.AgenticMessage); ok {
			userMsg = originalMsg
		}
	}
	if userMsg == nil {
		for i := len(state.Messages) - 2; i >= 0; i-- {
			if state.Messages[i] != nil && state.Messages[i].Role == schema.AgenticRoleTypeUser {
				userMsg = state.Messages[i]
				break
			}
		}
	}
	assistantMsg := latestMsg

	var messagesToMemorize []*schema.AgenticMessage
	if userMsg != nil {
		messagesToMemorize = append(messagesToMemorize, userMsg)
	}
	messagesToMemorize = append(messagesToMemorize, assistantMsg)

	if len(messagesToMemorize) == 0 {
		return ctx, state, nil
	}

	// 深拷贝消息，normalizer 与异步写入都不与调用方共享对象
	req := &MemorizeRequest{
		UserID:    uid,
		SessionID: sid,
		Messages:  cloneMessages(messagesToMemorize),
	}
	for _, normalizer := range m.config.Normalizers {
		normalized, err := normalizer(ctx, req)
		if err != nil {
			log.Printf("MemoryMiddleware: memorize normalizer failed, skip this turn: %v", err)
			return ctx, state, nil
		}
		if normalized == nil {
			// normalizer 显式丢弃本次写入
			return ctx, state, nil
		}
		req = normalized
	}

	// provider 实现了受管异步写入时走统一生命周期（Flush/Shutdown 可等待），
	// 否则回退到游离 goroutine 的兼容路径。
	if async, ok := m.provider.(AsyncMemorizer); ok {
		if err := async.EnqueueMemorize(ctx, req); err != nil {
			log.Printf("MemoryMiddleware: EnqueueMemorize failed: %v", err)
		}
		return ctx, state, nil
	}

	messages := req.Messages
	timeout := m.config.MemorizeTimeout
	provider := m.provider
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := provider.Memorize(bgCtx, &MemorizeRequest{
			UserID:    req.UserID,
			SessionID: req.SessionID,
			Messages:  messages,
		}); err != nil {
			log.Printf("MemoryMiddleware: Memorize failed: %v", err)
		}
	}()

	return ctx, state, nil
}

func cloneMessages(messages []*schema.AgenticMessage) []*schema.AgenticMessage {
	cloned := make([]*schema.AgenticMessage, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		cloned = append(cloned, agmsg.Clone(msg))
	}
	return cloned
}

func (m *MemoryMiddleware) beforeModelRewriteStateKey() string {
	return fmt.Sprintf("__aggo_memory_middleware_prepared_%p", m)
}

func (m *MemoryMiddleware) originalUserMessageKey() string {
	return fmt.Sprintf("__aggo_memory_middleware_original_user_%p", m)
}

func buildRuntimeContext(result *RetrieveResult, currentUserText string, now time.Time) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	if !hasCurrentTimeContext(currentUserText) {
		b.WriteString("<current_time>")
		b.WriteString(formatRuntimeCurrentTime(now))
		b.WriteString("</current_time>")
	}
	for _, messages := range [][]*schema.AgenticMessage{result.ContextMessages, result.SystemMessages} {
		for _, msg := range messages {
			content := strings.TrimSpace(agmsg.Text(msg))
			if content == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(content)
		}
	}
	return b.String()
}

func latestUserText(messages []*schema.AgenticMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.AgenticRoleTypeUser {
			return agmsg.Text(messages[i])
		}
	}
	return ""
}

func appendRuntimeContextToLatestUser(messages []*schema.AgenticMessage, runtimeContext string) (*schema.AgenticMessage, bool) {
	if strings.TrimSpace(runtimeContext) == "" {
		return nil, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.AgenticRoleTypeUser {
			original := agmsg.Clone(messages[i])
			augmented := agmsg.Clone(messages[i])
			agmsg.AppendUserText(augmented, runtimeContext)
			messages[i] = augmented
			return original, true
		}
	}
	return nil, false
}

func runtimeContextSuffix(currentUserText, runtimeContext string) string {
	if strings.TrimSpace(runtimeContext) == "" {
		return ""
	}
	if strings.Contains(currentUserText, runtimeContextDivider) {
		return "\n" + runtimeContext
	}
	return runtimeContextDivider + runtimeContext
}

func hasCurrentTimeContext(text string) bool {
	return strings.Contains(text, "<current_time") || strings.Contains(text, "[当前时间]")
}

func formatRuntimeCurrentTime(t time.Time) string {
	return t.Format(runtimeContextLayout)
}
