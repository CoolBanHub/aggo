package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/embedding"
)

// KnowledgeManager 知识库管理器
// 负责管理文档存储、向量搜索和知识检索
type KnowledgeManager struct {
	// 向量数据库
	vectorDB VectorDB
	// 知识库存储
	storage KnowledgeStorage
	// 嵌入器
	embedder embedding.Embedder
	// 分块策略
	chunkingStrategy ChunkingStrategy
	// 知识库配置
	config *KnowledgeConfig

	// 并发控制
	mu sync.RWMutex
	// 上下文和取消函数
	ctx    context.Context
	cancel context.CancelFunc
}

// NewKnowledgeManager 创建新的知识库管理器
func NewKnowledgeManager(config *KnowledgeConfig) (*KnowledgeManager, error) {
	if config == nil {
		config = &KnowledgeConfig{
			DefaultSearchOptions: SearchOptions{
				Limit:     10,
				Threshold: 0.7,
			},
			DefaultLoadOptions: LoadOptions{
				EnableChunking: true,
				ChunkSize:      1000,
				ChunkOverlap:   200,
			},
		}
	}

	if config.Storage != nil {
		if config.StorageTablePrefix != "" {
			config.Storage.SetTablePrefix(config.StorageTablePrefix)
		}

		// 自动迁移数据库表结构
		if err := config.Storage.AutoMigrate(); err != nil {
			return nil, fmt.Errorf("failed to auto migrate: %w", err)
		}
	}

	// 创建分块策略
	chunkingStrategy := NewFixedSizeChunkingStrategy(
		config.DefaultLoadOptions.ChunkSize,
		config.DefaultLoadOptions.ChunkOverlap,
	)

	ctx, cancel := context.WithCancel(context.Background())

	manager := &KnowledgeManager{
		vectorDB:         config.VectorDB,
		storage:          config.Storage,
		embedder:         config.Em,
		chunkingStrategy: chunkingStrategy,
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
	}

	return manager, nil
}

// LoadDocuments 加载文档到知识库
func (km *KnowledgeManager) LoadDocuments(ctx context.Context, docs []Document, options LoadOptions) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if options.ChunkSize == 0 {
		options.ChunkSize = km.config.DefaultLoadOptions.ChunkSize
	}
	if options.ChunkOverlap == 0 {
		options.ChunkOverlap = km.config.DefaultLoadOptions.ChunkOverlap
	}

	var processedDocs []Document

	for _, doc := range docs {
		// 设置时间戳
		now := time.Now()
		if doc.CreatedAt.IsZero() {
			doc.CreatedAt = now
		}
		doc.UpdatedAt = now

		if options.EnableChunking {
			// 对文档进行分块
			chunks, err := km.chunkingStrategy.Chunk(doc)
			if err != nil {
				return fmt.Errorf("文档分块失败: %w", err)
			}

			// 为每个分块生成嵌入
			for i, chunk := range chunks {
				vector, err := km.embed(ctx, chunk.Content)
				if err != nil {
					return fmt.Errorf("生成嵌入失败: %w", err)
				}

				// 创建包含分块信息的文档
				chunkDoc := Document{
					ID:        fmt.Sprintf("%s_chunk_%d", doc.ID, i),
					Content:   chunk.Content,
					Vector:    vector,
					CreatedAt: now,
					UpdatedAt: now,
					Metadata: map[string]interface{}{
						"original_doc_id": doc.ID,
						"chunk_index":     chunk.Index,
						"start_offset":    chunk.StartOffset,
						"end_offset":      chunk.EndOffset,
						"is_chunk":        true,
					},
				}

				// 合并原始元数据
				for k, v := range doc.Metadata {
					if _, exists := chunkDoc.Metadata[k]; !exists {
						chunkDoc.Metadata[k] = v
					}
				}

				processedDocs = append(processedDocs, chunkDoc)
			}
		} else {
			// 为整个文档生成嵌入
			vector, err := km.embed(ctx, doc.Content)
			if err != nil {
				return fmt.Errorf("生成嵌入失败: %w", err)
			}

			doc.Vector = vector
			processedDocs = append(processedDocs, doc)
		}

		// 保存原始文档到存储
		if err := km.storage.SaveDocument(ctx, &doc); err != nil {
			return fmt.Errorf("保存文档失败: %w", err)
		}
	}

	// 插入到向量数据库
	if options.Upsert {
		return km.vectorDB.Upsert(ctx, processedDocs)
	} else {
		return km.vectorDB.Insert(ctx, processedDocs)
	}
}

// Search 搜索知识库
func (km *KnowledgeManager) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	vector, err := km.embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("生成嵌入失败: %w", err)
	}

	if options.Limit == 0 {
		options.Limit = km.GetConfig().DefaultSearchOptions.Limit
	}
	if options.Threshold == 0 {
		options.Threshold = km.GetConfig().DefaultSearchOptions.Threshold
	}

	// 使用向量数据库进行搜索
	results, err := km.vectorDB.Search(ctx, vector, options.Limit, options.Filters, options.Threshold)
	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}
	// 过滤低分结果
	filteredResults := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if result.Score >= options.Threshold {
			filteredResults = append(filteredResults, result)
		}
	}

	return filteredResults, nil
}

// AddDocument 添加单个文档
func (km *KnowledgeManager) AddDocument(ctx context.Context, doc Document) error {
	return km.LoadDocuments(ctx, []Document{doc}, km.config.DefaultLoadOptions)
}

// UpdateDocument 更新文档
func (km *KnowledgeManager) UpdateDocument(ctx context.Context, doc Document) error {

	km.mu.Lock()
	defer km.mu.Unlock()

	// 设置更新时间
	doc.UpdatedAt = time.Now()

	// 更新存储
	if err := km.storage.UpdateDocument(ctx, &doc); err != nil {
		return fmt.Errorf("更新文档失败: %w", err)
	}

	// 更新向量数据库
	if err := km.vectorDB.UpdateDocument(ctx, doc); err != nil {
		return fmt.Errorf("更新向量数据库失败: %w", err)
	}

	return nil
}

// DeleteDocument 删除文档
func (km *KnowledgeManager) DeleteDocument(ctx context.Context, docID string) error {

	km.mu.Lock()
	defer km.mu.Unlock()

	// 从存储删除
	if err := km.storage.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("从存储删除文档失败: %w", err)
	}

	// 从向量数据库删除
	if err := km.vectorDB.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("从向量数据库删除文档失败: %w", err)
	}

	return nil
}

// GetDocument 获取文档
func (km *KnowledgeManager) GetDocument(ctx context.Context, docID string) (*Document, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	return km.storage.GetDocument(ctx, docID)
}

// ListDocuments 列出文档
func (km *KnowledgeManager) ListDocuments(ctx context.Context, limit, offset int) ([]*Document, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	return km.storage.ListDocuments(ctx, limit, offset)
}

// GetConfig 获取配置
func (km *KnowledgeManager) GetConfig() *KnowledgeConfig {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.config
}

// UpdateConfig 更新配置
func (km *KnowledgeManager) UpdateConfig(config *KnowledgeConfig) {
	if config == nil {
		return
	}

	km.mu.Lock()
	defer km.mu.Unlock()
	km.config = config
}

// Close 关闭管理器
func (km *KnowledgeManager) Close() error {
	km.cancel()

	var errors []error

	if km.storage != nil {
		if err := km.storage.Close(); err != nil {
			errors = append(errors, fmt.Errorf("关闭存储失败: %w", err))
		}
	}

	if km.vectorDB != nil {
		if err := km.vectorDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("关闭向量数据库失败: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("关闭知识库管理器时出现错误: %v", errors)
	}

	return nil
}

func (km *KnowledgeManager) embed(ctx context.Context, content string) ([]float32, error) {
	vectorsList, err := km.embedder.EmbedStrings(ctx, []string{content})
	if err != nil {
		return nil, err
	}
	if len(vectorsList) == 0 {
		return nil, errors.New("embedding failed")
	}
	vectors := vectorsList[0]
	// 转换float64到float32
	result := make([]float32, len(vectors))
	for i, v := range vectors {
		result[i] = float32(v)
	}
	return result, nil
}
