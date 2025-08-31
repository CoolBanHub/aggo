package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQLDialect SQL数据库方言类型
type SQLDialect string

const (
	// DialectMySQL MySQL方言
	DialectMySQL SQLDialect = "mysql"
	// DialectPostgreSQL PostgreSQL方言
	DialectPostgreSQL SQLDialect = "postgres"
	// DialectSQLite SQLite方言
	DialectSQLite SQLDialect = "sqlite"
)

// SQLStore 通用SQL存储实现
// 支持MySQL、PostgreSQL和SQLite
type SQLStore struct {
	db      *gorm.DB
	config  *SQLConfig
	dialect SQLDialect
}

// SQLConfig SQL数据库配置选项
type SQLConfig struct {
	// 数据库方言 (mysql/postgres/sqlite)
	Dialect SQLDialect `json:"dialect"`
	// 连接DSN (Data Source Name)
	// MySQL: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	// PostgreSQL: host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=Asia/Shanghai
	// SQLite: file:test.db?cache=shared&mode=rwc
	DSN string `json:"dsn"`
	// 最大空闲连接数
	MaxIdleConns int `json:"maxIdleConns"`
	// 最大打开连接数
	MaxOpenConns int `json:"maxOpenConns"`
	// 连接最大生存时间
	ConnMaxLifetime time.Duration `json:"connMaxLifetime"`
	// 是否启用日志
	EnableLog bool `json:"enableLog"`
	// 日志级别
	LogLevel logger.LogLevel `json:"logLevel"`
}

// DefaultSQLConfig 默认SQL配置
func DefaultSQLConfig(dialect SQLDialect) *SQLConfig {
	config := &SQLConfig{
		Dialect:         dialect,
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: time.Hour,
		EnableLog:       true,
		LogLevel:        logger.Info,
	}

	switch dialect {
	case DialectMySQL:
		config.DSN = "user:password@tcp(localhost:3306)/aggo?charset=utf8mb4&parseTime=True&loc=Local"
	case DialectPostgreSQL:
		config.DSN = "host=localhost user=aggo password=aggo dbname=aggo port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	case DialectSQLite:
		config.DSN = "file:aggo.db?cache=shared&mode=rwc"
	}

	return config
}

// NewSQLStore 创建新的SQL存储实例
func NewSQLStore(config *SQLConfig) (*SQLStore, error) {
	if config == nil {
		return nil, errors.New("配置不能为空")
	}
	if config.Dialect == "" {
		return nil, errors.New("数据库方言不能为空")
	}

	// 配置GORM日志
	var gormLogger logger.Interface
	if config.EnableLog {
		gormLogger = logger.Default.LogMode(config.LogLevel)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	// 根据方言选择驱动
	var dialector gorm.Dialector
	switch config.Dialect {
	case DialectMySQL:
		dialector = mysql.Open(config.DSN)
	case DialectPostgreSQL:
		dialector = postgres.Open(config.DSN)
	case DialectSQLite:
		dialector = sqlite.Open(config.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库方言: %s", config.Dialect)
	}

	// 连接数据库
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("%s连接失败: %v", config.Dialect, err)
	}

	// 获取底层sql.DB以配置连接池（SQLite不支持连接池）
	if config.Dialect != DialectSQLite {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("获取SQL数据库实例失败: %v", err)
		}

		// 设置连接池参数
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
		sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)

		// 测试连接
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("%s连接测试失败: %v", config.Dialect, err)
		}
	}

	store := &SQLStore{
		db:      db,
		config:  config,
		dialect: config.Dialect,
	}

	// 自动迁移表结构
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %v", err)
	}

	return store, nil
}

// NewSQLStoreWithDB 使用现有的 GORM 实例创建SQL存储实例
// 这允许多个存储实例共享同一个数据库连接
func NewSQLStoreWithDB(db *gorm.DB, dialect SQLDialect) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance cannot be nil")
	}

	if dialect == "" {
		return nil, fmt.Errorf("dialect cannot be empty")
	}

	// 创建默认配置，但不包含连接相关的配置
	config := &SQLConfig{
		Dialect:         dialect,
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: time.Hour,
		EnableLog:       true,
		LogLevel:        logger.Info,
	}

	store := &SQLStore{
		db:      db,
		config:  config,
		dialect: dialect,
	}

	// 自动迁移表结构
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("数据库迁移失败: %v", err)
	}

	return store, nil
}

// migrate 自动迁移表结构
func (s *SQLStore) migrate() error {
	return s.db.AutoMigrate(
		&UserMemoryModel{},
		&SessionSummaryModel{},
		&ConversationMessageModel{},
	)
}

// SaveUserMemory 保存用户记忆
func (s *SQLStore) SaveUserMemory(ctx context.Context, memory *memory.UserMemory) error {
	if memory == nil {
		return errors.New("记忆对象不能为空")
	}
	if memory.UserID == "" {
		return errors.New("用户ID不能为空")
	}
	if memory.Memory == "" {
		return errors.New("记忆内容不能为空")
	}

	// 如果没有ID，生成一个
	if memory.ID == "" {
		memory.ID = fmt.Sprintf("mem_%d_%s", time.Now().UnixNano(), memory.UserID[:min(8, len(memory.UserID))])
	}

	// 设置时间戳
	now := time.Now()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now

	// 转换为数据库模型
	model := &UserMemoryModel{}
	model.FromUserMemory(memory)

	// 保存到数据库
	if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("保存记忆到%s失败: %v", s.dialect, err)
	}

	return nil
}

// GetUserMemories 获取用户的记忆列表
func (s *SQLStore) GetUserMemories(ctx context.Context, userID string, limit int, retrieval memory.MemoryRetrieval) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var models []UserMemoryModel
	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	if limit > 0 {
		// 根据检索方式确定排序和限制
		switch retrieval {
		case memory.RetrievalLastN:
			query = query.Order("updated_at DESC")
		case memory.RetrievalFirstN:
			query = query.Order("created_at ASC")
		default:
			query = query.Order("updated_at DESC")
		}
		query = query.Limit(limit)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("获取用户记忆失败: %v", err)
	}

	// 转换为业务模型
	var memories []*memory.UserMemory
	for _, model := range models {
		memories = append(memories, model.ToUserMemory())
	}

	return memories, nil
}

// UpdateUserMemory 更新用户记忆
func (s *SQLStore) UpdateUserMemory(ctx context.Context, userMemory *memory.UserMemory) error {
	if userMemory == nil {
		return errors.New("记忆对象不能为空")
	}
	if userMemory.ID == "" {
		return errors.New("记忆ID不能为空")
	}
	if userMemory.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 检查记忆是否存在
	var exists bool
	if err := s.db.WithContext(ctx).Model(&UserMemoryModel{}).
		Select("count(*) > 0").Where("id = ?", userMemory.ID).
		Find(&exists).Error; err != nil {
		return fmt.Errorf("检查记忆是否存在失败: %v", err)
	}
	if !exists {
		return errors.New("记忆不存在")
	}

	// 更新时间戳
	userMemory.UpdatedAt = time.Now()

	// 转换为数据库模型
	model := &UserMemoryModel{}
	model.FromUserMemory(userMemory)

	// 更新数据库
	if err := s.db.WithContext(ctx).Save(model).Error; err != nil {
		return fmt.Errorf("更新记忆到%s失败: %v", s.dialect, err)
	}

	return nil
}

// DeleteUserMemory 删除用户记忆
func (s *SQLStore) DeleteUserMemory(ctx context.Context, memoryID string) error {
	if memoryID == "" {
		return errors.New("记忆ID不能为空")
	}

	result := s.db.WithContext(ctx).Delete(&UserMemoryModel{}, "id = ?", memoryID)
	if result.Error != nil {
		return fmt.Errorf("删除记忆失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("记忆不存在")
	}

	return nil
}

func (s *SQLStore) DeleteUserMemoriesByIds(ctx context.Context, userID string, memoryIDs []string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if len(memoryIDs) == 0 {
		return nil
	}
	result := s.db.WithContext(ctx).Delete(&UserMemoryModel{}, "user_id = ? and id in ?", userID, memoryIDs)
	if result.Error != nil {
		return fmt.Errorf("删除记忆失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("记忆不存在")
	}

	return nil
}

// ClearUserMemories 清空用户的所有记忆
func (s *SQLStore) ClearUserMemories(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&UserMemoryModel{}).Error; err != nil {
		return fmt.Errorf("清空用户记忆失败: %v", err)
	}

	return nil
}

// SearchUserMemories 搜索用户记忆（基于内容关键词搜索）
func (s *SQLStore) SearchUserMemories(ctx context.Context, userID string, query string, limit int) ([]*memory.UserMemory, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	if query == "" {
		return []*memory.UserMemory{}, nil
	}

	var models []UserMemoryModel
	searchQuery := "%" + query + "%"

	// 根据数据库类型使用不同的搜索语法
	dbQuery := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if s.dialect == DialectPostgreSQL {
		// PostgreSQL使用ILIKE进行大小写不敏感的搜索
		dbQuery = dbQuery.Where("memory ILIKE ? OR input ILIKE ?", searchQuery, searchQuery)
	} else {
		// MySQL和SQLite使用LIKE
		dbQuery = dbQuery.Where("memory LIKE ? OR input LIKE ?", searchQuery, searchQuery)
	}

	dbQuery = dbQuery.Order("updated_at DESC")

	if limit > 0 {
		dbQuery = dbQuery.Limit(limit)
	}

	if err := dbQuery.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("搜索用户记忆失败: %v", err)
	}

	// 转换为业务模型
	var memories []*memory.UserMemory
	for _, model := range models {
		memories = append(memories, model.ToUserMemory())
	}

	return memories, nil
}

// SaveSessionSummary 保存会话摘要
func (s *SQLStore) SaveSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 设置时间戳
	now := time.Now()
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = now
	}
	summary.UpdatedAt = now

	// 转换为数据库模型
	model := &SessionSummaryModel{}
	model.FromSessionSummary(summary)

	// 使用UPSERT语义
	if err := s.db.WithContext(ctx).Save(model).Error; err != nil {
		return fmt.Errorf("保存会话摘要到%s失败: %v", s.dialect, err)
	}

	return nil
}

// GetSessionSummary 获取会话摘要
func (s *SQLStore) GetSessionSummary(ctx context.Context, sessionID string, userID string) (*memory.SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var model SessionSummaryModel
	err := s.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 摘要不存在
		}
		return nil, fmt.Errorf("获取会话摘要失败: %v", err)
	}

	return model.ToSessionSummary(), nil
}

// UpdateSessionSummary 更新会话摘要
func (s *SQLStore) UpdateSessionSummary(ctx context.Context, summary *memory.SessionSummary) error {
	if summary == nil {
		return errors.New("摘要对象不能为空")
	}
	if summary.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if summary.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 检查摘要是否存在
	var exists bool
	if err := s.db.WithContext(ctx).Model(&SessionSummaryModel{}).
		Select("count(*) > 0").Where("session_id = ? AND user_id = ?", summary.SessionID, summary.UserID).
		Find(&exists).Error; err != nil {
		return fmt.Errorf("检查会话摘要是否存在失败: %v", err)
	}
	if !exists {
		return errors.New("会话摘要不存在")
	}

	// 更新时间戳
	summary.UpdatedAt = time.Now()

	// 转换为数据库模型
	model := &SessionSummaryModel{}
	model.FromSessionSummary(summary)

	// 更新数据库
	if err := s.db.WithContext(ctx).Save(model).Error; err != nil {
		return fmt.Errorf("更新会话摘要到%s失败: %v", s.dialect, err)
	}

	return nil
}

// DeleteSessionSummary 删除会话摘要
func (s *SQLStore) DeleteSessionSummary(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	result := s.db.WithContext(ctx).Delete(&SessionSummaryModel{}, "session_id = ? AND user_id = ?", sessionID, userID)
	if result.Error != nil {
		return fmt.Errorf("删除会话摘要失败: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("会话摘要不存在")
	}

	return nil
}

// SaveMessage 保存对话消息
func (s *SQLStore) SaveMessage(ctx context.Context, message *memory.ConversationMessage) error {
	if message == nil {
		return errors.New("消息对象不能为空")
	}
	if message.SessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if message.UserID == "" {
		return errors.New("用户ID不能为空")
	}

	// 如果没有ID，生成一个
	if message.ID == "" {
		message.ID = fmt.Sprintf("%s_%d", message.UserID[:min(8, len(message.UserID))], time.Now().UnixNano())
	}

	// 设置时间戳
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}

	// 转换为数据库模型
	model := &ConversationMessageModel{}
	model.FromConversationMessage(message)

	// 保存到数据库
	if err := s.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("保存消息到%s失败: %v", s.dialect, err)
	}

	return nil
}

// GetMessages 获取会话的消息历史
func (s *SQLStore) GetMessages(ctx context.Context, sessionID string, userID string, limit int) ([]*memory.ConversationMessage, error) {
	if sessionID == "" {
		return nil, errors.New("会话ID不能为空")
	}
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var models []ConversationMessageModel
	query := s.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).
		Order("created_at DESC")

	if limit > 0 {
		// 获取最新的limit条消息
		query = query.Limit(limit)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("获取消息历史失败: %v", err)
	}

	// 转换为业务模型
	var messages []*memory.ConversationMessage
	for _, model := range models {
		messages = append(messages, model.ToConversationMessage())
	}

	return messages, nil
}

// DeleteMessages 删除会话的消息历史
func (s *SQLStore) DeleteMessages(ctx context.Context, sessionID string, userID string) error {
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userID == "" {
		return errors.New("用户ID不能为空")
	}

	if err := s.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).
		Delete(&ConversationMessageModel{}).Error; err != nil {
		return fmt.Errorf("删除消息历史失败: %v", err)
	}

	return nil
}

// Close 关闭数据库连接
func (s *SQLStore) Close() error {
	if s.dialect == DialectSQLite {
		// SQLite不需要关闭连接池
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Health 检查数据库健康状态
func (s *SQLStore) Health(ctx context.Context) error {
	if s.dialect == DialectSQLite {
		// SQLite简单检查
		var result int
		return s.db.WithContext(ctx).Raw("SELECT 1").Scan(&result).Error
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// min 辅助函数，获取两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
