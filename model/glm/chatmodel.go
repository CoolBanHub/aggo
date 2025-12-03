package glm

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var _ model.ToolCallingChatModel = (*ChatModel)(nil)

// ChatModelConfig parameters detail see:
type ChatModelConfig struct {

	// APIKey is your authentication key
	// Use OpenAI API key or Azure API key depending on the service
	// Required
	APIKey string `json:"api_key"`

	// Timeout specifies the maximum duration to wait for API responses
	// If HTTPClient is set, Timeout will not be used.
	// Optional. Default: no timeout
	Timeout time.Duration `json:"timeout"`

	// HTTPClient specifies the client to send HTTP requests.
	// If HTTPClient is set, Timeout will not be used.
	// Optional. Default &http.Client{Timeout: Timeout}
	HTTPClient *http.Client `json:"http_client"`

	// BaseURL specifies the QLM endpoint URL
	BaseURL string `json:"base_url"`

	// The following fields correspond to OpenAI's chat completion API parameters
	// Ref: https://platform.openai.com/docs/api-reference/chat/create

	// Model specifies the ID of the model to use
	// Required
	Model string `json:"model"`

	// MaxTokens limits the maximum number of tokens that can be generated in the chat completion
	// Optional. Default: model's maximum
	MaxTokens *int `json:"max_tokens,omitempty"`

	// Temperature specifies what sampling temperature to use
	// Generally recommend altering this or TopP but not both.
	// Range: 0.0 to 2.0. Higher values make output more random
	// Optional. Default: 1.0
	Temperature *float32 `json:"temperature,omitempty"`

	// TopP controls diversity via nucleus sampling
	// Generally recommend altering this or Temperature but not both.
	// Range: 0.0 to 1.0. Lower values make output more focused
	// Optional. Default: 1.0
	TopP *float32 `json:"top_p,omitempty"`

	// Stop sequences where the API will stop generating further tokens
	// Optional. Example: []string{"\n", "User:"}
	Stop []string `json:"stop,omitempty"`

	// PresencePenalty prevents repetition by penalizing tokens based on presence
	// Range: -2.0 to 2.0. Positive values increase likelihood of new topics
	// Optional. Default: 0
	PresencePenalty *float32 `json:"presence_penalty,omitempty"`

	// ResponseFormat specifies the format of the model's response
	// Optional. Use for structured outputs
	ResponseFormat *openai.ChatCompletionResponseFormat `json:"response_format,omitempty"`

	// Seed enables deterministic sampling for consistent outputs
	// Optional. Set for reproducible results
	Seed *int `json:"seed,omitempty"`

	// FrequencyPenalty prevents repetition by penalizing tokens based on frequency
	// Range: -2.0 to 2.0. Positive values decrease likelihood of repetition
	// Optional. Default: 0
	FrequencyPenalty *float32 `json:"frequency_penalty,omitempty"`

	// LogitBias modifies likelihood of specific tokens appearing in completion
	// Optional. Map token IDs to bias values from -100 to 100
	LogitBias map[string]int `json:"logit_bias,omitempty"`

	// User unique identifier representing end-user
	// Optional. Helps OpenAI monitor and detect abuse
	User *string `json:"user,omitempty"`

	// Thinking enables thinking mode
	// Optional. Default: base on the Model
	Thinking *string `json:"thinking,omitempty"`
}

type ChatModel struct {
	cli *openai.Client

	extraOptions *options
}

func NewChatModel(ctx context.Context, config *ChatModelConfig) (*ChatModel, error) {
	if config == nil {
		return nil, fmt.Errorf("[NewChatModel] config not provided")
	}

	var httpClient *http.Client

	if config.HTTPClient != nil {
		httpClient = config.HTTPClient
	} else {
		httpClient = &http.Client{Timeout: config.Timeout}
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}

	nConfig := &openai.Config{
		BaseURL:          config.BaseURL,
		APIKey:           config.APIKey,
		HTTPClient:       httpClient,
		Model:            config.Model,
		MaxTokens:        config.MaxTokens,
		Temperature:      config.Temperature,
		TopP:             config.TopP,
		Stop:             config.Stop,
		PresencePenalty:  config.PresencePenalty,
		ResponseFormat:   config.ResponseFormat,
		Seed:             config.Seed,
		FrequencyPenalty: config.FrequencyPenalty,
		LogitBias:        config.LogitBias,
		User:             config.User,
	}

	cli, err := openai.NewClient(ctx, nConfig)

	if err != nil {
		return nil, err
	}

	return &ChatModel{
		cli: cli,
		extraOptions: &options{
			Thinking: config.Thinking,
		},
	}, nil
}

func (cm *ChatModel) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (
	outMsg *schema.Message, err error) {
	ctx = callbacks.EnsureRunInfo(ctx, cm.GetType(), components.ComponentOfChatModel)
	opts = cm.parseCustomOptions(opts...)
	return cm.cli.Generate(ctx, in, opts...)
}

func (cm *ChatModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (outStream *schema.StreamReader[*schema.Message], err error) {
	ctx = callbacks.EnsureRunInfo(ctx, cm.GetType(), components.ComponentOfChatModel)
	opts = cm.parseCustomOptions(opts...)
	outStream, err = cm.cli.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}

	var lastIndex *int

	sr := schema.StreamReaderWithConvert(outStream, func(msg *schema.Message) (*schema.Message, error) {
		if len(msg.ToolCalls) > 0 {
			firstToolCall := msg.ToolCalls[0]

			if msg.ResponseMeta == nil || len(msg.ResponseMeta.FinishReason) == 0 {
				lastIndex = firstToolCall.Index
				return msg, nil
			}

			if firstToolCall.Index == nil && len(msg.ResponseMeta.FinishReason) != 0 {
				firstToolCall.Index = lastIndex
				msg.ToolCalls[0] = firstToolCall
			}
		}
		return msg, nil
	})
	return sr, nil
}

func (cm *ChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cli, err := cm.cli.WithToolsForClient(tools)
	if err != nil {
		return nil, err
	}
	return &ChatModel{cli: cli, extraOptions: cm.extraOptions}, nil
}

func (cm *ChatModel) BindTools(tools []*schema.ToolInfo) error {
	return cm.cli.BindTools(tools)
}

func (cm *ChatModel) BindForcedTools(tools []*schema.ToolInfo) error {
	return cm.cli.BindForcedTools(tools)
}

func (cm *ChatModel) parseCustomOptions(opts ...model.Option) []model.Option {
	glmOpts := model.GetImplSpecificOptions(&options{
		Thinking: cm.extraOptions.Thinking,
	}, opts...)

	// Using extra fields to pass the custom options to the underlying client
	extraFields := make(map[string]any)
	if glmOpts.Thinking != nil {
		extraFields["thinking"] = map[string]any{
			"type": *glmOpts.Thinking,
		}
	}
	if len(extraFields) > 0 {
		opts = append(opts, openai.WithExtraFields(extraFields))
	}
	return opts
}

const typ = "GLM"

func (cm *ChatModel) GetType() string {
	return typ
}

func (cm *ChatModel) IsCallbacksEnabled() bool {
	return cm.cli.IsCallbacksEnabled()
}
