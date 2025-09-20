package vectordb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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
	Embedding      embedding.Embedder
}

// MilvusConfig Milvus连接配置
type MilvusConfig struct {
	Client         *milvusclient.Client
	CollectionName string `json:"collectionName"`
	EmbeddingDim   int    `json:"embeddingDim"`
	Embedding      embedding.Embedder
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
		Embedding:      config.Embedding,
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
	_schema := entity.NewSchema().WithDynamicFieldEnabled(true).
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
	err = m.client.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(m.collectionName, _schema).WithIndexOptions(indexOptions...))
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

// Store 存储文档（实现indexer.Indexer接口）
func (m *MilvusVectorDB) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {

	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfIndexer)
	// callback info on start
	ctx = callbacks.OnStart(ctx, &indexer.CallbackInput{
		Docs: docs,
	})
	var err error
	defer func() {
		if err != nil {
			callbacks.OnError(ctx, err)
		}
	}()

	if len(docs) == 0 {
		err = errors.New("docs is empty")
		return nil, err
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
		metadataBytes, err1 := marshalMetadata(doc.MetaData)
		if err1 != nil {
			err = fmt.Errorf("序列化metadata失败: %w", err1)
			return nil, err
		}
		metadatas[i] = metadataBytes

		// 处理时间戳
		now := time.Now().Unix()
		dslInfo := doc.DSLInfo()
		if createdAt, ok := dslInfo["created_at"].(time.Time); ok {
			createdAts[i] = createdAt.Unix()
		} else {
			createdAts[i] = now
		}
		if updatedAt, ok := dslInfo["updated_at"].(time.Time); ok {
			updatedAts[i] = updatedAt.Unix()
		} else {
			updatedAts[i] = now
		}

		// 向量化
		vectorData, err2 := m.Embedding.EmbedStrings(ctx, []string{doc.Content})
		if err2 != nil {
			err = fmt.Errorf("向量化失败: %w", err2)
			return nil, err
		}
		if len(vectorData) == 0 || len(vectorData[0]) == 0 {
			err = fmt.Errorf("向量化失败: 向量数据为空")
			return nil, err
		}
		vectors[i] = Float64ToFloat32(vectorData[0])
	}

	// 执行插入
	_, err = m.client.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(m.collectionName).
		WithVarcharColumn("id", ids).
		WithVarcharColumn("content", contents).
		WithColumns(column.NewColumnJSONBytes("metadata", metadatas)).
		WithColumns(column.NewColumnInt64("created_at", createdAts)).
		WithColumns(column.NewColumnInt64("updated_at", updatedAts)).
		WithFloatVectorColumn("vector", m.embeddingDim, vectors))

	if err != nil {
		return nil, fmt.Errorf("插入文档失败: %w", err)
	}

	callbacks.OnEnd(ctx, &indexer.CallbackOutput{
		IDs: ids,
	})

	return ids, nil
}

// Retrieve 实现retriever.Retriever接口
func (m *MilvusVectorDB) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	specOpts := retriever.GetImplSpecificOptions(&Option{}, opts...)
	filterExpr := buildFilterExpression(specOpts.filters)
	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfRetriever)
	// callback info on start
	ctx = callbacks.OnStart(ctx, &retriever.CallbackInput{
		Query:          query,
		TopK:           specOpts.TopK,
		Filter:         filterExpr,
		ScoreThreshold: options.ScoreThreshold,
		Extra:          map[string]any{},
	})
	var err error
	defer func() {
		if err != nil {
			callbacks.OnError(ctx, err)
		}
	}()

	vectorsList, err1 := m.Embedding.EmbedStrings(ctx, []string{query})
	if err1 != nil {
		err = fmt.Errorf("查询向量化失败: %w", err1)
		return nil, err
	}
	if len(vectorsList) == 0 || len(vectorsList[0]) == 0 {
		err = fmt.Errorf("查询向量化失败: 向量数据为空")
		return nil, err
	}
	queryVector := vectorsList[0]

	vectors := []entity.Vector{entity.FloatVector(Float64ToFloat32(queryVector))}

	// 构建搜索参数
	annParam := index.NewCustomAnnParam()
	if options.ScoreThreshold != nil {
		annParam.WithRadius(*options.ScoreThreshold)
		annParam.WithRangeFilter(1.0)
	}

	searchOption := milvusclient.NewSearchOption(m.collectionName, specOpts.TopK, vectors).
		WithOutputFields("id", "content", "metadata", "created_at", "updated_at").
		WithAnnParam(annParam).
		WithFilter(filterExpr)

	// 执行搜索
	results, err2 := m.client.Search(ctx, searchOption)
	if err2 != nil {
		err = fmt.Errorf("搜索失败: %w", err2)
		return nil, err
	}

	// 处理搜索结果
	var searchResults []*schema.Document
	for _, result := range results {
		for i := 0; i < result.IDs.Len(); i++ {
			doc, err3 := m.buildDocumentFromResult(result, i)
			if err3 != nil {
				continue // 跳过无法构建的文档
			}

			// 设置搜索分数
			doc.WithScore(float64(result.Scores[i]))
			searchResults = append(searchResults, doc)
		}
	}
	// callback info on end
	callbacks.OnEnd(ctx, &retriever.CallbackOutput{Docs: searchResults})
	return searchResults, nil
}

// GetDocument 获取文档
func (m *MilvusVectorDB) GetDocument(ctx context.Context, docID string) (*schema.Document, error) {
	resultSet, err := m.client.Get(ctx, milvusclient.NewQueryOption(m.collectionName).
		WithIDs(column.NewColumnVarChar("id", []string{docID})).
		WithOutputFields("id", "content", "metadata", "created_at", "updated_at", "vector"))
	if err != nil {
		return nil, fmt.Errorf("查询文档失败: %w", err)
	}

	if resultSet.ResultCount <= 0 {
		return nil, fmt.Errorf("文档未找到: %s", docID)
	}

	doc, err := m.buildDocumentFromResult(resultSet, 0)
	if err != nil {
		return nil, fmt.Errorf("构建文档失败: %w", err)
	}
	return doc, nil
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

// buildDocumentFromResult 从搜索结果构建文档对象
func (m *MilvusVectorDB) buildDocumentFromResult(result milvusclient.ResultSet, index int) (*schema.Document, error) {
	id, err := result.IDs.Get(index)
	if err != nil {
		return nil, fmt.Errorf("获取ID失败: %w", err)
	}

	content, err := getColumnValue(result, "content", index)
	if err != nil {
		return nil, fmt.Errorf("获取content失败: %w", err)
	}

	metadataBytes, err := getColumnValue(result, "metadata", index)
	if err != nil {
		return nil, fmt.Errorf("获取metadata失败: %w", err)
	}

	// 解析metadata
	metadata, err := unmarshalMetadata(metadataBytes.([]byte))
	if err != nil {
		return nil, fmt.Errorf("解析metadata失败: %w", err)
	}

	// 获取向量数据（如果存在）
	var vector []float32
	if vectorValue, err := getColumnValue(result, "vector", index); err == nil {
		if v, ok := vectorValue.([]float32); ok {
			vector = v
		}
	}

	doc := &schema.Document{
		ID:       id.(string),
		Content:  content.(string),
		MetaData: metadata,
	}

	// 设置向量数据
	if len(vector) > 0 {
		doc.WithDenseVector(Float32ToFloat64(vector))
	}

	// 添加DSL时间字段
	dslInfo := make(map[string]any)
	if createdAtValue, err := getColumnValue(result, "created_at", index); err == nil {
		if createdAtInt, ok := createdAtValue.(int64); ok {
			dslInfo["created_at"] = time.Unix(createdAtInt, 0)
		}
	}
	if updatedAtValue, err := getColumnValue(result, "updated_at", index); err == nil {
		if updatedAtInt, ok := updatedAtValue.(int64); ok {
			dslInfo["updated_at"] = time.Unix(updatedAtInt, 0)
		}
	}
	if len(dslInfo) > 0 {
		doc.WithDSLInfo(dslInfo)
	}
	return doc, nil
}

func (m *MilvusVectorDB) GetType() string {
	return "Milvus"
}
