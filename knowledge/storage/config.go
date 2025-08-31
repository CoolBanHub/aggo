package storage

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseType 数据库类型
type DatabaseType string

const (
	MySQL      DatabaseType = "mysql"
	PostgreSQL DatabaseType = "postgres"
	SQLite     DatabaseType = "sqlite"
)

// Config 数据库配置
type Config struct {
	// 数据库类型：mysql, postgres, sqlite
	Type DatabaseType `json:"type" yaml:"type"`

	// 数据库连接信息
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Database string `json:"database" yaml:"database"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`

	// SQLite 专用：文件路径
	FilePath string `json:"filePath" yaml:"filePath"`

	// 连接池配置
	MaxOpenConns int `json:"maxOpenConns" yaml:"maxOpenConns"`
	MaxIdleConns int `json:"maxIdleConns" yaml:"maxIdleConns"`

	// GORM 配置
	DisableAutomaticPing bool `json:"disableAutomaticPing" yaml:"disableAutomaticPing"`
	LogLevel             int  `json:"logLevel" yaml:"logLevel"` // 1: Silent, 2: Error, 3: Warn, 4: Info
}

// DSN 生成数据源名称
func (c *Config) DSN() string {
	switch c.Type {
	case MySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			c.Username, c.Password, c.Host, c.Port, c.Database)
	case PostgreSQL:
		return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai",
			c.Host, c.Username, c.Password, c.Database, c.Port)
	case SQLite:
		if c.FilePath == "" {
			return "knowledge.db"
		}
		return c.FilePath
	default:
		return ""
	}
}

// NewGORMDB 创建 GORM 数据库连接
func (c *Config) NewGORMDB() (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch c.Type {
	case MySQL:
		dialector = mysql.Open(c.DSN())
	case PostgreSQL:
		dialector = postgres.Open(c.DSN())
	case SQLite:
		dialector = sqlite.Open(c.DSN())
	default:
		return nil, fmt.Errorf("unsupported database type: %s", c.Type)
	}

	// GORM 配置
	config := &gorm.Config{
		DisableAutomaticPing: c.DisableAutomaticPing,
	}

	// 设置日志级别
	if c.LogLevel > 0 {
		switch c.LogLevel {
		case 1:
			config.Logger = logger.Default.LogMode(logger.Silent)
		case 2:
			config.Logger = logger.Default.LogMode(logger.Error)
		case 3:
			config.Logger = logger.Default.LogMode(logger.Warn)
		case 4:
			config.Logger = logger.Default.LogMode(logger.Info)
		}
	}

	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	if c.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	}
	if c.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	}

	return db, nil
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Type:                 SQLite,
		FilePath:             "knowledge.db",
		MaxOpenConns:         25,
		MaxIdleConns:         5,
		DisableAutomaticPing: false,
		LogLevel:             3, // Warn
	}
}

// NewMySQLConfig 创建 MySQL 配置
func NewMySQLConfig(host string, port int, database, username, password string) *Config {
	config := DefaultConfig()
	config.Type = MySQL
	config.Host = host
	config.Port = port
	config.Database = database
	config.Username = username
	config.Password = password
	return config
}

// NewPostgreSQLConfig 创建 PostgreSQL 配置
func NewPostgreSQLConfig(host string, port int, database, username, password string) *Config {
	config := DefaultConfig()
	config.Type = PostgreSQL
	config.Host = host
	config.Port = port
	config.Database = database
	config.Username = username
	config.Password = password
	return config
}

// NewSQLiteConfig 创建 SQLite 配置
func NewSQLiteConfig(filePath string) *Config {
	config := DefaultConfig()
	config.Type = SQLite
	config.FilePath = filePath
	return config
}
