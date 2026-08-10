package quality

import "fmt"

// Dimension 质量评估维度（Go 侧对照真源）。
//
// 这是「质量评估维度」跨服务 wire 契约在 Go 侧的类型化真源。每个常量的字符串
// 值与 Python 侧 services/cognida-python/services/quality/dimension_names.py 的
// Dimension（StrEnum）逐一对应，二者互为跨语言锚点——任一侧新增/改名/删除维度
// 都必须同步另一侧，并由双侧锁定测试守护（Go: dimensions_test.go；
// Python: tests/quality/test_dimension_names.py）。切勿改动既有 wire 值（向后兼容）。
type Dimension string

const (
	// 结构化维度
	DimensionCompleteness Dimension = "completeness"
	DimensionAccuracy     Dimension = "accuracy"
	DimensionConsistency  Dimension = "consistency"
	DimensionValidity     Dimension = "validity"
	DimensionUniqueness   Dimension = "uniqueness"
	DimensionTimeliness   Dimension = "timeliness"

	// 非结构化维度
	DimensionReadability        Dimension = "readability"
	DimensionInformationDensity Dimension = "information_density"
	DimensionLanguageQuality    Dimension = "language_quality"
	DimensionDuplication        Dimension = "duplication"
	DimensionPIIDetector        Dimension = "pii_detector"
	DimensionRelevance          Dimension = "relevance"
)

// StructuredDimensions 结构化数据评估维度（有序，与 Python 侧顺序一致）。
var StructuredDimensions = []Dimension{
	DimensionCompleteness,
	DimensionAccuracy,
	DimensionConsistency,
	DimensionValidity,
	DimensionUniqueness,
	DimensionTimeliness,
}

// UnstructuredDimensions 非结构化文本评估维度（有序）。
var UnstructuredDimensions = []Dimension{
	DimensionReadability,
	DimensionInformationDensity,
	DimensionLanguageQuality,
	DimensionDuplication,
	DimensionPIIDetector,
	DimensionRelevance,
}

// structuredSet / unstructuredSet 供 O(1) 白名单校验。
var (
	structuredSet   = newDimensionSet(StructuredDimensions)
	unstructuredSet = newDimensionSet(UnstructuredDimensions)
)

func newDimensionSet(dims []Dimension) map[Dimension]struct{} {
	set := make(map[Dimension]struct{}, len(dims))
	for _, d := range dims {
		set[d] = struct{}{}
	}
	return set
}

// validateDimensions 对用户传入的维度做白名单校验：任一未知维度即显式拒绝，
// 替代过去「静默透传给 Python、无对应评估器则悄悄跳过」的行为。空切片（nil）
// 表示「全部维度」，直接放行，保持既有默认行为不变。
func validateDimensions(dims []string, allowed map[Dimension]struct{}, op string) error {
	for _, d := range dims {
		if _, ok := allowed[Dimension(d)]; !ok {
			return &InvalidDimensionError{Op: op, Dimension: d}
		}
	}
	return nil
}

// InvalidDimensionError 表示请求携带了未知的质量评估维度。属客户端可修正的输入
// 问题（拼写错误 / 用错评估类型），handler 应据此返回 400 而非 500。
type InvalidDimensionError struct {
	Op        string // 操作名，如 "结构化质量评估"
	Dimension string // 触发拒绝的未知维度
}

func (e *InvalidDimensionError) Error() string {
	return fmt.Sprintf("%s失败: 未知的质量维度 %q", e.Op, e.Dimension)
}
