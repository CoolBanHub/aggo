package main

import (
	"context"
	"encoding/base64"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
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
	a, err := agent.NewAgent(ctx, cm)
	if err != nil {
		log.Fatalf("创建agent失败：%v", err)
	}
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

	msg, err := a.Generate(ctx, msgList)
	if err != nil {
		log.Fatalf("生成消息失败：%v", err)
	}

	log.Printf("result: %s", msg.String())
}
