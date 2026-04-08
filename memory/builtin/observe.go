package builtin

import (
	"context"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
)

func DefaultAsyncTaskTraceName(taskType string) string {
	switch taskType {
	case "summary":
		return "builtin-session-summary"
	case "memory":
		fallthrough
	default:
		return "builtin-memory"
	}
}

func withObservationName(ctx context.Context, cm model.ToolCallingChatModel, name string) context.Context {
	runInfo := &callbacks.RunInfo{
		Name:      name,
		Component: components.ComponentOfChatModel,
	}
	if typ, ok := components.GetType(cm); ok {
		runInfo.Type = typ
	}
	return callbacks.ReuseHandlers(ctx, runInfo)
}
