package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/CoolBanHub/aggo/utils"
	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/schema"
)

// postgresDocumentToDocument 将PostgreSQL文档转换为知识库文档
func (p *Postgres) postgresDocumentToDocument(pgDoc *PostgresVectorDocument) (*schema.Document, error) {
	// 反序列化metadata
	var metadata map[string]interface{}
	if pgDoc.Metadata != "" {
		if err := sonic.Unmarshal([]byte(pgDoc.Metadata), &metadata); err != nil {
			return nil, fmt.Errorf("反序列化元数据失败: %w", err)
		}
	}

	// 解析向量
	vector, err := utils.StringToVector(pgDoc.Vector)
	if err != nil {
		return nil, fmt.Errorf("解析向量失败: %w", err)
	}

	// 创建文档
	doc := &schema.Document{
		ID:       pgDoc.ID,
		Content:  pgDoc.Content,
		MetaData: metadata,
	}

	// 设置向量和时间信息
	if len(vector) > 0 {
		doc.WithDenseVector(utils.Float32ToFloat64(vector))
	}
	doc.WithDSLInfo(map[string]any{
		"created_at": pgDoc.CreatedAt,
		"updated_at": pgDoc.UpdatedAt,
	})

	return doc, nil
}

// mapToDocument 将查询结果map转换为Document
func (p *Postgres) mapToDocument(result map[string]interface{}) (*schema.Document, error) {
	doc := &schema.Document{
		ID:       result["id"].(string),
		Content:  result["content"].(string),
		MetaData: make(map[string]interface{}),
	}

	// 处理向量
	if vectorStr, ok := result["vector"].(string); ok {
		if vector, err := utils.StringToVector(vectorStr); err == nil && len(vector) > 0 {
			doc.WithDenseVector(utils.Float32ToFloat64(vector))
		}
	}

	// 处理metadata
	if metadataStr, ok := result["metadata"].(string); ok && metadataStr != "" {
		var metadata map[string]interface{}
		if err := sonic.Unmarshal([]byte(metadataStr), &metadata); err == nil {
			doc.MetaData = metadata
		}
	}

	// 处理时间字段
	dslInfo := make(map[string]any)
	if createdAt, ok := result["created_at"].(time.Time); ok {
		dslInfo["created_at"] = createdAt
	}
	if updatedAt, ok := result["updated_at"].(time.Time); ok {
		dslInfo["updated_at"] = updatedAt
	}
	if len(dslInfo) > 0 {
		doc.WithDSLInfo(dslInfo)
	}

	return doc, nil
}

// documentToPostgresDocument 将知识库文档转换为PostgreSQL文档
func (p *Postgres) documentToPostgresDocument(ctx context.Context, doc schema.Document) (*PostgresVectorDocument, error) {
	// 序列化metadata
	var metadataJSON string
	if doc.MetaData != nil {
		var err error
		metadataJSON, err = sonic.MarshalString(doc.MetaData)
		if err != nil {
			return nil, fmt.Errorf("序列化元数据失败: %w", err)
		}
	}

	// 生成向量
	vectorData, err := p.Embedding.EmbedStrings(ctx, []string{doc.Content})
	if err != nil {
		return nil, fmt.Errorf("向量化失败: %w", err)
	}
	if len(vectorData) == 0 {
		return nil, fmt.Errorf("向量化失败: 向量数据为空")
	}

	// 处理时间字段
	now := time.Now()
	dslInfo := doc.DSLInfo()
	createdAt, _ := dslInfo["created_at"].(time.Time)
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt, _ := dslInfo["updated_at"].(time.Time)
	if updatedAt.IsZero() {
		updatedAt = now
	}

	return &PostgresVectorDocument{
		ID:        doc.ID,
		Content:   doc.Content,
		Vector:    utils.Vector64ToString(vectorData[0]),
		Metadata:  metadataJSON,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
