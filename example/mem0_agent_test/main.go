package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/mem0"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()

	if err := loadEnv(); err != nil {
		log.Printf("提示: 未找到可用的 .env 文件，将使用系统环境变量")
	}

	chatModel, err := model.NewChatModel(
		model.WithBaseUrl(requiredEnv("BaseUrl")),
		model.WithAPIKey(requiredEnv("APIKey")),
		model.WithModel(envOrDefault("Model", "gpt-5-nano")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}

	provider, err := memory.GlobalRegistry().CreateProvider("mem0", &mem0.ProviderConfig{
		BaseURL:           requiredEnv("MEM0_BASE_URL"),
		APIKey:            os.Getenv("MEM0_API_KEY"),
		Mode:              mem0.Mode(envOrDefault("MEM0_MODE", string(mem0.ModeHosted))),
		AuthHeader:        os.Getenv("MEM0_AUTH_HEADER"),
		AuthScheme:        os.Getenv("MEM0_AUTH_SCHEME"),
		AddPath:           os.Getenv("MEM0_ADD_PATH"),
		SearchPath:        os.Getenv("MEM0_SEARCH_PATH"),
		SearchMsgLimit:    6,
		SearchResultLimit: 5,
		OutputMemoryLimit: 5,
		UseSessionAsRunID: envBool("MEM0_USE_SESSION_AS_RUN_ID"),
		SearchBySession:   envBool("MEM0_SEARCH_BY_SESSION"),
		AgentID:           os.Getenv("MEM0_AGENT_ID"),
		AppID:             os.Getenv("MEM0_APP_ID"),
		OrgID:             os.Getenv("MEM0_ORG_ID"),
		ProjectID:         os.Getenv("MEM0_PROJECT_ID"),
	})
	if err != nil {
		log.Fatalf("创建 mem0 provider 失败: %v", err)
	}
	defer provider.Close()

	ag, err := agent.NewAgentBuilder(chatModel).
		WithInstruction("你是一个有长期记忆能力的助手，回答简洁准确。").
		WithMemory(provider).
		Build(ctx)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})

	userID := envOrDefault("MEM0_DEMO_USER_ID", fmt.Sprintf("mem0-demo-user-%d", time.Now().UnixNano()))
	sessionID := envOrDefault("MEM0_DEMO_SESSION_ID", fmt.Sprintf("mem0-demo-session-%d", time.Now().UnixNano()))
	log.Printf("使用 userID=%s sessionID=%s", userID, sessionID)

	conversations := []string{
		"请记住：我叫 Alice，我喜欢摄影和 Go。",
		"你还记得我叫什么、喜欢什么吗？",
	}

	for i, input := range conversations {
		log.Printf("User: %s", input)
		iter := runner.Run(ctx, []*schema.Message{
			schema.UserMessage(input),
		}, adk.WithSessionValues(map[string]any{
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
				log.Fatalf("生成回复失败: %v", event.Err)
			}
			if event.Output != nil && event.Output.MessageOutput != nil {
				if msg, err := event.Output.MessageOutput.GetMessage(); err == nil && msg != nil {
					response = msg.Content
				}
			}
		}

		log.Printf("AI: %s", response)
		if i < len(conversations)-1 {
			if err := waitForMemory(ctx, provider, userID, sessionID, conversations[i+1], "Alice", "摄影", "Go"); err != nil {
				log.Printf("mem0 记忆还未完成索引，继续下一轮可能仍然取不到：%v", err)
			}
		}
	}
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s 环境变量必须设置", key)
	}
	return value
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func loadEnv() error {
	paths := []string{
		".env",
	}
	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			log.Printf("已加载环境变量文件: %s", path)
			return nil
		}
	}
	return os.ErrNotExist
}

func waitForMemory(ctx context.Context, provider memory.MemoryProvider, userID, sessionID, nextQuestion string, expected ...string) error {
	const (
		timeout  = 3 * time.Minute
		interval = 2 * time.Second
	)

	log.Printf("mem0 写入是异步的，等待记忆可检索后再发送下一轮问题")

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := provider.Retrieve(ctx, &memory.RetrieveRequest{
			UserID:    userID,
			SessionID: sessionID,
			Messages: []*schema.Message{
				schema.UserMessage(nextQuestion),
			},
		})
		if err == nil && containsExpectedMemory(result, expected...) {
			log.Printf("mem0 记忆已经可检索")
			return nil
		}
		time.Sleep(interval)
	}

	return context.DeadlineExceeded
}

func systemMessageContents(result *memory.RetrieveResult) []string {
	if result == nil || len(result.SystemMessages) == 0 {
		return nil
	}
	out := make([]string, 0, len(result.SystemMessages))
	for _, msg := range result.SystemMessages {
		if msg == nil {
			continue
		}
		out = append(out, msg.Content)
	}
	return out
}

func containsExpectedMemory(result *memory.RetrieveResult, expected ...string) bool {
	contents := strings.Join(systemMessageContents(result), "\n")
	if strings.TrimSpace(contents) == "" {
		return false
	}
	for _, item := range expected {
		if item == "" {
			continue
		}
		if !strings.Contains(contents, item) {
			return false
		}
	}
	return true
}
