package main

import (
	"context"
	"os"

	"github.com/cloudwego/eino-ext/components/model/agenticgemini"
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
	cm, err := agenticgemini.New(ctx, &agenticgemini.Config{
		Client: client,
		Model:  "gemini-2.5-flash-image",
		ResponseModalities: []genai.Modality{
			genai.ModalityImage,
		},
	})
	if err != nil {
		slog.Error("创建gemini模型失败", "err", err)
		return
	}
	m, err := cm.Generate(ctx, []*schema.AgenticMessage{
		schema.UserAgenticMessage("生成一张超写实的美食级芝士汉堡信息图表，将其解构以展示烤布里欧修面包的质地、肉饼的焦香外壳以及奶酪晶莹的融化状态。"),
	})
	if err != nil {
		slog.Error("创建gemini模型失败", "err", err)
		return
	}
	for _, block := range m.ContentBlocks {
		if block == nil || block.AssistantGenImage == nil {
			continue
		}
		slog.Debugf("图片,类型:%s,basedata:%s", block.AssistantGenImage.MIMEType, block.AssistantGenImage.Base64Data)
	}
}
