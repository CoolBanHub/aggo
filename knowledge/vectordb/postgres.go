package vectordb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// PostgresVectorDB PostgreSQL向量数据库实现
// 使用pgvector扩展进行向量存储和搜索
type PostgresVectorDB struct {
	db                *gorm.DB
	collectionName    string
	vectorDimension   int
	tableNameProvider *TableNameProvider
	Embedding         embedding.Embedder
}

// PostgresVectorDocument PostgreSQL向量文档模型
type PostgresVectorDocument struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	Content   string    `gorm:"column:content;type:text" json:"content"`
	Vector    string    `gorm:"column:vector;type:vector" json:"vector"` // pgvector类型
	Metadata  string    `gorm:"column:metadata;type:text" json:"metadata"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableNameProvider 表名提供器
type TableNameProvider struct {
	collectionName string
}

func NewTableNameProvider(collectionName string) *TableNameProvider {
	return &TableNameProvider{collectionName: collectionName}
}

func (t *TableNameProvider) GetVectorTableName() string {
	return t.collectionName
}

// PostgresConfig PostgreSQL配置
type PostgresConfig struct {
	Client          *gorm.DB
	CollectionName  string // 集合名称
	VectorDimension int    // 向量维度
	Embedding       embedding.Embedder
}

// NewPostgresVectorDB 使用现有GORM实例创建PostgreSQL向量数据库
func NewPostgresVectorDB(config PostgresConfig) (*PostgresVectorDB, error) {

	if config.VectorDimension <= 0 {
		config.VectorDimension = 1536 // 默认OpenAI embedding维度
	}

	if config.CollectionName == "" {
		config.CollectionName = "aggo_knowledge_vectors"
	}

	tableNameProvider := NewTableNameProvider(config.CollectionName)

	vectorDB := &PostgresVectorDB{
		db:                config.Client,
		collectionName:    config.CollectionName,
		vectorDimension:   config.VectorDimension,
		tableNameProvider: tableNameProvider,
		Embedding:         config.Embedding,
	}

	// 初始化表
	if err := vectorDB.initTable(); err != nil {
		return nil, fmt.Errorf("初始化表失败: %w", err)
	}

	return vectorDB, nil
}

// initTable 初始化PostgreSQL表
func (p *PostgresVectorDB) initTable() error {
	// 检查表是否存在
	if !p.Exists() {
		// 表不存在，创建表
		return p.Create()
	}
	// 表已存在，无需创建
	return nil
}

// Create 创建向量表
func (p *PostgresVectorDB) Create() error {
	tableName := p.tableNameProvider.GetVectorTableName()

	// 检查pgvector扩展是否已安装
	var count int64
	err := p.db.Raw("SELECT COUNT(*) FROM pg_extension WHERE extname = 'vector'").Scan(&count).Error
	if err != nil {
		return fmt.Errorf("检查pgvector扩展失败: %w", err)
	}

	if count == 0 {
		// 尝试安装pgvector扩展
		err = p.db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
		if err != nil {
			return fmt.Errorf("安装pgvector扩展失败: %w，请确保已安装pgvector扩展", err)
		}
	}

	// 创建表
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			content TEXT NOT NULL,
			vector vector(%d) NOT NULL,
			metadata TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`, tableName, p.vectorDimension)

	err = p.db.Exec(createTableSQL).Error
	if err != nil {
		return fmt.Errorf("创建向量表失败: %w", err)
	}

	// 创建向量索引以提高查询性能
	indexSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_vector_idx ON %s USING ivfflat (vector vector_cosine_ops) WITH (lists = 100)`,
		tableName, tableName)

	err = p.db.Exec(indexSQL).Error
	if err != nil {
		// 索引创建失败不是致命错误，记录警告即可
		fmt.Printf("警告: 创建向量索引失败: %v\n", err)
	}

	return nil
}

// Exists 检查表是否存在
func (p *PostgresVectorDB) Exists() bool {
	tableName := p.tableNameProvider.GetVectorTableName()

	var count int64
	err := p.db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", tableName).Scan(&count).Error
	if err != nil {
		return false
	}

	return count > 0
}

// Store 存储文档（实现indexer.Indexer接口）
func (p *PostgresVectorDB) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
	ctx = callbacks.EnsureRunInfo(ctx, p.GetType(), components.ComponentOfIndexer)
	ctx = callbacks.OnStart(ctx, &indexer.CallbackInput{Docs: docs})

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

	tableName := p.tableNameProvider.GetVectorTableName()
	ids := make([]string, len(docs))

	// 使用PostgreSQL的ON CONFLICT进行批量Upsert
	for i, doc := range docs {
		pgDoc, err1 := p.documentToPostgresDocument(ctx, *doc)
		if err1 != nil {
			err = err1
			return nil, fmt.Errorf("转换文档失败: %w", err)
		}
		ids[i] = doc.ID

		// PostgreSQL的ON CONFLICT处理
		upsertSQL := fmt.Sprintf(`
			INSERT INTO %s (id, content, vector, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				vector = EXCLUDED.vector,
				metadata = EXCLUDED.metadata,
				updated_at = EXCLUDED.updated_at
		`, tableName)

		err = p.db.WithContext(ctx).Exec(upsertSQL,
			pgDoc.ID, pgDoc.Content, pgDoc.Vector, pgDoc.Metadata,
			pgDoc.CreatedAt, pgDoc.UpdatedAt).Error
		if err != nil {
			return nil, fmt.Errorf("upsert文档失败: %w", err)
		}
	}

	callbacks.OnEnd(ctx, &indexer.CallbackOutput{IDs: ids})
	return ids, nil
}

// Upsert 插入或更新文档（保持兼容性）
func (p *PostgresVectorDB) Upsert(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	// 转换为指针切片
	docPtrs := make([]*schema.Document, len(docs))
	for i := range docs {
		docPtrs[i] = &docs[i]
	}

	_, err := p.Store(ctx, docPtrs)
	return err
}

// Search 向量搜索
func (p *PostgresVectorDB) Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}, threshold float64) ([]*schema.Document, error) {
	tableName := p.tableNameProvider.GetVectorTableName()

	// 将float32向量转换为字符串格式
	vectorStr := p.vectorToString(queryVector)

	// 构建查询SQL，使用余弦相似度
	query := p.db.WithContext(ctx).Table(tableName)

	// 添加相似度计算和过滤
	selectSQL := fmt.Sprintf("*, (1 - (vector <=> '%s')) as similarity", vectorStr)
	query = query.Select(selectSQL)

	// 添加相似度阈值过滤
	if threshold > 0 {
		query = query.Where(fmt.Sprintf("(1 - (vector <=> '%s')) >= ?", vectorStr), threshold)
	}

	// 添加元数据过滤
	if len(filters) > 0 {
		for key, value := range filters {
			// 简单的元数据JSON查询，可以根据需要优化
			query = query.Where("metadata::jsonb ->> ? = ?", key, fmt.Sprintf("%v", value))
		}
	}

	// 按相似度排序并限制结果数量
	query = query.Order(fmt.Sprintf("(vector <=> '%s') ASC", vectorStr)).Limit(limit)

	// 执行查询
	var results []map[string]interface{}
	err := query.Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}

	// 转换结果
	searchResults := make([]*schema.Document, len(results))
	for i, result := range results {
		doc, err := p.mapToDocument(result)
		if err != nil {
			return nil, fmt.Errorf("转换搜索结果失败: %w", err)
		}

		similarity, ok := result["similarity"].(float64)
		if !ok {
			similarity = 0
		}

		// 设置搜索分数
		doc.WithScore(similarity)
		searchResults[i] = doc
	}

	return searchResults, nil
}

// GetDocument 获取文档
func (p *PostgresVectorDB) GetDocument(ctx context.Context, docID string) (*schema.Document, error) {
	tableName := p.tableNameProvider.GetVectorTableName()

	var pgDoc PostgresVectorDocument
	err := p.db.WithContext(ctx).Table(tableName).Where("id = ?", docID).First(&pgDoc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("文档未找到: %s", docID)
		}
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}

	return p.postgresDocumentToDocument(&pgDoc)
}

// DeleteDocument 删除文档
func (p *PostgresVectorDB) DeleteDocument(ctx context.Context, docID string) error {
	tableName := p.tableNameProvider.GetVectorTableName()

	result := p.db.WithContext(ctx).Table(tableName).Where("id = ?", docID).Delete(&PostgresVectorDocument{})
	if result.Error != nil {
		return fmt.Errorf("删除文档失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("文档未找到: %s", docID)
	}

	return nil
}

// 辅助方法

// documentToPostgresDocument 将知识库文档转换为PostgreSQL文档
func (p *PostgresVectorDB) documentToPostgresDocument(ctx context.Context, doc schema.Document) (*PostgresVectorDocument, error) {
	// 序列化metadata
	var metadataJSON string
	if doc.MetaData != nil {
		if metadataBytes, err := json.Marshal(doc.MetaData); err != nil {
			return nil, fmt.Errorf("序列化元数据失败: %w", err)
		} else {
			metadataJSON = string(metadataBytes)
		}
	}

	// 生成向量
	vectorData, err := p.Embedding.EmbedStrings(ctx, []string{doc.Content})
	if err != nil {
		return nil, fmt.Errorf("向量化失败: %w", err)
	}
	if len(vectorData) == 0 {
		return nil, fmt.Errorf("向量化失败: 向量数据为空")
	}

	// 处理时间字段
	now := time.Now()
	dslInfo := doc.DSLInfo()
	createdAt, _ := dslInfo["created_at"].(time.Time)
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt, _ := dslInfo["updated_at"].(time.Time)
	if updatedAt.IsZero() {
		updatedAt = now
	}

	return &PostgresVectorDocument{
		ID:        doc.ID,
		Content:   doc.Content,
		Vector:    p.vector64ToString(vectorData[0]),
		Metadata:  metadataJSON,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// postgresDocumentToDocument 将PostgreSQL文档转换为知识库文档
func (p *PostgresVectorDB) postgresDocumentToDocument(pgDoc *PostgresVectorDocument) (*schema.Document, error) {
	// 反序列化metadata
	var metadata map[string]interface{}
	if pgDoc.Metadata != "" {
		if err := json.Unmarshal([]byte(pgDoc.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("反序列化元数据失败: %w", err)
		}
	}

	// 解析向量
	vector, err := p.stringToVector(pgDoc.Vector)
	if err != nil {
		return nil, fmt.Errorf("解析向量失败: %w", err)
	}

	// 创建文档
	doc := &schema.Document{
		ID:       pgDoc.ID,
		Content:  pgDoc.Content,
		MetaData: metadata,
	}

	// 设置向量和时间信息
	if len(vector) > 0 {
		doc.WithDenseVector(Float32ToFloat64(vector))
	}
	doc.WithDSLInfo(map[string]any{
		"created_at": pgDoc.CreatedAt,
		"updated_at": pgDoc.UpdatedAt,
	})

	return doc, nil
}

// mapToDocument 将查询结果map转换为Document
func (p *PostgresVectorDB) mapToDocument(result map[string]interface{}) (*schema.Document, error) {
	doc := &schema.Document{
		ID:       result["id"].(string),
		Content:  result["content"].(string),
		MetaData: make(map[string]interface{}),
	}

	// 处理向量
	if vectorStr, ok := result["vector"].(string); ok {
		if vector, err := p.stringToVector(vectorStr); err == nil && len(vector) > 0 {
			doc.WithDenseVector(Float32ToFloat64(vector))
		}
	}

	// 处理metadata
	if metadataStr, ok := result["metadata"].(string); ok && metadataStr != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err == nil {
			doc.MetaData = metadata
		}
	}

	// 处理时间字段
	dslInfo := make(map[string]any)
	if createdAt, ok := result["created_at"].(time.Time); ok {
		dslInfo["created_at"] = createdAt
	}
	if updatedAt, ok := result["updated_at"].(time.Time); ok {
		dslInfo["updated_at"] = updatedAt
	}
	if len(dslInfo) > 0 {
		doc.WithDSLInfo(dslInfo)
	}

	return doc, nil
}

// Retrieve 实现retriever.Retriever接口
func (p *PostgresVectorDB) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	specOpts := retriever.GetImplSpecificOptions(&Option{}, opts...)

	ctx = callbacks.EnsureRunInfo(ctx, p.GetType(), components.ComponentOfRetriever)
	// callback info on start
	ctx = callbacks.OnStart(ctx, &retriever.CallbackInput{
		Query:          query,
		TopK:           specOpts.TopK,
		Filter:         fmt.Sprintf("%v", specOpts.filters),
		ScoreThreshold: options.ScoreThreshold,
		Extra:          map[string]any{},
	})
	var err error
	defer func() {
		if err != nil {
			callbacks.OnError(ctx, err)
		}
	}()

	// 使用embedding生成查询向量
	vectorsList, err1 := p.Embedding.EmbedStrings(ctx, []string{query})
	if err1 != nil {
		err = err1
		return nil, err
	}
	if len(vectorsList) == 0 {
		return nil, nil
	}
	queryVector := vectorsList[0]

	// 转换float64向量为float32
	queryVectorFloat32 := make([]float32, len(queryVector))
	for i, v := range queryVector {
		queryVectorFloat32[i] = float32(v)
	}

	// 调用Search方法进行向量搜索
	threshold := 0.0
	if options.ScoreThreshold != nil {
		threshold = *options.ScoreThreshold
	}

	limit := 10 // 默认限制
	if specOpts.TopK > 0 {
		limit = specOpts.TopK
	}

	searchResults, err2 := p.Search(ctx, queryVectorFloat32, limit, specOpts.filters, threshold)
	if err2 != nil {
		err = err2
		return nil, err
	}

	// callback info on end
	callbacks.OnEnd(ctx, &retriever.CallbackOutput{Docs: searchResults})
	return searchResults, nil
}

// GetType 返回组件类型
func (p *PostgresVectorDB) GetType() string {
	return "PostgresVector"
}
