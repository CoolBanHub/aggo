package callbacks

import (
	"context"
	"io"
	"log"
	"runtime/debug"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ChatModelCallback struct {
	storeAssistantMessageFunc storeAssistantMessage
}

type storeAssistantMessage func(ctx context.Context, message *schema.Message)

func NewChatModelCallback(storeAssistantMessageFunc storeAssistantMessage) *ChatModelCallback {
	return &ChatModelCallback{
		storeAssistantMessageFunc: storeAssistantMessageFunc,
	}
}

func (this *ChatModelCallback) OnStart(ctx context.Context, runInfo *callbacks.RunInfo, input *model.CallbackInput) context.Context {
	return ctx
}

func (this *ChatModelCallback) OnEnd(ctx context.Context, runInfo *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
	newCtx := context.WithoutCancel(ctx)
	go func() {
		if this.storeAssistantMessageFunc != nil {
			this.storeAssistantMessageFunc(newCtx, output.Message)
		}
	}()
	return ctx
}
func (this *ChatModelCallback) OnError(ctx context.Context, runInfo *callbacks.RunInfo, err error) context.Context {
	return ctx
}
func (this *ChatModelCallback) OnEndWithStreamOutput(ctx context.Context, runInfo *callbacks.RunInfo, output *schema.StreamReader[*model.CallbackOutput]) context.Context {
	newCtx := context.WithoutCancel(ctx)
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
			content += chunk.Message.Content
		}
		if this.storeAssistantMessageFunc != nil {
			this.storeAssistantMessageFunc(newCtx, &schema.Message{Content: content})
		}
	}()

	return ctx
}
