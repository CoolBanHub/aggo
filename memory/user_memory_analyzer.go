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
	cm model.ToolCallingChatModel
}

// NewUserMemoryAnalyzer creates a new MemoryClassifier instance
func NewUserMemoryAnalyzer(cm model.ToolCallingChatModel) *UserMemoryAnalyzer {
	return &UserMemoryAnalyzer{
		cm: cm,
	}
}

// ShouldUpdateMemory determines if a message should be added to memory
func (u *UserMemoryAnalyzer) ShouldUpdateMemory(ctx context.Context, input string, userMemoryList []*UserMemory) ([]UserMemoryAnalyzerParam, error) {

	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `# 用户记忆分析任务

## 目标
分析用户输入，判断是否包含值得长期记忆的个性化信息，并对现有记忆进行相应操作。

## 值得记忆的信息类型
- **个人基础信息**：姓名、年龄、职业、学历、所在地等
- **兴趣偏好**：爱好、喜好、厌恶、品味、习惯等  
- **重要经历**：工作经历、教育背景、生活事件等
- **关系网络**：家庭成员、朋友、同事等重要人际关系
- **目标计划**：短期计划、长期目标、正在进行的项目等
- **观点态度**：价值观、信仰、对特定话题的看法等
- **技能专长**：专业技能、特殊能力、擅长领域等
- **生活状况**：当前处境、面临的挑战、健康状况等

## 操作类型
- **create**: 创建新记忆（发现新信息时）
- **update**: 更新现有记忆（信息有变化或需要补充时）  
- **del**: 删除记忆（信息过时或错误时）

## 输出格式
严格按照以下JSON数组格式输出，不要包含任何其他文字：
[{"op":"操作类型","id":"记忆ID","memory":"记忆内容"}]

## 字段说明
- **op**: 必填，值为 "create"、"update" 或 "del"
- **id**: create时不填，update/del时必填现有记忆的ID
- **memory**: create/update时必填，del时不填

## 处理规则
1. 如果用户输入不包含任何值得记忆的信息，输出：[]
2. 优先更新现有记忆而非创建重复记忆
3. 记忆内容应简洁明确，去除冗余信息
4. 一次输入可能需要多个操作，全部放入数组中
5. 删除明确过时或矛盾的记忆

## 示例
用户说"我叫张三，今年换了新工作"，且已有记忆"姓名：张三"：
[{"op":"create","memory":"最近换了新工作"}]`,
		},
	}

	if len(userMemoryList) > 0 {

		var existingMemories strings.Builder
		existingMemories.WriteString("## 现有记忆列表\n")
		for _, v := range userMemoryList {
			existingMemories.WriteString(fmt.Sprintf("- **ID**: %s\n  **内容**: %s\n", v.ID, v.Memory))
		}
		existingMemories.WriteString("\n请基于以上现有记忆和用户新输入，决定执行的操作。")

		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: existingMemories.String(),
		})
	}

	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: input,
	})

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
