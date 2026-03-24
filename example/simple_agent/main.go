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

/*
=============================================================================
最简单的 Agent 示例
=============================================================================

本示例展示如何创建一个最基础的 AI Agent，进行简单的对话。

使用方法:
1. 复制 .env.example 为 .env
2. 填写你的 API 配置
3. 运行: go run main.go
*/

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("提示: 未找到 .env 文件，将使用系统环境变量")
	}
	// ============================================================
	// Step 1: 创建聊天模型
	// ============================================================
	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	// ============================================================
	// Step 2: 创建 Agent
	// ============================================================
	ag, err := agent.NewAgent(ctx, chatModel, agent.WithSystemPrompt("你是一个友好的 {name}助手"))
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	// ============================================================
	// Step 3: 进行对话
	// 注意：session values 需要通过 WithSessionValues 传递
	// ============================================================
	response, err := ag.Generate(ctx, []*schema.Message{
		schema.UserMessage("你是什么助手"),
	}, agent.WithAdkAgentRunOptions(adk.WithSessionValues(map[string]any{
		"name": "大昌",
	})))
	if err != nil {
		log.Fatalf("生成回复失败: %v", err)
	}

	// ============================================================
	// Step 4: 输出结果
	// ============================================================
	fmt.Printf("🤖 AI: %s\n", response.Content)
}
