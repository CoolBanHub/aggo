package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/agent"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/memory/builtin/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

func main() {
	ctx := context.Background()
	loadEnv()

	// 这个示例会同时用同一个 ChatModel 做两件事：
	// 1. 正常回复用户；
	// 2. builtin memory 在后台分析对话并生成用户长期记忆/会话摘要。
	// 因此 .env 里需要提供可用的 BaseUrl、APIKey 和 Model。
	baseUrl := os.Getenv("BaseUrl")
	apiKey := os.Getenv("APIKey")
	_model := os.Getenv("Model")
	if baseUrl == "" || apiKey == "" || _model == "" {
		log.Fatal("BaseUrl and APIKey environment variables must be set")
		return
	}

	cm, err := model.NewChatModel(model.WithBaseUrl(baseUrl),
		model.WithAPIKey(apiKey),
		model.WithModel(_model),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}
	// 如需跨进程保留记忆，可以改用 GormStorage。
	// 当前示例使用 MemoryStore，所有记忆只保存在当前进程内，程序退出后会丢失。
	//gormSql, err := NewMysqlGrom("root:123456@tcp(127.0.0.1:3306)/aggo", logger.Silent)
	//if err != nil {
	//	log.Fatalf("创建数据库连接失败: %v", err)
	//	return
	//}
	//s, err := storage.NewGormStorage(gormSql)
	//if err != nil {
	//	log.Fatalf("new sql store fail,err:%s", err)
	//	return
	//}
	userID := "alice"
	sessionID := utils.GetULID()
	// 默认 debounce 是 30 秒，短示例程序可能在定时器触发前就退出。
	// 这里设为 0，表示每轮 assistant 回复后立即提交用户记忆分析任务，便于观察触发效果。
	debounceSeconds := 0
	provider, err := memory.GlobalRegistry().CreateProvider("builtin", &builtin.ProviderConfig{
		ChatModel: cm,
		Storage:   storage.NewMemoryStore(),
		MemoryConfig: &builtin.MemoryConfig{
			EnableSessionSummary: true,
			EnableUserMemories:   true,
			// 用户记忆分析会读取最近 MemoryLimit/2 条消息。
			// 示例里连续跑多轮对话，调大一点避免早期的姓名/职业/地址被截掉。
			MemoryLimit:           30,
			Retrieval:             builtin.RetrievalLastN,
			DebounceWindowSeconds: &debounceSeconds,
		},
	})
	if err != nil {
		log.Fatalf("new provider fail,err:%s", err)
		return
	}
	// Close 会停止未触发的 debounce timer 并关闭后台 worker。
	// 因此本示例在 Close 前会先 waitAndPrintUserMemory，等待异步记忆写入完成。
	defer provider.Close()
	ag, err := agent.NewAgentBuilder(cm).
		WithInstruction("你是一名28岁的厦门女孩,名叫mary,是一名短视频剪辑师，主要剪辑电影类。喜欢养狗，家里有一只金毛，它叫小黄。喜欢宅在家里刷剧。不喜欢吃水果。性格内向高冷，别人不问的时候，一般都不回答自己的信息").
		// WithMemory 会把 memory middleware 挂到 agent 上：
		// - 模型调用前注入已有用户记忆/会话摘要/历史消息；
		// - 模型回复后异步保存本轮 user + assistant 消息并触发记忆分析。
		WithMemory(provider).
		Build(ctx)
	if err != nil {
		log.Fatalf("new agent fail,err:%s", err)
		return
	}

	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: ag})

	// 这些消息会按同一个 userID/sessionID 顺序跑多轮，用来模拟同一用户同一会话。
	// builtin memory 不是简单保存原文，而是由 analyzer 判断哪些事实值得写入长期记忆。
	conversations := []*schema.AgenticMessage{
		schema.UserAgenticMessage("你好，我是Alice"),
		schema.UserAgenticMessage("我是一名软件工程师，专门做后端开发"),
		schema.UserAgenticMessage("我住在北京，今年28岁"),
		schema.UserAgenticMessage("你有什么爱好吗?"),
		schema.UserAgenticMessage("我喜欢读书和摄影，特别是科幻小说"),
		agmsg.UserMessageFromInputParts([]schema.MessageInputPart{
			{
				Type: schema.ChatMessagePartTypeText,
				Text: "这图片里面有什么？",
			},
			{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: utils.ValueToPtr("https://cdn.deepseek.com/logo.png"),
					},
				},
			},
		}),
		//schema.UserAgenticMessage("我最近在学习Go语言和云原生技术"),
		//schema.UserAgenticMessage("我的工作主要涉及微服务架构设计"),
		//schema.UserAgenticMessage("周末我通常会去公园拍照或者在家看书"),
		//schema.UserAgenticMessage("你能给我推荐一些适合我的技术书籍吗？"),
		//schema.UserAgenticMessage("你还记得我之前说过我的职业是什么吗？"),
		//schema.UserAgenticMessage("基于你对我的了解，你觉得我适合学习什么新技术？"),
		//schema.UserAgenticMessage("我们年龄相差多少岁呢"),
		//schema.UserAgenticMessage("你喜欢吃什么水果吗？我喜欢吃苹果"),
		//schema.UserAgenticMessage("你知道我的住哪里吗"),
	}

	for _, conversation := range conversations {
		j, _ := sonic.MarshalString(conversation)
		log.Printf("User: %s", j)
		iter := runner.Run(ctx, []*schema.AgenticMessage{
			conversation,
		}, adk.WithSessionValues(map[string]any{
			// MemoryMiddleware 依赖这两个 session value。
			// 缺少任意一个时，只会正常跑 agent，不会检索或写入记忆。
			"userID":    userID,
			"sessionID": sessionID,
		}))
		var response string
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				log.Fatalf("generate fail,err:%s", event.Err)
				return
			}
			if event.Output != nil && event.Output.MessageOutput != nil {
				if msg, err := event.Output.MessageOutput.GetMessage(); err == nil && msg != nil {
					response = agmsg.Text(msg)
				}
			}
		}
		log.Printf("AI:%s", response)
	}

	// 用户记忆分析在 assistant 回复后异步执行。
	// 这里轮询打印最终用户记忆，避免程序刚结束就退出导致看不到结果。
	waitAndPrintUserMemory(ctx, provider, userID, 60*time.Second)
}

func loadEnv() {
	// 兼容从仓库根目录、example 子模块目录、或 mem_agent_test 目录运行。
	for _, filename := range []string{".env", "mem_agent_test/.env", "example/mem_agent_test/.env"} {
		if err := godotenv.Load(filename); err == nil {
			log.Printf("已加载环境配置: %s", filename)
			return
		}
	}
	log.Println("警告: 无法加载 .env 文件，将尝试从系统环境变量读取配置")
}

type userMemoryGetter interface {
	GetUserMemory(ctx context.Context, userID string) (*builtin.UserMemory, error)
}

// waitAndPrintUserMemory 仅用于示例调试。
// 生产代码通常不需要轮询；只要复用同一个 provider/storage，后续请求会自动检索并注入记忆。
func waitAndPrintUserMemory(ctx context.Context, provider memory.MemoryProvider, userID string, timeout time.Duration) {
	getter, ok := provider.(userMemoryGetter)
	if !ok {
		log.Printf("provider 不支持读取 builtin 用户记忆")
		return
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mem, err := getter.GetUserMemory(ctx, userID)
		if err == nil && mem != nil && strings.TrimSpace(mem.Memory) != "" {
			log.Printf("UserMemory:\n%s", mem.Memory)
			return
		}
		time.Sleep(time.Second)
	}
	log.Printf("等待用户记忆写入超时，可能 analyzer 返回 noop 或模型调用失败")
}

func NewMysqlGrom(source string, logLevel logger.LogLevel) (*gorm.DB, error) {
	if !strings.Contains(source, "parseTime") {
		source += "?charset=utf8mb4&parseTime=True&loc=Local"
	}
	gdb, err := gorm.Open(mysql.Open(source), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		NamingStrategy: schemaGrom.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		panic("数据库连接失败: " + err.Error())
	}

	// 配置GORM日志
	var gormLogger logger.Interface
	if logLevel > 0 {
		gormLogger = logger.Default.LogMode(logLevel)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	gdb.Logger = gormLogger

	return gdb, nil
}
