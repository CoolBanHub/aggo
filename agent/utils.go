package agent

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func ConvToolsToCompose(tools []tool.BaseTool) []compose.Option {
	toolInfos, _ := genToolInfos(context.Background(), tools)

	return []compose.Option{
		compose.WithToolsNodeOption(compose.WithToolList(tools...)),
		compose.WithChatModelOption(model.WithTools(toolInfos)),
	}
}

func genToolInfos(ctx context.Context, tools []tool.BaseTool) ([]*schema.ToolInfo, error) {
	toolInfos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		tl, err := t.Info(ctx)
		if err != nil {
			return nil, err
		}

		toolInfos = append(toolInfos, tl)
	}

	return toolInfos, nil
}
