package main

import (
	"context"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}

	bot, err := agent.NewAgent(ctx, cm,
		agent.WithSystemPrompt("你是一个linux大师"),
		agent.WithTools(shell.GetExecuteTools()),
	)
	if err != nil {
		log.Fatalf("new agent fail,err:%s", err)
		return
	}

	conversations := []string{
		"帮我看一下当前目录有什么文件",
		"帮我看一下内存使用情况",
	}

	for _, conversation := range conversations {
		log.Printf("User: %s", conversation)
		out, err := bot.Generate(ctx, []*schema.Message{
			schema.UserMessage(conversation),
		}, agent.WithChatTools(shell.GetTools()))
		if err != nil {
			log.Fatalf("generate fail,err:%s", err)
			return
		}
		log.Printf("AI:%s", out.Content)
	}
}
