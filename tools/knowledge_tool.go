package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/CoolBanHub/aggo/knowledge/readers"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func GetKnowledgeTools(manager *knowledge.KnowledgeManager) []tool.BaseTool {
	return []tool.BaseTool{
		NewLoadDocumentsTool(manager),
		NewSearchDocumentsTool(manager),
		NewGetDocumentTool(manager),
		NewUpdateDocumentTool(manager),
		NewDeleteDocumentTool(manager),
		NewListDocumentsTool(manager),
	}
}

// LoadDocumentsTool 文档加载工具
// 提供将文档加载到知识库的功能
type LoadDocumentsTool struct {
	manager *knowledge.KnowledgeManager
}

// SearchDocumentsTool 文档搜索工具
// 提供在知识库中搜索文档的功能
type SearchDocumentsTool struct {
	manager *knowledge.KnowledgeManager
}

// GetDocumentTool 获取文档工具
// 提供获取单个文档的功能
type GetDocumentTool struct {
	manager *knowledge.KnowledgeManager
}

// UpdateDocumentTool 更新文档工具
// 提供更新文档内容和元数据的功能
type UpdateDocumentTool struct {
	manager *knowledge.KnowledgeManager
}

// DeleteDocumentTool 删除文档工具
// 提供删除文档的功能
type DeleteDocumentTool struct {
	manager *knowledge.KnowledgeManager
}

// ListDocumentsTool 列出文档工具
// 提供列出文档的功能
type ListDocumentsTool struct {
	manager *knowledge.KnowledgeManager
}

// LoadDocumentsParams 加载文档的参数
type LoadDocumentsParams struct {
	// 文档来源类型：text_files, urls, directory, memory
	SourceType string `json:"sourceType" jsonschema:"description=文档来源类型,required,enum=text_files,enum=urls,enum=directory,enum=memory"`

	// 文本文件路径列表（当sourceType为text_files时使用）
	FilePaths []string `json:"filePaths,omitempty" jsonschema:"description=文本文件路径列表"`

	// URL列表（当sourceType为urls时使用）
	URLs []string `json:"urls,omitempty" jsonschema:"description=URL列表"`

	// 目录路径（当sourceType为directory时使用）
	DirectoryPath string `json:"directoryPath,omitempty" jsonschema:"description=目录路径"`

	// 文件扩展名过滤器（当sourceType为directory时使用）
	Extensions []string `json:"extensions,omitempty" jsonschema:"description=文件扩展名过滤器,例如: ['.txt', '.md']"`

	// 是否递归搜索（当sourceType为directory时使用）
	Recursive bool `json:"recursive,omitempty" jsonschema:"description=是否递归搜索子目录"`

	// 内存文档（当sourceType为memory时使用）
	Documents []DocumentInput `json:"documents,omitempty" jsonschema:"description=内存文档列表"`

	// 加载选项
	LoadOptions LoadOptionsInput `json:"loadOptions,omitempty" jsonschema:"description=加载选项配置"`
}

// DocumentInput 文档输入结构
type DocumentInput struct {
	ID       string                 `json:"id" jsonschema:"description=文档ID,required"`
	Content  string                 `json:"content" jsonschema:"description=文档内容,required"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"description=文档元数据"`
}

// LoadOptionsInput 加载选项输入结构
type LoadOptionsInput struct {
	Recreate       bool `json:"recreate,omitempty" jsonschema:"description=是否重新创建知识库"`
	Upsert         bool `json:"upsert,omitempty" jsonschema:"description=是否使用插入或更新操作"`
	SkipExisting   bool `json:"skipExisting,omitempty" jsonschema:"description=是否跳过已存在的文档"`
	EnableChunking bool `json:"enableChunking,omitempty" jsonschema:"description=是否启用文档分块"`
	ChunkSize      int  `json:"chunkSize,omitempty" jsonschema:"description=分块大小（字符数），默认1000"`
	ChunkOverlap   int  `json:"chunkOverlap,omitempty" jsonschema:"description=分块重叠大小（字符数），默认200"`
}

// SearchParams 搜索参数
type SearchParams struct {
	Query     string                 `json:"query" jsonschema:"description=搜索查询,required"`
	Limit     int                    `json:"limit,omitempty" jsonschema:"description=返回结果数量限制,默认10"`
	Threshold float64                `json:"threshold,omitempty" jsonschema:"description=相似度阈值,默认0.7"`
	Filters   map[string]interface{} `json:"filters,omitempty" jsonschema:"description=元数据过滤条件"`
}

// GetDocumentParams 获取文档参数
type GetDocumentParams struct {
	DocID string `json:"docId" jsonschema:"description=文档ID,required"`
}

// UpdateDocumentParams 更新文档参数
type UpdateDocumentParams struct {
	DocID    string                 `json:"docId" jsonschema:"description=文档ID,required"`
	Content  string                 `json:"content,omitempty" jsonschema:"description=文档内容"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"description=文档元数据"`
}

// DeleteDocumentParams 删除文档参数
type DeleteDocumentParams struct {
	DocID string `json:"docId" jsonschema:"description=文档ID,required"`
}

// ListDocumentsParams 列出文档参数
type ListDocumentsParams struct {
	Limit  int `json:"limit,omitempty" jsonschema:"description=列表限制数量,默认10"`
	Offset int `json:"offset,omitempty" jsonschema:"description=列表偏移量,默认0"`
}

// NewLoadDocumentsTool 创建文档加载工具实例
func NewLoadDocumentsTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &LoadDocumentsTool{
		manager: manager,
	}
	name := "load_documents"
	desc := "将文档加载到知识库。支持多种文档来源（文本文件、URL、目录、内存），提供文档分块、重建知识库等功能。"
	t, _ := utils.InferTool(name, desc, this.loadDocuments)
	return t
}

// NewSearchDocumentsTool 创建文档搜索工具实例
func NewSearchDocumentsTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &SearchDocumentsTool{
		manager: manager,
	}
	name := "search_documents"
	desc := "在知识库中搜索文档。使用向量相似度匹配，支持设置搜索限制、相似度阈值和元数据过滤。"
	t, _ := utils.InferTool(name, desc, this.searchDocuments)
	return t
}

// NewGetDocumentTool 创建获取文档工具实例
func NewGetDocumentTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &GetDocumentTool{
		manager: manager,
	}
	name := "get_document"
	desc := "根据文档ID获取单个文档的详细信息，包括内容和元数据。"
	t, _ := utils.InferTool(name, desc, this.getDocument)
	return t
}

// NewUpdateDocumentTool 创建更新文档工具实例
func NewUpdateDocumentTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &UpdateDocumentTool{
		manager: manager,
	}
	name := "update_document"
	desc := "更新知识库中指定文档的内容和元数据。"
	t, _ := utils.InferTool(name, desc, this.updateDocument)
	return t
}

// NewDeleteDocumentTool 创建删除文档工具实例
func NewDeleteDocumentTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &DeleteDocumentTool{
		manager: manager,
	}
	name := "delete_document"
	desc := "从知识库中删除指定的文档。"
	t, _ := utils.InferTool(name, desc, this.deleteDocument)
	return t
}

// NewListDocumentsTool 创建列出文档工具实例
func NewListDocumentsTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &ListDocumentsTool{
		manager: manager,
	}
	name := "list_documents"
	desc := "列出知识库中的文档，支持分页查询。"
	t, _ := utils.InferTool(name, desc, this.listDocuments)
	return t
}

// loadDocuments 加载文档到知识库
func (t *LoadDocumentsTool) loadDocuments(ctx context.Context, params LoadDocumentsParams) (interface{}, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	// 创建文档读取器
	var reader knowledge.DocumentReader
	var err error

	switch strings.ToLower(params.SourceType) {
	case "text_files":
		if len(params.FilePaths) == 0 {
			return nil, fmt.Errorf("文本文件路径不能为空")
		}
		reader = readers.NewTextFileReader(params.FilePaths)

	case "urls":
		if len(params.URLs) == 0 {
			return nil, fmt.Errorf("URL列表不能为空")
		}
		reader = readers.NewURLReader(params.URLs)

	case "directory":
		if params.DirectoryPath == "" {
			return nil, fmt.Errorf("目录路径不能为空")
		}
		reader = readers.NewDirectoryReader(params.DirectoryPath, params.Extensions, params.Recursive)

	case "memory":
		if len(params.Documents) == 0 {
			return nil, fmt.Errorf("内存文档不能为空")
		}
		var docs []knowledge.Document
		for _, docInput := range params.Documents {
			doc := knowledge.Document{
				ID:       docInput.ID,
				Content:  docInput.Content,
				Metadata: docInput.Metadata,
			}
			docs = append(docs, doc)
		}
		reader = readers.NewInMemoryReader(docs)

	default:
		return nil, fmt.Errorf("不支持的文档来源类型: %s", params.SourceType)
	}

	// 读取文档
	documents, err := reader.ReadDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取文档失败: %w", err)
	}

	// 转换加载选项
	loadOptions := knowledge.LoadOptions{
		Recreate:       params.LoadOptions.Recreate,
		Upsert:         params.LoadOptions.Upsert,
		SkipExisting:   params.LoadOptions.SkipExisting,
		EnableChunking: params.LoadOptions.EnableChunking,
		ChunkSize:      params.LoadOptions.ChunkSize,
		ChunkOverlap:   params.LoadOptions.ChunkOverlap,
	}

	// 设置默认值
	if loadOptions.ChunkSize == 0 {
		loadOptions.ChunkSize = 1000
	}
	if loadOptions.ChunkOverlap == 0 {
		loadOptions.ChunkOverlap = 200
	}

	// 加载文档到知识库
	err = t.manager.LoadDocuments(ctx, documents, loadOptions)
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
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	// 设置默认值
	if params.Limit == 0 {
		params.Limit = 10
	}
	if params.Threshold == 0 {
		params.Threshold = 0.7
	}

	searchOptions := knowledge.SearchOptions{
		Limit:     params.Limit,
		Threshold: params.Threshold,
		Filters:   params.Filters,
	}

	// 执行搜索
	results, err := t.manager.Search(ctx, params.Query, searchOptions)
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

// getDocument 获取文档
func (t *GetDocumentTool) getDocument(ctx context.Context, params GetDocumentParams) (interface{}, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	if params.DocID == "" {
		return nil, fmt.Errorf("文档ID不能为空")
	}

	doc, err := t.manager.GetDocument(ctx, params.DocID)
	if err != nil {
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}

	return map[string]interface{}{
		"operation": "get_document",
		"document":  doc,
		"success":   true,
	}, nil
}

// updateDocument 更新文档
func (t *UpdateDocumentTool) updateDocument(ctx context.Context, params UpdateDocumentParams) (interface{}, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	if params.DocID == "" {
		return nil, fmt.Errorf("文档ID不能为空")
	}

	doc := knowledge.Document{
		ID:       params.DocID,
		Content:  params.Content,
		Metadata: params.Metadata,
	}

	err := t.manager.UpdateDocument(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("更新文档失败: %w", err)
	}

	return map[string]interface{}{
		"operation": "update_document",
		"doc_id":    params.DocID,
		"success":   true,
		"message":   "文档更新成功",
	}, nil
}

// deleteDocument 删除文档
func (t *DeleteDocumentTool) deleteDocument(ctx context.Context, params DeleteDocumentParams) (interface{}, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	if params.DocID == "" {
		return nil, fmt.Errorf("文档ID不能为空")
	}

	err := t.manager.DeleteDocument(ctx, params.DocID)
	if err != nil {
		return nil, fmt.Errorf("删除文档失败: %w", err)
	}

	return map[string]interface{}{
		"operation": "delete_document",
		"doc_id":    params.DocID,
		"success":   true,
		"message":   "文档删除成功",
	}, nil
}

// listDocuments 列出文档
func (t *ListDocumentsTool) listDocuments(ctx context.Context, params ListDocumentsParams) (interface{}, error) {
	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	if t.manager == nil {
		return nil, fmt.Errorf("知识库管理器未初始化")
	}

	limit := params.Limit
	if limit == 0 {
		limit = 10
	}

	docs, err := t.manager.ListDocuments(ctx, limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("列出文档失败: %w", err)
	}

	return map[string]interface{}{
		"operation":       "list_documents",
		"documents_count": len(docs),
		"documents":       docs,
		"limit":           limit,
		"offset":          params.Offset,
		"success":         true,
	}, nil
}
