package memory

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// MemoryMiddleware implements adk.ChatModelAgentMiddleware.
// It delegates to a MemoryProvider for retrieval and memorization.
type MemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	provider MemoryProvider
}

// NewMemoryMiddleware creates a MemoryMiddleware with a MemoryProvider.
func NewMemoryMiddleware(provider MemoryProvider) *MemoryMiddleware {
	return &MemoryMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		provider:                     provider,
	}
}

// BeforeAgent is called before the agent runs.
func (m *MemoryMiddleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	return ctx, runCtx, nil
}

// BeforeModelRewriteState injects memory context before a model call.
func (m *MemoryMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
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
	var systemMsg *schema.Message
	var restMessages []*schema.Message
	for _, msg := range state.Messages {
		if systemMsg == nil && msg.Role == schema.System {
			systemMsg = msg
		} else {
			restMessages = append(restMessages, msg)
		}
	}

	// Merge memory context into the system prompt content.
	if len(result.SystemMessages) > 0 && systemMsg != nil {
		var memoryBlock strings.Builder
		for i, sm := range result.SystemMessages {
			if sm.Content != "" {
				if i > 0 {
					memoryBlock.WriteString("\n")
				}
				memoryBlock.WriteString(sm.Content)
				memoryBlock.WriteString("\n")
			}
		}
		systemMsg.Content = systemMsg.Content + "\n\n" + memoryBlock.String()
	}

	// Reassemble: system prompt → history → rest of conversation.
	enhanced := make([]*schema.Message, 0, 1+len(result.HistoryMessages)+len(restMessages))
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
func (m *MemoryMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, mc *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
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
	if latestMsg == nil || latestMsg.Role != schema.Assistant {
		return ctx, state, nil
	}

	// Only persist the final natural-language assistant reply for this turn.
	// Intermediate assistant messages that contain tool calls are not final
	// user-visible answers and should never be stored as memories, even if
	// providers/models also include explanatory text in the same message.
	if len(latestMsg.ToolCalls) > 0 || strings.TrimSpace(latestMsg.Content) == "" {
		return ctx, state, nil
	}

	// Find the latest user message for the current turn.
	var userMsg *schema.Message
	for i := len(state.Messages) - 2; i >= 0; i-- {
		if state.Messages[i].Role == schema.User {
			userMsg = state.Messages[i]
			break
		}
	}
	assistantMsg := latestMsg

	var messagesToMemorize []*schema.Message
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
