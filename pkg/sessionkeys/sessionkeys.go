// Package sessionkeys 定义 AGGO 工具与 middleware 从 adk 运行会话中解析身份时
// 使用的标准 session value 键，并提供常用的解析函数。
//
// 应用方应通过 adk.WithSessionValues / adk.AddSessionValue 注入这些键，
// 而不是在业务代码里散落硬编码字符串。
package sessionkeys

import (
	"context"

	"github.com/cloudwego/eino/adk"
)

// AGGO 约定的标准身份键。
const (
	// KeyUserID 用户身份键。单租户应用通常直接用它标识操作者，
	// 与 KeyActorID 语义等价，解析时两者互相兜底。
	KeyUserID = "userID"

	// KeyActorID 实际发言人/操作者 ID，用于权限判断与审计。
	KeyActorID = "actorID"

	// KeyConversationID 对话目标 ID（群聊或私聊），用于对话隔离与通知投递。
	KeyConversationID = "conversationID"

	// KeySessionID 一次会话窗口 ID，用于上下文窗口与记忆隔离。
	KeySessionID = "sessionID"
)

// String 读取指定键的字符串值；缺失、非字符串或空串都视为不存在。
func String(ctx context.Context, key string) (string, bool) {
	v, ok := adk.GetSessionValue(ctx, key)
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// ActorID 解析实际操作者：actorID 优先，其次 userID。
func ActorID(ctx context.Context) (string, bool) {
	if id, ok := String(ctx, KeyActorID); ok {
		return id, true
	}
	return String(ctx, KeyUserID)
}

// ResolveOwner 解析资源归属者（如 cron 任务的 owner）：
// actorID → userID → sessionID。
//
// sessionID 兜底是为了兼容把“会话”当作归属单元的存量部署
// （例如群聊机器人以 chatID 作为 userID）。找不到任何身份时返回 ok=false，
// 调用方必须显式拒绝，不能退化为空字符串。
func ResolveOwner(ctx context.Context) (string, bool) {
	if id, ok := ActorID(ctx); ok {
		return id, true
	}
	return String(ctx, KeySessionID)
}

// ResolveConversation 解析对话目标：conversationID 优先，其次 sessionID。
func ResolveConversation(ctx context.Context) (string, bool) {
	if id, ok := String(ctx, KeyConversationID); ok {
		return id, true
	}
	return String(ctx, KeySessionID)
}
