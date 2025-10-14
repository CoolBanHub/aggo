package main

import (
	"context"
	"io"
	"log"
	"os"
	"runtime/debug"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/callbacks"
	model2 "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

func main() {

	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Printf("警告: 无法加载 .env 文件: %v", err)
		log.Println("将尝试从系统环境变量读取配置")
	}

	ctx := context.Background()
	cm, err := model.NewChatModel(model.WithBaseUrl(os.Getenv("BaseUrl")),
		model.WithAPIKey(os.Getenv("APIKey")),
		model.WithModel("gpt-5-nano"),
	)
	if err != nil {
		log.Fatalf("new chat model fail,err:%s", err)
		return
	}

	callbacks.AppendGlobalHandlers(NewChatModelCallback())

	bot, err := agent.NewAgent(ctx, cm,
		agent.WithName("linux大师"),
		agent.WithDescription("我是一个linux大师，请勿使用此工具进行非法操作"),
		agent.WithSystemPrompt("你是一个linux大师"))
	if err != nil {
		log.Fatalf("new agent fail,err:%s", err)
		return
	}
	conversations := []string{
		"你好，我是Alice",
		"我是一名软件工程师，专门做后端开发",
		"我住在北京，今年28岁",
		"你有什么爱好吗?",
		"我喜欢读书和摄影，特别是科幻小说",
		"我最近在学习Go语言和云原生技术",
		"我的工作主要涉及微服务架构设计",
		"周末我通常会去公园拍照或者在家看书",
		"你能给我推荐一些适合我的技术书籍吗？",
		"你还记得我之前说过我的职业是什么吗？",
		"基于你对我的了解，你觉得我适合学习什么新技术？",
		"我们年龄相差多少岁呢",
		"你喜欢吃什么水果吗？我喜欢吃苹果",
	}
	for _, conversation := range conversations {
		log.Printf("User: %s", conversation)
		out, err := bot.Stream(context.Background(), []*schema.Message{
			schema.UserMessage(conversation),
		})
		if err != nil {
			log.Fatalf("generate fail,err:%s", err)
			return
		}
		for {
			o, err2 := out.Recv()
			if err2 != nil {
				log.Println("err2:", err2)
				break
			}
			log.Printf("AI:%s", o.Content)
		}
	}
}

type ChatModelCallback struct {
}

func NewChatModelCallback() *ChatModelCallback {
	return &ChatModelCallback{}
}

func (this *ChatModelCallback) OnStart(ctx context.Context, runInfo *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	log.Printf("input: %+v, runinfo: %+v", input, runInfo)
	return ctx
}

func (this *ChatModelCallback) OnEnd(ctx context.Context, runInfo *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	log.Printf("output: %+v, runinfo: %+v", output, runInfo)
	return ctx
}
func (this *ChatModelCallback) OnError(ctx context.Context, runInfo *callbacks.RunInfo, err error) context.Context {
	log.Printf("error: %s, runinfo: %+v, stack: %s", err, runInfo, string(debug.Stack()))
	return ctx
}

func (this *ChatModelCallback) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo,
	input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	log.Printf("input: %+v, runinfo: %+v", input, info)
	return ctx
}
func (this *ChatModelCallback) OnEndWithStreamOutput(ctx context.Context, runInfo *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	go func() {
		defer func() {
			e := recover()
			if e != nil {
				log.Printf("recover update langfuse span panic: %v, runinfo: %+v, stack: %s", e, runInfo, string(debug.Stack()))
			}
			output.Close()
		}()
		var outs []callbacks.CallbackOutput
		content := ""
		for {
			chunk, err := output.Recv()
			if err == io.EOF {
				break
			}
			outs = append(outs, chunk)
			_chunk := chunk.(*model2.CallbackOutput)
			content += _chunk.Message.Content
		}
		log.Printf("content: %s", content)
	}()

	return ctx
}
