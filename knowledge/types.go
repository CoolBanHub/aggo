package knowledge

import (
	"time"

	"github.com/cloudwego/eino/components/embedding"
)

// Document 文档结构，表示知识库中的一个文档
type Document struct {
	// 文档唯一标识符
	ID string `json:"id"`
	// 文档内容
	Content string `json:"content"`
	// 文档元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// 向量表示（用于语义搜索）
	Vector []float32 `json:"vector,omitempty"`
	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// 更新时间
	UpdatedAt time.Time `json:"updatedAt"`
}

// Chunk 文档分块结构
type Chunk struct {
	// 分块唯一标识符
	ID string `json:"id"`
	// 原文档ID
	DocumentID string `json:"documentId"`
	// 分块内容
	Content string `json:"content"`
	// 分块元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// 向量表示
	Vector []float32 `json:"vector,omitempty"`
	// 分块索引（在原文档中的位置）
	Index int `json:"index"`
	// 分块开始位置
	StartOffset int `json:"startOffset"`
	// 分块结束位置
	EndOffset int `json:"endOffset"`
}

// SearchOptions 搜索选项配置
type SearchOptions struct {
	// 返回结果数量限制
	Limit int `json:"limit"`
	// 元数据过滤条件
	Filters map[string]interface{} `json:"filters,omitempty"`
	// 相似度阈值
	Threshold float64 `json:"threshold,omitempty"`
}

// LoadOptions 加载选项配置
type LoadOptions struct {
	// 是否重新创建知识库
	Recreate bool `json:"recreate"`
	// 是否使用插入或更新操作
	Upsert bool `json:"upsert"`
	// 是否跳过已存在的文档
	SkipExisting bool `json:"skip_existing"`
	// 是否启用文档分块
	EnableChunking bool `json:"enable_chunking"`
	// 分块大小（字符数）
	ChunkSize int `json:"chunk_size"`
	// 分块重叠大小（字符数）
	ChunkOverlap int `json:"chunk_overlap"`
}

// SearchResult 搜索结果
type SearchResult struct {
	// 文档信息
	Document Document `json:"document"`
	// 相似度得分
	Score float64 `json:"score"`
	// 匹配的分块（如果启用了分块）
	Chunk *Chunk `json:"chunk,omitempty"`
}

// KnowledgeConfig 知识库配置
type KnowledgeConfig struct {
	Storage KnowledgeStorage //存储数据

	StorageTablePrefix string

	VectorDB VectorDB //存储向量数据
	// 嵌入模型
	Em embedding.Embedder
	// 默认搜索配置
	DefaultSearchOptions SearchOptions `json:"defaultSearchOptions"`
	// 默认加载配置
	DefaultLoadOptions LoadOptions `json:"defaultLoadOptions"`
}
