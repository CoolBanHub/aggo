package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/schema"
)

func main() {
	ctx := context.Background()
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-nano"),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}

	bot, err := agent.NewAgent(ctx, cm,
		agent.WithSystemPrompt("你是一个linux大师"))

	conversations := []string{
		"你好，我是Alice",
		"我是一名软件工程师，专门做后端开发",
		"我住在北京，今年28岁",
		"你有什么爱好吗?",
		"我喜欢读书和摄影，特别是科幻小说",
		"我最近在学习Go语言和云原生技术",
		"我的工作主要涉及微服务架构设计",
		"周末我通常会去公园拍照或者在家看书",
		"你能给我推荐一些适合我的技术书籍吗？",
		"你还记得我之前说过我的职业是什么吗？",
		"基于你对我的了解，你觉得我适合学习什么新技术？",
		"我们年龄相差多少岁呢",
		"你喜欢吃什么水果吗？我喜欢吃苹果",
	}
	for _, conversation := range conversations {
		log.Printf("User: %s", conversation)
		out, err := bot.Stream(context.Background(), []*schema.Message{
			schema.UserMessage(conversation),
		})
		if err != nil {
			log.Fatalf("generate fail,err:%s", err)
			return
		}
		for {
			o, err2 := out.Recv()
			if err2 != nil {
				fmt.Println("err2:", err2)
				break
			}
			log.Printf("AI:%s", o.Content)
		}
	}
}
