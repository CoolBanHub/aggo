package storage

import (
	"fmt"

	"gorm.io/gorm"
)

const (
	// DialectMySQL MySQL方言
	DialectMySQL string = "mysql"
	// DialectPostgreSQL PostgreSQL方言
	DialectPostgreSQL string = "postgres"
	// DialectSQLite SQLite方言
	DialectSQLite string = "sqlite"
)

// SQLStore 通用SQL存储实现
// 支持MySQL、PostgreSQL和SQLite
type SQLStore struct {
	db                *gorm.DB
	tableNameProvider *TableNameProvider
}

// NewGormStorage 创建新的SQL存储实例
func NewGormStorage(db *gorm.DB) (*SQLStore, error) {
	return NewGormStorageWithPrefix(db, "")
}

// NewGormStorageWithPrefix 创建带自定义表名前缀的 SQL 存储实例。
// prefix 为空时使用默认值 "aggo_mem"。
func NewGormStorageWithPrefix(db *gorm.DB, prefix string) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance cannot be nil")
	}
	store := &SQLStore{
		db:                db,
		tableNameProvider: NewTableNameProvider(prefix),
	}
	return store, nil
}

// AutoMigrate 自动迁移表结构
func (s *SQLStore) AutoMigrate() error {
	// 使用实例的表名提供器来指定表名
	if err := s.db.Table(s.tableNameProvider.GetUserMemoryTableName()).AutoMigrate(&UserMemoryModel{}); err != nil {
		return err
	}
	if err := s.db.Table(s.tableNameProvider.GetSessionSummaryTableName()).AutoMigrate(&SessionSummaryModel{}); err != nil {
		return err
	}
	if err := s.db.Table(s.tableNameProvider.GetConversationMessageTableName()).AutoMigrate(&ConversationMessageModel{}); err != nil {
		return err
	}
	if err := s.db.Table(s.tableNameProvider.GetUserMemoryEventTableName()).AutoMigrate(&UserMemoryEventModel{}); err != nil {
		return err
	}
	return nil
}

// Close 关闭数据库连接
func (s *SQLStore) Close() error {
	if s.db.Config.Dialector.Name() == DialectSQLite {
		// SQLite不需要关闭连接池
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *SQLStore) ConversationDB() *gorm.DB {
	return s.db
}

func (s *SQLStore) ConversationMessageTableName() string {
	return s.tableNameProvider.GetConversationMessageTableName()
}
