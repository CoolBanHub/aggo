package state

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

type ChatSate struct {
	UserID    string
	SessionID string
	Input     []*schema.Message
}

func SetChatChatSate(ctx context.Context, state *ChatSate) context.Context {
	ctx = context.WithValue(ctx, "chat_context", state)
	return ctx
}

func GetChatChatSate(ctx context.Context) *ChatSate {
	v := ctx.Value("chat_context")
	if v == nil {
		return nil
	}
	return v.(*ChatSate)
}

// GetSessionID 从 context 中获取 sessionID 的便捷方法
func GetSessionID(ctx context.Context) string {
	state := GetChatChatSate(ctx)
	if state == nil {
		return ""
	}
	return state.SessionID
}

// GetUserID 从 context 中获取 userID 的便捷方法
func GetUserID(ctx context.Context) string {
	state := GetChatChatSate(ctx)
	if state == nil {
		return ""
	}
	return state.UserID
}
