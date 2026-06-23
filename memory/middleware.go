package memory

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMemorizeTimeout = 2 * time.Minute
	runtimeContextLayout   = "2006-01-02 15:04:05 -07:00"
	runtimeContextDivider  = "\n\n-----\n"
)

// MemoryMiddleware implements adk.ChatModelAgentMiddleware.
// It delegates to a MemoryProvider for retrieval and memorization.
type MemoryMiddleware struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
	provider MemoryProvider
}

// NewMemoryMiddleware creates a MemoryMiddleware with a MemoryProvider.
func NewMemoryMiddleware(provider MemoryProvider) *MemoryMiddleware {
	return &MemoryMiddleware{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]{},
		provider:                          provider,
	}
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

	sessionID, _ := adk.GetSessionValue(ctx, "sessionID")
	userID, _ := adk.GetSessionValue(ctx, "userID")
	sid, _ := sessionID.(string)
	uid, _ := userID.(string)
	if sid == "" || uid == "" {
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

	if len(messagesToMemorize) > 0 {
		messagesToMemorize = append([]*schema.AgenticMessage(nil), messagesToMemorize...)
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), defaultMemorizeTimeout)
			defer cancel()
			if err := m.provider.Memorize(bgCtx, &MemorizeRequest{
				UserID:    uid,
				SessionID: sid,
				Messages:  messagesToMemorize,
			}); err != nil {
				log.Printf("MemoryMiddleware: Memorize failed: %v", err)
			}
		}()
	}

	return ctx, state, nil
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
