package milvus

import (
	"context"
	"fmt"

	milvus2indexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
	milvus2retriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

var milvusOutputFields = []string{"id", "content", "metadata", "created_at", "updated_at"}

// Milvus wraps eino-ext milvus2 indexer + retriever as a unified Database.
type Milvus struct {
	indexer   indexer.Indexer
	retriever retriever.Retriever
}

// MilvusConfig holds configuration for creating a Milvus Database.
type MilvusConfig struct {
	Client         *milvusclient.Client
	CollectionName string
	EmbeddingDim   int
	Embedding      embedding.Embedder
}

// NewMilvus creates a Milvus Database instance backed by eino-ext milvus2 components.
func NewMilvus(ctx context.Context, config MilvusConfig) (*Milvus, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("milvus client不能为空")
	}
	if config.Embedding == nil {
		return nil, fmt.Errorf("embedding组件不能为空")
	}
	if config.EmbeddingDim <= 0 {
		return nil, fmt.Errorf("embedding维度必须大于0")
	}
	if config.CollectionName == "" {
		config.CollectionName = "aggo_knowledge_vectors"
	}

	idx, err := milvus2indexer.NewIndexer(ctx, &milvus2indexer.IndexerConfig{
		Client:     config.Client,
		Collection: config.CollectionName,
		Vector: &milvus2indexer.VectorConfig{
			Dimension:  int64(config.EmbeddingDim),
			MetricType: milvus2indexer.COSINE,
		},
		Embedding: config.Embedding,
	})
	if err != nil {
		return nil, fmt.Errorf("创建indexer失败: %w", err)
	}

	ret, err := milvus2retriever.NewRetriever(ctx, buildRetrieverConfig(config))
	if err != nil {
		return nil, fmt.Errorf("创建retriever失败: %w", err)
	}

	return &Milvus{
		indexer:   idx,
		retriever: ret,
	}, nil
}

func buildRetrieverConfig(config MilvusConfig) *milvus2retriever.RetrieverConfig {
	return &milvus2retriever.RetrieverConfig{
		Client:       config.Client,
		Collection:   config.CollectionName,
		OutputFields: append([]string(nil), milvusOutputFields...),
		TopK:         10,
		SearchMode:   search_mode.NewApproximate(milvus2retriever.COSINE),
		Embedding:    config.Embedding,
	}
}

// Store stores documents via the eino-ext indexer.
func (m *Milvus) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	return m.indexer.Store(ctx, docs, opts...)
}

// Retrieve retrieves documents via the eino-ext retriever.
func (m *Milvus) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	docs, err := m.retriever.Retrieve(ctx, query, opts...)
	if err != nil {
		return nil, err
	}

	commonOptions := retriever.GetCommonOptions(nil, opts...)
	if commonOptions.ScoreThreshold == nil {
		return docs, nil
	}

	threshold := *commonOptions.ScoreThreshold
	filtered := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		if doc != nil && doc.Score() >= threshold {
			filtered = append(filtered, doc)
		}
	}

	return filtered, nil
}

// GetType returns the component type name.
func (m *Milvus) GetType() string {
	return "Milvus"
}
