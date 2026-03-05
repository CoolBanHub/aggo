/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package main demonstrates how to use the skill middleware with ChatModelAgent.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/compose"
	"github.com/joho/godotenv"
)

/*
=============================================================================
Skill Middleware 使用示例
=============================================================================

本示例演示如何使用 skill middleware 创建一个能够帮助用户创建新 skill 的 Agent。

工作流程:
1. Agent 收到用户请求
2. Agent 识别需要使用 skill-creator skill
3. Agent 调用 skill 工具加载 skill 内容（包含 skill 创建指南）
4. Agent 根据 skill 指令帮助用户创建或更新 skill

Skills 目录结构:
----------------
./skills/
└── skill-creator/
    └── SKILL.md      # Skill 创建指南
*/

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	// ============================================================
	// Step 1: 创建实际的 ChatModel (从环境变量获取配置)
	// ============================================================
	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}

	// ============================================================
	// Step 2: 获取 shell 工具
	// ============================================================
	shellTools := shell.GetTools()

	// ============================================================
	// Step 3: 创建 LocalBackend 从文件系统加载 skills
	// ============================================================
	skillsDir := "./skills"

	localBackend, err := skill.NewLocalBackend(&skill.LocalBackendConfig{
		BaseDir: skillsDir,
	})
	if err != nil {
		log.Fatalf("Failed to create local backend: %v", err)
	}

	// ============================================================
	// Step 4: 创建 skill middleware
	// ============================================================
	skillMiddleware, err := skill.New(ctx, &skill.Config{
		Backend:    localBackend,
		UseChinese: true,
	})
	if err != nil {
		log.Fatalf("Failed to create skill middleware: %v", err)
	}

	fmt.Println("=== Skill Middleware ===")
	fmt.Println("AdditionalInstruction 长度:", len(skillMiddleware.AdditionalInstruction))
	fmt.Println("AdditionalTools 数量:", len(skillMiddleware.AdditionalTools))
	fmt.Println()

	// ============================================================
	// Step 5: 创建 ChatModelAgent
	// ============================================================
	// 合并所有工具: skill 工具 (由 middleware 自动添加) + shell 工具
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "skill-creator-assistant",
		Description: "Skill 创建助手，可以帮助用户创建和更新 AgentSkills",
		Instruction: `你是一个 Skill 创建助手。

## 工作流程
当用户请求创建或更新 skill 时，请按以下步骤操作：

1. **加载 Skill**: 首先使用 skill 工具加载 "skill-creator" skill
2. **理解需求**: 根据 skill 创建指南，了解用户的具体需求
3. **创建 Skill**: 按照 skill-creator 中的指南，帮助用户创建或更新 skill
4. **返回结果**: 将创建的 skill 结构和内容清晰地返回给用户

## 重要提示
- skill-creator skill 中包含了完整的 skill 创建流程和最佳实践
- 遵循渐进式披露原则，保持 SKILL.md 简洁
- 使用正确的 YAML frontmatter 格式`,
		Model: chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: shellTools, // 添加 shell 工具
			},
		},
		Middlewares: []adk.AgentMiddleware{skillMiddleware},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// ============================================================
	// Step 6: 创建 runner 并运行 agent
	// ============================================================
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	})

	// ============================================================
	// Step 7: 执行查询
	// ============================================================
	query := "请帮我创建一个用于处理 PDF 文件的 skill"

	fmt.Println("=== Running Agent ===")
	fmt.Printf("Query: %s\n\n", query)

	iter := runner.Query(ctx, query)

	// 处理来自 agent 的事件
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			fmt.Printf("❌ Error: %v\n", event.Err)
			continue
		}

		if event.Output != nil && event.Output.MessageOutput != nil {
			msg := event.Output.MessageOutput.Message
			if msg != nil && msg.Content != "" {
				role := event.Output.MessageOutput.Role
				if role == "assistant" {
					fmt.Printf("🤖 Assistant: %s\n", msg.Content)
				} else if role == "tool" {
					fmt.Printf("🔧 Tool [%s]: %s\n", event.Output.MessageOutput.ToolName, truncate(msg.Content, 500))
				}
			}
		}

		if event.Action != nil {
			if event.Action.Interrupted != nil {
				fmt.Printf("⏸️ Agent interrupted: %+v\n", event.Action.Interrupted)
			}
			if event.Action.Exit {
				fmt.Println("✅ Agent exited")
			}
			if event.Action.TransferToAgent != nil {
				fmt.Printf("🔄 Transferring to agent: %s\n", event.Action.TransferToAgent.DestAgentName)
			}
		}
	}

	fmt.Println()
	fmt.Println("=== Agent Execution Completed ===")
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
