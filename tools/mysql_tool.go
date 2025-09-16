package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetMySQLTools 获取MySQL工具列表
func GetMySQLTools(db *gorm.DB) []tool.BaseTool {
	return []tool.BaseTool{
		NewMySQLListDatabasesTool(db),
		NewMySQLListTablesTool(db),
		NewMySQLDescribeTableTool(db),
		NewMySQLReadQueryTool(db),
		NewMySQLWriteQueryTool(db),
		NewMySQLUpdateQueryTool(db),
		NewMySQLDeleteQueryTool(db),
		NewMySQLCountQueryTool(db),
		NewMySQLCreateTableTool(db),
		NewMySQLAlterTableTool(db),
		NewMySQLShowIndexesTool(db),
		NewMySQLCreateIndexTool(db),
	}
}

// MySQLListDatabasesTool 列出数据库工具
type MySQLListDatabasesTool struct {
	db *gorm.DB
}

// MySQLListTablesTool 列出表工具
type MySQLListTablesTool struct {
	db *gorm.DB
}

// MySQLDescribeTableTool 描述表结构工具
type MySQLDescribeTableTool struct {
	db *gorm.DB
}

// MySQLReadQueryTool 查询数据工具
type MySQLReadQueryTool struct {
	db *gorm.DB
}

// MySQLWriteQueryTool 写入数据工具
type MySQLWriteQueryTool struct {
	db *gorm.DB
}

// MySQLUpdateQueryTool 更新数据工具
type MySQLUpdateQueryTool struct {
	db *gorm.DB
}

// MySQLDeleteQueryTool 删除数据工具
type MySQLDeleteQueryTool struct {
	db *gorm.DB
}

// MySQLCountQueryTool 计数查询工具
type MySQLCountQueryTool struct {
	db *gorm.DB
}

// MySQLCreateTableTool 创建表工具
type MySQLCreateTableTool struct {
	db *gorm.DB
}

// MySQLAlterTableTool 修改表工具
type MySQLAlterTableTool struct {
	db *gorm.DB
}

// MySQLShowIndexesTool 显示索引工具
type MySQLShowIndexesTool struct {
	db *gorm.DB
}

// MySQLCreateIndexTool 创建索引工具
type MySQLCreateIndexTool struct {
	db *gorm.DB
}

// MySQLListDatabasesParams 列出数据库参数
type MySQLListDatabasesParams struct{}

// MySQLListTablesParams 列出表参数
type MySQLListTablesParams struct {
	Database string `json:"database,omitempty" jsonschema:"description=数据库名称，不指定时使用当前数据库"`
}

// MySQLDescribeTableParams 描述表参数
type MySQLDescribeTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
}

// MySQLReadQueryParams 查询参数
type MySQLReadQueryParams struct {
	Query  string        `json:"query" jsonschema:"description=SQL查询语句,required"`
	Params []interface{} `json:"params,omitempty" jsonschema:"description=查询参数"`
	Limit  int           `json:"limit,omitempty" jsonschema:"description=结果限制数量，默认100"`
}

// MySQLWriteQueryParams 写入参数
type MySQLWriteQueryParams struct {
	TableName   string                   `json:"tableName" jsonschema:"description=表名,required"`
	Data        []map[string]interface{} `json:"data" jsonschema:"description=要插入的数据,required"`
	OnDuplicate string                   `json:"onDuplicate,omitempty" jsonschema:"description=重复键处理方式：ignore, update, replace"`
}

// MySQLUpdateQueryParams 更新参数
type MySQLUpdateQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where" jsonschema:"description=更新条件,required"`
	Data      map[string]interface{} `json:"data" jsonschema:"description=要更新的数据,required"`
}

// MySQLDeleteQueryParams 删除参数
type MySQLDeleteQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where" jsonschema:"description=删除条件,required"`
}

// MySQLCountQueryParams 计数参数
type MySQLCountQueryParams struct {
	TableName string                 `json:"tableName" jsonschema:"description=表名,required"`
	Where     map[string]interface{} `json:"where,omitempty" jsonschema:"description=计数条件"`
}

// MySQLCreateTableParams 创建表参数
type MySQLCreateTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
	SQL       string `json:"sql" jsonschema:"description=创建表的SQL语句,required"`
	IfExists  bool   `json:"ifExists,omitempty" jsonschema:"description=如果表存在是否跳过，默认false"`
	Engine    string `json:"engine,omitempty" jsonschema:"description=存储引擎，默认InnoDB"`
	Charset   string `json:"charset,omitempty" jsonschema:"description=字符集，默认utf8mb4"`
	Collate   string `json:"collate,omitempty" jsonschema:"description=排序规则，默认utf8mb4_unicode_ci"`
}

// MySQLAlterTableParams 修改表参数
type MySQLAlterTableParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
	SQL       string `json:"sql" jsonschema:"description=修改表的SQL语句,required"`
}

// MySQLShowIndexesParams 显示索引参数
type MySQLShowIndexesParams struct {
	TableName string `json:"tableName" jsonschema:"description=表名,required"`
}

// MySQLCreateIndexParams 创建索引参数
type MySQLCreateIndexParams struct {
	TableName string   `json:"tableName" jsonschema:"description=表名,required"`
	IndexName string   `json:"indexName" jsonschema:"description=索引名称,required"`
	Columns   []string `json:"columns" jsonschema:"description=索引列名列表,required"`
	IndexType string   `json:"indexType,omitempty" jsonschema:"description=索引类型：BTREE, HASH, FULLTEXT, SPATIAL"`
	Unique    bool     `json:"unique,omitempty" jsonschema:"description=是否为唯一索引"`
}

// NewMySQLListDatabasesTool 创建列出数据库工具实例
func NewMySQLListDatabasesTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLListDatabasesTool{db: db}
	name := "mysql_list_databases"
	desc := "列出MySQL实例中的所有数据库。"
	t, _ := utils.InferTool(name, desc, this.listDatabases)
	return t
}

// NewMySQLListTablesTool 创建列出表工具实例
func NewMySQLListTablesTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLListTablesTool{db: db}
	name := "mysql_list_tables"
	desc := "列出指定数据库中的所有表，包括表的存储引擎、字符集等信息。"
	t, _ := utils.InferTool(name, desc, this.listTables)
	return t
}

// NewMySQLDescribeTableTool 创建描述表工具实例
func NewMySQLDescribeTableTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLDescribeTableTool{db: db}
	name := "mysql_describe_table"
	desc := "获取表的结构信息，包括列名、数据类型、约束、默认值等详细信息。"
	t, _ := utils.InferTool(name, desc, this.describeTable)
	return t
}

// NewMySQLReadQueryTool 创建查询数据工具实例
func NewMySQLReadQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLReadQueryTool{db: db}
	name := "mysql_read_query"
	desc := "执行SELECT查询并返回结果。支持参数化查询和结果限制。"
	t, _ := utils.InferTool(name, desc, this.readQuery)
	return t
}

// NewMySQLWriteQueryTool 创建写入数据工具实例
func NewMySQLWriteQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLWriteQueryTool{db: db}
	name := "mysql_write_query"
	desc := "向指定表插入数据。支持批量插入和重复键处理策略。"
	t, _ := utils.InferTool(name, desc, this.writeQuery)
	return t
}

// NewMySQLUpdateQueryTool 创建更新数据工具实例
func NewMySQLUpdateQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLUpdateQueryTool{db: db}
	name := "mysql_update_query"
	desc := "根据条件更新表中的数据。"
	t, _ := utils.InferTool(name, desc, this.updateQuery)
	return t
}

// NewMySQLDeleteQueryTool 创建删除数据工具实例
func NewMySQLDeleteQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLDeleteQueryTool{db: db}
	name := "mysql_delete_query"
	desc := "根据条件删除表中的数据。"
	t, _ := utils.InferTool(name, desc, this.deleteQuery)
	return t
}

// NewMySQLCountQueryTool 创建计数查询工具实例
func NewMySQLCountQueryTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLCountQueryTool{db: db}
	name := "mysql_count_query"
	desc := "统计表中符合条件的记录数量。"
	t, _ := utils.InferTool(name, desc, this.countQuery)
	return t
}

// NewMySQLCreateTableTool 创建表工具实例
func NewMySQLCreateTableTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLCreateTableTool{db: db}
	name := "mysql_create_table"
	desc := "创建新的数据表，支持指定存储引擎、字符集等MySQL特性。"
	t, _ := utils.InferTool(name, desc, this.createTable)
	return t
}

// NewMySQLAlterTableTool 创建修改表工具实例
func NewMySQLAlterTableTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLAlterTableTool{db: db}
	name := "mysql_alter_table"
	desc := "修改现有数据表的结构。"
	t, _ := utils.InferTool(name, desc, this.alterTable)
	return t
}

// NewMySQLShowIndexesTool 创建显示索引工具实例
func NewMySQLShowIndexesTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLShowIndexesTool{db: db}
	name := "mysql_show_indexes"
	desc := "显示表的所有索引信息。"
	t, _ := utils.InferTool(name, desc, this.showIndexes)
	return t
}

// NewMySQLCreateIndexTool 创建索引工具实例
func NewMySQLCreateIndexTool(db *gorm.DB) tool.InvokableTool {
	this := &MySQLCreateIndexTool{db: db}
	name := "mysql_create_index"
	desc := "为表创建索引，支持普通索引、唯一索引、全文索引等。"
	t, _ := utils.InferTool(name, desc, this.createIndex)
	return t
}

// listDatabases 列出数据库
func (t *MySQLListDatabasesTool) listDatabases(ctx context.Context, params MySQLListDatabasesParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	var databases []map[string]interface{}
	query := `
		SELECT
			SCHEMA_NAME as name,
			DEFAULT_CHARACTER_SET_NAME as charset,
			DEFAULT_COLLATION_NAME as collation
		FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
		ORDER BY SCHEMA_NAME
	`

	rows, err := t.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询数据库列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, charset, collation string
		if err := rows.Scan(&name, &charset, &collation); err != nil {
			return nil, fmt.Errorf("扫描数据库信息失败: %w", err)
		}
		databases = append(databases, map[string]interface{}{
			"name":      name,
			"charset":   charset,
			"collation": collation,
		})
	}

	return map[string]interface{}{
		"operation": "mysql_list_databases",
		"databases": databases,
		"count":     len(databases),
		"success":   true,
	}, nil
}

// listTables 列出表
func (t *MySQLListTablesTool) listTables(ctx context.Context, params MySQLListTablesParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	var tables []map[string]interface{}
	query := `
		SELECT
			TABLE_NAME as name,
			TABLE_SCHEMA as database_name,
			ENGINE as engine,
			TABLE_ROWS as rows,
			DATA_LENGTH as data_length,
			INDEX_LENGTH as index_length,
			TABLE_COLLATION as collation,
			TABLE_COMMENT as comment
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME
	`

	if params.Database != "" {
		query = `
			SELECT
				TABLE_NAME as name,
				TABLE_SCHEMA as database_name,
				ENGINE as engine,
				TABLE_ROWS as rows,
				DATA_LENGTH as data_length,
				INDEX_LENGTH as index_length,
				TABLE_COLLATION as collation,
				TABLE_COMMENT as comment
			FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = ?
			ORDER BY TABLE_NAME
		`
	}

	var rows *sql.Rows
	var err error
	if params.Database != "" {
		rows, err = t.db.WithContext(ctx).Raw(query, params.Database).Rows()
	} else {
		rows, err = t.db.WithContext(ctx).Raw(query).Rows()
	}

	if err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, databaseName, engine, collation, comment string
		var tableRows, dataLength, indexLength interface{}

		if err := rows.Scan(&name, &databaseName, &engine, &tableRows, &dataLength, &indexLength, &collation, &comment); err != nil {
			return nil, fmt.Errorf("扫描表信息失败: %w", err)
		}

		tables = append(tables, map[string]interface{}{
			"name":         name,
			"database":     databaseName,
			"engine":       engine,
			"rows":         tableRows,
			"data_length":  dataLength,
			"index_length": indexLength,
			"collation":    collation,
			"comment":      comment,
		})
	}

	return map[string]interface{}{
		"operation": "mysql_list_tables",
		"tables":    tables,
		"count":     len(tables),
		"success":   true,
	}, nil
}

// describeTable 描述表结构
func (t *MySQLDescribeTableTool) describeTable(ctx context.Context, params MySQLDescribeTableParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	var columns []map[string]interface{}
	query := `
		SELECT
			COLUMN_NAME as name,
			DATA_TYPE as type,
			IS_NULLABLE as nullable,
			COLUMN_DEFAULT as default_value,
			CHARACTER_MAXIMUM_LENGTH as max_length,
			NUMERIC_PRECISION as precision,
			NUMERIC_SCALE as scale,
			COLUMN_KEY as key_type,
			EXTRA as extra,
			COLUMN_COMMENT as comment
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := t.db.WithContext(ctx).Raw(query, params.TableName).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询表结构失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, dataType, nullable, keyType, extra, comment string
		var defaultValue, maxLength, precision, scale interface{}

		if err := rows.Scan(&name, &dataType, &nullable, &defaultValue,
			&maxLength, &precision, &scale, &keyType, &extra, &comment); err != nil {
			return nil, fmt.Errorf("扫描列信息失败: %w", err)
		}

		columns = append(columns, map[string]interface{}{
			"name":       name,
			"type":       dataType,
			"nullable":   nullable == "YES",
			"default":    defaultValue,
			"max_length": maxLength,
			"precision":  precision,
			"scale":      scale,
			"key_type":   keyType,
			"extra":      extra,
			"comment":    comment,
		})
	}

	return map[string]interface{}{
		"operation":  "mysql_describe_table",
		"table_name": params.TableName,
		"columns":    columns,
		"success":    true,
	}, nil
}

// readQuery 执行查询
func (t *MySQLReadQueryTool) readQuery(ctx context.Context, params MySQLReadQueryParams) (interface{}, error) {
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
		"operation": "mysql_read_query",
		"query":     params.Query,
		"results":   results,
		"count":     len(results),
		"success":   true,
	}, nil
}

// writeQuery 写入数据
func (t *MySQLWriteQueryTool) writeQuery(ctx context.Context, params MySQLWriteQueryParams) (interface{}, error) {
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
			var result *gorm.DB

			switch strings.ToLower(params.OnDuplicate) {
			case "ignore":
				// MySQL特有的INSERT IGNORE语法
				result = tx.Table(params.TableName).Clauses(clause.Insert{Modifier: "IGNORE"}).Create(row)
			case "replace":
				// MySQL特有的REPLACE INTO语法
				result = tx.Table(params.TableName).Clauses(clause.Insert{Modifier: "REPLACE"}).Create(row)
			case "update":
				// ON DUPLICATE KEY UPDATE语法
				result = tx.Table(params.TableName).Clauses(clause.OnConflict{
					UpdateAll: true,
				}).Create(row)
			default:
				result = tx.Table(params.TableName).Create(row)
			}

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
		"operation":      "mysql_write_query",
		"table_name":     params.TableName,
		"inserted_count": insertedCount,
		"on_duplicate":   params.OnDuplicate,
		"success":        true,
		"message":        fmt.Sprintf("成功插入 %d 条记录", insertedCount),
	}, nil
}

// updateQuery 更新数据
func (t *MySQLUpdateQueryTool) updateQuery(ctx context.Context, params MySQLUpdateQueryParams) (interface{}, error) {
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
		"operation":     "mysql_update_query",
		"table_name":    params.TableName,
		"updated_count": result.RowsAffected,
		"success":       true,
		"message":       fmt.Sprintf("成功更新 %d 条记录", result.RowsAffected),
	}, nil
}

// deleteQuery 删除数据
func (t *MySQLDeleteQueryTool) deleteQuery(ctx context.Context, params MySQLDeleteQueryParams) (interface{}, error) {
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
		"operation":     "mysql_delete_query",
		"table_name":    params.TableName,
		"deleted_count": result.RowsAffected,
		"success":       true,
		"message":       fmt.Sprintf("成功删除 %d 条记录", result.RowsAffected),
	}, nil
}

// countQuery 计数查询
func (t *MySQLCountQueryTool) countQuery(ctx context.Context, params MySQLCountQueryParams) (interface{}, error) {
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
		"operation":  "mysql_count_query",
		"table_name": params.TableName,
		"count":      count,
		"success":    true,
	}, nil
}

// createTable 创建表
func (t *MySQLCreateTableTool) createTable(ctx context.Context, params MySQLCreateTableParams) (interface{}, error) {
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
		checkQuery := "SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?"
		if err := t.db.WithContext(ctx).Raw(checkQuery, params.TableName).Scan(&exists).Error; err != nil {
			return nil, fmt.Errorf("检查表是否存在失败: %w", err)
		}

		if exists {
			return map[string]interface{}{
				"operation":  "mysql_create_table",
				"table_name": params.TableName,
				"success":    true,
				"message":    "表已存在，跳过创建",
				"skipped":    true,
			}, nil
		}
	}

	// 构建完整的创建表语句
	sql := params.SQL

	// 添加存储引擎、字符集等MySQL特性
	if params.Engine != "" || params.Charset != "" || params.Collate != "" {
		var options []string

		engine := params.Engine
		if engine == "" {
			engine = "InnoDB"
		}
		options = append(options, fmt.Sprintf("ENGINE=%s", engine))

		charset := params.Charset
		if charset == "" {
			charset = "utf8mb4"
		}
		options = append(options, fmt.Sprintf("DEFAULT CHARSET=%s", charset))

		collate := params.Collate
		if collate == "" {
			collate = "utf8mb4_unicode_ci"
		}
		options = append(options, fmt.Sprintf("COLLATE=%s", collate))

		// 如果SQL语句中没有包含这些选项，则添加
		sqlUpper := strings.ToUpper(sql)
		if !strings.Contains(sqlUpper, "ENGINE=") && !strings.Contains(sqlUpper, "DEFAULT CHARSET=") {
			sql = fmt.Sprintf("%s %s", sql, strings.Join(options, " "))
		}
	}

	if err := t.db.WithContext(ctx).Exec(sql).Error; err != nil {
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	return map[string]interface{}{
		"operation":  "mysql_create_table",
		"table_name": params.TableName,
		"engine":     params.Engine,
		"charset":    params.Charset,
		"collate":    params.Collate,
		"success":    true,
		"message":    "表创建成功",
	}, nil
}

// alterTable 修改表
func (t *MySQLAlterTableTool) alterTable(ctx context.Context, params MySQLAlterTableParams) (interface{}, error) {
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
		"operation":  "mysql_alter_table",
		"table_name": params.TableName,
		"success":    true,
		"message":    "表修改成功",
	}, nil
}

// showIndexes 显示索引
func (t *MySQLShowIndexesTool) showIndexes(ctx context.Context, params MySQLShowIndexesParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	var indexes []map[string]interface{}
	query := "SHOW INDEX FROM " + params.TableName

	rows, err := t.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询索引失败: %w", err)
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
			return nil, fmt.Errorf("扫描索引信息失败: %w", err)
		}

		index := make(map[string]interface{})
		for i, col := range columns {
			index[strings.ToLower(col)] = values[i]
		}
		indexes = append(indexes, index)
	}

	return map[string]interface{}{
		"operation":  "mysql_show_indexes",
		"table_name": params.TableName,
		"indexes":    indexes,
		"count":      len(indexes),
		"success":    true,
	}, nil
}

// createIndex 创建索引
func (t *MySQLCreateIndexTool) createIndex(ctx context.Context, params MySQLCreateIndexParams) (interface{}, error) {
	if t.db == nil {
		return nil, fmt.Errorf("数据库连接未初始化")
	}

	if params.TableName == "" {
		return nil, fmt.Errorf("表名不能为空")
	}

	if params.IndexName == "" {
		return nil, fmt.Errorf("索引名称不能为空")
	}

	if len(params.Columns) == 0 {
		return nil, fmt.Errorf("索引列不能为空")
	}

	// 构建索引语句
	var sqlBuilder strings.Builder
	sqlBuilder.WriteString("CREATE ")

	if params.Unique {
		sqlBuilder.WriteString("UNIQUE ")
	}

	if params.IndexType == "FULLTEXT" {
		sqlBuilder.WriteString("FULLTEXT ")
	} else if params.IndexType == "SPATIAL" {
		sqlBuilder.WriteString("SPATIAL ")
	}

	sqlBuilder.WriteString("INDEX ")
	sqlBuilder.WriteString(params.IndexName)
	sqlBuilder.WriteString(" ON ")
	sqlBuilder.WriteString(params.TableName)
	sqlBuilder.WriteString(" (")
	sqlBuilder.WriteString(strings.Join(params.Columns, ", "))
	sqlBuilder.WriteString(")")

	// 添加索引类型（BTREE, HASH等）
	if params.IndexType != "" && params.IndexType != "FULLTEXT" && params.IndexType != "SPATIAL" {
		sqlBuilder.WriteString(" USING ")
		sqlBuilder.WriteString(params.IndexType)
	}

	sql := sqlBuilder.String()

	if err := t.db.WithContext(ctx).Exec(sql).Error; err != nil {
		return nil, fmt.Errorf("创建索引失败: %w", err)
	}

	return map[string]interface{}{
		"operation":  "mysql_create_index",
		"table_name": params.TableName,
		"index_name": params.IndexName,
		"columns":    params.Columns,
		"index_type": params.IndexType,
		"unique":     params.Unique,
		"success":    true,
		"message":    "索引创建成功",
	}, nil
}
