package knowledge

import (
	"context"
)

// KnowledgeBase 知识库接口，定义所有知识库实现的契约
type KnowledgeBase interface {

	// Load 加载文档到知识库
	Load(ctx context.Context, options LoadOptions) error

	// Search 搜索匹配查询的文档
	Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error)

	// AddDocuments 批量添加文档到知识库
	AddDocuments(ctx context.Context, docs []Document) error

	// AddDocument 添加单个文档到知识库
	AddDocument(ctx context.Context, doc Document) error

	// UpdateDocument 更新文档
	UpdateDocument(ctx context.Context, doc Document) error

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, docID string) error

	// GetDocument 获取文档
	GetDocument(ctx context.Context, docID string) (*Document, error)

	// Exists 检查知识库是否存在
	Exists() bool

	// Delete 删除知识库
	Delete() error

	// GetValidFilters 获取此知识库的有效元数据过滤器
	GetValidFilters() map[string]bool

	// GetConfig 获取知识库配置
	GetConfig() *KnowledgeConfig

	// UpdateConfig 更新知识库配置
	UpdateConfig(config *KnowledgeConfig)

	// Close 关闭知识库连接
	Close() error
}

// VectorDB 向量数据库接口，定义向量数据库实现的契约
type VectorDB interface {
	// Insert 向向量数据库插入文档
	Insert(ctx context.Context, docs []Document) error

	// Upsert 向向量数据库插入或更新文档
	Upsert(ctx context.Context, docs []Document) error

	// Search 在向量数据库中搜索相似文档
	Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}, sort float64) ([]SearchResult, error)

	// DocExists 检查文档是否存在于向量数据库中
	DocExists(ctx context.Context, docID string) (bool, error)

	// GetDocument 获取文档
	GetDocument(ctx context.Context, docID string) (*Document, error)

	// UpdateDocument 更新文档
	UpdateDocument(ctx context.Context, doc Document) error

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, docID string) error

	// Exists 检查集合/表是否存在
	Exists() bool

	// Create 创建集合/表
	Create() error

	// Drop 删除集合/表
	Drop() error

	// UpsertAvailable 返回向量数据库是否支持upsert操作
	UpsertAvailable() bool

	// Close 关闭数据库连接
	Close() error
}

// DocumentReader 文档读取器接口，定义从各种源读取文档的契约
type DocumentReader interface {
	// ReadDocuments 从源读取文档
	ReadDocuments(ctx context.Context) ([]Document, error)
}

// ChunkingStrategy 文档分块策略接口
type ChunkingStrategy interface {
	// Chunk 将文档分割成更小的块
	Chunk(doc Document) ([]Chunk, error)

	// GetChunkSize 获取分块大小
	GetChunkSize() int

	// GetChunkOverlap 获取分块重叠大小
	GetChunkOverlap() int

	// SetChunkSize 设置分块大小
	SetChunkSize(size int)

	// SetChunkOverlap 设置分块重叠大小
	SetChunkOverlap(overlap int)
}

// KnowledgeStorage 知识库存储接口
type KnowledgeStorage interface {
	AutoMigrate() error

	//SetTablePrefix 设置表前缀
	SetTablePrefix(prefix string)

	// SaveDocument 保存文档
	SaveDocument(ctx context.Context, doc *Document) error

	// GetDocument 获取文档
	GetDocument(ctx context.Context, docID string) (*Document, error)

	// UpdateDocument 更新文档
	UpdateDocument(ctx context.Context, doc *Document) error

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, docID string) error

	// ListDocuments 列出文档
	ListDocuments(ctx context.Context, limit int, offset int) ([]*Document, error)

	// SearchDocuments 搜索文档
	SearchDocuments(ctx context.Context, query string, limit int) ([]*Document, error)

	// Close 关闭存储连接
	Close() error
}
