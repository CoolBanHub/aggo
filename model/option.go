package model

import (
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
)

type Option struct {
	Platform   string
	Model      string
	BaseUrl    string
	APIKey     string `json:"apiKey"`
	Dimensions int
	MaxTokens  int

	//openai参数
	ReasoningEffortLevel openai.ReasoningEffortLevel

	// HTTPClient 设置后 Timeout 不生效，由 client 自行控制超时
	HTTPClient *http.Client
	// Timeout 单次请求超时，<=0 时不设置
	Timeout time.Duration
	// CustomHeaders 附加到每个请求的自定义 HTTP 头
	CustomHeaders map[string]string
	// ExtraFields 透传给底层 provider 的额外参数
	// （ReasoningEffortLevel 之外的手动扩展字段，键冲突时以后设置者为准）
	ExtraFields map[string]any
}

type OptionFunc func(option *Option)

// WithPlatform 保留兼容；当前实现仅支持 OpenAI 兼容协议，该字段不生效。
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

// WithBaseUrl 设置 OpenAI 兼容服务地址。
//
// Deprecated: 命名不符合 Go initialism 约定，请使用 WithBaseURL。
func WithBaseUrl(baseUrl string) OptionFunc {
	return func(option *Option) {
		option.BaseUrl = baseUrl
	}
}

// WithBaseURL 设置 OpenAI 兼容服务地址（WithBaseUrl 的规范命名版本）。
func WithBaseURL(baseURL string) OptionFunc {
	return func(option *Option) {
		option.BaseUrl = baseURL
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

// WithHTTPClient 设置自定义 HTTP 客户端（设置后 Timeout 选项不生效）。
func WithHTTPClient(client *http.Client) OptionFunc {
	return func(option *Option) {
		option.HTTPClient = client
	}
}

// WithTimeout 设置单次请求超时。
func WithTimeout(timeout time.Duration) OptionFunc {
	return func(option *Option) {
		option.Timeout = timeout
	}
}

// WithCustomHeaders 附加自定义 HTTP 头到每个请求。
func WithCustomHeaders(headers map[string]string) OptionFunc {
	return func(option *Option) {
		option.CustomHeaders = headers
	}
}

// WithExtraFields 设置透传给底层 provider 的额外参数。
func WithExtraFields(fields map[string]any) OptionFunc {
	return func(option *Option) {
		option.ExtraFields = fields
	}
}
