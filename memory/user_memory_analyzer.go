package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// UserMemoryAnalyzer determines if a message should be stored as a memory
type UserMemoryAnalyzer struct {
	cm           model.ToolCallingChatModel
	systemPrompt string
}

// NewUserMemoryAnalyzer creates a new MemoryClassifier instance
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

// ShouldUpdateMemoryWithParts determines if a message with multi-part content should be added to memory
func (u *UserMemoryAnalyzer) ShouldUpdateMemoryWithParts(ctx context.Context, userMemoryList []*UserMemory, historyMessages []*ConversationMessage) ([]UserMemoryAnalyzerParam, error) {

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: u.systemPrompt,
		},
	}

	if len(userMemoryList) > 0 {

		var existingMemories strings.Builder
		existingMemories.WriteString("## 现有记忆列表\n")
		for _, v := range userMemoryList {
			existingMemories.WriteString(fmt.Sprintf("- **ID**: %s\n  **内容**: %s\n", v.ID, v.Memory))
		}

		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: existingMemories.String(),
		})
	}

	// 添加历史消息作为上下文
	if len(historyMessages) > 0 {
		for _, v := range historyMessages {
			// 构建用户消息，支持多部分内容
			userMessage := &schema.Message{
				Role: schema.RoleType(v.Role),
			}
			userMessage.Content = v.Content
			// 如果有多部分内容，使用UserInputMultiContent
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
		return nil, fmt.Errorf("failed to classify memory: %w", err)
	}

	var params []UserMemoryAnalyzerParam

	err = json.Unmarshal([]byte(response.Content), &params)
	if err != nil {
		return nil, err
	}
	return params, nil
}
