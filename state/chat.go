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
