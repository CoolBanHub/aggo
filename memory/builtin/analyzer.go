package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	// 替换时间占位符
	prompt := strings.ReplaceAll(u.systemPrompt, "{{current_time}}", time.Now().Format("2006-01-02 15:04"))

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: prompt,
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
	for _, v := range historyMessages {
		messages = append(messages, v.ToSchemaMessage())
	}

	response, err := u.cm.Generate(ctx, messages)
	if err != nil {
		return false, "", fmt.Errorf("分析用户记忆失败: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	if content == "" {
		return false, "", nil
	}

	var param UserMemoryAnalyzerParam
	err = json.Unmarshal([]byte(content), &param)
	if err != nil {
		return false, "", fmt.Errorf("解析用户记忆响应失败(raw=%q): %w", content, err)
	}

	// 如果是 noop 操作，不需要更新
	if param.Op == UserMemoryOpNoop {
		return false, "", nil
	}

	return true, param.Memory, nil
}
