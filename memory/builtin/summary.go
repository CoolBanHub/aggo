package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// SessionSummaryGenerator 基于AI的会话摘要生成器
type SessionSummaryGenerator struct {
	cm                model.ToolCallingChatModel
	summaryPrompt     string
	incrementalPrompt string
}

// NewSessionSummaryGenerator 创建新的会话摘要生成器
func NewSessionSummaryGenerator(cm model.ToolCallingChatModel) *SessionSummaryGenerator {
	return &SessionSummaryGenerator{
		cm:                cm,
		summaryPrompt:     DefaultSessionSummaryPrompt,
		incrementalPrompt: DefaultIncrementalSessionSummaryPrompt,
	}
}

// SetSummaryPrompt 自定义完整摘要系统提示词
func (s *SessionSummaryGenerator) SetSummaryPrompt(prompt string) {
	s.summaryPrompt = prompt
}

// SetIncrementalPrompt 自定义增量摘要系统提示词
func (s *SessionSummaryGenerator) SetIncrementalPrompt(prompt string) {
	s.incrementalPrompt = prompt
}

// GenerateSummary 生成会话摘要
func (s *SessionSummaryGenerator) GenerateSummary(ctx context.Context, messages []*ConversationMessage, existingSummary string) (string, error) {
	if len(messages) == 0 {
		return existingSummary, nil
	}

	// 构建提示消息
	systemPrompt := strings.ReplaceAll(s.summaryPrompt, "{{current_time}}", time.Now().Format("2006-01-02 15:04"))

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

	// 将对话消息转换为 schema.Message
	for _, msg := range messages {
		promptMessages = append(promptMessages, msg.ToSchemaMessage())
	}

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

	systemPrompt := strings.ReplaceAll(s.incrementalPrompt, "{{current_time}}", time.Now().Format("2006-01-02 15:04"))

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

	// 将最新对话消息转换
	for _, msg := range recentMessages {
		promptMessages = append(promptMessages, msg.ToSchemaMessage())
	}

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
