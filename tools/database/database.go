package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"
)

// GetTools 获取通用数据库工具列表
func GetTools(db *gorm.DB) []tool.BaseTool {
	return []tool.BaseTool{
		NewDatabaseExecuteTool(db),
	}
}

// ============================================================
// Tool 结构体定义
// ============================================================

// DatabaseExecuteTool 数据库执行工具
type DatabaseExecuteTool struct {
	db *gorm.DB
}

// ============================================================
// Params 结构体定义
// ============================================================

// ExecuteParams 查询参数
type ExecuteParams struct {
	Query  string        `json:"query" jsonschema:"description=要执行的SQL查询语句,required"`
	Params []interface{} `json:"params,omitempty" jsonschema:"description=查询参数（可选）"`
}

// ============================================================
// 工具构造函数
// ============================================================

// NewDatabaseExecuteTool 创建执行核心工具实例
func NewDatabaseExecuteTool(db *gorm.DB) tool.InvokableTool {
	this := &DatabaseExecuteTool{db: db}
	name := "database_execute"
	desc := "执行任意格式的数据库SQL查询(SELECT)和操作(INSERT, UPDATE, DELETE, CREATE 等)并返回结果。支持所有主流关系型数据库（MySQL, PostgreSQL，SQLite）。"
	t, _ := utils.InferTool(name, desc, this.execute)
	return t
}

// ============================================================
// 工具方法实现
// ============================================================

// execute 执行 SQL
func (t *DatabaseExecuteTool) execute(ctx context.Context, params ExecuteParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.Query == "" {
		return nil, fmt.Errorf("SQL语句不能为空")
	}

	// 检查是否是返回结果集的语句类型
	queryUpper := strings.ToUpper(strings.TrimSpace(params.Query))
	isSelect := strings.HasPrefix(queryUpper, "SELECT") || strings.HasPrefix(queryUpper, "SHOW") || strings.HasPrefix(queryUpper, "DESCRIBE") || strings.HasPrefix(queryUpper, "EXPLAIN") || strings.HasPrefix(queryUpper, "PRAGMA")

	if isSelect {
		// 查询结果集
		var results []map[string]interface{}
		rows, err := t.db.WithContext(ctx).Raw(params.Query, params.Params...).Rows()
		if err != nil {
			return nil, fmt.Errorf("执行SQL失败: %w", err)
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("获取列名失败: %w", err)
		}

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("扫描结果失败: %w", err)
			}

			row := make(map[string]interface{})
			for i, col := range columns {
				// Convert byte slices to strings to safely marshal into JSON
				val := values[i]
				if b, ok := val.([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = val
				}
			}
			results = append(results, row)
		}

		// Truncate results if row count is massively large (e.g. > 1000)
		if len(results) > 1000 {
			truncatedMsg := fmt.Sprintf("... (truncated %d more rows. Consider adding LIMIT to your query)", len(results)-1000)
			results = results[:1000]
			return map[string]interface{}{
				"operation": "database_execute",
				"query":     params.Query,
				"results":   results,
				"count":     len(results),
				"success":   true,
				"message":   truncatedMsg,
			}, nil
		}

		return map[string]interface{}{
			"operation": "database_execute",
			"query":     params.Query,
			"results":   results,
			"count":     len(results),
			"success":   true,
		}, nil
	}

	// 执行修改数据的语句，返回受影响行数
	result := t.db.WithContext(ctx).Exec(params.Query, params.Params...)
	if result.Error != nil {
		return nil, fmt.Errorf("执行SQL失败: %w", result.Error)
	}

	return map[string]interface{}{
		"operation":     "database_execute",
		"query":         params.Query,
		"rows_affected": result.RowsAffected,
		"success":       true,
		"message":       fmt.Sprintf("命令执行成功，受影响的行数：%d", result.RowsAffected),
	}, nil
}
