package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
	"gorm.io/gorm"
)

type Postgres struct {
	db              *gorm.DB
	collectionName  string
	vectorDimension int
	Embedding       embedding.Embedder
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

// PostgresConfig PostgreSQL配置
type PostgresConfig struct {
	Client          *gorm.DB
	CollectionName  string // 集合名称
	VectorDimension int    // 向量维度
	Embedding       embedding.Embedder
}

// NewPostgres 使用现有GORM实例创建PostgreSQL向量数据库
func NewPostgres(config PostgresConfig) (*Postgres, error) {
	if config.Client == nil {
		return nil, errors.New("postgres client不能为空")
	}
	if config.Embedding == nil {
		return nil, errors.New("embedding组件不能为空")
	}

	if config.VectorDimension <= 0 {
		config.VectorDimension = 1536 // 默认OpenAI embedding维度
	}

	if config.CollectionName == "" {
		config.CollectionName = "aggo_knowledge_vectors"
	}

	vectorDB := &Postgres{
		db:              config.Client,
		collectionName:  config.CollectionName,
		vectorDimension: config.VectorDimension,
		Embedding:       config.Embedding,
	}

	// 初始化表
	if err := vectorDB.initTable(); err != nil {
		return nil, fmt.Errorf("初始化表失败: %w", err)
	}

	return vectorDB, nil
}

// initTable 初始化PostgreSQL表
func (p *Postgres) initTable() error {
	// 检查表是否存在
	if !p.Exists() {
		// 表不存在，创建表
		return p.Create()
	}
	// 表已存在，无需创建
	return nil
}

// Create 创建向量表
func (p *Postgres) Create() error {
	tableName := p.collectionName

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
		slog.Errorf("创建向量索引失败: %v", err)
	}

	return nil
}

// Exists 检查表是否存在
func (p *Postgres) Exists() bool {
	tableName := p.collectionName

	var count int64
	err := p.db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_name = ?", tableName).Scan(&count).Error
	if err != nil {
		return false
	}

	return count > 0
}

// Store 存储文档（实现indexer.Indexer接口）
func (p *Postgres) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) ([]string, error) {
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

	tableName := p.collectionName
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

// Search 向量搜索
func (p *Postgres) Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}, threshold float64) ([]*schema.Document, error) {
	tableName := p.collectionName
	// 将float32向量转换为字符串格式
	vectorStr := utils.VectorToString(queryVector)

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

// Retrieve 实现retriever.Retriever接口
func (p *Postgres) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	options := retriever.GetCommonOptions(nil, opts...)
	specOpts := retriever.GetImplSpecificOptions(&Option{}, opts...)
	if specOpts.TopK == 0 {
		specOpts.TopK = 10
	}

	if options.ScoreThreshold == nil {
		options.ScoreThreshold = utils.ValueToPtr(0.1)
	}
	ctx = callbacks.EnsureRunInfo(ctx, p.GetType(), components.ComponentOfRetriever)
	// callback info on start
	ctx = callbacks.OnStart(ctx, &retriever.CallbackInput{
		Query:          query,
		TopK:           specOpts.TopK,
		Filter:         fmt.Sprintf("%v", specOpts.Filters),
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
		err = fmt.Errorf("查询向量化失败: %w", err1)
		return nil, err
	}
	if len(vectorsList) == 0 || len(vectorsList[0]) == 0 {
		err = errors.New("查询向量化失败: 向量数据为空")
		return nil, err
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

	searchResults, err2 := p.Search(ctx, queryVectorFloat32, limit, specOpts.Filters, threshold)
	if err2 != nil {
		err = err2
		return nil, err
	}

	// callback info on end
	callbacks.OnEnd(ctx, &retriever.CallbackOutput{Docs: searchResults})
	return searchResults, nil
}

// GetType 返回组件类型
func (p *Postgres) GetType() string {
	return "Postgres"
}
