// Package memory 提供与 aggo memory 模块配套使用的 Agent 工具。
package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/memoryevent"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// SearchUserMemoryParams 是 search_user_memory 工具参数。
type SearchUserMemoryParams struct {
	Keywords []string `json:"keywords,omitempty" jsonschema:"description=关键词列表。命中事件 summary 或事件自身关键词时计为命中。建议传 1~5 个具体关键词，例如 ['酷企','1418']"`
	Match    string   `json:"match,omitempty" jsonschema:"description=关键词匹配方式：any=任意命中即可，all=必须全部命中。默认 any,enum=any,enum=all"`
	Type     string   `json:"type,omitempty" jsonschema:"description=事件类型过滤：milestone(任务里程碑) / event(事件记录)。留空匹配全部,enum=milestone,enum=event"`
	Since    string   `json:"since,omitempty" jsonschema:"description=起始日期，格式 YYYY-MM-DD 或 RFC3339。仅返回 EventDate >= since 的事件"`
	Until    string   `json:"until,omitempty" jsonschema:"description=结束日期，格式 YYYY-MM-DD 或 RFC3339。仅返回 EventDate <= until 的事件"`
	Limit    int      `json:"limit,omitempty" jsonschema:"description=返回上限，默认 10，最大 30"`
	UserID   string   `json:"user_id,omitempty" jsonschema:"description=用户ID。未传时自动取 session 中的 userID"`
}

// SearchUserMemoryResultItem 是 search_user_memory 工具返回的单条事件。
type SearchUserMemoryResultItem struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Date     string   `json:"date"`
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords,omitempty"`
}

// SearchUserMemoryResult 是 search_user_memory 工具返回值。
type SearchUserMemoryResult struct {
	Total  int                          `json:"total"`
	Events []SearchUserMemoryResultItem `json:"events"`
}

// SearchUserMemoryTool 构造 search_user_memory 工具。
//
//	provider: 实现了 UserMemoryEventSearcher 接口的对象（一般直接传 memory provider）。
//
// 工具会自动从 adk.SessionValues 取 userID；调用方也可以显式传 user_id 覆盖。
func SearchUserMemoryTool(provider memory.UserMemoryEventSearcher) (tool.BaseTool, error) {
	if provider == nil {
		return nil, errors.New("provider 不能为空")
	}

	name := "search_user_memory"
	desc := "检索当前用户的长期记忆事件（任务里程碑 / 事件记录）。" +
		"system 中已经常驻最近若干条事件，调用本工具用于查找更早、更大范围或针对特定关键词的事件。" +
		"支持关键词、时间窗、事件类型过滤。"

	return utils.InferTool(name, desc, func(ctx context.Context, params SearchUserMemoryParams) (interface{}, error) {
		return searchUserMemory(ctx, provider, params)
	})
}

func searchUserMemory(ctx context.Context, provider memory.UserMemoryEventSearcher, params SearchUserMemoryParams) (*SearchUserMemoryResult, error) {
	userID := strings.TrimSpace(params.UserID)
	if userID == "" {
		userID = sessionString(ctx, "userID")
	}
	if userID == "" {
		return nil, errors.New("无法确定 userID，请确保 adk session 中存在 userID 或显式传入 user_id 参数")
	}

	query := &memoryevent.Query{
		UserID:   userID,
		Type:     strings.TrimSpace(params.Type),
		Keywords: params.Keywords,
		Match:    strings.TrimSpace(params.Match),
		Limit:    clampLimit(params.Limit, 10, 30),
	}

	if since, ok := parseToolDate(params.Since); ok {
		query.Since = &since
	}
	if until, ok := parseToolDate(params.Until); ok {
		query.Until = &until
	}

	events, err := provider.SearchUserMemoryEvents(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("检索失败: %w", err)
	}

	result := &SearchUserMemoryResult{
		Total:  len(events),
		Events: make([]SearchUserMemoryResultItem, 0, len(events)),
	}
	for _, evt := range events {
		if evt == nil {
			continue
		}
		item := SearchUserMemoryResultItem{
			ID:       evt.ID,
			Type:     evt.Type,
			Summary:  evt.Summary,
			Keywords: evt.Keywords,
		}
		if !evt.EventDate.IsZero() {
			item.Date = evt.EventDate.Format("2006-01-02")
		}
		result.Events = append(result.Events, item)
	}
	return result, nil
}

func sessionString(ctx context.Context, key string) string {
	if v, ok := adk.GetSessionValue(ctx, key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func clampLimit(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func parseToolDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{"2006-01-02", "2006-01-02 15:04", "2006/01/02", time.RFC3339, "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
