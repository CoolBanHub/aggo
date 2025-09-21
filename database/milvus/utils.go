package milvus

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// marshalMetadata 序列化metadata为JSON字节数组
func marshalMetadata(metadata map[string]interface{}) ([]byte, error) {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	return json.Marshal(metadata)
}

// unmarshalMetadata 反序列化JSON字节数组为metadata
func unmarshalMetadata(data []byte) (map[string]interface{}, error) {
	var metadata map[string]interface{}
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}
	err := json.Unmarshal(data, &metadata)
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

// buildFilterExpression 根据过滤条件构建Milvus查询表达式
func buildFilterExpression(filters map[string]interface{}) string {
	if len(filters) == 0 {
		return ""
	}

	var conditions []string
	for key, value := range filters {
		switch v := value.(type) {
		case string:
			conditions = append(conditions, fmt.Sprintf(`json_contains(metadata, '"%s":"%s"')`, key, v))
		case int, int32, int64:
			conditions = append(conditions, fmt.Sprintf(`json_extract(metadata, "$.%s") == %v`, key, v))
		case float32, float64:
			conditions = append(conditions, fmt.Sprintf(`json_extract(metadata, "$.%s") == %v`, key, v))
		case bool:
			conditions = append(conditions, fmt.Sprintf(`json_extract(metadata, "$.%s") == %t`, key, v))
		default:
			// 对于其他类型，转换为字符串处理
			conditions = append(conditions, fmt.Sprintf(`json_contains(metadata, '"%s":"%s"')`, key, fmt.Sprintf("%v", v)))
		}
	}

	return strings.Join(conditions, " and ")
}

// buildDocumentFromResult 从搜索结果构建文档对象
func (m *Milvus) buildDocumentFromResult(result milvusclient.ResultSet, index int) (*schema.Document, error) {
	id, err := result.IDs.Get(index)
	if err != nil {
		return nil, fmt.Errorf("获取ID失败: %w", err)
	}

	content, err := getColumnValue(result, "content", index)
	if err != nil {
		return nil, fmt.Errorf("获取content失败: %w", err)
	}

	metadataBytes, err := getColumnValue(result, "metadata", index)
	if err != nil {
		return nil, fmt.Errorf("获取metadata失败: %w", err)
	}

	// 解析metadata
	metadata, err := unmarshalMetadata(metadataBytes.([]byte))
	if err != nil {
		return nil, fmt.Errorf("解析metadata失败: %w", err)
	}

	// 获取向量数据（如果存在）
	var vector []float32
	if vectorValue, err := getColumnValue(result, "vector", index); err == nil {
		if v, ok := vectorValue.([]float32); ok {
			vector = v
		}
	}

	doc := &schema.Document{
		ID:       id.(string),
		Content:  content.(string),
		MetaData: metadata,
	}

	// 设置向量数据
	if len(vector) > 0 {
		doc.WithDenseVector(utils.Float32ToFloat64(vector))
	}

	// 添加DSL时间字段
	dslInfo := make(map[string]any)
	if createdAtValue, err := getColumnValue(result, "created_at", index); err == nil {
		if createdAtInt, ok := createdAtValue.(int64); ok {
			dslInfo["created_at"] = time.Unix(createdAtInt, 0)
		}
	}
	if updatedAtValue, err := getColumnValue(result, "updated_at", index); err == nil {
		if updatedAtInt, ok := updatedAtValue.(int64); ok {
			dslInfo["updated_at"] = time.Unix(updatedAtInt, 0)
		}
	}
	if len(dslInfo) > 0 {
		doc.WithDSLInfo(dslInfo)
	}
	return doc, nil
}

// getColumnValue 从结果集中获取列值
func getColumnValue(result milvusclient.ResultSet, columnName string, index int) (interface{}, error) {
	column := result.GetColumn(columnName)
	if column == nil {
		return nil, fmt.Errorf("列 %s 不存在", columnName)
	}

	value, err := column.Get(index)
	if err != nil {
		return nil, fmt.Errorf("获取列 %s 第 %d 行数据失败: %w", columnName, index, err)
	}

	return value, nil
}
