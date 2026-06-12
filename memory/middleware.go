package memory

import (
	"context"
	"fmt"
	"log"
	"strings"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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

	// Split state.Messages into: first system message + rest
	var systemMsg *schema.AgenticMessage
	var restMessages []*schema.AgenticMessage
	for _, msg := range state.Messages {
		if systemMsg == nil && msg.Role == schema.AgenticRoleTypeSystem {
			systemMsg = msg
		} else {
			restMessages = append(restMessages, msg)
		}
	}

	// Merge memory context into the system prompt content.
	if len(result.SystemMessages) > 0 && systemMsg != nil {
		var memoryBlock strings.Builder
		for i, sm := range result.SystemMessages {
			content := agmsg.Text(sm)
			if content != "" {
				if i > 0 {
					memoryBlock.WriteString("\n")
				}
				memoryBlock.WriteString(content)
				memoryBlock.WriteString("\n")
			}
		}
		agmsg.AppendUserText(systemMsg, "\n\n"+memoryBlock.String())
	}

	// Reassemble: system prompt → history → rest of conversation.
	enhanced := make([]*schema.AgenticMessage, 0, 1+len(result.HistoryMessages)+len(restMessages))
	if systemMsg != nil {
		enhanced = append(enhanced, systemMsg)
	}
	enhanced = append(enhanced, result.HistoryMessages...)
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

	// Find the latest user message for the current turn.
	var userMsg *schema.AgenticMessage
	for i := len(state.Messages) - 2; i >= 0; i-- {
		if state.Messages[i].Role == schema.AgenticRoleTypeUser {
			userMsg = state.Messages[i]
			break
		}
	}
	assistantMsg := latestMsg

	var messagesToMemorize []*schema.AgenticMessage
	if userMsg != nil {
		messagesToMemorize = append(messagesToMemorize, userMsg)
	}
	messagesToMemorize = append(messagesToMemorize, assistantMsg)

	if len(messagesToMemorize) > 0 {
		go func() {
			bgCtx := context.Background()
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
