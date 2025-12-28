package model

import (
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	openai2 "github.com/meguminnnnnnnnn/go-openai"
)

func NewChatModel(opts ...OptionFunc) (model.ToolCallingChatModel, error) {
	o := &Option{}
	for _, opt := range opts {
		opt(o)
	}
	//目前就只支持了一种，后续增加
	return getChatByOpenai(o)
}

func getChatByOpenai(o *Option) (model.ToolCallingChatModel, error) {
	_model := o.Model

	param := &openai.ChatModelConfig{
		APIKey:          o.APIKey, // OpenAI API 密钥
		BaseURL:         o.BaseUrl,
		Model:           _model, // 使用的模型版本
		ReasoningEffort: o.ReasoningEffortLevel,
	}

	if o.ReasoningEffortLevel != "" {
		param.ReasoningEffort = o.ReasoningEffortLevel
	}

	if o.MaxTokens > 0 {
		param.MaxTokens = &o.MaxTokens
	}

	cm, err := openai.NewChatModel(context.Background(), param)
	return cm, err
}

// OutMessageEinoToOpenai 将 Eino 的 schema.Message 转换为 OpenAI 的 ChatCompletionResponse
// 该方法用于将模型返回的消息转换为 OpenAI API 格式的响应
// 参数:
//   - msg: Eino schema.Message 对象，包含模型返回的消息内容
//
// 返回:
//   - *openai2.ChatCompletionResponse: OpenAI 格式的聊天完成响应对象
func OutMessageEinoToOpenai(msg *schema.Message) *openai2.ChatCompletionResponse {
	if msg == nil {
		return nil
	}

	// 构建 ChatCompletionMessage
	message := openai2.ChatCompletionMessage{
		Role:             string(msg.Role),
		Content:          msg.Content,
		Name:             msg.Name,
		ReasoningContent: msg.ReasoningContent,
	}

	// 转换 ToolCalls
	if len(msg.ToolCalls) > 0 {
		message.ToolCalls = make([]openai2.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, openai2.ToolCall{
				Index: tc.Index,
				ID:    tc.ID,
				Type:  openai2.ToolType(tc.Type),
				Function: openai2.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	// 如果是工具消息，设置 ToolCallID
	if msg.ToolCallID != "" {
		message.ToolCallID = msg.ToolCallID
	}

	// 构建 ChatCompletionChoice
	choice := openai2.ChatCompletionChoice{
		Index:   0,
		Message: message,
	}

	// 设置 FinishReason
	if msg.ResponseMeta != nil {
		choice.FinishReason = openai2.FinishReason(msg.ResponseMeta.FinishReason)
	}

	// 构建 Usage 信息
	usage := openai2.Usage{}
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		usage.PromptTokens = msg.ResponseMeta.Usage.PromptTokens
		usage.CompletionTokens = msg.ResponseMeta.Usage.CompletionTokens
		usage.TotalTokens = msg.ResponseMeta.Usage.TotalTokens
	}

	// 构建最终的 ChatCompletionResponse
	response := &openai2.ChatCompletionResponse{
		ID:      "",
		Object:  "chat.completion",
		Created: 0,
		Model:   "",
		Choices: []openai2.ChatCompletionChoice{choice},
		Usage:   usage,
	}

	return response
}

// OutStreamMessageEinoToOpenai 将 schema.Message 转换为 ChatCompletionStreamResponse
// 这是一个辅助方法，用于在流式处理中将每个消息块转换为 OpenAI 格式
// 参数:
//   - msg: schema.Message 对象
//   - index: 当前消息在流中的索引
//
// 返回:
//   - *openai2.ChatCompletionStreamResponse: OpenAI 格式的流式响应块
func OutStreamMessageEinoToOpenai(msg *schema.Message, index int) *openai2.ChatCompletionStreamResponse {
	if msg == nil {
		return nil
	}

	// 构建 Delta (流式响应中的增量内容)
	delta := openai2.ChatCompletionStreamChoiceDelta{
		Role:             string(msg.Role),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
	}

	// 转换 ToolCalls
	if len(msg.ToolCalls) > 0 {
		delta.ToolCalls = make([]openai2.ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			delta.ToolCalls = append(delta.ToolCalls, openai2.ToolCall{
				Index: tc.Index,
				ID:    tc.ID,
				Type:  openai2.ToolType(tc.Type),
				Function: openai2.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	// 构建 Choice
	choice := openai2.ChatCompletionStreamChoice{
		Index: index,
		Delta: delta,
	}

	// 设置 FinishReason
	if msg.ResponseMeta != nil {
		choice.FinishReason = openai2.FinishReason(msg.ResponseMeta.FinishReason)
	}

	// 构建流式响应
	response := &openai2.ChatCompletionStreamResponse{
		ID:      "",
		Object:  "chat.completion.chunk",
		Created: 0,
		Model:   "",
		Choices: []openai2.ChatCompletionStreamChoice{choice},
	}

	// 如果是最后一个块，包含 Usage 信息
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		response.Usage = &openai2.Usage{
			PromptTokens:     msg.ResponseMeta.Usage.PromptTokens,
			CompletionTokens: msg.ResponseMeta.Usage.CompletionTokens,
			TotalTokens:      msg.ResponseMeta.Usage.TotalTokens,
		}
	}

	return response
}
