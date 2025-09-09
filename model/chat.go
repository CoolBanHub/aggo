package model

import (
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
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
		APIKey:  o.APIKey, // OpenAI API 密钥
		BaseURL: o.BaseUrl,
		Model:   _model, // 使用的模型版本
	}

	if o.MaxTokens > 0 {
		param.MaxTokens = &o.MaxTokens
	}

	cm, err := openai.NewChatModel(context.Background(), param)
	return cm, err
}
