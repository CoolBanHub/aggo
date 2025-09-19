package knowledge

import (
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/retriever"
)

// 搜索模式常量
const (
	// SearchModeVector 向量搜索模式，使用语义相似度搜索
	SearchModeVector = "vector"
	// SearchModeFuzzy 模糊搜索模式，使用文本匹配搜索
	SearchModeFuzzy = "fuzzy"
	// SearchModeHybrid 混合搜索模式，结合向量搜索和模糊搜索
	SearchModeHybrid = "hybrid"
)

type ChunkType string

const (
	ChunkTypeText = "text"
)

// SearchOptions 搜索选项配置
type SearchOptions struct {
	// 返回结果数量限制
	Limit int `json:"limit"`
	// 元数据过滤条件
	Filters map[string]interface{} `json:"filters,omitempty"`
	// 相似度阈值
	Threshold float64 `json:"threshold,omitempty"`

	//搜索模式 向量搜索，模糊搜索，混合搜索
	Mode string

	RetrieverOptions []retriever.Option
}

// LoadOptions 加载选项配置
type LoadOptions struct {
	// 是否重新创建知识库
	Recreate bool `json:"recreate"`
	// 是否使用插入或更新操作
	Upsert bool `json:"upsert"`
	// 是否跳过已存在的文档
	SkipExisting bool `json:"skipExisting"`

	EnableChunking bool `json:"enableChunking"`

	ChunkType string `json:"chunkType"` //切割类型

	TransformerOptions []document.TransformerOption `json:"transformerOption"`
}

// KnowledgeConfig 知识库配置
type KnowledgeConfig struct {
	Storage KnowledgeStorage //存储数据

	StorageTablePrefix string

	Indexer VectorDB

	Retriever retriever.Retriever

	// 默认搜索配置,没有设置SearchOptions的时候使用此配置
	DefaultSearchOptions SearchOptions `json:"defaultSearchOptions"`
	// 默认加载配置
	DefaultLoadOptions LoadOptions `json:"defaultLoadOptions"`

	Transformers map[string]document.Transformer
}
