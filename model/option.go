package model

type Option struct {
	Platform   string
	Model      string
	BaseUrl    string
	APIKey     string `json:"apiKey"`
	APIVersion string `json:"apiVersion"`
	ByAzure    bool
	Dimensions int
	MaxTokens  int
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

func WithAPIVersion(apiVersion string) OptionFunc {
	return func(option *Option) {
		option.APIVersion = apiVersion
	}
}

func WithMaxTokens(maxTokens int) OptionFunc {
	return func(option *Option) {
		option.MaxTokens = maxTokens
	}
}

func WithAzure(byAzure bool) OptionFunc {
	return func(option *Option) {
		option.ByAzure = byAzure
	}
}

func WithDimensions(dimensions int) OptionFunc {
	return func(option *Option) {
		option.Dimensions = dimensions
	}
}
