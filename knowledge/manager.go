package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// KnowledgeManager 知识库管理器
// 负责管理文档存储、向量搜索和知识检索
type KnowledgeManager struct {
	// 知识库存储
	storage KnowledgeStorage

	indexer VectorDB

	retriever retriever.Retriever

	// 分块策略
	transformers map[string]document.Transformer

	loaders map[string]document.Loader

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
				Threshold: 0.1,
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

	ctx, cancel := context.WithCancel(context.Background())

	manager := &KnowledgeManager{
		storage:      config.Storage,
		indexer:      config.Indexer,
		retriever:    config.Retriever,
		transformers: config.Transformers,
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
	}

	return manager, nil
}

// LoadDocuments 加载文档到知识库
func (km *KnowledgeManager) LoadDocuments(ctx context.Context, docs []*schema.Document, options LoadOptions) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if options.EnableChunking {
		if km.transformers == nil {
			return errors.New("not config transformers")
		}
		if _, ok := km.transformers[options.ChunkType]; !ok {
			return errors.New(fmt.Sprintf("not found transformer [%s]", options.ChunkType))
		}
		transformers := km.transformers[options.ChunkType]
		docList, err := transformers.Transform(ctx, docs, options.TransformerOptions...)
		if err != nil {
			return err
		}
		docs = docList
	}

	if km.storage != nil {
		err := km.storage.Store(ctx, docs)
		if err != nil {
			return err
		}
	}
	if km.indexer != nil {
		_, err := km.indexer.Store(ctx, docs)
		if err != nil {
			return err
		}
	}

	return nil
}

// Search 搜索知识库
func (km *KnowledgeManager) Search(ctx context.Context, query string, options SearchOptions) ([]*schema.Document, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	if options.Limit == 0 {
		options.Limit = km.GetConfig().DefaultSearchOptions.Limit
	}
	if options.Threshold == 0 {
		options.Threshold = km.GetConfig().DefaultSearchOptions.Threshold
	}
	if options.Mode == "" {
		options.Mode = km.GetConfig().DefaultSearchOptions.Mode
	}

	// 根据搜索模式选择使用哪种搜索，默认是向量搜索,目前有向量搜索，模糊搜索，混合搜索
	switch options.Mode {
	case SearchModeFuzzy:
		return km.fuzzySearch(ctx, query, options)
	case SearchModeHybrid:
		return km.hybridSearch(ctx, query, options)
	case SearchModeVector:
		fallthrough
	default: // 默认使用向量搜索
		return km.vectorSearch(ctx, query, options)
	}
}

// vectorSearch 向量搜索
func (km *KnowledgeManager) vectorSearch(ctx context.Context, query string, options SearchOptions) ([]*schema.Document, error) {
	// 使用向量数据库进行搜索
	return km.retriever.Retrieve(ctx, query, options.RetrieverOptions...)
}

// fuzzySearch 模糊搜索
func (km *KnowledgeManager) fuzzySearch(ctx context.Context, query string, options SearchOptions) ([]*schema.Document, error) {
	if km.storage == nil {
		return nil, fmt.Errorf("存储未配置，无法进行模糊搜索")
	}
	// 使用存储接口进行模糊搜索
	return km.storage.SearchDocuments(ctx, query, options.Limit)
}

// hybridSearch 混合搜索
func (km *KnowledgeManager) hybridSearch(ctx context.Context, query string, options SearchOptions) ([]*schema.Document, error) {
	wg := sync.WaitGroup{}
	wg.Add(2)
	// 分别进行向量搜索和模糊搜索
	var vectorResults []*schema.Document
	var err1 error
	go func() {
		defer wg.Done()
		vectorResults, err1 = km.vectorSearch(ctx, query, options)
	}()
	var fuzzyResults []*schema.Document
	var err2 error
	go func() {
		defer wg.Done()
		fuzzyResults, err2 = km.fuzzySearch(ctx, query, options)
	}()
	wg.Wait()

	if err1 != nil {
		return nil, fmt.Errorf("混合搜索中向量搜索失败: %w", err1)
	}

	if err2 != nil {
		// 如果模糊搜索失败，仅使用向量搜索结果
		return vectorResults, nil
	}

	// 合并和去重结果
	resultMap := make(map[string]*schema.Document)

	// 添加向量搜索结果（权重0.7）
	for _, result := range vectorResults {
		result.WithScore(result.Score() * 0.7)
		resultMap[result.ID] = result
	}

	// 添加模糊搜索结果（权重0.3）
	for _, result := range fuzzyResults {
		if existing, exists := resultMap[result.ID]; exists {
			// 如果文档已存在，合并分数
			existing.WithScore(existing.Score() * 0.3)
			resultMap[result.ID] = existing
		} else {
			// 新文档，设置模糊搜索权重
			result.WithScore(result.Score() * 0.3)
			resultMap[result.ID] = result
		}
	}

	// 转换为切片并按分数排序
	finalResults := make([]*schema.Document, 0, len(resultMap))
	for _, result := range resultMap {
		if result.Score() >= options.Threshold {
			finalResults = append(finalResults, result)
		}
	}

	// 按分数降序排序
	for i := 0; i < len(finalResults)-1; i++ {
		for j := i + 1; j < len(finalResults); j++ {
			if finalResults[i].Score() < finalResults[j].Score() {
				finalResults[i], finalResults[j] = finalResults[j], finalResults[i]
			}
		}
	}

	// 限制返回结果数量
	if len(finalResults) > options.Limit {
		finalResults = finalResults[:options.Limit]
	}

	return finalResults, nil
}

// DeleteDocument 删除文档
func (km *KnowledgeManager) DeleteDocument(ctx context.Context, docID string) error {

	km.mu.Lock()
	defer km.mu.Unlock()
	if km.storage != nil {
		// 从存储删除
		if err := km.storage.DeleteDocument(ctx, docID); err != nil {
			return fmt.Errorf("从存储删除文档失败: %w", err)
		}
	}

	//// 从向量数据库删除
	if err := km.indexer.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("从向量数据库删除文档失败: %w", err)
	}

	return nil
}

// GetDocument 获取文档
func (km *KnowledgeManager) GetDocument(ctx context.Context, docID string) (*schema.Document, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.storage != nil {
		return km.storage.GetDocument(ctx, docID)
	}

	if km.indexer != nil {
		return km.indexer.GetDocument(ctx, docID)
	}
	return nil, errors.New("not config storage")
}

// ListDocuments 列出文档
func (km *KnowledgeManager) ListDocuments(ctx context.Context, limit, offset int) ([]*schema.Document, error) {

	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.storage != nil {
		return km.storage.ListDocuments(ctx, limit, offset)
	}
	return nil, errors.New("not support")
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

	if len(errors) > 0 {
		return fmt.Errorf("关闭知识库管理器时出现错误: %v", errors)
	}

	return nil
}
