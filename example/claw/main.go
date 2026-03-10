package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/agent/cron_agent"
	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/tool/httprequest"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
	}

	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
		model.WithReasoningEffortLevel("low"),
	)
	if err != nil {
		log.Fatalf("Failed to create chat model: %v", err)
	}

	cronSubAgent, err := cron_agent.New(ctx, chatModel, cron_agent.WithFileStore("cron_jobs.json"),
		cron_agent.WithOnJobProcessed(func(job *cronPkg.CronJob, response string, err error) {
			if err != nil {
				fmt.Printf("\n❌ [任务处理失败] %s: %v\n", job.Name, err)
			} else {
				fmt.Printf("\n🔔 [任务处理完成] %s \n", response)
			}
		}))
	if err != nil {
		log.Fatalf("Failed to create cron agent: %v", err)
	}

	// 启动调度服务
	if err := cronSubAgent.Start(); err != nil {
		log.Fatalf("启动调度服务失败: %v", err)
	}
	defer cronSubAgent.Stop()

	agentTools := shell.GetTools()
	httpTools, err := httprequest.NewToolKit(ctx, nil)
	if err != nil {
		log.Fatalf("创建 http 工具失败: %v", err)
	}
	agentTools = append(agentTools, httpTools...)

	// 获取 skills 目录的绝对路径
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
		log.Fatalf("Failed to create local backend: %v", err)
	}

	skillMiddleware, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: backend,
	})
	if err != nil {
		log.Fatalf("Failed to create skill middleware: %v", err)
	}

	// 创建 SQLite 存储用于 Memory (静默日志)
	sqliteDb, err := gorm.Open(sqlite.Open("memory.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("Failed to open sqlite database: %v", err)
	}
	sqliteStorage, err := storage.NewGormStorage(sqliteDb)
	if err != nil {
		log.Fatalf("Failed to create sqlite storage: %v", err)
	}

	// 创建 MemoryManager
	memoryManager, err := memory.NewMemoryManager(chatModel, sqliteStorage, &memory.MemoryConfig{
		EnableUserMemories:   true,
		EnableSessionSummary: true,
		MemoryLimit:          20,
		AsyncProcessing:      true,
		AsyncWorkerPoolSize:  3,
	})
	if err != nil {
		log.Fatalf("Failed to create memory manager: %v", err)
	}
	defer memoryManager.Close()

	systemPrompt := `你是一个智能助手。

## 工作原则
1. 根据需求选择合适工具
2. 不需要经过我同意，直接做决定就行
3. 回复简洁准确`

	opts := []agent.Option{
		agent.WithName("assistant"),
		agent.WithDescription("小助手"),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(agentTools),
		agent.WithAdkAgentMiddlewares([]adk.ChatModelAgentMiddleware{skillMiddleware}),
		agent.WithSubAgent([]adk.Agent{cronSubAgent}),
		agent.WithMemoryManager(memoryManager),
	}
	a, err := agent.NewAgent(ctx, chatModel, opts...)

	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	conversations := []string{
		//`帮我定时10秒，10秒后提醒我看书`,
		"帮我创建一个google搜索的skill",
		"直接帮我将技能保存到技能目录内，格式需要按照skill的规范进行保存",
	}

	for i, msg := range conversations {
		fmt.Printf("【问题 %d】: %s\n", i+1, msg)
		out, err := a.Generate(ctx, []*schema.Message{
			schema.UserMessage(msg),
		})
		if err != nil {
			log.Printf("生成失败: %v", err)
			continue
		}
		fmt.Printf("【回答】: %s\n\n", out.Content)
	}

	// 等待信号退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
