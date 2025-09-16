package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"
)

// GetPostgresTools 获取PostgreSQL工具列表
func GetPostgresTools(db *gorm.DB) []tool.BaseTool {
	return []tool.BaseTool{
		NewListDatabasesTool(db),
		NewListTablesTool(db),
		NewDescribeTableTool(db),
		NewReadQueryTool(db),
		NewWriteQueryTool(db),
		NewUpdateQueryTool(db),
		NewDeleteQueryTool(db),
		NewCountQueryTool(db),
		NewCreateTableTool(db),
		NewAlterTableTool(db),
	}
}

// ListDatabasesTool 列出数据库工具
type ListDatabasesTool struct {
	db *gorm.DB
}

// ListTablesTool 列出表工具
type ListTablesTool struct {
	db *gorm.DB
}

// DescribeTableTool 描述表结构工具
type DescribeTableTool struct {
	db *gorm.DB
}

// ReadQueryTool 查询数据工具
type ReadQueryTool struct {
	db *gorm.DB
}

// WriteQueryTool 写入数据工具
type WriteQueryTool struct {
	db *gorm.DB
}

// UpdateQueryTool 更新数据工具
type UpdateQueryTool struct {
	db *gorm.DB
}

// DeleteQueryTool 删除数据工具
type DeleteQueryTool struct {
	db *gorm.DB
}

// CountQueryTool 计数查询工具
type CountQueryTool struct {
	db *gorm.DB
}

// CreateTableTool 创建表工具
type CreateTableTool struct {
	db *gorm.DB
}

// AlterTableTool 修改表工具
type AlterTableTool struct {
	db *gorm.DB
}

// ListDatabasesParams 列出数据库参数
type ListDatabasesParams struct{}

// ListTablesParams 列出表参数
type ListTablesParams struct {
	Database string `json:"database,omitempty" jsonschema:"description=数据库名称，不指定时使用当前数据库"`
}

// DescribeTableParams 描述表参数
type DescribeTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
}

// ReadQueryParams 查询参数
type ReadQueryParams struct {
	Query  string        `json:"query" jsonschema:"description=SQL查询语句,required"`
	Params []interface{} `json:"params,omitempty" jsonschema:"description=查询参数"`
	Limit  int           `json:"limit,omitempty" jsonschema:"description=结果限制数量，默认100"`
}

// WriteQueryParams 写入参数
type WriteQueryParams struct {
	TableName string                   `json:"tableName" jsonschema:"description=表名,required"`
	Data      []map[string]interface{} `json:"data" jsonschema:"description=要插入的数据,required"`
}

// UpdateQueryParams 更新参数
type UpdateQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where" jsonschema:"description=更新条件,required"`
	Data      map[string]interface{} `json:"data" jsonschema:"description=要更新的数据,required"`
}

// DeleteQueryParams 删除参数
type DeleteQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where" jsonschema:"description=删除条件,required"`
}

// CountQueryParams 计数参数
type CountQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where,omitempty" jsonschema:"description=计数条件"`
}

// CreateTableParams 创建表参数
type CreateTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
	SQL       string `json:"sql" jsonschema:"description=创建表的SQL语句,required"`
	IfExists  bool   `json:"ifExists,omitempty" jsonschema:"description=如果表存在是否跳过，默认false"`
}

// AlterTableParams 修改表参数
type AlterTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
	SQL       string `json:"sql" jsonschema:"description=修改表的SQL语句,required"`
}

// NewListDatabasesTool 创建列出数据库工具实例
func NewListDatabasesTool(db *gorm.DB) tool.InvokableTool {
	this := &ListDatabasesTool{db: db}
	name := "list_databases"
	desc := "列出PostgreSQL实例中的所有数据库。"
	t, _ := utils.InferTool(name, desc, this.listDatabases)
	return t
}

// NewListTablesTool 创建列出表工具实例
func NewListTablesTool(db *gorm.DB) tool.InvokableTool {
	this := &ListTablesTool{db: db}
	name := "list_tables"
	desc := "列出指定数据库中的所有表。"
	t, _ := utils.InferTool(name, desc, this.listTables)
	return t
}

// NewDescribeTableTool 创建描述表工具实例
func NewDescribeTableTool(db *gorm.DB) tool.InvokableTool {
	this := &DescribeTableTool{db: db}
	name := "describe_table"
	desc := "获取表的结构信息，包括列名、数据类型、约束等。"
	t, _ := utils.InferTool(name, desc, this.describeTable)
	return t
}

// NewReadQueryTool 创建查询数据工具实例
func NewReadQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &ReadQueryTool{db: db}
	name := "read_query"
	desc := "执行SELECT查询并返回结果。支持参数化查询和结果限制。"
	t, _ := utils.InferTool(name, desc, this.readQuery)
	return t
}

// NewWriteQueryTool 创建写入数据工具实例
func NewWriteQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &WriteQueryTool{db: db}
	name := "write_query"
	desc := "向指定表插入数据。支持批量插入。"
	t, _ := utils.InferTool(name, desc, this.writeQuery)
	return t
}

// NewUpdateQueryTool 创建更新数据工具实例
func NewUpdateQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &UpdateQueryTool{db: db}
	name := "update_query"
	desc := "根据条件更新表中的数据。"
	t, _ := utils.InferTool(name, desc, this.updateQuery)
	return t
}

// NewDeleteQueryTool 创建删除数据工具实例
func NewDeleteQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &DeleteQueryTool{db: db}
	name := "delete_query"
	desc := "根据条件删除表中的数据。"
	t, _ := utils.InferTool(name, desc, this.deleteQuery)
	return t
}

// NewCountQueryTool 创建计数查询工具实例
func NewCountQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &CountQueryTool{db: db}
	name := "count_query"
	desc := "统计表中符合条件的记录数量。"
	t, _ := utils.InferTool(name, desc, this.countQuery)
	return t
}

// NewCreateTableTool 创建表工具实例
func NewCreateTableTool(db *gorm.DB) tool.InvokableTool {
	this := &CreateTableTool{db: db}
	name := "create_table"
	desc := "创建新的数据表。"
	t, _ := utils.InferTool(name, desc, this.createTable)
	return t
}

// NewAlterTableTool 创建修改表工具实例
func NewAlterTableTool(db *gorm.DB) tool.InvokableTool {
	this := &AlterTableTool{db: db}
	name := "alter_table"
	desc := "修改现有数据表的结构。"
	t, _ := utils.InferTool(name, desc, this.alterTable)
	return t
}

// listDatabases 列出数据库
func (t *ListDatabasesTool) listDatabases(ctx context.Context, params ListDatabasesParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	var databases []string
	query := "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname"

	rows, err := t.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, fmt.Errorf("扫描数据库名称失败: %w", err)
		}
		databases = append(databases, dbName)
	}

	return map[string]interface{}{
		"operation": "list_databases",
		"databases": databases,
		"count":     len(databases),
		"success":   true,
	}, nil
}

// listTables 列出表
func (t *ListTablesTool) listTables(ctx context.Context, params ListTablesParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	var tables []map[string]interface{}
	query := `
		SELECT
			table_name,
			table_schema,
			table_type
		FROM information_schema.tables
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
		ORDER BY table_schema, table_name
	`

	rows, err := t.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, tableSchema, tableType string
		if err := rows.Scan(&tableName, &tableSchema, &tableType); err != nil {
			return nil, fmt.Errorf("扫描表信息失败: %w", err)
		}
		tables = append(tables, map[string]interface{}{
			"name":   tableName,
			"schema": tableSchema,
			"type":   tableType,
		})
	}

	return map[string]interface{}{
		"operation": "list_tables",
		"tables":    tables,
		"count":     len(tables),
		"success":   true,
	}, nil
}

// describeTable 描述表结构
func (t *DescribeTableTool) describeTable(ctx context.Context, params DescribeTableParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	var columns []map[string]interface{}
	query := `
		SELECT
			column_name,
			data_type,
			is_nullable,
			column_default,
			character_maximum_length,
			numeric_precision,
			numeric_scale
		FROM information_schema.columns
		WHERE table_name = ?
		ORDER BY ordinal_position
	`

	rows, err := t.db.WithContext(ctx).Raw(query, params.TableName).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询表结构失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var columnName, dataType, isNullable string
		var columnDefault, characterMaxLength, numericPrecision, numericScale interface{}

		if err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault,
			&characterMaxLength, &numericPrecision, &numericScale); err != nil {
			return nil, fmt.Errorf("扫描列信息失败: %w", err)
		}

		columns = append(columns, map[string]interface{}{
			"name":                 columnName,
			"type":                 dataType,
			"nullable":             isNullable == "YES",
			"default":              columnDefault,
			"character_max_length": characterMaxLength,
			"numeric_precision":    numericPrecision,
			"numeric_scale":        numericScale,
		})
	}

	return map[string]interface{}{
		"operation":  "describe_table",
		"table_name": params.TableName,
		"columns":    columns,
		"success":    true,
	}, nil
}

// readQuery 执行查询
func (t *ReadQueryTool) readQuery(ctx context.Context, params ReadQueryParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.Query == "" {
		return nil, fmt.Errorf("查询语句不能为空")
	}

	// 验证是否为SELECT查询
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(params.Query)), "SELECT") {
		return nil, fmt.Errorf("只允许执行SELECT查询")
	}

	// 设置默认限制
	if params.Limit == 0 {
		params.Limit = 100
	}

	// 添加LIMIT子句
	query := params.Query
	if !strings.Contains(strings.ToUpper(query), "LIMIT") {
		query = fmt.Sprintf("%s LIMIT %d", query, params.Limit)
	}

	var results []map[string]interface{}
	rows, err := t.db.WithContext(ctx).Raw(query, params.Params...).Rows()
	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	// 获取列名
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
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return map[string]interface{}{
		"operation": "read_query",
		"query":     params.Query,
		"results":   results,
		"count":     len(results),
		"success":   true,
	}, nil
}

// writeQuery 写入数据
func (t *WriteQueryTool) writeQuery(ctx context.Context, params WriteQueryParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if len(params.Data) == 0 {
		return nil, fmt.Errorf("插入数据不能为空")
	}

	var insertedCount int64
	err := t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range params.Data {
			result := tx.Table(params.TableName).Create(row)
			if result.Error != nil {
				return result.Error
			}
			insertedCount += result.RowsAffected
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("插入数据失败: %w", err)
	}

	return map[string]interface{}{
		"operation":      "write_query",
		"table_name":     params.TableName,
		"inserted_count": insertedCount,
		"success":        true,
		"message":        fmt.Sprintf("成功插入 %d 条记录", insertedCount),
	}, nil
}

// updateQuery 更新数据
func (t *UpdateQueryTool) updateQuery(ctx context.Context, params UpdateQueryParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if len(params.Where) == 0 {
		return nil, fmt.Errorf("更新条件不能为空")
	}

	if len(params.Data) == 0 {
		return nil, fmt.Errorf("更新数据不能为空")
	}

	result := t.db.WithContext(ctx).Table(params.TableName).Where(params.Where).Updates(params.Data)
	if result.Error != nil {
		return nil, fmt.Errorf("更新数据失败: %w", result.Error)
	}

	return map[string]interface{}{
		"operation":     "update_query",
		"table_name":    params.TableName,
		"updated_count": result.RowsAffected,
		"success":       true,
		"message":       fmt.Sprintf("成功更新 %d 条记录", result.RowsAffected),
	}, nil
}

// deleteQuery 删除数据
func (t *DeleteQueryTool) deleteQuery(ctx context.Context, params DeleteQueryParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if len(params.Where) == 0 {
		return nil, fmt.Errorf("删除条件不能为空")
	}

	result := t.db.WithContext(ctx).Table(params.TableName).Where(params.Where).Delete(nil)
	if result.Error != nil {
		return nil, fmt.Errorf("删除数据失败: %w", result.Error)
	}

	return map[string]interface{}{
		"operation":     "delete_query",
		"table_name":    params.TableName,
		"deleted_count": result.RowsAffected,
		"success":       true,
		"message":       fmt.Sprintf("成功删除 %d 条记录", result.RowsAffected),
	}, nil
}

// countQuery 计数查询
func (t *CountQueryTool) countQuery(ctx context.Context, params CountQueryParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	var count int64
	query := t.db.WithContext(ctx).Table(params.TableName)

	if len(params.Where) > 0 {
		query = query.Where(params.Where)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, fmt.Errorf("计数查询失败: %w", err)
	}

	return map[string]interface{}{
		"operation":  "count_query",
		"table_name": params.TableName,
		"count":      count,
		"success":    true,
	}, nil
}

// createTable 创建表
func (t *CreateTableTool) createTable(ctx context.Context, params CreateTableParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if params.SQL == "" {
		return nil, fmt.Errorf("SQL语句不能为空")
	}

	// 检查表是否存在
	if params.IfExists {
		var exists bool
		checkQuery := `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_name = ?
			)
		`
		if err := t.db.WithContext(ctx).Raw(checkQuery, params.TableName).Scan(&exists).Error; err != nil {
			return nil, fmt.Errorf("检查表是否存在失败: %w", err)
		}

		if exists {
			return map[string]interface{}{
				"operation":  "create_table",
				"table_name": params.TableName,
				"success":    true,
				"message":    "表已存在，跳过创建",
				"skipped":    true,
			}, nil
		}
	}

	if err := t.db.WithContext(ctx).Exec(params.SQL).Error; err != nil {
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	return map[string]interface{}{
		"operation":  "create_table",
		"table_name": params.TableName,
		"success":    true,
		"message":    "表创建成功",
	}, nil
}

// alterTable 修改表
func (t *AlterTableTool) alterTable(ctx context.Context, params AlterTableParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if params.SQL == "" {
		return nil, fmt.Errorf("SQL语句不能为空")
	}

	if err := t.db.WithContext(ctx).Exec(params.SQL).Error; err != nil {
		return nil, fmt.Errorf("修改表失败: %w", err)
	}

	return map[string]interface{}{
		"operation":  "alter_table",
		"table_name": params.TableName,
		"success":    true,
		"message":    "表修改成功",
	}, nil
}
