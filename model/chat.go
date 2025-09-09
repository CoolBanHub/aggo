package model

import (
	"context"

	"github.com/CoolBanHub/aggo/model/openai"
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

	param := &openai.Config{
		APIKey:     o.APIKey, // OpenAI API 密钥
		ByAzure:    o.ByAzure,
		BaseURL:    o.BaseUrl,
		APIVersion: o.APIVersion,
		Model:      _model, // 使用的模型版本
	}

	if o.MaxTokens > 0 {
		param.MaxTokens = &o.MaxTokens
	}

	cm, err := openai.NewClient(context.Background(), param)
	return cm, err
}
