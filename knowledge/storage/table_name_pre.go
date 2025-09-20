package storage

// TableNameProvider 提供可配置前缀的表名
type TableNameProvider struct {
	tablePrefix string
}

// NewTableNameProvider 创建表名提供器
func NewTableNameProvider(prefix string) *TableNameProvider {
	if prefix == "" {
		prefix = "aggo_knowledge"
	}
	return &TableNameProvider{tablePrefix: prefix}
}

// GetDocumentTableName 获取文档表名
func (p *TableNameProvider) GetDocumentTableName() string {
	return p.tablePrefix + "_documents"
}
