package model

import "github.com/cloudwego/eino-ext/components/model/openai"

type Option struct {
	Platform   string
	Model      string
	BaseUrl    string
	APIKey     string `json:"apiKey"`
	Dimensions int
	MaxTokens  int

	//openai参数
	ReasoningEffortLevel openai.ReasoningEffortLevel
}

type OptionFunc func(option *Option)

func WithPlatform(platform string) OptionFunc {
	return func(option *Option) {
		option.Platform = platform
	}
}

func WithModel(model string) OptionFunc {
	return func(option *Option) {
		option.Model = model
	}
}

func WithBaseUrl(baseUrl string) OptionFunc {
	return func(option *Option) {
		option.BaseUrl = baseUrl
	}
}

func WithAPIKey(apiKey string) OptionFunc {
	return func(option *Option) {
		option.APIKey = apiKey
	}
}

func WithMaxTokens(maxTokens int) OptionFunc {
	return func(option *Option) {
		option.MaxTokens = maxTokens
	}
}

func WithDimensions(dimensions int) OptionFunc {
	return func(option *Option) {
		option.Dimensions = dimensions
	}
}
func WithReasoningEffortLevel(reasoningEffortLevel openai.ReasoningEffortLevel) OptionFunc {
	return func(option *Option) {
		option.ReasoningEffortLevel = reasoningEffortLevel
	}
}
