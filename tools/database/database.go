package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
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
	desc := "执行数据库SQL并返回结果。默认只允许单条查询（SELECT/SHOW/DESCRIBE/EXPLAIN SELECT），并由数据库只读事务执行；" +
		"CTE（WITH 开头）与多语句默认视为不安全，如需执行必须由工具创建方显式启用 WithAllowWrite(true)。" +
		"支持MySQL、PostgreSQL、SQLite。"
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
	isSelect := isSafeReadQuery(params.Query)
	if !isSelect && !t.allowWrite {
		return nil, fmt.Errorf("database_execute is read-only by default; enable WithAllowWrite(true) to run %s statements", queryUpper)
	}

	if isSelect {
		// GORM implements nested Transaction with a savepoint and ignores
		// TxOptions there. Refuse pre-existing transactions rather than run
		// a supposedly read-only query inside an unknown writable transaction.
		if !t.allowWrite {
			if _, nested := t.db.Statement.ConnPool.(gorm.TxCommitter); nested {
				return nil, fmt.Errorf("database_execute: read-only mode requires a database handle outside an existing transaction")
			}
		}
		var output interface{}
		read := func(db *gorm.DB) error {
			var err error
			output, err = t.readRows(ctx, db, params)
			return err
		}
		if t.allowWrite {
			err := read(t.db.WithContext(ctx))
			return output, err
		}
		var err error
		switch t.db.Dialector.Name() {
		case "sqlite":
			// SQLite ignores sql.TxOptions.ReadOnly. Pin one connection and
			// enable query_only on that connection, restoring it before reuse.
			err = t.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
				var previous int
				if err := db.Raw("PRAGMA query_only").Scan(&previous).Error; err != nil {
					return err
				}
				if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
					return err
				}
				defer func() {
					// Restoration must not inherit the query's expired context.
					if restoreErr := db.WithContext(context.Background()).Exec(fmt.Sprintf("PRAGMA query_only = %d", previous)).Error; restoreErr != nil {
						if conn, ok := db.Statement.ConnPool.(*sql.Conn); ok {
							_ = conn.Raw(func(any) error { return driver.ErrBadConn })
						}
					}
				}()
				return read(db)
			})
		case "postgres", "mysql":
			err = t.db.WithContext(ctx).Transaction(read, &sql.TxOptions{ReadOnly: true})
		default:
			err = fmt.Errorf("database_execute: unsupported read-only dialect %q", t.db.Dialector.Name())
		}
		return output, err
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

func (t *DatabaseExecuteTool) readRows(ctx context.Context, db *gorm.DB, params ExecuteParams) (interface{}, error) {
	rows, err := db.WithContext(ctx).Raw(params.Query, params.Params...).Rows()
	if err != nil {
		return nil, fmt.Errorf("执行SQL失败: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := make([]map[string]interface{}, 0)
	truncated := false
	for rows.Next() {
		if t.maxResultRows > 0 && len(results) >= t.maxResultRows {
			truncated = true
			break
		}
		values := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"operation": "database_execute", "query": params.Query, "results": results, "count": len(results), "success": true, "truncated": truncated}, nil
}

// sqlTokens accepts a deliberately conservative common SQL subset. It skips
// ordinary comments and quoted values, rejects dialect-dependent escapes and
// executable comments, and permits only one trailing statement delimiter.
// The database read-only transaction is the final enforcement boundary.
func sqlTokens(query string) ([]string, bool) {
	var tokens []string
	ended := false
	for i := 0; i < len(query); {
		c := query[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		if strings.HasPrefix(query[i:], "--") {
			j := strings.IndexByte(query[i:], '\n')
			if j < 0 {
				break
			}
			i += j + 1
			continue
		}
		if strings.HasPrefix(query[i:], "/*") {
			depth := 1
			i += 2
			if i < len(query) && (query[i] == '!' || query[i] == '+') {
				return nil, false
			}
			for i < len(query) && depth > 0 {
				if strings.HasPrefix(query[i:], "/*") {
					// Block comment nesting differs between SQL dialects.
					// Reject it so the scanner cannot hide a delimiter that
					// SQLite or MySQL would treat as executable SQL.
					return nil, false
				} else if strings.HasPrefix(query[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, false
			}
			continue
		}
		if ended {
			return nil, false
		}
		if c == ';' {
			ended = true
			i++
			continue
		}
		if c == '#' || c == '\\' || c == '$' {
			return nil, false
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			quote := c
			if c == '[' {
				quote = ']'
			}
			i++
			closed := false
			for i < len(query) {
				if query[i] == '\\' {
					return nil, false
				}
				if query[i] == quote {
					if i+1 < len(query) && query[i+1] == quote {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, false
			}
			tokens = append(tokens, "<quoted>")
			continue
		}
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' {
			j := i + 1
			for j < len(query) && ((query[j] >= 'a' && query[j] <= 'z') || (query[j] >= 'A' && query[j] <= 'Z') || (query[j] >= '0' && query[j] <= '9') || query[j] == '_') {
				j++
			}
			tokens = append(tokens, strings.ToUpper(query[i:j]))
			i = j
			continue
		}
		tokens = append(tokens, string(c))
		i++
	}
	return tokens, len(tokens) > 0
}

func firstSQLKeyword(query string) string {
	tokens, _ := sqlTokens(query)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}
func isReadOnlyKeyword(keyword string) bool {
	switch keyword {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return true
	}
	return false
}
func isSafeReadQuery(query string) bool {
	tokens, ok := sqlTokens(query)
	if !ok || !isReadOnlyKeyword(tokens[0]) {
		return false
	}
	if tokens[0] == "EXPLAIN" && (len(tokens) < 2 || tokens[1] != "SELECT") {
		return false
	}
	for _, token := range tokens {
		switch token {
		case "INTO", "OUTFILE", "DUMPFILE", "UPDATE", "DELETE", "INSERT", "REPLACE", "CREATE", "DROP", "ALTER", "TRUNCATE", "ANALYZE", "LOCK", "FOR", "WITH", "PRAGMA", "ATTACH", "DETACH", "LOAD_EXTENSION":
			return false
		}
	}
	return true
}
func hasMultipleStatements(query string) bool { _, ok := sqlTokens(query); return !ok }
