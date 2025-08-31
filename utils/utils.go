package utils

import (
	"encoding/json"
)

// ToJSONString 将字符串切片序列化为JSON字符串
func ToJSONString(slice []string) string {
	if len(slice) == 0 {
		return ""
	}
	data, err := json.Marshal(slice)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseJSONStringArray 解析JSON字符串为字符串切片
func ParseJSONStringArray(jsonStr string) []string {
	if jsonStr == "" {
		return nil
	}
	var slice []string
	if err := json.Unmarshal([]byte(jsonStr), &slice); err != nil {
		return nil
	}
	return slice
}
