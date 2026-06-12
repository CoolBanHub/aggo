package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/CoolBanHub/aggo/agent"
	cronPkg "github.com/CoolBanHub/aggo/cron"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/builtin"
	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/pkg/langfuse"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	memorytool "github.com/CoolBanHub/aggo/tools/memory"
	"github.com/CoolBanHub/aggo/tools/shell"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/tool/httprequest"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/callbacks"
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

	closeLangfuse := setupLangfuse()
	langfuseEnabled := closeLangfuse != nil
	if closeLangfuse != nil {
		defer closeLangfuse()
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

	skillMiddleware, err := skill.NewTyped[*schema.AgenticMessage](ctx, &skill.TypedConfig[*schema.AgenticMessage]{
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
			// 启用“事件检索”模式：常驻短文档 + 最近 N 条事件注入 system，
			// 更早的事件通过 search_user_memory 工具按关键词/时间检索。
			EnableEventSearch:   true,
			RecentEventLimit:    10,
			MemoryLimit:         20,
			AsyncWorkerPoolSize: 3,
			Search: &builtin.SearchConfig{
				Mode: builtinsearch.ModeKeyword,
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create memory provider: %v", err)
	}
	defer provider.Close()

	// 注册 search_user_memory 工具：用于回溯更早或更宽范围的长期事件
	if searcher, ok := any(provider).(memory.UserMemoryEventSearcher); ok {
		memSearchTool, err := memorytool.SearchUserMemoryTool(searcher)
		if err != nil {
			log.Fatalf("创建 search_user_memory 工具失败: %v", err)
		}
		agentTools = append(agentTools, memSearchTool)
	}
	agentTools = append(agentTools, adk.NewTypedAgentTool[*schema.AgenticMessage](ctx, cronAgentResult.Agent))

	systemPrompt := `你是一个智能助手。

## 工作原则
1. 根据需求选择合适工具
2. 不需要经过我同意，直接做决定就行
3. 回复简洁准确

## 用户长期记忆规则
1. system 中已注入两块内容：
   - <user_memory> 常驻短文档（核心约定 + 基础信息）
   - <user_memory_recent_events> 最近若干条任务里程碑/事件记录（每条已含日期/类型/摘要）
2. **优先使用上述常驻内容**回答；只有命中不到时才调用 search_user_memory
3. search_user_memory 用于**结构化长期事件**（按关键词/类型/时间窗回溯旧的里程碑或事件），查的是已被提炼的事实
4. 关键词优先传具体名词（账号ID、客户名、产品、动作）；必要时用 type=milestone/event、since、until 缩小范围
5. 如果常驻短文档+最近事件已能回答，**不要重复调工具**；找不到时再追问用户或继续检索`

	ag, err := agent.NewAgentBuilder(chatModel).
		WithName("assistant").
		WithDescription("小助手").
		WithInstruction(systemPrompt).
		WithTools(agentTools...).
		WithMiddlewares(skillMiddleware).
		WithMemory(provider).
		Build(ctx)

	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: ag})

	conversations := []string{
		"帮我创建一个google搜索的skill",
		"直接帮我将技能保存到技能目录内，格式需要按照skill的规范进行保存",
	}

	userID := utils.GetULID()
	sessionID := utils.GetULID()
	for i, msg := range conversations {
		fmt.Printf("【问题 %d】: %s\n", i+1, msg)

		runCtx := ctx
		if langfuseEnabled {
			runCtx = langfuse.SetTrace(ctx,
				langfuse.WithID(utils.GetULID()),
				langfuse.WithName("claw_conversation_turn"),
				langfuse.WithUserID(userID),
				langfuse.WithSessionID(sessionID),
				langfuse.WithTags("example", "claw"),
				langfuse.WithMetadata(map[string]string{
					"turn": fmt.Sprintf("%d", i+1),
				}),
			)
		}

		iter := runner.Run(runCtx, []*schema.AgenticMessage{
			schema.UserAgenticMessage(msg),
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
					fmt.Printf("【回答】: %s\n\n", agmsg.Text(m))
				}
			}
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func setupLangfuse() func() {
	host := firstEnv("LANGFUSE_HOST", "LANGFUSE_BASE_URL")
	publicKey := strings.TrimSpace(os.Getenv("LANGFUSE_PUBLIC_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("LANGFUSE_SECRET_KEY"))
	if host == "" || publicKey == "" || secretKey == "" {
		log.Println("提示: 未配置 Langfuse，跳过初始化")
		return nil
	}

	handler, closeFn, err := langfuse.NewHandler(langfuse.Config{
		ClientConfig: langfuse.ClientConfig{
			Host:            host,
			PublicKey:       publicKey,
			SecretKey:       secretKey,
			Release:         strings.TrimSpace(os.Getenv("LANGFUSE_RELEASE")),
			Environment:     strings.TrimSpace(os.Getenv("LANGFUSE_ENVIRONMENT")),
			RequestTimeout:  10 * time.Second,
			LogIngestErrors: true,
			IngestionMetadata: map[string]any{
				"sdk":         "aggo-example-claw",
				"integration": "eino",
			},
		},
		Name: firstNonEmpty(os.Getenv("LANGFUSE_TRACE_NAME"), "claw_agent"),
	})
	if err != nil {
		log.Printf("初始化 Langfuse 失败: %v", err)
		return nil
	}

	callbacks.AppendGlobalHandlers(handler)
	log.Println("Langfuse 已启用")
	return closeFn
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
