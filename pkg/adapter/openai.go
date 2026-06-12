package adapter

import (
	"fmt"
	"reflect"

	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	"github.com/cloudwego/eino/schema"
	openai "github.com/meguminnnnnnnnn/go-openai"
)

// MessageToOpenaiResponse 将 Eino schema.AgenticMessage 转换为 OpenAI 格式的 ChatCompletionResponse
func MessageToOpenaiResponse(msg *schema.AgenticMessage) *openai.ChatCompletionResponse {
	if msg == nil {
		return nil
	}

	message := openai.ChatCompletionMessage{
		Role:             string(msg.Role),
		Content:          agmsg.Text(msg),
		ReasoningContent: reasoningText(msg),
	}

	message.ToolCalls = openaiToolCalls(msg)

	choice := openai.ChatCompletionChoice{
		Index:   0,
		Message: message,
	}

	if finishReason := openaiFinishReason(msg); finishReason != "" {
		choice.FinishReason = finishReason
	}

	usage := openai.Usage{}
	if msg.ResponseMeta != nil && msg.ResponseMeta.TokenUsage != nil {
		usage.PromptTokens = msg.ResponseMeta.TokenUsage.PromptTokens
		usage.CompletionTokens = msg.ResponseMeta.TokenUsage.CompletionTokens
		usage.TotalTokens = msg.ResponseMeta.TokenUsage.TotalTokens
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

// MessageToOpenaiStreamResponse 将 schema.AgenticMessage 转换为 OpenAI 格式的流式响应
func MessageToOpenaiStreamResponse(msg *schema.AgenticMessage, index int) *openai.ChatCompletionStreamResponse {
	if msg == nil {
		return nil
	}

	delta := openai.ChatCompletionStreamChoiceDelta{
		Role:             string(msg.Role),
		Content:          agmsg.Text(msg),
		ReasoningContent: reasoningText(msg),
		ToolCalls:        openaiToolCalls(msg),
	}

	choice := openai.ChatCompletionStreamChoice{
		Index: index,
		Delta: delta,
	}

	if finishReason := openaiFinishReason(msg); finishReason != "" {
		choice.FinishReason = finishReason
	}

	response := &openai.ChatCompletionStreamResponse{
		ID:      "",
		Object:  "chat.completion.chunk",
		Created: 0,
		Model:   "",
		Choices: []openai.ChatCompletionStreamChoice{choice},
	}

	if msg.ResponseMeta != nil && msg.ResponseMeta.TokenUsage != nil {
		response.Usage = &openai.Usage{
			PromptTokens:     msg.ResponseMeta.TokenUsage.PromptTokens,
			CompletionTokens: msg.ResponseMeta.TokenUsage.CompletionTokens,
			TotalTokens:      msg.ResponseMeta.TokenUsage.TotalTokens,
		}
	}

	return response
}

func openaiToolCalls(msg *schema.AgenticMessage) []openai.ToolCall {
	if msg == nil {
		return nil
	}
	var calls []openai.ToolCall
	for _, block := range msg.ContentBlocks {
		if block == nil || block.FunctionToolCall == nil {
			continue
		}
		call := block.FunctionToolCall
		toolCall := openai.ToolCall{
			ID:   call.CallID,
			Type: openai.ToolType("function"),
			Function: openai.FunctionCall{
				Name:      call.Name,
				Arguments: call.Arguments,
			},
		}
		if block.StreamingMeta != nil {
			index := block.StreamingMeta.Index
			toolCall.Index = &index
		}
		calls = append(calls, toolCall)
	}
	return calls
}

func reasoningText(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	var text string
	for _, block := range msg.ContentBlocks {
		if block != nil && block.Reasoning != nil {
			text += block.Reasoning.Text
		}
	}
	return text
}

func openaiFinishReason(msg *schema.AgenticMessage) openai.FinishReason {
	if msg == nil || msg.ResponseMeta == nil {
		return ""
	}
	switch {
	case msg.ResponseMeta.GeminiExtension != nil:
		return openai.FinishReason(msg.ResponseMeta.GeminiExtension.FinishReason)
	default:
		return finishReasonFromExtension(msg.ResponseMeta.Extension)
	}
}

func finishReasonFromExtension(extension any) openai.FinishReason {
	if extension == nil {
		return ""
	}
	v := reflect.ValueOf(extension)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		field := v.FieldByName("FinishReason")
		if field.IsValid() && field.Kind() == reflect.String {
			return openai.FinishReason(field.String())
		}
	}
	if value := fmt.Sprint(extension); value != "" && value != "<nil>" {
		return openai.FinishReason(value)
	}
	return ""
}
