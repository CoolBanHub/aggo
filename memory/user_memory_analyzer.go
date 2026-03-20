package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// UserMemoryAnalyzer 分析对话并更新用户记忆
type UserMemoryAnalyzer struct {
	cm           model.ToolCallingChatModel
	systemPrompt string
}

// NewUserMemoryAnalyzer 创建新的用户记忆分析器
func NewUserMemoryAnalyzer(cm model.ToolCallingChatModel) *UserMemoryAnalyzer {
	systemPrompt := DefaultUserMemoryPrompt
	return &UserMemoryAnalyzer{
		cm:           cm,
		systemPrompt: systemPrompt,
	}
}

func (u *UserMemoryAnalyzer) SetSystemPrompt(systemPrompt string) {
	u.systemPrompt = systemPrompt
}

// ShouldUpdateMemory 分析对话并生成更新后的记忆内容
// 返回值: (是否需要更新, 更新后的记忆内容, 错误)
func (u *UserMemoryAnalyzer) ShouldUpdateMemory(ctx context.Context, existingMemory *UserMemory, historyMessages []*ConversationMessage) (bool, string, error) {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: u.systemPrompt,
		},
	}

	// 添加现有记忆（如果有）
	if existingMemory != nil && existingMemory.Memory != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("## 现有记忆\n%s", existingMemory.Memory),
		})
	}

	// 添加历史消息作为上下文
	if len(historyMessages) > 0 {
		for _, v := range historyMessages {
			userMessage := &schema.Message{
				Role: schema.RoleType(v.Role),
			}
			userMessage.Content = v.Content
			if len(v.Parts) > 0 {
				multiContent := make([]schema.MessageInputPart, 0, len(v.Parts))
				multiContent = append(multiContent, v.Parts...)
				userMessage.UserInputMultiContent = multiContent
			}
			messages = append(messages, userMessage)
		}
	}

	response, err := u.cm.Generate(ctx, messages)
	if err != nil {
		return false, "", fmt.Errorf("分析用户记忆失败: %w", err)
	}

	var param UserMemoryAnalyzerParam
	err = json.Unmarshal([]byte(response.Content), &param)
	if err != nil {
		return false, "", err
	}

	// 如果是 noop 操作，不需要更新
	if param.Op == UserMemoryOpNoop {
		return false, "", nil
	}

	return true, param.Memory, nil
}
