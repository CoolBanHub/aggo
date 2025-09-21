package utils

import (
	"fmt"
	"strings"
)

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

// VectorToString 将float32向量转换为PostgreSQL向量字符串格式
func VectorToString(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = fmt.Sprintf("%.6f", v)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

func Vector64ToString(vector []float64) string {
	if len(vector) == 0 {
		return "[]"
	}

	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = fmt.Sprintf("%.6f", v)
	}

	return "[" + strings.Join(parts, ",") + "]"
}

// StringToVector 将PostgreSQL向量字符串转换为float32向量
func StringToVector(vectorStr string) ([]float32, error) {
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

func NewFloat64Ptr(f float64) *float64 {
	return &f
}
