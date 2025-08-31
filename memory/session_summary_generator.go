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

	// 构建对话历史字符串
	var conversationHistory strings.Builder
	for _, msg := range messages {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		conversationHistory.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	// 构建提示消息
	systemPrompt := `# 会话摘要生成任务

## 目标
基于提供的对话历史，生成一份简洁而全面的会话摘要。

## 摘要要求
1. **简洁性**: 摘要应简明扼要，避免冗余信息
2. **全面性**: 涵盖对话的主要主题、关键信息和重要结论
3. **连贯性**: 保持逻辑清晰，便于理解对话脉络
4. **重点突出**: 着重记录用户的需求、问题解决过程和重要决策

## 摘要内容应包含
- **主要话题**: 对话涉及的核心主题
- **用户需求**: 用户提出的主要问题或需求
- **解决方案**: 提供的建议、方案或答案
- **关键信息**: 重要的事实、数据或结论
- **待解决问题**: 尚未完全解决的问题或后续计划

## 输出格式
直接输出摘要内容，无需其他格式标记。摘要应在150-300字之间。

## 注意事项
- 保持客观中立的语调
- 避免重复已有摘要中的信息（如果提供了现有摘要）
- 重点关注最新的对话内容
- 如果是延续性对话，应与之前的摘要形成连贯的整体`

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

	// 添加对话历史
	promptMessages = append(promptMessages, &schema.Message{
		Role:    schema.User,
		Content: fmt.Sprintf("## 对话历史\n%s\n请为以上对话生成摘要。", conversationHistory.String()),
	})

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

	// 构建最新对话内容
	var recentContent strings.Builder
	for _, msg := range recentMessages {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		recentContent.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	systemPrompt := `# 增量摘要更新任务

## 目标
基于现有摘要和最新的对话内容，生成更新后的摘要。

## 更新原则
1. **保持连贯**: 确保新摘要与之前的内容逻辑连贯
2. **整合信息**: 将新对话的关键信息融入现有摘要
3. **避免重复**: 不要重复现有摘要中已包含的信息
4. **突出重点**: 重点关注新对话中的重要进展或变化

## 输出要求
直接输出更新后的完整摘要，保持在150-400字之间。`

	promptMessages := []*schema.Message{
		{
			Role:    schema.System,
			Content: systemPrompt,
		},
		{
			Role:    schema.System,
			Content: fmt.Sprintf("## 现有摘要\n%s", existingSummary),
		},
		{
			Role:    schema.User,
			Content: fmt.Sprintf("## 最新对话内容\n%s\n请更新摘要以包含最新的对话信息。", recentContent.String()),
		},
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
