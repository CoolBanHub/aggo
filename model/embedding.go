package model

import (
	"context"

	embopenai "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

func NewEmbModel(opts ...OptionFunc) (embedding.Embedder, error) {
	o := &Option{}
	for _, opt := range opts {
		opt(o)
	}
	//目前就只支持了一种，后续增加
	return getEmbeddingByOpenai(o)
}

func getEmbeddingByOpenai(o *Option) (embedding.Embedder, error) {
	_model := o.Model
	dimensions := 1024
	cmb, err := embopenai.NewEmbedder(context.Background(), &embopenai.EmbeddingConfig{
		BaseURL:    o.BaseUrl,
		Model:      _model,   // 使用的模型版本
		APIKey:     o.APIKey, // OpenAI API 密钥
		APIVersion: o.APIVersion,
		ByAzure:    o.ByAzure,
		Dimensions: &dimensions, // 设置向量维度为1024
	})
	return cmb, err
}

func GetEmbByText(ctx context.Context, text string, opts ...OptionFunc) ([]float64, error) {
	cmb, err := NewEmbModel(opts...)
	if err != nil {
		return nil, err
	}
	embRes, err := cmb.EmbedStrings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embRes) == 0 {
		return nil, nil
	}
	if len(embRes[0]) == 0 {
		return nil, nil
	}
	return embRes[0], nil
}
