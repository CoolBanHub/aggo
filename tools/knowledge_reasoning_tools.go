package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/knowledge"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetKnowledgeReasoningTools 获取知识推理工具集合
func GetKnowledgeReasoningTools(manager *knowledge.KnowledgeManager) []tool.BaseTool {
	return []tool.BaseTool{
		NewKnowledgeThinkTool(manager),
		NewKnowledgeSearchTool(manager),
		NewKnowledgeAnalysisTool(manager),
	}
}

// KnowledgeThinkTool 知识推理思考工具
// 提供思考和推理功能，用于知识探索策略规划
type KnowledgeThinkTool struct {
	manager *knowledge.KnowledgeManager
}

// KnowledgeSearchTool 知识搜索工具
// 提供知识库搜索功能
type KnowledgeSearchTool struct {
	manager *knowledge.KnowledgeManager
}

// KnowledgeAnalysisTool 知识分析工具
// 提供搜索结果分析功能
type KnowledgeAnalysisTool struct {
	manager *knowledge.KnowledgeManager
}

// 结果结构体定义

// ThinkResult 思考结果
type ThinkResult struct {
	Thought     string   `json:"thought"`
	ThoughtsLog []string `json:"thoughtsLog"`
	Success     bool     `json:"success"`
	Error       string   `json:"error,omitempty"`
	Operation   string   `json:"operation"`
	Timestamp   int64    `json:"timestamp"`
}

// KnowledgeSearchResult 知识搜索结果
type KnowledgeSearchResult struct {
	Query         string                   `json:"query"`
	Documents     []knowledge.SearchResult `json:"documents"`
	DocumentCount int                      `json:"documentCount"`
	Success       bool                     `json:"success"`
	Error         string                   `json:"error,omitempty"`
	Operation     string                   `json:"operation"`
	Timestamp     int64                    `json:"timestamp"`
}

// AnalysisResult 分析结果
type AnalysisResult struct {
	Analysis    string   `json:"analysis"`
	AnalysisLog []string `json:"analysisLog"`
	Success     bool     `json:"success"`
	Error       string   `json:"error,omitempty"`
	Operation   string   `json:"operation"`
	Timestamp   int64    `json:"timestamp"`
}

// 参数结构体定义

// ThinkParams 思考参数
type ThinkParams struct {
	Thought string `json:"thought" jsonschema:"description=思考内容和推理过程,required"`
}

// KnowledgeSearchParams 知识搜索参数
type KnowledgeSearchParams struct {
	Query string `json:"query" jsonschema:"description=搜索查询内容,required"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=搜索结果数量限制,默认为10"`
}

// AnalysisParams 分析参数
type AnalysisParams struct {
	Analysis string `json:"analysis" jsonschema:"description=对搜索结果的分析和评估,required"`
}

// 工具构造函数

// NewKnowledgeThinkTool 创建知识思考工具实例
func NewKnowledgeThinkTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &KnowledgeThinkTool{
		manager: manager,
	}
	name := "knowledge_think"
	desc := "用作思考和推理的工具，帮助规划知识探索策略。在需要分析问题、制定搜索策略或完善方法时使用此工具。思考内容不会暴露给用户，仅用于内部推理。"
	t, _ := utils.InferTool(name, desc, this.think)
	return t
}

// NewKnowledgeSearchTool 创建知识搜索工具实例
func NewKnowledgeSearchTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &KnowledgeSearchTool{
		manager: manager,
	}
	name := "knowledge_search"
	desc := "搜索知识库获取相关信息。在思考后使用此工具多次搜索相关信息。支持多种搜索策略，如精确短语（使用引号）、OR操作符和聚焦关键词。"
	t, _ := utils.InferTool(name, desc, this.search)
	return t
}

// NewKnowledgeAnalysisTool 创建知识分析工具实例
func NewKnowledgeAnalysisTool(manager *knowledge.KnowledgeManager) tool.InvokableTool {
	this := &KnowledgeAnalysisTool{
		manager: manager,
	}
	name := "knowledge_analysis"
	desc := "分析和评估搜索结果的质量、相关性和完整性。获得搜索结果后使用此工具分析信息质量。如果结果不足，可以返回使用Think或Search工具优化查询。"
	t, _ := utils.InferTool(name, desc, this.analyze)
	return t
}

// think 执行思考操作
func (t *KnowledgeThinkTool) think(ctx context.Context, params ThinkParams) (*ThinkResult, error) {
	if params.Thought == "" {
		return &ThinkResult{
			Success:   false,
			Error:     "思考内容不能为空",
			Operation: "Think",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// 从context中获取思考历史（这里需要调用方在context中设置思考历史）
	var thoughtsLog []string
	if existingThoughts := ctx.Value("knowledge_thoughts"); existingThoughts != nil {
		if thoughts, ok := existingThoughts.([]string); ok {
			thoughtsLog = thoughts
		}
	}

	// 添加新的思考内容
	thoughtsLog = append(thoughtsLog, params.Thought)

	// 格式化返回所有思考历史
	var formattedThoughts string
	for i, thought := range thoughtsLog {
		formattedThoughts += fmt.Sprintf("- 思考 %d: %s\n", i+1, thought)
	}

	return &ThinkResult{
		Thought:     formattedThoughts,
		ThoughtsLog: thoughtsLog,
		Success:     true,
		Operation:   "Think",
		Timestamp:   time.Now().Unix(),
	}, nil
}

// search 执行搜索操作
func (t *KnowledgeSearchTool) search(ctx context.Context, params KnowledgeSearchParams) (*KnowledgeSearchResult, error) {
	if t.manager == nil {
		return &KnowledgeSearchResult{
			Query:     params.Query,
			Success:   false,
			Error:     "知识库管理器未初始化",
			Operation: "Search",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	if params.Query == "" {
		return &KnowledgeSearchResult{
			Query:     params.Query,
			Success:   false,
			Error:     "搜索查询不能为空",
			Operation: "Search",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// 设置默认值
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}

	// 构建搜索选项
	searchOptions := t.manager.GetConfig().DefaultSearchOptions

	// 执行搜索
	results, err := t.manager.Search(ctx, params.Query, searchOptions)
	if err != nil {
		return &KnowledgeSearchResult{
			Query:     params.Query,
			Success:   false,
			Error:     fmt.Sprintf("搜索失败: %v", err),
			Operation: "Search",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	return &KnowledgeSearchResult{
		Query:         params.Query,
		Documents:     results,
		DocumentCount: len(results),
		Success:       true,
		Operation:     "Search",
		Timestamp:     time.Now().Unix(),
	}, nil
}

// analyze 执行分析操作
func (t *KnowledgeAnalysisTool) analyze(ctx context.Context, params AnalysisParams) (*AnalysisResult, error) {
	if params.Analysis == "" {
		return &AnalysisResult{
			Success:   false,
			Error:     "分析内容不能为空",
			Operation: "Analysis",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// 从context中获取分析历史
	var analysisLog []string
	if existingAnalysis := ctx.Value("knowledge_analysis"); existingAnalysis != nil {
		if analysis, ok := existingAnalysis.([]string); ok {
			analysisLog = analysis
		}
	}

	// 添加新的分析内容
	analysisLog = append(analysisLog, params.Analysis)

	// 格式化返回所有分析历史
	var formattedAnalysis string
	for i, analysis := range analysisLog {
		formattedAnalysis += fmt.Sprintf("- 分析 %d: %s\n", i+1, analysis)
	}

	return &AnalysisResult{
		Analysis:    formattedAnalysis,
		AnalysisLog: analysisLog,
		Success:     true,
		Operation:   "Analysis",
		Timestamp:   time.Now().Unix(),
	}, nil
}
