package vectordb

import (
	"encoding/json"
	"fmt"
	"strings"

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

func Float64ToFloat32(src []float64) []float32 {
	if src == nil {
		return nil
	}

	dst := make([]float32, len(src))
	for i, v := range src {
		dst[i] = float32(v)
	}
	return dst
}

func Float32ToFloat64(src []float32) []float64 {
	if src == nil {
		return nil
	}

	dst := make([]float64, len(src))
	for i, v := range src {
		dst[i] = float64(v)
	}
	return dst
}

// vectorToString 将float32向量转换为PostgreSQL向量字符串格式
func (p *PostgresVectorDB) vectorToString(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = fmt.Sprintf("%.6f", v)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

func (p *PostgresVectorDB) vector64ToString(vector []float64) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = fmt.Sprintf("%.6f", v)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

// stringToVector 将PostgreSQL向量字符串转换为float32向量
func (p *PostgresVectorDB) stringToVector(vectorStr string) ([]float32, error) {
	// 简单的向量字符串解析
	if vectorStr == "" || vectorStr == "[]" {
		return []float32{}, nil
	}

	// 移除方括号
	vectorStr = strings.Trim(vectorStr, "[]")
	parts := strings.Split(vectorStr, ",")

	vector := make([]float32, len(parts))
	for i, part := range parts {
		var f float64
		_, err := fmt.Sscanf(strings.TrimSpace(part), "%f", &f)
		if err != nil {
			return nil, fmt.Errorf("解析向量元素失败: %w", err)
		}
		vector[i] = float32(f)
	}

	return vector, nil
}
