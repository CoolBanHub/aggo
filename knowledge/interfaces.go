package knowledge

import (
	"context"

	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
)

// VectorDB 向量数据库接口，定义向量数据库实现的契约
type VectorDB interface {
	indexer.Indexer

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, docID string) error

	// GetDocument 获取文档
	GetDocument(ctx context.Context, docID string) (*schema.Document, error)
}

// KnowledgeStorage 知识库存储接口
type KnowledgeStorage interface {
	AutoMigrate() error

	//SetTablePrefix 设置表前缀
	SetTablePrefix(prefix string)

	Store(ctx context.Context, docs []*schema.Document) (err error) // invoke

	// GetDocument 获取文档
	GetDocument(ctx context.Context, docID string) (*schema.Document, error)

	// UpdateDocument 更新文档
	UpdateDocument(ctx context.Context, doc *schema.Document) error

	// DeleteDocument 删除文档
	DeleteDocument(ctx context.Context, docID string) error

	// ListDocuments 列出文档
	ListDocuments(ctx context.Context, limit int, offset int) ([]*schema.Document, error)

	// SearchDocuments 搜索文档
	SearchDocuments(ctx context.Context, query string, limit int) ([]*schema.Document, error)

	// Close 关闭存储连接
	Close() error
}
