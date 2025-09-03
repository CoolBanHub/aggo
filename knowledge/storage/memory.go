package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/CoolBanHub/aggo/knowledge"
)

// MemoryStorage 内存存储实现，用于测试和演示
type MemoryStorage struct {
	documents map[string]*knowledge.Document
	mu        sync.RWMutex
}

// NewMemoryStorage 创建内存存储实例
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		documents: make(map[string]*knowledge.Document),
	}
}

func (gs *MemoryStorage) SetTablePrefix(prefix string) {
}

// SaveDocument 保存文档
func (ms *MemoryStorage) SaveDocument(ctx context.Context, doc *knowledge.Document) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}
	doc.UpdatedAt = time.Now()

	ms.documents[doc.ID] = doc
	return nil
}

// GetDocument 获取文档
func (ms *MemoryStorage) GetDocument(ctx context.Context, docID string) (*knowledge.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	doc, exists := ms.documents[docID]
	if !exists {
		return nil, fmt.Errorf("文档未找到: %s", docID)
	}
	return doc, nil
}

// UpdateDocument 更新文档
func (ms *MemoryStorage) UpdateDocument(ctx context.Context, doc *knowledge.Document) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.documents[doc.ID]; !exists {
		return fmt.Errorf("文档未找到: %s", doc.ID)
	}

	doc.UpdatedAt = time.Now()
	ms.documents[doc.ID] = doc
	return nil
}

// DeleteDocument 删除文档
func (ms *MemoryStorage) DeleteDocument(ctx context.Context, docID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.documents[docID]; !exists {
		return fmt.Errorf("文档未找到: %s", docID)
	}

	delete(ms.documents, docID)
	return nil
}

// ListDocuments 列出文档
func (ms *MemoryStorage) ListDocuments(ctx context.Context, limit int, offset int) ([]*knowledge.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	docs := make([]*knowledge.Document, 0, len(ms.documents))
	i := 0
	for _, doc := range ms.documents {
		if i >= offset && len(docs) < limit {
			docs = append(docs, doc)
		}
		i++
	}
	return docs, nil
}

// SearchDocuments 搜索文档
func (ms *MemoryStorage) SearchDocuments(ctx context.Context, query string, limit int) ([]*knowledge.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	docs := make([]*knowledge.Document, 0)
	count := 0
	for _, doc := range ms.documents {
		if count >= limit {
			break
		}
		// 简单的文本包含检查
		if strings.Contains(strings.ToLower(doc.Content), strings.ToLower(query)) {
			docs = append(docs, doc)
			count++
		}
	}
	return docs, nil
}

// Close 关闭存储连接
func (ms *MemoryStorage) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.documents = nil
	return nil
}
