package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}

	shellTools := shell.GetTools()

	cwd, _ := os.Getwd()
	skillsDir := filepath.Join(cwd, "skills")

	localBackend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		log.Fatalf("Failed to create local backend: %v", err)
	}

	backend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: localBackend,
		BaseDir: skillsDir,
	})
	if err != nil {
		log.Fatalf("Failed to create backend: %v", err)
	}

	skillMiddleware, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: backend,
	})
	if err != nil {
		log.Fatalf("Failed to create skill middleware: %v", err)
	}

	systemPrompt := `你是一个 Skill 创建助手。

## 工作流程
当用户请求创建或更新 skill 时，请按以下步骤操作：

1. **加载 Skill**: 首先使用 skill 工具加载 "skill-creator" skill
2. **理解需求**: 根据 skill 创建指南，了解用户的具体需求
3. **创建 Skill**: 按照 skill-creator 中的指南，帮助用户创建或更新 skill
4. **返回结果**: 将创建的 skill 结构和内容清晰地返回给用户

## 重要提示
- skill-creator skill 中包含了完整的 skill 创建流程和最佳实践
- 遵循渐进式披露原则，保持 SKILL.md 简洁
- 使用正确的 YAML frontmatter 格式`

	ag, err := agent.NewAgentBuilder(chatModel).
		WithName("skill-creator-assistant").
		WithDescription("Skill 创建助手，可以帮助用户创建和更新 AgentSkills").
		WithInstruction(systemPrompt).
		WithTools(shellTools...).
		WithMiddlewares(skillMiddleware).
		Build(ctx)

	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})

	query := "请帮我创建一个用于处理 PDF 文件的 skill"
	iter := runner.Run(ctx, []*schema.Message{
		schema.UserMessage(query),
	})

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
