package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/CoolBanHub/aggo/model"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	cm, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	// 创建 CronService 和工具
	service := cronPkg.NewCronService(cronPkg.NewFileStore("cron_jobs.json"), nil)
	tools := cronTool.GetTools(service)

	// 创建 CronAgent
	result, err := cronPkg.NewCronAgent(ctx, cm, tools,
		cronPkg.WithOnJobProcessed(func(job *cronPkg.CronJob, response string, err error) {
			if err != nil {
				fmt.Printf("\n❌ [任务处理失败] %s: %v\n", job.Name, err)
			} else {
				fmt.Printf("\n🔔 [任务处理完成] %s\n   Agent 回复: %s\n", job.Name, response)
			}
		}),
	)
	if err != nil {
		log.Fatalf("创建 CronAgent 失败: %v", err)
	}

	// 启动调度服务
	if err := result.Service.Start(); err != nil {
		log.Fatalf("启动调度服务失败: %v", err)
	}
	defer result.Service.Stop()

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: result.Agent})

	fmt.Println("=== Cron Agent 定时任务示例 ===")

	conversations := []string{
		"帮我添加一个定时任务，每60秒提醒我喝水",
		"帮我添加一个一次性定时，30秒后提醒我开会",
		"帮我看看现在有哪些定时任务",
		"帮我添加一个定时任务，每30秒增加一个一次性定时任务，10秒后开会",
	}

	for i, msg := range conversations {
		fmt.Printf("【问题 %d】: %s\n", i+1, msg)
		iter := runner.Run(ctx, []*schema.Message{
			schema.UserMessage(msg),
		})
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				log.Printf("生成失败: %v", event.Err)
				break
			}
			if event.Output != nil && event.Output.MessageOutput != nil {
				if m, err := event.Output.MessageOutput.GetMessage(); err == nil && m != nil {
					fmt.Printf("【回答】: %s\n\n", m.Content)
				}
			}
		}
	}

	fmt.Println("定时任务已创建，等待触发中（按 Ctrl+C 退出）...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n=== 示例结束 ===")
}
