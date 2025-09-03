package main

import (
	"context"
	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools"
	"github.com/cloudwego/eino/schema"
	"log"
	"os"
)

func main() {
	ctx := context.Background()
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIVersion(os.Getenv("APIVersion")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-mini"),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}

	bot, err := agent.NewAgent(ctx, cm,
		agent.WithSystemPrompt("你是一个linux大师"),
		agent.WithTools(tools.GetExecuteTool()),
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
		}, agent.ConvToolsToCompose(tools.GetSysInfoTool())...)
		if err != nil {
			log.Fatalf("generate fail,err:%s", err)
			return
		}
		log.Printf("AI:%s", out.Content)
	}
}
