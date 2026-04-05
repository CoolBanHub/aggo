package adapter

import (
	"github.com/cloudwego/eino/schema"
	openai "github.com/meguminnnnnnnnn/go-openai"
)

// MessageToOpenaiResponse 将 Eino schema.Message 转换为 OpenAI 格式的 ChatCompletionResponse
func MessageToOpenaiResponse(msg *schema.Message) *openai.ChatCompletionResponse {
	if msg == nil {
		return nil
	}

	message := openai.ChatCompletionMessage{
		Role:             string(msg.Role),
		Content:          msg.Content,
		Name:             msg.Name,
		ReasoningContent: msg.ReasoningContent,
	}

	if len(msg.ToolCalls) > 0 {
		message.ToolCalls = make([]openai.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, openai.ToolCall{
				Index: tc.Index,
				ID:    tc.ID,
				Type:  openai.ToolType(tc.Type),
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	if msg.ToolCallID != "" {
		message.ToolCallID = msg.ToolCallID
	}

	choice := openai.ChatCompletionChoice{
		Index:   0,
		Message: message,
	}

	if msg.ResponseMeta != nil {
		choice.FinishReason = openai.FinishReason(msg.ResponseMeta.FinishReason)
	}

	usage := openai.Usage{}
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		usage.PromptTokens = msg.ResponseMeta.Usage.PromptTokens
		usage.CompletionTokens = msg.ResponseMeta.Usage.CompletionTokens
		usage.TotalTokens = msg.ResponseMeta.Usage.TotalTokens
	}

	return &openai.ChatCompletionResponse{
		ID:      "",
		Object:  "chat.completion",
		Created: 0,
		Model:   "",
		Choices: []openai.ChatCompletionChoice{choice},
		Usage:   usage,
	}
}

// MessageToOpenaiStreamResponse 将 schema.Message 转换为 OpenAI 格式的流式响应
func MessageToOpenaiStreamResponse(msg *schema.Message, index int) *openai.ChatCompletionStreamResponse {
	if msg == nil {
		return nil
	}

	delta := openai.ChatCompletionStreamChoiceDelta{
		Role:             string(msg.Role),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
	}

	if len(msg.ToolCalls) > 0 {
		delta.ToolCalls = make([]openai.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			delta.ToolCalls = append(delta.ToolCalls, openai.ToolCall{
				Index: tc.Index,
				ID:    tc.ID,
				Type:  openai.ToolType(tc.Type),
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	choice := openai.ChatCompletionStreamChoice{
		Index: index,
		Delta: delta,
	}

	if msg.ResponseMeta != nil {
		choice.FinishReason = openai.FinishReason(msg.ResponseMeta.FinishReason)
	}

	response := &openai.ChatCompletionStreamResponse{
		ID:      "",
		Object:  "chat.completion.chunk",
		Created: 0,
		Model:   "",
		Choices: []openai.ChatCompletionStreamChoice{choice},
	}

	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		response.Usage = &openai.Usage{
			PromptTokens:     msg.ResponseMeta.Usage.PromptTokens,
			CompletionTokens: msg.ResponseMeta.Usage.CompletionTokens,
			TotalTokens:      msg.ResponseMeta.Usage.TotalTokens,
		}
	}

	return response
}
