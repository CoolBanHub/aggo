package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// SessionSummaryGenerator 基于AI的会话摘要生成器
type SessionSummaryGenerator struct {
	cm model.ToolCallingChatModel
}

// NewSessionSummaryGenerator 创建新的会话摘要生成器
func NewSessionSummaryGenerator(cm model.ToolCallingChatModel) *SessionSummaryGenerator {
	return &SessionSummaryGenerator{
		cm: cm,
	}
}

// GenerateSummary 生成会话摘要
func (s *SessionSummaryGenerator) GenerateSummary(ctx context.Context, messages []*ConversationMessage, existingSummary string) (string, error) {
	if len(messages) == 0 {
		return existingSummary, nil
	}

	// 构建提示消息
	systemPrompt := DefaultSessionSummaryPrompt

	promptMessages := []*schema.Message{
		{
			Role:    schema.System,
			Content: systemPrompt,
		},
	}

	// 如果有现有摘要，添加到上下文中
	if existingSummary != "" {
		promptMessages = append(promptMessages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("## 现有摘要\n%s\n\n请基于现有摘要和新的对话内容，生成更新后的摘要。", existingSummary),
		})
	}

	// 将对话消息转换为多部分内容格式
	schemaMessages := s.convertConversationMessages(messages)
	promptMessages = append(promptMessages, schemaMessages...)

	// 生成摘要
	response, err := s.cm.Generate(ctx, promptMessages)
	if err != nil {
		return "", fmt.Errorf("生成会话摘要失败: %w", err)
	}

	// 清理并返回摘要内容
	summary := strings.TrimSpace(response.Content)
	if summary == "" {
		return existingSummary, nil
	}

	return summary, nil
}

// GenerateIncrementalSummary 生成增量摘要（基于最新消息更新现有摘要）
func (s *SessionSummaryGenerator) GenerateIncrementalSummary(ctx context.Context, recentMessages []*ConversationMessage, existingSummary string) (string, error) {
	if len(recentMessages) == 0 {
		return existingSummary, nil
	}

	// 如果没有现有摘要，直接生成新摘要
	if existingSummary == "" {
		return s.GenerateSummary(ctx, recentMessages, "")
	}

	systemPrompt := DefaultIncrementalSessionSummaryPrompt

	promptMessages := []*schema.Message{
		{
			Role:    schema.System,
			Content: systemPrompt,
		},
		{
			Role:    schema.System,
			Content: fmt.Sprintf("## 现有摘要\n%s", existingSummary),
		},
	}

	// 将最新对话消息转换为多部分内容格式
	schemaMessages := s.convertConversationMessages(recentMessages)
	promptMessages = append(promptMessages, schemaMessages...)

	// 添加更新指令
	promptMessages = append(promptMessages, &schema.Message{
		Role:    schema.User,
		Content: "请基于以上最新对话内容，更新摘要以包含新的信息。",
	})

	response, err := s.cm.Generate(ctx, promptMessages)
	if err != nil {
		return existingSummary, fmt.Errorf("生成增量摘要失败: %w", err)
	}

	summary := strings.TrimSpace(response.Content)
	if summary == "" {
		return existingSummary, nil
	}

	return summary, nil
}

// convertConversationMessages 将ConversationMessage转换为schema.Message，支持多部分内容
func (s *SessionSummaryGenerator) convertConversationMessages(messages []*ConversationMessage) []*schema.Message {
	schemaMessages := make([]*schema.Message, 0, len(messages))

	for _, msg := range messages {
		schemaMsg := &schema.Message{
			Role: schema.RoleType(msg.Role),
		}
		schemaMsg.Content = msg.Content
		if len(msg.Parts) > 0 {
			multiContent := make([]schema.MessageInputPart, 0, len(msg.Parts))
			multiContent = append(multiContent, msg.Parts...)
			schemaMsg.UserInputMultiContent = multiContent
		}
		schemaMessages = append(schemaMessages, schemaMsg)
	}

	return schemaMessages
}
