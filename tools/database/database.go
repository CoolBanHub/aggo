package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"
)

const defaultMaxResultRows = 1000

// GetTools 获取通用数据库工具列表
func GetTools(db *gorm.DB, opts ...Option) []tool.BaseTool {
	return []tool.BaseTool{
		NewDatabaseExecuteTool(db, opts...),
	}
}

// ============================================================
// Tool 结构体定义
// ============================================================

// DatabaseExecuteTool 数据库执行工具
type DatabaseExecuteTool struct {
	db            *gorm.DB
	allowWrite    bool
	maxResultRows int
	timeout       time.Duration
}

// Option configures the database execution tool.
type Option func(*DatabaseExecuteTool)

// WithAllowWrite explicitly permits INSERT/UPDATE/DELETE/DDL statements.
// The tool is read-only by default.
func WithAllowWrite(allow bool) Option {
	return func(t *DatabaseExecuteTool) {
		t.allowWrite = allow
	}
}

// WithMaxResultRows caps SELECT-like result sets.
func WithMaxResultRows(max int) Option {
	return func(t *DatabaseExecuteTool) {
		if max > 0 {
			t.maxResultRows = max
		}
	}
}

// WithTimeout adds a timeout around each SQL execution when positive.
func WithTimeout(timeout time.Duration) Option {
	return func(t *DatabaseExecuteTool) {
		if timeout > 0 {
			t.timeout = timeout
		}
	}
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
func NewDatabaseExecuteTool(db *gorm.DB, opts ...Option) tool.InvokableTool {
	this := &DatabaseExecuteTool{
		db:            db,
		maxResultRows: defaultMaxResultRows,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(this)
		}
	}
	name := "database_execute"
	desc := "执行数据库SQL并返回结果。默认只允许只读查询；如需写操作，工具创建方必须显式启用。支持MySQL、PostgreSQL、SQLite。"
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

	ctx, cancel := t.withTimeout(ctx)
	defer cancel()

	// 检查是否是返回结果集的语句类型
	queryUpper := firstSQLKeyword(params.Query)
	isSelect := isReadOnlyKeyword(queryUpper)
	if !isSelect && !t.allowWrite {
		return nil, fmt.Errorf("database_execute is read-only by default; enable WithAllowWrite(true) to run %s statements", queryUpper)
	}

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

		// Truncate results if row count is massively large.
		if t.maxResultRows > 0 && len(results) > t.maxResultRows {
			truncatedMsg := fmt.Sprintf("... (truncated %d more rows. Consider adding LIMIT to your query)", len(results)-t.maxResultRows)
			results = results[:t.maxResultRows]
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

func (t *DatabaseExecuteTool) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if t.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, t.timeout)
}

func firstSQLKeyword(query string) string {
	query = strings.TrimSpace(query)
	for {
		if strings.HasPrefix(query, "--") {
			idx := strings.IndexByte(query, '\n')
			if idx < 0 {
				return ""
			}
			query = strings.TrimSpace(query[idx+1:])
			continue
		}
		if strings.HasPrefix(query, "/*") {
			idx := strings.Index(query, "*/")
			if idx < 0 {
				return ""
			}
			query = strings.TrimSpace(query[idx+2:])
			continue
		}
		break
	}
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(strings.Trim(fields[0], " \t\r\n("))
}

func isReadOnlyKeyword(keyword string) bool {
	switch keyword {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "PRAGMA", "WITH":
		return true
	default:
		return false
	}
}
