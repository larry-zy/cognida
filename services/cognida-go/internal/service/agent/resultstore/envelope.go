// Package resultstore 实现 Data Agent 的 data-by-reference：完整结果集按 result_id
// 落到后端（Redis），回灌 LLM 的只是"结果信封"（列/dtype/行数/样本/聚合），
// 避免把上千行原始数据逐字灌进上下文。见 openspec/changes/data-agent-evolution。
package resultstore

import (
	"fmt"
	"strconv"
	"time"
)

// DefaultSampleRows 信封默认携带的样本行上限。
const DefaultSampleRows = 20

// Result 是被持久化的完整结果集，仅在工具间按 result_id 传递引用。
type Result struct {
	ResultID  string                   `json:"result_id"`
	Owner     string                   `json:"owner"` // 归属键 tenant:session，用于跨会话隔离
	Columns   []string                 `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	CreatedAt int64                    `json:"created_at"`
}

// Envelope 是回灌 LLM 的紧凑结构，绝不包含完整原始行。
type Envelope struct {
	ResultID   string                   `json:"result_id"`
	Columns    []string                 `json:"columns"`
	Dtypes     map[string]string        `json:"dtypes"`
	RowCount   int                      `json:"row_count"`
	Samples    []map[string]interface{} `json:"samples"`
	Aggregates map[string]interface{}   `json:"aggregates,omitempty"`
	Truncated  bool                     `json:"truncated"` // 样本是否少于总行数
}

// BuildEnvelope 从完整结果集派生信封，样本行数不超过 sampleN。
func BuildEnvelope(r *Result, sampleN int) *Envelope {
	if sampleN <= 0 {
		sampleN = DefaultSampleRows
	}

	rowCount := len(r.Rows)
	sampleCount := rowCount
	if sampleCount > sampleN {
		sampleCount = sampleN
	}

	samples := make([]map[string]interface{}, 0, sampleCount)
	for i := 0; i < sampleCount; i++ {
		samples = append(samples, r.Rows[i])
	}

	return &Envelope{
		ResultID:   r.ResultID,
		Columns:    r.Columns,
		Dtypes:     inferDtypes(r.Columns, r.Rows),
		RowCount:   rowCount,
		Samples:    samples,
		Aggregates: computeAggregates(r.Columns, r.Rows),
		Truncated:  sampleCount < rowCount,
	}
}

// inferDtypes 依据每列首个非空值推断类型（number/bool/string/null）。
func inferDtypes(columns []string, rows []map[string]interface{}) map[string]string {
	dtypes := make(map[string]string, len(columns))
	for _, col := range columns {
		dtypes[col] = "null"
		for _, row := range rows {
			v, ok := row[col]
			if !ok || v == nil {
				continue
			}
			dtypes[col] = kindOf(v)
			break
		}
	}
	return dtypes
}

// computeAggregates 对可解析为数值的列给出 min/max/sum/count；非数值列给出非空计数。
func computeAggregates(columns []string, rows []map[string]interface{}) map[string]interface{} {
	if len(rows) == 0 {
		return nil
	}
	aggs := make(map[string]interface{})
	for _, col := range columns {
		var (
			numericCount int
			nonNull      int
			sum          float64
			min          float64
			max          float64
		)
		for _, row := range rows {
			v, ok := row[col]
			if !ok || v == nil {
				continue
			}
			nonNull++
			f, isNum := toFloat(v)
			if !isNum {
				continue
			}
			if numericCount == 0 {
				min, max = f, f
			} else {
				if f < min {
					min = f
				}
				if f > max {
					max = f
				}
			}
			sum += f
			numericCount++
		}
		if numericCount > 0 {
			aggs[col] = map[string]interface{}{
				"min":   min,
				"max":   max,
				"sum":   sum,
				"count": numericCount,
			}
		} else if nonNull > 0 {
			aggs[col] = map[string]interface{}{"non_null": nonNull}
		}
	}
	if len(aggs) == 0 {
		return nil
	}
	return aggs
}

func kindOf(v interface{}) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return "number"
	case string:
		// 字符串可能承载数值（SQL 驱动常把数值列以 []byte→string 返回）
		if _, ok := toFloat(v); ok {
			return "number"
		}
		return "string"
	default:
		return "string"
	}
}

// toFloat 尽力把值转为 float64，兼容 SQL 驱动把数值列以字符串返回的情况。
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// OwnerKey 由租户与会话拼装归属键，用于跨会话隔离校验。
func OwnerKey(tenantID int64, sessionID string) string {
	return fmt.Sprintf("%d:%s", tenantID, sessionID)
}

// nowUnix 便于测试替换（生产用真实时间）。
var nowUnix = func() int64 { return time.Now().Unix() }
