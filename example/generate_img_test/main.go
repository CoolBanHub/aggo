package main

import (
	"context"
	"os"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warnf("警告: 无法加载 .env 文件: %v", err)
		slog.Info("将尝试从系统环境变量读取配置")
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE-API-KEY"),
	})
	if err != nil {
		slog.Error("创建gemini模型失败", "err", err)
		return
	}
	cm, err := gemini.NewChatModel(ctx, &gemini.Config{
		Client: client,
		Model:  "gemini-2.5-flash-image",
		ResponseModalities: []gemini.GeminiResponseModality{
			gemini.GeminiResponseModalityImage,
		},
	})
	if err != nil {
		slog.Error("创建gemini模型失败", "err", err)
		return
	}
	m, err := cm.Generate(ctx, []*schema.Message{
		&schema.Message{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{
					Type: "text",
					Text: "生成一张超写实的美食级芝士汉堡信息图表，将其解构以展示烤布里欧修面包的质地、肉饼的焦香外壳以及奶酪晶莹的融化状态。",
				},
			},
		},
	})
	if err != nil {
		slog.Error("创建gemini模型失败", "err", err)
		return
	}
	for _, v := range m.AssistantGenMultiContent {
		slog.Debugf("图片,类型:%s,basedata:%s", v.Image.MIMEType, *v.Image.Base64Data)
	}
}
