package tools

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/loader/url"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func GetKnowledgeTools(indexer indexer.Indexer, retriever retriever.Retriever, retrieverOptions *retriever.Options) []tool.BaseTool {
	return []tool.BaseTool{
		NewLoadDocumentsTool(indexer),
		NewSearchDocumentsTool(retriever, retrieverOptions),
	}
}

// LoadDocumentsTool 文档加载工具
// 提供将文档加载到知识库的功能
type LoadDocumentsTool struct {
	indexer indexer.Indexer
}

// SearchDocumentsTool 文档搜索工具
// 提供在知识库中搜索文档的功能
type SearchDocumentsTool struct {
	retriever retriever.Retriever
	options   *retriever.Options
}

// LoadDocumentsParams 加载文档的参数
type LoadDocumentsParams struct {
	// 文档来源类型：file, url
	SourceType LoadDocumentSourceType `json:"sourceType" jsonschema:"description=文档来源类型,required,enum=file,enum=url"`

	Uri string `json:"uri"`
}

// SearchParams 搜索参数
type SearchParams struct {
	Query string `json:"query" jsonschema:"description=搜索查询,required"`
}

// NewLoadDocumentsTool 创建文档加载工具实例
func NewLoadDocumentsTool(indexer indexer.Indexer) tool.InvokableTool {
	this := &LoadDocumentsTool{
		indexer: indexer,
	}
	name := "load_documents"
	desc := "将文档加载到知识库。支持多种文档来源（文本文件、URL、目录、内存），提供文档分块、重建知识库等功能。"
	t, _ := utils.InferTool(name, desc, this.loadDocuments)
	return t
}

// NewSearchDocumentsTool 创建文档搜索工具实例
func NewSearchDocumentsTool(retriever retriever.Retriever, options *retriever.Options) tool.InvokableTool {
	this := &SearchDocumentsTool{
		retriever: retriever,
		options:   options,
	}
	name := "search_documents"
	desc := "在知识库中搜索文档。使用向量相似度匹配，支持设置搜索限制、相似度阈值和元数据过滤。"
	t, _ := utils.InferTool(name, desc, this.searchDocuments)
	return t
}

type LoadDocumentSourceType string

func (l LoadDocumentSourceType) String() string {
	return string(l)
}

const (
	LoadDocumentSourceTypeFile LoadDocumentSourceType = "file"
	LoadDocumentSourceTypeUrl  LoadDocumentSourceType = "url"
)

// loadDocuments 加载文档到知识库
func (t *LoadDocumentsTool) loadDocuments(ctx context.Context, params LoadDocumentsParams) (interface{}, error) {
	if t.indexer == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	// 创建文档读取器
	var err error
	var loader document.Loader
	switch params.SourceType {
	case LoadDocumentSourceTypeFile:
		fileLoader, err := file.NewFileLoader(ctx, &file.FileLoaderConfig{})
		if err != nil {
			return nil, err
		}
		loader = fileLoader
	case LoadDocumentSourceTypeUrl:
		urlLoader, err := url.NewLoader(ctx, &url.LoaderConfig{})
		if err != nil {
			return nil, err
		}
		loader = urlLoader

	default:
		return nil, fmt.Errorf("不支持的文档来源类型: %s", params.SourceType)
	}
	documents, err := loader.Load(ctx, document.Source{URI: params.Uri})
	if err != nil {
		return nil, fmt.Errorf("加载文档失败: %w", err)
	}

	// 加载文档到知识库
	_, err = t.indexer.Store(ctx, documents)
	if err != nil {
		return nil, fmt.Errorf("加载文档到知识库失败: %w", err)
	}

	return map[string]interface{}{
		"operation":       "load_documents",
		"source_type":     params.SourceType,
		"documents_count": len(documents),
		"success":         true,
		"message":         fmt.Sprintf("成功加载 %d 个文档到知识库", len(documents)),
	}, nil
}

// searchDocuments 搜索文档
func (t *SearchDocumentsTool) searchDocuments(ctx context.Context, params SearchParams) (interface{}, error) {
	if t.retriever == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	if t.options == nil {
		t.options = &retriever.Options{}
	}

	topK := 10
	if t.options.TopK != nil {
		topK = *t.options.TopK
	}
	threshold := 0.1
	if t.options.ScoreThreshold != nil {
		threshold = *t.options.ScoreThreshold
	}

	// 执行搜索
	results, err := t.retriever.Retrieve(ctx, params.Query,
		retriever.WithTopK(topK),
		retriever.WithScoreThreshold(threshold))
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}

	return map[string]interface{}{
		"operation":     "search",
		"query":         params.Query,
		"results_count": len(results),
		"results":       results,
		"success":       true,
	}, nil
}
