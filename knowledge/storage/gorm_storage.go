package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/CoolBanHub/aggo/knowledge"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// GormStorage GORM 存储实现
type GormStorage struct {
	db                *gorm.DB
	tableNameProvider *TableNameProvider
}

// NewGormStorage 使用现有的 GORM 实例创建存储实例
func NewGormStorage(db *gorm.DB) (*GormStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance cannot be nil")
	}

	storage := &GormStorage{
		db:                db,
		tableNameProvider: NewTableNameProvider("aggo_knowledge"),
	}

	// 自动迁移数据库表结构
	if err := storage.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}

	return storage, nil
}

func (gs *GormStorage) SetTablePrefix(prefix string) {
	gs.tableNameProvider = NewTableNameProvider(prefix)
}

// AutoMigrate 自动迁移数据库表结构
func (gs *GormStorage) AutoMigrate() error {
	return gs.db.Table(gs.tableNameProvider.GetDocumentTableName()).AutoMigrate(&DocumentModel{})
}

// documentToModel 将知识库文档转换为 GORM 模型（不包含向量数据）
func (gs *GormStorage) documentToModel(doc *knowledge.Document) (*DocumentModel, error) {
	model := &DocumentModel{
		ID:        doc.ID,
		Content:   doc.Content,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}

	if err := model.SetMetadata(doc.Metadata); err != nil {
		return nil, fmt.Errorf("failed to set metadata: %w", err)
	}

	return model, nil
}

// modelToDocument 将 GORM 模型转换为知识库文档（不包含向量数据）
func (gs *GormStorage) modelToDocument(model *DocumentModel) (*knowledge.Document, error) {
	metadata, err := model.GetMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	return &knowledge.Document{
		ID:        model.ID,
		Content:   model.Content,
		Metadata:  metadata,
		Vector:    nil, // 向量数据由 VectorDB 管理，Storage 不处理
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}, nil
}

// SaveDocument 保存文档
func (gs *GormStorage) SaveDocument(ctx context.Context, doc *knowledge.Document) error {
	model, err := gs.documentToModel(doc)
	if err != nil {
		return fmt.Errorf("failed to convert document to model: %w", err)
	}

	// 使用 GORM 的 Save 方法，自动处理插入或更新
	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Save(model).Error; err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// 更新文档的时间字段
	doc.CreatedAt = model.CreatedAt
	doc.UpdatedAt = model.UpdatedAt

	return nil
}

// GetDocument 获取文档
func (gs *GormStorage) GetDocument(ctx context.Context, docID string) (*knowledge.Document, error) {
	var model DocumentModel

	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Where("id = ?", docID).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("文档未找到: %s", docID)
		}
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	return gs.modelToDocument(&model)
}

// UpdateDocument 更新文档
func (gs *GormStorage) UpdateDocument(ctx context.Context, doc *knowledge.Document) error {
	// 首先检查文档是否存在
	var count int64
	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Where("id = ?", doc.ID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check document existence: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("文档未找到: %s", doc.ID)
	}

	model, err := gs.documentToModel(doc)
	if err != nil {
		return fmt.Errorf("failed to convert document to model: %w", err)
	}

	// 使用 Updates 方法更新（会自动更新 updated_at）
	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Where("id = ?", doc.ID).Updates(model).Error; err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	// 获取更新后的时间戳
	var updatedModel DocumentModel
	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Where("id = ?", doc.ID).First(&updatedModel).Error; err != nil {
		return fmt.Errorf("failed to get updated document: %w", err)
	}

	doc.UpdatedAt = updatedModel.UpdatedAt

	return nil
}

// DeleteDocument 删除文档
func (gs *GormStorage) DeleteDocument(ctx context.Context, docID string) error {
	result := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Where("id = ?", docID).Delete(&DocumentModel{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete document: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("文档未找到: %s", docID)
	}

	return nil
}

// ListDocuments 列出文档
func (gs *GormStorage) ListDocuments(ctx context.Context, limit int, offset int) ([]*knowledge.Document, error) {
	var models []DocumentModel

	query := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName())

	if limit > 0 {
		query = query.Limit(limit)
	}

	if offset > 0 {
		query = query.Offset(offset)
	}

	// 按创建时间排序
	if err := query.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	documents := make([]*knowledge.Document, len(models))
	for i, model := range models {
		doc, err := gs.modelToDocument(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert model to document: %w", err)
		}
		documents[i] = doc
	}

	return documents, nil
}

// SearchDocuments 搜索文档（基于内容的简单文本搜索）
func (gs *GormStorage) SearchDocuments(ctx context.Context, query string, limit int) ([]*knowledge.Document, error) {
	var models []DocumentModel

	// 构建搜索查询
	dbQuery := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName())

	// 根据数据库类型使用不同的搜索策略
	switch gs.db.Config.Dialector.Name() {
	case mysql.DefaultDriverName:
		// MySQL 使用 MATCH AGAINST 或 LIKE
		dbQuery = dbQuery.Where("content LIKE ?", "%"+query+"%")
	case "postgres":
		// PostgreSQL 使用 ILIKE 进行大小写不敏感搜索
		dbQuery = dbQuery.Where("content ILIKE ?", "%"+query+"%")
	case "sqlite":
		// SQLite 使用 LIKE
		dbQuery = dbQuery.Where("content LIKE ?", "%"+strings.ToLower(query)+"%")
	default:
		// 默认使用 LIKE
		dbQuery = dbQuery.Where("content LIKE ?", "%"+query+"%")
	}

	if limit > 0 {
		dbQuery = dbQuery.Limit(limit)
	}

	// 按相关性排序（这里简单按创建时间排序）
	if err := dbQuery.Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}

	documents := make([]*knowledge.Document, len(models))
	for i, model := range models {
		doc, err := gs.modelToDocument(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert model to document: %w", err)
		}
		documents[i] = doc
	}

	return documents, nil
}

// Close 关闭存储连接
func (gs *GormStorage) Close() error {
	sqlDB, err := gs.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	return sqlDB.Close()
}

// GetDB 获取底层 GORM 数据库实例（用于高级操作）
func (gs *GormStorage) GetDB() *gorm.DB {
	return gs.db
}

// Count 获取文档总数
func (gs *GormStorage) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count documents: %w", err)
	}
	return count, nil
}

// BatchSaveDocuments 批量保存文档（更高效）
func (gs *GormStorage) BatchSaveDocuments(ctx context.Context, docs []*knowledge.Document, batchSize int) error {
	if len(docs) == 0 {
		return nil
	}

	models := make([]*DocumentModel, len(docs))
	for i, doc := range docs {
		model, err := gs.documentToModel(doc)
		if err != nil {
			return fmt.Errorf("failed to convert document to model: %w", err)
		}
		models[i] = model
	}

	// 分批处理
	if batchSize <= 0 {
		batchSize = 100 // 默认批次大小
	}

	for i := 0; i < len(models); i += batchSize {
		end := i + batchSize
		if end > len(models) {
			end = len(models)
		}

		batch := models[i:end]
		if err := gs.db.WithContext(ctx).Table(gs.tableNameProvider.GetDocumentTableName()).CreateInBatches(batch, len(batch)).Error; err != nil {
			return fmt.Errorf("failed to batch save documents: %w", err)
		}
	}

	return nil
}
