package model

type Option struct {
	Platform   string
	Model      string
	BaseUrl    string
	APIKey     string `json:"apiKey"`
	APIVersion string `json:"apiVersion"`
	ByAzure    bool
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
