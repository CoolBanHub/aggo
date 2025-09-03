package storage

// TableNameProvider provides table names with configurable prefix
type TableNameProvider struct {
	tablePrefix string
}

// NewTableNameProvider creates a new table name provider with the given prefix
func NewTableNameProvider(prefix string) *TableNameProvider {
	if prefix == "" {
		prefix = "aggo_knowledge" // default prefix
	}
	return &TableNameProvider{tablePrefix: prefix}
}

// GetDocumentTableName 文档表名
func (p *TableNameProvider) GetDocumentTableName() string {
	return p.tablePrefix + "_documents"
}
