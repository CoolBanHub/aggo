// Demo: routing aggo agent calls through an AILens360 proxy project.
//
// What this exercises end-to-end
//
//  1. project_key in X-AILens-Project-Key header (one fixed value per app,
//     installed on an http.Client.Transport — not per request).
//  2. X-AILens-User / X-AILens-Session / X-AILens-Tag / X-AILens-Trace-*
//     from the caller's context. The RoundTripper reads req.Context() on
//     every call, so one ChatModel can serve different users/sessions
//     without being rebuilt — just stamp ctx before agent.Run.
//  3. metadata in the model request body via agenticopenai.WithExtraFields.
//     Because it is passed to runner.Run, every model call in this agent run
//     carries the same metadata.
//  4. Tool calling. The ReAct agent loops:
//     model.Stream() → if tool_calls present → run tool → feed result back
//     → model.Stream() … until the model produces a final answer.
//     Each model call inside one run shares the same trace_id.
//
// Required env vars (load via .env or export them before `go run`):
//
//	BaseUrl                  upstream OpenAI-compatible base URL
//	APIKey                   upstream API key (透传给上游, AILens360 不持有)
//	AILENS360_PROXY_PREFIX   e.g. https://ailens360.example.com/p
//	AILENS360_PROJECT_KEY    64-char project key from AILens360 console
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/CoolBanHub/aggo/agent"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/CoolBanHub/aggo/pkg/ailens360"
	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

type weatherIn struct {
	City string `json:"city" jsonschema:"description=The city name, e.g. Shanghai" jsonschema_required:"true"`
	Unit string `json:"unit,omitempty" jsonschema:"description=celsius | fahrenheit (default celsius)"`
}

type weatherOut struct {
	City        string `json:"city"`
	Unit        string `json:"unit"`
	Temperature int    `json:"temperature"`
	Condition   string `json:"condition"`
	AsOf        string `json:"as_of"`
}

func getWeather(_ context.Context, in *weatherIn) (*weatherOut, error) {
	unit := in.Unit
	if unit == "" {
		unit = "celsius"
	}
	return &weatherOut{
		City:        in.City,
		Unit:        unit,
		Temperature: 21,
		Condition:   "晴间多云",
		AsOf:        time.Now().Format(time.RFC3339),
	}, nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("提示: 未找到 .env 文件，将使用系统环境变量")
	}

	ctx := context.Background()

	baseURL := os.Getenv("BaseUrl")
	apiKey := os.Getenv("APIKey")
	modelName := os.Getenv("Model")
	proxyPrefix := os.Getenv("AILENS360_PROXY_PREFIX")
	projectKey := os.Getenv("AILENS360_PROJECT_KEY")
	if baseURL == "" || apiKey == "" || modelName == "" {
		log.Fatal("BaseUrl / APIKey / Model 必须配置")
	}

	// 1) Build the decorator. NewDecorator returns nil if either input is
	//    empty, which makes Apply a no-op — the agent then talks directly
	//    to the upstream, just without AILens360 observability.
	decorator := ailens360.NewDecorator(proxyPrefix, projectKey)
	if decorator == nil {
		log.Printf("warn: AILENS360_PROXY_PREFIX / AILENS360_PROJECT_KEY 未配置，跳过 AILens360 代理")
	} else {
		log.Printf("ailens360 enabled: proxy=%s", proxyPrefix)
	}

	// 2) Build an openai chat model and apply the decorator. After Apply:
	//    - cfg.BaseURL becomes "<proxy>/<upstream>"
	//    - cfg.HTTPClient gains a RoundTripper that stamps headers
	chatCfg := &agenticopenai.ChatConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	}
	if decorator != nil {
		decorator.ApplyAgentic(chatCfg)
	}

	chatModel, err := agenticopenai.NewChatModel(ctx, chatCfg)
	if err != nil {
		log.Fatalf("new chat model: %v", err)
	}

	// 3) Build a typed tool. aggo  agent builder accepts the same eino
	//    tool interface, so InferTool works directly.
	weatherTool, err := utils.InferTool(
		"get_weather",
		"Get the current weather for a given city.",
		getWeather,
	)
	if err != nil {
		log.Fatalf("infer tool: %v", err)
	}

	ag, err := agent.NewAgentBuilder(chatModel).
		WithName("weather_bot").
		WithInstruction("你是个简洁的助理。需要天气时必须先调用 get_weather 工具，再用返回的数据回答用户。").
		WithTools(weatherTool).
		Build(ctx)
	if err != nil {
		log.Fatalf("build agent: %v", err)
	}

	// 4) Per-call telemetry: stamp user/session/tag into ctx, then open a
	//    fresh trace for this agent run. Every model call inside the run
	//    shares the trace_id, grouped as one Langfuse-style trace.
	sessionID := fmt.Sprintf("sess_%d", time.Now().Unix())
	traceID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
	userID := "user_alice_42"
	ctx = ailens360.WithTrace(ctx, ailens360.TraceConfig{
		ID:        traceID,
		Name:      "weather_demo_turn",
		UserID:    userID,
		SessionID: sessionID,
		Tag:       "demo,react-agent",
	})

	log.Printf("user_id    = %s", userID)
	log.Printf("session_id = %s", sessionID)
	log.Printf("trace_id   = %s", traceID)

	// 5) Run the agent and stream the final answer.
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: ag, EnableStreaming: true})
	userMsg := schema.UserAgenticMessage("上海现在天气怎么样？请用中文回答。")
	iter := runner.Run(ctx, []*schema.AgenticMessage{
		userMsg,
	}, adk.WithChatModelOptions([]model.Option{
		agenticopenai.WithExtraFields(map[string]any{
			"user_id": userID,
		}),
	}))

	fmt.Println("\n--- streaming answer ---")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			if errors.Is(event.Err, io.EOF) {
				break
			}
			log.Fatalf("agent: %v", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil || msg == nil {
			continue
		}
		if text := agmsg.Text(msg); text != "" {
			fmt.Print(text)
		}
	}
	fmt.Println()

	if decorator != nil {
		fmt.Println("\n==> 打开 AILens360 控制台 → Traces：")
		fmt.Printf("    · 应看到一条名为 weather_demo_turn 的 trace（trace_id=%s）\n", traceID)
		fmt.Printf("    · session=%s 可在过滤栏复用\n", sessionID)
		fmt.Println("    · 第一次模型调用 response 含 tool_calls=get_weather，第二次给出中文最终答案")
	}
}
