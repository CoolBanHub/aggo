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
	cronPkg "github.com/CoolBanHub/aggo/cron"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"github.com/CoolBanHub/aggo/model"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/CoolBanHub/aggo/utils"
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

	// 创建 CronService 和工具
	cronService := cronPkg.NewCronService(cronPkg.NewFileStore("cron_jobs.json"), nil)
	cronTools := cronTool.GetTools(cronService, cronTool.WithOnJobTriggered(func(job *cronPkg.CronJob) {
		fmt.Printf("\n🔔 [任务触发] %s: %s\n", job.Name, job.Payload.Message)
	}))

	// 启动调度服务
	if err := cronService.Start(); err != nil {
		log.Fatalf("启动调度服务失败: %v", err)
	}
	defer cronService.Stop()

	// 创建 Cron 子 Agent
	cronAgentResult, err := cronPkg.NewCronAgent(ctx, chatModel, cronTools,
		cronPkg.WithOnJobProcessed(func(job *cronPkg.CronJob, response string, err error) {
			if err != nil {
				fmt.Printf("\n❌ [任务处理失败] %s: %v\n", job.Name, err)
			} else {
				fmt.Printf("\n🔔 [任务处理完成] %s\n", response)
			}
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create cron agent: %v", err)
	}

	agentTools := shell.GetTools()
	httpTools, err := httprequest.NewToolKit(ctx, nil)
	if err != nil {
		log.Fatalf("创建 http 工具失败: %v", err)
	}
	agentTools = append(agentTools, httpTools...)

	// 获取 skills 目录
	cwd, _ := os.Getwd()
	skillsDir := filepath.Join(cwd, "skills")

	localBackend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		log.Fatalf("Failed to create local backend: %v", err)
	}

	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: localBackend,
		BaseDir: skillsDir,
	})
	if err != nil {
		log.Fatalf("Failed to create skill backend: %v", err)
	}

	skillMiddleware, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: skillBackend,
	})
	if err != nil {
		log.Fatalf("Failed to create skill middleware: %v", err)
	}

	// SQLite 存储
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

	provider, err := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
		ChatModel: chatModel,
		Storage:   sqliteStorage,
		MemoryConfig: &builtin.MemoryConfig{
			EnableUserMemories:   true,
			EnableSessionSummary: true,
			MemoryLimit:          20,
			AsyncWorkerPoolSize:  3,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create memory provider: %v", err)
	}
	defer provider.Close()

	systemPrompt := `你是一个智能助手。

## 工作原则
1. 根据需求选择合适工具
2. 不需要经过我同意，直接做决定就行
3. 回复简洁准确`

	ag, err := agent.NewAgentBuilder(chatModel).
		WithName("assistant").
		WithDescription("小助手").
		WithInstruction(systemPrompt).
		WithTools(agentTools...).
		WithMiddlewares(skillMiddleware).
		WithSubAgents(agent.SubAgentModeDefault, cronAgentResult.Agent).
		WithMemory(provider).
		Build(ctx)

	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})

	conversations := []string{
		"帮我创建一个google搜索的skill",
		"直接帮我将技能保存到技能目录内，格式需要按照skill的规范进行保存",
	}

	userID := utils.GetULID()
	sessionID := utils.GetULID()
	for i, msg := range conversations {
		fmt.Printf("【问题 %d】: %s\n", i+1, msg)
		iter := runner.Run(ctx, []*schema.Message{
			schema.UserMessage(msg),
		}, adk.WithSessionValues(map[string]any{
			"userID":    userID,
			"sessionID": sessionID,
		}))
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
