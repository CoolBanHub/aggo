package main

import (
	"context"
	"encoding/base64"
	"log"
	"os"

	"github.com/CoolBanHub/aggo/agent"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
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
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: ag})

	link := "https://cdn.deepseek.com/logo.png"
	msgList := []*schema.AgenticMessage{
		agmsg.UserMessageFromInputParts([]schema.MessageInputPart{
			{
				Type: schema.ChatMessagePartTypeText,
				Text: "这个图片里面有什么",
			},
			{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: &link,
						//Base64Data: &base64Img,
						//MIMEType: "image/jpeg",
					},
				},
			},
		}),
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
				response = agmsg.Text(msg)
			}
		}
	}

	log.Printf("result: %s", response)
}
