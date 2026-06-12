package model

import (
	"context"

	"github.com/CoolBanHub/aggo/pkg/ailens360"
	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
)

func NewChatModel(opts ...OptionFunc) (model.AgenticModel, error) {
	o := &Option{}
	for _, opt := range opts {
		opt(o)
	}
	//目前就只支持了一种，后续增加
	return getChatByOpenai(o)
}

func getChatByOpenai(o *Option) (model.AgenticModel, error) {
	_model := o.Model

	param := &agenticopenai.ChatConfig{
		APIKey:  o.APIKey,
		BaseURL: o.BaseUrl,
		Model:   _model,
	}

	if o.ReasoningEffortLevel != "" {
		param.ExtraFields = map[string]any{
			"reasoning_effort": o.ReasoningEffortLevel,
		}
	}

	if o.MaxTokens > 0 {
		param.MaxCompletionTokens = &o.MaxTokens
	}

	// If a global AILens360 decorator is installed (via ailens360.SetGlobal),
	// route this chat model through the proxy and inject the telemetry-header
	// RoundTripper. No-op when not configured.
	ailens360.ApplyGlobalAgentic(param)

	cm, err := agenticopenai.NewChatModel(context.Background(), param)
	return cm, err
}
