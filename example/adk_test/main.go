package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/tools"
	"github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
		log.Println("将尝试从系统环境变量读取配置")
	}

	// 初始化 Langfuse 回调处理器（用于跟踪执行情况）
	langfuseHost := os.Getenv("LANGFUSE_HOST")
	langfusePublicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	langfuseSecretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	if langfuseHost != "" && langfusePublicKey != "" && langfuseSecretKey != "" {
		cbh, _ := langfuse.NewLangfuseHandler(&langfuse.Config{
			Host:      langfuseHost,
			PublicKey: langfusePublicKey,
			SecretKey: langfuseSecretKey,
		})
		callbacks.AppendGlobalHandlers(cbh)
		log.Println("✓ Langfuse 回调处理器已启用")
	} else {
		log.Println("提示: 未配置 Langfuse，跳过初始化（可在 .env 文件中配置）")
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

	fmt.Println("=== ADK 多 Agent 路由架构示例 ===\n")

	// 1. 创建三个专业子 Agent，每个配置不同的工具
	mathAgent, err := createMathAgent(ctx, cm)
	if err != nil {
		log.Fatalf("创建数学助手失败: %v", err)
	}

	weatherAgent, err := createWeatherAgent(ctx, cm)
	if err != nil {
		log.Fatalf("创建天气助手失败: %v", err)
	}

	timeAgent, err := createTimeAgent(ctx, cm)
	if err != nil {
		log.Fatalf("创建时间助手失败: %v", err)
	}

	// 2. 创建主路由 Agent，它会根据问题自动选择合适的子 Agent
	mainAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "智能助手",
		Description: "我是一个智能助手，可以处理数学计算、天气查询和时间处理等各类问题",
		Instruction: "你是一个智能路由助手。根据用户的问题类型，选择合适的专业助手来回答：" +
			"1. 数学计算相关问题，请使用数学助手；" +
			"2. 天气查询相关问题，请使用天气助手；" +
			"3. 时间日期相关问题，请使用时间助手。" +
			"请仔细理解用户问题，选择最合适的助手进行处理。",
		Model:         cm,
		MaxIterations: 10,
	})
	if err != nil {
		log.Fatalf("创建主 Agent 失败: %v", err)
	}

	// 3. 将子 Agent 注册到主 Agent
	routerAgent, err := adk.SetSubAgents(ctx, mainAgent, []adk.Agent{
		adk.AgentWithOptions(ctx, mathAgent, adk.WithDisallowTransferToParent()),
		adk.AgentWithOptions(ctx, weatherAgent, adk.WithDisallowTransferToParent()),
		adk.AgentWithOptions(ctx, timeAgent, adk.WithDisallowTransferToParent()),
	})
	if err != nil {
		log.Fatalf("设置子 Agent 失败: %v", err)
	}

	bot, err := agent.NewAgentFromADK(routerAgent)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	// 4. 测试不同类型的问题，主 Agent 会自动路由到对应的子 Agent
	testQuestions := []string{
		"请帮我计算 123.5 + 456.8 等于多少？",
		"北京今天的天气怎么样？",
		"现在几点了？",
		"如果我有 1000 元，分给 8 个人，每人能分到多少？",
		"帮我查一下上海和深圳的天气情况",
		"请帮我计算从 2024-01-01 到 2024-12-31 一共有多少天？",
	}

	fmt.Println("开始测试多 Agent 路由功能...")

	for i, question := range testQuestions {
		fmt.Printf("\n【问题 %d】: %s\n", i+1, question)

		// 为每个问题创建独立的 trace，主动生成id避免多个agent的执行被分在多条trace
		ctx = langfuse.SetTrace(context.Background(), langfuse.WithID(uuid.NewString()))

		// 直接调用 Agent 运行
		out, err := bot.Generate(ctx, []*schema.Message{
			{Role: schema.User, Content: question},
		})
		if err != nil {
			log.Printf("生成失败: %v", err)
		}

		fmt.Printf("【回答】: %s\n", out.Content)
		break
	}

	fmt.Println("\n=== 测试完成 ===")
}

// createMathAgent 创建数学计算专家 Agent
func createMathAgent(ctx context.Context, cm einoModel.ToolCallingChatModel) (adk.Agent, error) {
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "数学助手",
		Description: "专业的数学计算助手，擅长进行加减乘除等各类数学运算",
		Instruction: "你是一个专业的数学助手，擅长使用计算器工具进行精确计算。" +
			"当用户提出数学计算问题时，请使用 calculator 工具进行准确计算，并给出清晰的答案。",
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools.GetCalculatorTool(),
			},
		},
		MaxIterations: 5,
	})
}

// createWeatherAgent 创建天气查询专家 Agent
func createWeatherAgent(ctx context.Context, cm einoModel.ToolCallingChatModel) (adk.Agent, error) {
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "天气助手",
		Description: "专业的天气查询助手，可以查询各个城市的天气情况",
		Instruction: "你是一个专业的天气播报员，擅长使用天气工具为用户提供准确的天气信息。" +
			"当用户询问天气时，请使用 weather_query 工具查询相关城市的天气，并以友好的方式播报给用户。",
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools.GetWeatherTool(),
			},
		},
		MaxIterations: 5,
	})
}

// createTimeAgent 创建时间处理专家 Agent
func createTimeAgent(ctx context.Context, cm einoModel.ToolCallingChatModel) (adk.Agent, error) {
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "时间助手",
		Description: "专业的时间处理助手，可以查询时间、格式化时间、计算时间差等",
		Instruction: "你是一个专业的时间管理助手，擅长使用时间工具处理时间查询、格式化和计算。" +
			"当用户询问时间相关问题时，请使用 time_tool 工具获取或处理时间信息，并给出准确的答案。",
		Model: cm,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools.GetTimeTool(),
			},
		},
		MaxIterations: 5,
	})
}
