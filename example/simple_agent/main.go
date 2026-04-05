package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf("提示: 未找到 .env 文件，将使用系统环境变量")
	}

	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	ag, err := agent.NewAgentBuilder(chatModel).
		WithInstruction("你是一个友好的 {name}助手").
		Build(ctx)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})
	iter := runner.Run(ctx, []*schema.Message{
		schema.UserMessage("你是什么助手"),
	}, adk.WithSessionValues(map[string]any{
		"name": "大昌",
	}))

	var response string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			log.Fatalf("生成回复失败: %v", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			if msg, err := event.Output.MessageOutput.GetMessage(); err == nil && msg != nil {
				response = msg.Content
			}
		}
	}

	fmt.Printf("🤖 AI: %s\n", response)
}
