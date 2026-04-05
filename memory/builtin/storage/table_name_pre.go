package storage

// TableNameProvider provides table names with configurable prefix
type TableNameProvider struct {
	tablePrefix string
}

// NewTableNameProvider creates a new table name provider with the given prefix
func NewTableNameProvider(prefix string) *TableNameProvider {
	if prefix == "" {
		prefix = "aggo_mem" // default prefix
	}
	return &TableNameProvider{tablePrefix: prefix}
}

// GetUserMemoryTableName returns the table name for user memories
func (p *TableNameProvider) GetUserMemoryTableName() string {
	return p.tablePrefix + "_user_memories"
}

// GetSessionSummaryTableName returns the table name for session summaries
func (p *TableNameProvider) GetSessionSummaryTableName() string {
	return p.tablePrefix + "_session_summaries"
}

// GetConversationMessageTableName returns the table name for conversation messages
func (p *TableNameProvider) GetConversationMessageTableName() string {
	return p.tablePrefix + "_conversation_messages"
}
