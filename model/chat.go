package model

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/pkg/ailens360"
	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
)

// NewChatModel 创建 OpenAI 兼容的聊天模型。
//
// Deprecated: 请使用 NewChatModelContext，避免模型构造脱离调用方 context。
func NewChatModel(opts ...OptionFunc) (model.AgenticModel, error) {
	return NewChatModelContext(context.Background(), opts...)
}

// NewChatModelContext 创建 OpenAI 兼容的聊天模型。
// ctx 用于底层 HTTP client 的构造与请求关联；Model 与 APIKey 必填。
// BaseURL 可选，缺省时使用 OpenAI 官方地址。
func NewChatModelContext(ctx context.Context, opts ...OptionFunc) (model.AgenticModel, error) {
	o := &Option{}
	for _, opt := range opts {
		opt(o)
	}
	if err := validateChatOption(o); err != nil {
		return nil, err
	}
	//目前就只支持了一种，后续增加
	return getChatByOpenai(ctx, o)
}

func validateChatOption(o *Option) error {
	if o.Model == "" {
		return fmt.Errorf("model: WithModel is required")
	}
	if o.APIKey == "" {
		return fmt.Errorf("model: WithAPIKey is required")
	}
	return nil
}

func getChatByOpenai(ctx context.Context, o *Option) (model.AgenticModel, error) {
	param := &agenticopenai.ChatConfig{
		APIKey:  o.APIKey,
		BaseURL: o.BaseUrl,
		Model:   o.Model,
	}

	if o.Timeout > 0 {
		param.Timeout = o.Timeout
	}
	if o.HTTPClient != nil {
		param.HTTPClient = o.HTTPClient
	}
	if len(o.CustomHeaders) > 0 {
		param.CustomHeaders = o.CustomHeaders
	}

	extraFields := make(map[string]any, len(o.ExtraFields)+1)
	for k, v := range o.ExtraFields {
		extraFields[k] = v
	}
	if o.ReasoningEffortLevel != "" {
		extraFields["reasoning_effort"] = o.ReasoningEffortLevel
	}
	if len(extraFields) > 0 {
		param.ExtraFields = extraFields
	}

	if o.MaxTokens > 0 {
		param.MaxCompletionTokens = &o.MaxTokens
	}

	// If a global AILens360 decorator is installed (via ailens360.SetGlobal),
	// route this chat model through the proxy and inject the telemetry-header
	// RoundTripper. No-op when not configured.
	ailens360.ApplyGlobalAgentic(param)

	cm, err := agenticopenai.NewChatModel(ctx, param)
	return cm, err
}
