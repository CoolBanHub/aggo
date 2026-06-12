package langfuse

import (
	"encoding/json"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type chatMessage struct {
	Role          schema.AgenticRoleType      `json:"role"`
	Content       string                      `json:"content,omitempty"`
	ContentBlocks []*schema.ContentBlock      `json:"content_blocks,omitempty"`
	ToolCalls     []schema.FunctionToolCall   `json:"tool_calls,omitempty"`
	ResponseMeta  *schema.AgenticResponseMeta `json:"response_meta,omitempty"`
	Extra         map[string]any              `json:"extra,omitempty"`
}

func chatModelInput(input *model.AgenticCallbackInput) any {
	if input == nil {
		return nil
	}

	out := map[string]any{
		"messages": convertMessages(input.Messages),
	}
	if input.Config != nil {
		if input.Config.Model != "" {
			out["model"] = input.Config.Model
		}
		if input.Config.MaxTokens > 0 {
			out["max_tokens"] = input.Config.MaxTokens
		}
		if input.Config.Temperature != 0 {
			out["temperature"] = input.Config.Temperature
		}
		if input.Config.TopP != 0 {
			out["top_p"] = input.Config.TopP
		}
	}
	if tools := convertTools(input.Tools); len(tools) > 0 {
		out["tools"] = tools
	}
	return out
}

func convertMessages(messages []*schema.AgenticMessage) []any {
	converted := make([]any, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		converted = append(converted, convertMessage(message))
	}
	return converted
}

func convertMessage(message *schema.AgenticMessage) any {
	if message == nil {
		return nil
	}
	return chatMessage{
		Role:          message.Role,
		Content:       agmsg.Text(message),
		ContentBlocks: message.ContentBlocks,
		ToolCalls:     functionToolCalls(message),
		ResponseMeta:  message.ResponseMeta,
		Extra:         message.Extra,
	}
}

func convertTools(tools []*schema.ToolInfo) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		item := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Desc,
				"parameters":  toolParameters(tool),
			},
		}
		if len(tool.Extra) > 0 {
			item["extra"] = tool.Extra
		}
		out = append(out, item)
	}
	return out
}

func toolParameters(tool *schema.ToolInfo) any {
	if tool == nil || tool.ParamsOneOf == nil {
		return nil
	}
	js, err := tool.ParamsOneOf.ToJSONSchema()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return js
}

func toolDefinitionsMetadata(tools []*schema.ToolInfo) any {
	converted := convertTools(tools)
	if len(converted) == 0 {
		return nil
	}
	return converted
}

func toolCallsMetadata(message *schema.AgenticMessage) []map[string]any {
	calls := functionToolCalls(message)
	if len(calls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		out = append(out, map[string]any{
			"id":        call.CallID,
			"name":      call.Name,
			"arguments": call.Arguments,
			"type":      "function",
		})
	}
	return out
}

func functionToolCalls(message *schema.AgenticMessage) []schema.FunctionToolCall {
	if message == nil {
		return nil
	}
	calls := make([]schema.FunctionToolCall, 0)
	for _, block := range message.ContentBlocks {
		if block == nil || block.FunctionToolCall == nil {
			continue
		}
		calls = append(calls, *block.FunctionToolCall)
	}
	return calls
}

func jsonRaw(value any) any {
	if value == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return value
	}
	return out
}
