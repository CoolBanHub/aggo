package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

// MemoryStorage 内存存储实现，用于测试和演示
type MemoryStorage struct {
	documents map[string]*schema.Document
	mu        sync.RWMutex
}

// NewMemoryStorage 创建内存存储实例
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		documents: make(map[string]*schema.Document),
	}
}

func (gs *MemoryStorage) SetTablePrefix(prefix string) {
}

func (gs *MemoryStorage) AutoMigrate() error {
	return nil
}

// Store 保存文档数组
func (ms *MemoryStorage) Store(ctx context.Context, docs []*schema.Document) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()
	for _, doc := range docs {
		ms.updateDocumentTime(doc, now)
		ms.documents[doc.ID] = doc
	}
	return nil
}

// GetDocument 获取文档
func (ms *MemoryStorage) GetDocument(ctx context.Context, docID string) (*schema.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	doc, exists := ms.documents[docID]
	if !exists {
		return nil, fmt.Errorf("文档未找到: %s", docID)
	}
	return doc, nil
}

// UpdateDocument 更新文档
func (ms *MemoryStorage) UpdateDocument(ctx context.Context, doc *schema.Document) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if existingDoc, exists := ms.documents[doc.ID]; !exists {
		return fmt.Errorf("文档未找到: %s", doc.ID)
	} else {
		// 保持原有的创建时间
		existingDSL := existingDoc.DSLInfo()
		if createdAt, ok := existingDSL["created_at"].(time.Time); ok {
			doc.WithDSLInfo(map[string]any{
				"created_at": createdAt,
				"updated_at": time.Now(),
			})
		} else {
			ms.updateDocumentTime(doc, time.Now())
		}
	}

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
func (ms *MemoryStorage) ListDocuments(ctx context.Context, limit int, offset int) ([]*schema.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	allDocs := make([]*schema.Document, 0, len(ms.documents))
	for _, doc := range ms.documents {
		allDocs = append(allDocs, doc)
	}

	// 应用分页
	start := offset
	if start >= len(allDocs) {
		return []*schema.Document{}, nil
	}

	end := start + limit
	if limit <= 0 || end > len(allDocs) {
		end = len(allDocs)
	}

	return allDocs[start:end], nil
}

// SearchDocuments 搜索文档
func (ms *MemoryStorage) SearchDocuments(ctx context.Context, query string, limit int) ([]*schema.Document, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	queryLower := strings.ToLower(query)
	docs := make([]*schema.Document, 0)

	for _, doc := range ms.documents {
		if limit > 0 && len(docs) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(doc.Content), queryLower) {
			docs = append(docs, doc)
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

// updateDocumentTime 更新文档的DSL时间信息
func (ms *MemoryStorage) updateDocumentTime(doc *schema.Document, now time.Time) {
	dslInfo := doc.DSLInfo()

	// 如果已经有创建时间，保持不变；否则设置为当前时间
	createdAt := now
	if existing, ok := dslInfo["created_at"].(time.Time); ok {
		createdAt = existing
	}

	doc.WithDSLInfo(map[string]any{
		"created_at": createdAt,
		"updated_at": now,
	})
}
