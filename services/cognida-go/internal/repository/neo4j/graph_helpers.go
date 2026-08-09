package neo4j

import (
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

// getValueAsString get string value from map
func getValueAsString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getValueAsFloat64 get float64 value from map
func getValueAsFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok && val != nil {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// getStringValue 从 Record 中安全地获取字符串值
func getStringValue(record *neo4j.Record, key string) string {
	if val, ok := record.Get(key); ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getFloat64Value 从 Record 中安全地获取 float64 值
func getFloat64Value(record *neo4j.Record, key string) float64 {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		}
	}
	return 0
}

// getStringSliceValue 从 Record 中安全地获取字符串切片值
func getStringSliceValue(record *neo4j.Record, key string) []string {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case []string:
			return v
		case []interface{}:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// getInterfaceSliceValue 从 Record 中安全地获取接口切片值
func getInterfaceSliceValue(record *neo4j.Record, key string) []interface{} {
	if val, ok := record.Get(key); ok && val != nil {
		if slice, ok := val.([]interface{}); ok {
			return slice
		}
	}
	return nil
}

// calculateRelationWeight 计算关系权重
func calculateRelationWeight(rel *knowledge.GraphRelation) float64 {
	// 简单的权重计算：基于强度和共同chunk数
	chunkFactor := float64(len(rel.ChunkIDs)) / 10.0
	if chunkFactor > 1.0 {
		chunkFactor = 1.0
	}
	return rel.Strength * (0.7 + 0.3*chunkFactor)
}
