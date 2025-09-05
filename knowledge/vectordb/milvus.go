package vectordb

import (
	"context"
	"fmt"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// MilvusVectorDB Milvus向量数据库实现
type MilvusVectorDB struct {
	client         *milvusclient.Client
	collectionName string
	embeddingDim   int
}

// MilvusConfig Milvus连接配置
type MilvusConfig struct {
	Client         *milvusclient.Client
	CollectionName string `json:"collectionName"`
	EmbeddingDim   int    `json:"embeddingDim"`
}

// NewMilvusVectorDB 创建Milvus向量数据库实例
func NewMilvusVectorDB(config MilvusConfig) (*MilvusVectorDB, error) {
	if config.CollectionName == "" {
		config.CollectionName = "aggo_knowledge_vectors"
	}
	db := &MilvusVectorDB{
		client:         config.Client,
		collectionName: config.CollectionName,
		embeddingDim:   config.EmbeddingDim,
	}

	// 初始化集合
	if err := db.initCollection(); err != nil {
		return nil, fmt.Errorf("初始化集合失败: %w", err)
	}

	return db, nil
}

// initCollection 初始化Milvus集合
func (m *MilvusVectorDB) initCollection() error {
	ctx := context.Background()

	// 检查集合是否存在
	exists, err := m.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(m.collectionName))
	if err != nil {
		return fmt.Errorf("检查集合是否存在失败: %w", err)
	}

	if exists {
		// 加载集合
		_, err = m.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(m.collectionName))
		return err
	}

	// 创建集合schema
	schema := entity.NewSchema().WithDynamicFieldEnabled(true).
		WithField(entity.NewField().WithName("id").WithIsAutoID(false).WithMaxLength(255).WithDataType(entity.FieldTypeVarChar).WithIsPrimaryKey(true)).
		WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeJSON)).
		WithField(entity.NewField().WithName("created_at").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("updated_at").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(m.embeddingDim)))

	// 创建索引选项
	indexOptions := []milvusclient.CreateIndexOption{
		milvusclient.NewCreateIndexOption(m.collectionName, "vector", index.NewHNSWIndex(entity.COSINE, 64, 512)),
		milvusclient.NewCreateIndexOption(m.collectionName, "id", index.NewAutoIndex(entity.COSINE)),
	}

	// 创建集合
	err = m.client.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(m.collectionName, schema).WithIndexOptions(indexOptions...))
	if err != nil {
		return fmt.Errorf("创建集合失败: %w", err)
	}

	// 加载集合
	_, err = m.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(m.collectionName))
	if err != nil {
		return fmt.Errorf("加载集合失败: %w", err)
	}

	return nil
}

// Insert 插入文档
func (m *MilvusVectorDB) Insert(ctx context.Context, docs []knowledge.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 准备数据
	ids := make([]string, len(docs))
	contents := make([]string, len(docs))
	metadatas := make([][]byte, len(docs))
	createdAts := make([]int64, len(docs))
	updatedAts := make([]int64, len(docs))
	vectors := make([][]float32, len(docs))

	for i, doc := range docs {
		ids[i] = doc.ID
		contents[i] = doc.Content

		// 序列化metadata为JSON
		metadataBytes, err := marshalMetadata(doc.Metadata)
		if err != nil {
			return fmt.Errorf("序列化metadata失败: %w", err)
		}
		metadatas[i] = metadataBytes

		createdAts[i] = doc.CreatedAt.Unix()
		updatedAts[i] = doc.UpdatedAt.Unix()
		vectors[i] = doc.Vector
	}

	// 执行插入
	_, err := m.client.Insert(ctx, milvusclient.NewColumnBasedInsertOption(m.collectionName).
		WithVarcharColumn("id", ids).
		WithVarcharColumn("content", contents).
		WithColumns(column.NewColumnJSONBytes("metadata", metadatas)).
		WithColumns(column.NewColumnInt64("created_at", createdAts)).
		WithColumns(column.NewColumnInt64("updated_at", updatedAts)).
		WithFloatVectorColumn("vector", m.embeddingDim, vectors))

	if err != nil {
		return fmt.Errorf("插入文档失败: %w", err)
	}

	return nil
}

// Upsert 插入或更新文档
func (m *MilvusVectorDB) Upsert(ctx context.Context, docs []knowledge.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 准备数据
	ids := make([]string, len(docs))
	contents := make([]string, len(docs))
	metadatas := make([][]byte, len(docs))
	createdAts := make([]int64, len(docs))
	updatedAts := make([]int64, len(docs))
	vectors := make([][]float32, len(docs))

	for i, doc := range docs {
		ids[i] = doc.ID
		contents[i] = doc.Content

		// 序列化metadata为JSON
		metadataBytes, err := marshalMetadata(doc.Metadata)
		if err != nil {
			return fmt.Errorf("序列化metadata失败: %w", err)
		}
		metadatas[i] = metadataBytes

		createdAts[i] = doc.CreatedAt.Unix()
		updatedAts[i] = doc.UpdatedAt.Unix()
		vectors[i] = doc.Vector
	}

	// 执行upsert
	_, err := m.client.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(m.collectionName).
		WithVarcharColumn("id", ids).
		WithVarcharColumn("content", contents).
		WithColumns(column.NewColumnJSONBytes("metadata", metadatas)).
		WithColumns(column.NewColumnInt64("created_at", createdAts)).
		WithColumns(column.NewColumnInt64("updated_at", updatedAts)).
		WithFloatVectorColumn("vector", m.embeddingDim, vectors))

	if err != nil {
		return fmt.Errorf("upsert文档失败: %w", err)
	}

	return nil
}

// Search 向量搜索
func (m *MilvusVectorDB) Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}, sort float64) ([]knowledge.SearchResult, error) {
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("查询向量不能为空")
	}

	vectors := []entity.Vector{entity.FloatVector(queryVector)}

	// 构建搜索参数
	annParam := index.NewCustomAnnParam()
	annParam.WithRadius(sort)
	annParam.WithRangeFilter(1.0)

	// 构建过滤器表达式
	filterExpr := buildFilterExpression(filters)
	searchOption := milvusclient.NewSearchOption(m.collectionName, limit, vectors).
		WithOutputFields("id", "content", "metadata", "created_at", "updated_at").
		WithAnnParam(annParam)

	if filterExpr != "" {
		searchOption.WithFilter(filterExpr)
	}

	// 执行搜索
	results, err := m.client.Search(ctx, searchOption)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}

	// 处理搜索结果
	var searchResults []knowledge.SearchResult
	for _, result := range results {
		for i := 0; i < result.IDs.Len(); i++ {
			_, err = result.IDs.Get(i)
			if err != nil {
				continue
			}

			doc, err := m.buildDocumentFromResult(result, i)
			if err != nil {
				continue
			}

			searchResult := knowledge.SearchResult{
				Document: doc,
				Score:    float64(result.Scores[i]),
			}
			searchResults = append(searchResults, searchResult)
		}
	}

	return searchResults, nil
}

// DocExists 检查文档是否存在
func (m *MilvusVectorDB) DocExists(ctx context.Context, docID string) (bool, error) {
	resultSet, err := m.client.Get(ctx, milvusclient.NewQueryOption(m.collectionName).
		WithIDs(column.NewColumnVarChar("id", []string{docID})).
		WithOutputFields("id"))
	if err != nil {
		return false, fmt.Errorf("查询文档失败: %w", err)
	}

	return resultSet.ResultCount > 0, nil
}

// GetDocument 获取文档
func (m *MilvusVectorDB) GetDocument(ctx context.Context, docID string) (*knowledge.Document, error) {
	resultSet, err := m.client.Get(ctx, milvusclient.NewQueryOption(m.collectionName).
		WithIDs(column.NewColumnVarChar("id", []string{docID})).
		WithOutputFields("id", "content", "metadata", "created_at", "updated_at", "vector"))
	if err != nil {
		return nil, fmt.Errorf("查询文档失败: %w", err)
	}

	if resultSet.ResultCount <= 0 {
		return nil, fmt.Errorf("文档未找到: %s", docID)
	}

	doc, err := m.buildDocumentFromResultSet(resultSet, 0)
	if err != nil {
		return nil, fmt.Errorf("构建文档失败: %w", err)
	}

	return &doc, nil
}

// UpdateDocument 更新文档
func (m *MilvusVectorDB) UpdateDocument(ctx context.Context, doc knowledge.Document) error {
	return m.Upsert(ctx, []knowledge.Document{doc})
}

// DeleteDocument 删除文档
func (m *MilvusVectorDB) DeleteDocument(ctx context.Context, docID string) error {
	_, err := m.client.Delete(ctx, milvusclient.NewDeleteOption(m.collectionName).
		WithStringIDs("id", []string{docID}))
	if err != nil {
		return fmt.Errorf("删除文档失败: %w", err)
	}
	return nil
}

// Exists 检查集合是否存在
func (m *MilvusVectorDB) Exists() bool {
	exists, err := m.client.HasCollection(context.Background(), milvusclient.NewHasCollectionOption(m.collectionName))
	if err != nil {
		return false
	}
	return exists
}

// Create 创建集合
func (m *MilvusVectorDB) Create() error {
	return m.initCollection()
}

// Drop 删除集合
func (m *MilvusVectorDB) Drop() error {
	err := m.client.DropCollection(context.Background(), milvusclient.NewDropCollectionOption(m.collectionName))
	if err != nil {
		return fmt.Errorf("删除集合失败: %w", err)
	}
	return nil
}

// UpsertAvailable 返回是否支持upsert操作
func (m *MilvusVectorDB) UpsertAvailable() bool {
	return true
}

// Close 关闭连接
func (m *MilvusVectorDB) Close() error {
	if m.client != nil {
		return m.client.Close(context.Background())
	}
	return nil
}
