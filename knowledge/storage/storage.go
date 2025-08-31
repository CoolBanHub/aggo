package storage

import (
	"fmt"

	"github.com/CoolBanHub/aggo/knowledge"
	"gorm.io/gorm"
)

// NewStorage 根据配置创建存储实例的便捷函数
func NewStorage(config *Config) (knowledge.KnowledgeStorage, error) {
	switch config.Type {
	case MySQL, PostgreSQL, SQLite:
		return NewGormStorage(config)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// NewSQLiteStorage 创建 SQLite 存储的便捷函数
func NewSQLiteStorage(filePath string) (knowledge.KnowledgeStorage, error) {
	config := NewSQLiteConfig(filePath)
	return NewGormStorage(config)
}

// NewMySQLStorage 创建 MySQL 存储的便捷函数
func NewMySQLStorage(host string, port int, database, username, password string) (knowledge.KnowledgeStorage, error) {
	config := NewMySQLConfig(host, port, database, username, password)
	return NewGormStorage(config)
}

// NewPostgresSQLStorage 创建 PostgreSQL 存储的便捷函数
func NewPostgresSQLStorage(host string, port int, database, username, password string) (knowledge.KnowledgeStorage, error) {
	config := NewPostgreSQLConfig(host, port, database, username, password)
	return NewGormStorage(config)
}

// NewStorageWithSharedDB 使用共享的 GORM 实例创建知识库存储
// 这允许与其他模块共享同一个数据库连接
func NewStorageWithSharedDB(db *gorm.DB, config *Config) (knowledge.KnowledgeStorage, error) {
	return NewGormStorageWithDB(db, config)
}

// StorageOptions 存储选项
type StorageOptions struct {
	MaxOpenConns         int
	MaxIdleConns         int
	DisableAutomaticPing bool
	LogLevel             int
}

// NewStorageWithOptions 根据配置和选项创建存储实例
func NewStorageWithOptions(config *Config, options *StorageOptions) (knowledge.KnowledgeStorage, error) {
	if options != nil {
		if options.MaxOpenConns > 0 {
			config.MaxOpenConns = options.MaxOpenConns
		}
		if options.MaxIdleConns > 0 {
			config.MaxIdleConns = options.MaxIdleConns
		}
		config.DisableAutomaticPing = options.DisableAutomaticPing
		if options.LogLevel > 0 {
			config.LogLevel = options.LogLevel
		}
	}

	return NewStorage(config)
}
