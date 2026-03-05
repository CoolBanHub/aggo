package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CoolBanHub/aggo/agent/cron_agent"
	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	// 创建聊天模型
	cm, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	// 创建 CronAgent（使用文件存储）
	ca, err := cron_agent.New(ctx, cm,
		cron_agent.WithFileStore("cron_jobs.json"),
		cron_agent.WithOnJobTriggered(func(job *cronPkg.CronJob) {
			fmt.Printf("\n🔔 [定时任务触发] %s: %s\n", job.Name, job.Payload.Message)
		}),
	)
	if err != nil {
		log.Fatalf("创建 CronAgent 失败: %v", err)
	}

	// 启动调度服务
	if err := ca.Start(); err != nil {
		log.Fatalf("启动调度服务失败: %v", err)
	}
	defer ca.Stop()

	fmt.Println("=== Cron Agent 定时任务示例 ===\n")

	//测试对话
	conversations := []string{
		//"帮我添加一个定时任务，每60秒提醒我喝水",
		//"帮我添加一个一次性定时，30秒后提醒我开会",
		//"帮我看看现在有哪些定时任务",
		"帮我添加一个定时任务，每60秒增加一个一次性定时任务，10秒后开会",
	}

	for i, msg := range conversations {
		fmt.Printf("【问题 %d】: %s\n", i+1, msg)
		out, err := ca.Generate(ctx, []*schema.Message{
			schema.UserMessage(msg),
		})
		if err != nil {
			log.Printf("生成失败: %v", err)
			continue
		}
		fmt.Printf("【回答】: %s\n\n", out.Content)
	}

	fmt.Println("定时任务已创建，等待触发中（按 Ctrl+C 退出）...")

	// 等待信号退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n=== 示例结束 ===")
}
