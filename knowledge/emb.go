package knowledge

import (
	"context"

	"github.com/CoolBanHub/aggo/model"
	"github.com/cloudwego/eino/components/embedding"
)

// embeddingAdapter 嵌入器适配器，将现有的embedding模型适配到知识库接口
type embeddingAdapter struct {
	embedder embedding.Embedder
}

// Embed 生成单个文本的嵌入
func (ea *embeddingAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := model.GetEmbByText(ctx, text)
	if err != nil {
		return nil, err
	}

	// 转换float64到float32
	result := make([]float32, len(vectors))
	for i, v := range vectors {
		result[i] = float32(v)
	}

	return result, nil
}

// EmbedBatch 批量生成嵌入
func (ea *embeddingAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors, err := ea.embedder.EmbedStrings(ctx, texts)
	if err != nil {
		return nil, err
	}

	result := make([][]float32, len(vectors))
	for i, vector := range vectors {
		result[i] = make([]float32, len(vector))
		for j, v := range vector {
			result[i][j] = float32(v)
		}
	}

	return result, nil
}

// GetDimension 获取嵌入向量维度
func (ea *embeddingAdapter) GetDimension() int {
	// text-embedding-3-large 的维度是 3072
	return 1024
}
