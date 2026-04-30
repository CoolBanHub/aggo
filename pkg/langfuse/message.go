package langfuse

import (
	"encoding/json"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type chatMessage struct {
	Role                     schema.RoleType            `json:"role"`
	Content                  string                     `json:"content,omitempty"`
	MultiContent             []schema.ChatMessagePart   `json:"multi_content,omitempty"`
	UserInputMultiContent    []schema.MessageInputPart  `json:"user_input_multi_content,omitempty"`
	AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_output_multi_content,omitempty"`
	Name                     string                     `json:"name,omitempty"`
	ToolCalls                []schema.ToolCall          `json:"tool_calls,omitempty"`
	ToolCallID               string                     `json:"tool_call_id,omitempty"`
	ToolName                 string                     `json:"tool_name,omitempty"`
	ResponseMeta             *schema.ResponseMeta       `json:"response_meta,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	Extra                    map[string]any             `json:"extra,omitempty"`
}

func chatModelInput(input *model.CallbackInput) any {
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
		if len(input.Config.Stop) > 0 {
			out["stop"] = input.Config.Stop
		}
	}
	if tools := convertTools(input.Tools); len(tools) > 0 {
		out["tools"] = tools
	}
	if toolChoice := toolChoiceValue(input.ToolChoice); toolChoice != nil {
		out["tool_choice"] = toolChoice
	}
	return out
}

func convertMessages(messages []*schema.Message) []any {
	converted := make([]any, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		converted = append(converted, convertMessage(message))
	}
	return converted
}

func convertMessage(message *schema.Message) any {
	if message == nil {
		return nil
	}
	return chatMessage{
		Role:                     message.Role,
		Content:                  message.Content,
		MultiContent:             message.MultiContent,
		UserInputMultiContent:    message.UserInputMultiContent,
		AssistantGenMultiContent: message.AssistantGenMultiContent,
		Name:                     message.Name,
		ToolCalls:                message.ToolCalls,
		ToolCallID:               message.ToolCallID,
		ToolName:                 message.ToolName,
		ResponseMeta:             message.ResponseMeta,
		ReasoningContent:         message.ReasoningContent,
		Extra:                    message.Extra,
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

func toolCallsMetadata(message *schema.Message) []map[string]any {
	if message == nil || len(message.ToolCalls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		out = append(out, map[string]any{
			"id":        call.ID,
			"name":      call.Function.Name,
			"arguments": call.Function.Arguments,
			"type":      call.Type,
			"index":     call.Index,
		})
	}
	return out
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
