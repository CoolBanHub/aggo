package main

import (
	"context"
	"encoding/base64"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func main() {

	f, err := os.ReadFile("1.png")
	if err != nil {
		log.Fatalf("读取图片失败: %v", err)
	}
	base64Img := base64.StdEncoding.EncodeToString(f)
	_ = base64Img
	// 创建聊天模型
	cm, err := model.NewChatModel(
		model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel(os.Getenv("Model")),
	)
	if err != nil {
		log.Fatalf("创建聊天模型失败: %v", err)
	}
	ctx := context.Background()
	ag, err := agent.NewAgentBuilder(cm).Build(ctx)
	if err != nil {
		log.Fatalf("创建agent失败：%v", err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag})

	link := "https://cdn.deepseek.com/logo.png"
	msgList := []*schema.Message{
		{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{
				{
					Type: "text",
					Text: "这个图片里面有什么",
				},
				{
					Type: "image_url",
					Image: &schema.MessageInputImage{
						MessagePartCommon: schema.MessagePartCommon{
							URL: &link,
							//Base64Data: &base64Img,
							//MIMEType: "image/jpeg",
						},
					},
				},
			},
		},
	}

	iter := runner.Run(ctx, msgList)
	var response string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			log.Fatalf("生成消息失败：%v", event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			if msg, err := event.Output.MessageOutput.GetMessage(); err == nil && msg != nil {
				response = msg.Content
			}
		}
	}

	log.Printf("result: %s", response)
}
