package vectordb

import (
	"context"
	"fmt"
	"sync"

	"github.com/CoolBanHub/aggo/knowledge"
)

// MockVectorDB Mock向量数据库实现，用于测试和演示
type MockVectorDB struct {
	documents []knowledge.Document
	mu        sync.RWMutex
}

// NewMockVectorDB 创建Mock向量数据库
func NewMockVectorDB() *MockVectorDB {
	return &MockVectorDB{
		documents: make([]knowledge.Document, 0),
	}
}

// Insert 插入文档
func (m *MockVectorDB) Insert(ctx context.Context, docs []knowledge.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents = append(m.documents, docs...)
	return nil
}

// Upsert 插入或更新文档
func (m *MockVectorDB) Upsert(ctx context.Context, docs []knowledge.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, newDoc := range docs {
		found := false
		for i, existingDoc := range m.documents {
			if existingDoc.ID == newDoc.ID {
				m.documents[i] = newDoc
				found = true
				break
			}
		}
		if !found {
			m.documents = append(m.documents, newDoc)
		}
	}
	return nil
}

// Search 向量搜索
func (m *MockVectorDB) Search(ctx context.Context, queryVector []float32, limit int, filters map[string]interface{}) ([]knowledge.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]knowledge.SearchResult, 0)
	count := 0

	for _, doc := range m.documents {
		if count >= limit {
			break
		}

		if passesFilter(doc, filters) {
			result := knowledge.SearchResult{
				Document: doc,
				Score:    0.8 - float32(count)*0.1, // Mock相似度得分
			}
			results = append(results, result)
			count++
		}
	}

	return results, nil
}

// DocExists 检查文档是否存在
func (m *MockVectorDB) DocExists(ctx context.Context, docID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, doc := range m.documents {
		if doc.ID == docID {
			return true, nil
		}
	}
	return false, nil
}

// GetDocument 获取文档
func (m *MockVectorDB) GetDocument(ctx context.Context, docID string) (*knowledge.Document, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, doc := range m.documents {
		if doc.ID == docID {
			return &doc, nil
		}
	}
	return nil, fmt.Errorf("文档未找到: %s", docID)
}

// UpdateDocument 更新文档
func (m *MockVectorDB) UpdateDocument(ctx context.Context, doc knowledge.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existingDoc := range m.documents {
		if existingDoc.ID == doc.ID {
			m.documents[i] = doc
			return nil
		}
	}
	return fmt.Errorf("文档未找到: %s", doc.ID)
}

// DeleteDocument 删除文档
func (m *MockVectorDB) DeleteDocument(ctx context.Context, docID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, doc := range m.documents {
		if doc.ID == docID {
			m.documents = append(m.documents[:i], m.documents[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("文档未找到: %s", docID)
}

// Exists 检查数据库是否存在
func (m *MockVectorDB) Exists() bool {
	return true
}

// Create 创建数据库
func (m *MockVectorDB) Create() error {
	return nil
}

// Drop 删除数据库
func (m *MockVectorDB) Drop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents = nil
	return nil
}

// UpsertAvailable 是否支持Upsert
func (m *MockVectorDB) UpsertAvailable() bool {
	return true
}

// Close 关闭连接
func (m *MockVectorDB) Close() error {
	return nil
}

// passesFilter 检查文档是否通过过滤器
func passesFilter(doc knowledge.Document, filters map[string]interface{}) bool {
	if filters == nil {
		return true
	}

	for key, value := range filters {
		if docValue, exists := doc.Metadata[key]; exists {
			if docValue != value {
				return false
			}
		} else {
			return false
		}
	}
	return true
}
