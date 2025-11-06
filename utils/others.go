package utils

import "fmt"

// ConvertToStringSlice 会尝试将任意类型转成字符串切片，常用于 JSONB 结果解析场景。
func ConvertToStringSlice(data any) ([]string, error) {
	if v, ok := data.([]string); ok {
		return v, nil
	}

	elements, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("当前数据类型无法转换为字符串切片")
	}

	result := make([]string, len(elements))
	for i, item := range elements {
		str, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("第 %d 个元素不是字符串", i)
		}
		result[i] = str
	}

	return result, nil
}
