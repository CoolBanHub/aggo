package vectordb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/knowledge"
	"gorm.io/gorm"
)

// PostgresVectorDB PostgreSQL向量数据库实现
// 使用pgvector扩展进行向量存储和搜索
type PostgresVectorDB struct {
	db                *gorm.DB
	collectionName    string
	vectorDimension   int
	tableNameProvider *TableNameProvider
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

// Drop 删除表
func (p *PostgresVectorDB) Drop() error {
	tableName := p.tableNameProvider.GetVectorTableName()

	dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return p.db.Exec(dropSQL).Error
}

// Insert 插入文档
func (p *PostgresVectorDB) Insert(ctx context.Context, docs []knowledge.Document) error {
	if len(docs) == 0 {
		return nil
	}

	tableName := p.tableNameProvider.GetVectorTableName()

	// 转换为PostgreSQL文档格式
	pgDocs := make([]PostgresVectorDocument, len(docs))
	for i, doc := range docs {
		pgDoc, err := p.documentToPostgresDocument(doc)
		if err != nil {
			return fmt.Errorf("转换文档失败: %w", err)
		}
		pgDocs[i] = *pgDoc
	}

	// 批量插入
	err := p.db.WithContext(ctx).Table(tableName).Create(&pgDocs).Error
	if err != nil {
		return fmt.Errorf("插入文档失败: %w", err)
	}

	return nil
}

// Upsert 插入或更新文档
func (p *PostgresVectorDB) Upsert(ctx context.Context, docs []knowledge.Document) error {
	if len(docs) == 0 {
		return nil
	}

	tableName := p.tableNameProvider.GetVectorTableName()

	for _, doc := range docs {
		pgDoc, err := p.documentToPostgresDocument(doc)
		if err != nil {
			return fmt.Errorf("转换文档失败: %w", err)
		}

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
			return fmt.Errorf("upsert文档失败: %w", err)
		}
	}

	return nil
}

// Search 向量搜索
func (p *PostgresVectorDB) Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}, threshold float64) ([]knowledge.SearchResult, error) {
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
	searchResults := make([]knowledge.SearchResult, len(results))
	for i, result := range results {
		doc, err := p.mapToDocument(result)
		if err != nil {
			return nil, fmt.Errorf("转换搜索结果失败: %w", err)
		}

		similarity, ok := result["similarity"].(float64)
		if !ok {
			similarity = 0
		}

		searchResults[i] = knowledge.SearchResult{
			Document: *doc,
			Score:    similarity,
		}
	}

	return searchResults, nil
}

// GetDocument 获取文档
func (p *PostgresVectorDB) GetDocument(ctx context.Context, docID string) (*knowledge.Document, error) {
	tableName := p.tableNameProvider.GetVectorTableName()

	var pgDoc PostgresVectorDocument
	err := p.db.WithContext(ctx).Table(tableName).Where("id = ?", docID).First(&pgDoc).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("文档未找到: %s", docID)
		}
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}

	return p.postgresDocumentToDocument(&pgDoc)
}

// UpdateDocument 更新文档
func (p *PostgresVectorDB) UpdateDocument(ctx context.Context, doc knowledge.Document) error {
	tableName := p.tableNameProvider.GetVectorTableName()

	pgDoc, err := p.documentToPostgresDocument(doc)
	if err != nil {
		return fmt.Errorf("转换文档失败: %w", err)
	}

	err = p.db.WithContext(ctx).Table(tableName).Where("id = ?", doc.ID).Updates(pgDoc).Error
	if err != nil {
		return fmt.Errorf("更新文档失败: %w", err)
	}

	return nil
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

// DocExists 检查文档是否存在
func (p *PostgresVectorDB) DocExists(ctx context.Context, docID string) (bool, error) {
	tableName := p.tableNameProvider.GetVectorTableName()

	var count int64
	err := p.db.WithContext(ctx).Table(tableName).Where("id = ?", docID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查文档存在性失败: %w", err)
	}

	return count > 0, nil
}

// UpsertAvailable 返回是否支持upsert操作
func (p *PostgresVectorDB) UpsertAvailable() bool {
	return true
}

// Close 关闭数据库连接
func (p *PostgresVectorDB) Close() error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	return sqlDB.Close()
}

// 辅助方法

// documentToPostgresDocument 将知识库文档转换为PostgreSQL文档
func (p *PostgresVectorDB) documentToPostgresDocument(doc knowledge.Document) (*PostgresVectorDocument, error) {
	var metadataJSON string
	if doc.Metadata != nil {
		metadataBytes, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("序列化元数据失败: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	vectorStr := p.vectorToString(doc.Vector)

	return &PostgresVectorDocument{
		ID:        doc.ID,
		Content:   doc.Content,
		Vector:    vectorStr,
		Metadata:  metadataJSON,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}, nil
}

// postgresDocumentToDocument 将PostgreSQL文档转换为知识库文档
func (p *PostgresVectorDB) postgresDocumentToDocument(pgDoc *PostgresVectorDocument) (*knowledge.Document, error) {
	var metadata map[string]interface{}
	if pgDoc.Metadata != "" {
		err := json.Unmarshal([]byte(pgDoc.Metadata), &metadata)
		if err != nil {
			return nil, fmt.Errorf("反序列化元数据失败: %w", err)
		}
	}

	vector, err := p.stringToVector(pgDoc.Vector)
	if err != nil {
		return nil, fmt.Errorf("解析向量失败: %w", err)
	}

	return &knowledge.Document{
		ID:        pgDoc.ID,
		Content:   pgDoc.Content,
		Vector:    vector,
		Metadata:  metadata,
		CreatedAt: pgDoc.CreatedAt,
		UpdatedAt: pgDoc.UpdatedAt,
	}, nil
}

// vectorToString 将float32向量转换为PostgreSQL向量字符串格式
func (p *PostgresVectorDB) vectorToString(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = fmt.Sprintf("%.6f", v)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

// stringToVector 将PostgreSQL向量字符串转换为float32向量
func (p *PostgresVectorDB) stringToVector(vectorStr string) ([]float32, error) {
	// 简单的向量字符串解析
	if vectorStr == "" || vectorStr == "[]" {
		return []float32{}, nil
	}

	// 移除方括号
	vectorStr = strings.Trim(vectorStr, "[]")
	parts := strings.Split(vectorStr, ",")

	vector := make([]float32, len(parts))
	for i, part := range parts {
		var f float64
		_, err := fmt.Sscanf(strings.TrimSpace(part), "%f", &f)
		if err != nil {
			return nil, fmt.Errorf("解析向量元素失败: %w", err)
		}
		vector[i] = float32(f)
	}

	return vector, nil
}

// mapToDocument 将查询结果map转换为Document
func (p *PostgresVectorDB) mapToDocument(result map[string]interface{}) (*knowledge.Document, error) {
	doc := &knowledge.Document{}

	if id, ok := result["id"].(string); ok {
		doc.ID = id
	}

	if content, ok := result["content"].(string); ok {
		doc.Content = content
	}

	if vectorStr, ok := result["vector"].(string); ok {
		vector, err := p.stringToVector(vectorStr)
		if err != nil {
			return nil, fmt.Errorf("解析向量失败: %w", err)
		}
		doc.Vector = vector
	}

	if metadataStr, ok := result["metadata"].(string); ok && metadataStr != "" {
		var metadata map[string]interface{}
		err := json.Unmarshal([]byte(metadataStr), &metadata)
		if err != nil {
			return nil, fmt.Errorf("解析元数据失败: %w", err)
		}
		doc.Metadata = metadata
	}

	if createdAt, ok := result["created_at"].(time.Time); ok {
		doc.CreatedAt = createdAt
	}

	if updatedAt, ok := result["updated_at"].(time.Time); ok {
		doc.UpdatedAt = updatedAt
	}

	return doc, nil
}
