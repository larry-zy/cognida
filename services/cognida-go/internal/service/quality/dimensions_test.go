package quality

import (
	"context"
	"errors"
	"testing"

	qualitypb "cognida/api/proto/quality"
)

// 跨语言锚点：与 Python 侧 tests/quality/test_dimension_names.py 的
// EXPECTED_STRUCTURED / EXPECTED_UNSTRUCTURED 逐字对应，切勿改动。
var expectedStructured = map[string]struct{}{
	"completeness": {},
	"accuracy":     {},
	"consistency":  {},
	"validity":     {},
	"uniqueness":   {},
	"timeliness":   {},
}

var expectedUnstructured = map[string]struct{}{
	"readability":         {},
	"information_density": {},
	"language_quality":    {},
	"duplication":         {},
	"pii_detector":        {},
	"relevance":           {},
}

func dimSet(dims []Dimension) map[string]struct{} {
	out := make(map[string]struct{}, len(dims))
	for _, d := range dims {
		out[string(d)] = struct{}{}
	}
	return out
}

func equalStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// TestStructuredDimensionsLock 锁定结构化维度常量集 == 期望 6 字面量。
func TestStructuredDimensionsLock(t *testing.T) {
	if got := dimSet(StructuredDimensions); !equalStringSet(got, expectedStructured) {
		t.Fatalf("结构化维度漂移: got=%v want=%v", got, expectedStructured)
	}
}

// TestUnstructuredDimensionsLock 锁定非结构化维度常量集 == 期望 6 字面量。
func TestUnstructuredDimensionsLock(t *testing.T) {
	if got := dimSet(UnstructuredDimensions); !equalStringSet(got, expectedUnstructured) {
		t.Fatalf("非结构化维度漂移: got=%v want=%v", got, expectedUnstructured)
	}
}

// TestDimensionGroupsDisjointAndComplete 两组不相交，并集恰为 12 个维度。
func TestDimensionGroupsDisjointAndComplete(t *testing.T) {
	seen := map[string]struct{}{}
	for _, d := range append(append([]Dimension{}, StructuredDimensions...), UnstructuredDimensions...) {
		if _, dup := seen[string(d)]; dup {
			t.Fatalf("维度 %q 在结构化/非结构化两组间重复", d)
		}
		seen[string(d)] = struct{}{}
	}
	if len(seen) != 12 {
		t.Fatalf("维度总数应为 12, got %d", len(seen))
	}
}

// TestDimensionConstsMatchLiterals 逐一钉死常量的 wire 字面值，防止改常量值破坏契约。
func TestDimensionConstsMatchLiterals(t *testing.T) {
	pairs := []struct {
		got  Dimension
		want string
	}{
		{DimensionCompleteness, "completeness"},
		{DimensionAccuracy, "accuracy"},
		{DimensionConsistency, "consistency"},
		{DimensionValidity, "validity"},
		{DimensionUniqueness, "uniqueness"},
		{DimensionTimeliness, "timeliness"},
		{DimensionReadability, "readability"},
		{DimensionInformationDensity, "information_density"},
		{DimensionLanguageQuality, "language_quality"},
		{DimensionDuplication, "duplication"},
		{DimensionPIIDetector, "pii_detector"},
		{DimensionRelevance, "relevance"},
	}
	for _, p := range pairs {
		if string(p.got) != p.want {
			t.Errorf("维度常量 wire 值漂移: got=%q want=%q", p.got, p.want)
		}
	}
}

// TestEvaluateStructuredRejectsUnknownDimension 未知结构化维度显式拒绝，
// 归类为 InvalidDimensionError（handler 据此返回 400），不再静默透传。
func TestEvaluateStructuredRejectsUnknownDimension(t *testing.T) {
	gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{Success: true}}
	s := NewService(gw, 0, nil, nil, nil)

	_, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据",
		[]byte("a,b\n1,2\n"), "csv", []string{"completeness", "not_a_dim"})
	var dimErr *InvalidDimensionError
	if !errors.As(err, &dimErr) {
		t.Fatalf("未知维度应归类为 InvalidDimensionError, got %T: %v", err, err)
	}
	if dimErr.Dimension != "not_a_dim" {
		t.Errorf("错误应指认具体未知维度, got %q", dimErr.Dimension)
	}
	// 校验发生在调用 Python 之前：gateway 不应被触达
	if gw.lastCSV != nil {
		t.Errorf("非法维度不应触达 Python gateway, lastCSV=%q", gw.lastCSV)
	}
}

// TestEvaluateStructuredAcceptsKnownAndEmptyDimensions 已知维度与空维度（=全部）放行。
func TestEvaluateStructuredAcceptsKnownAndEmptyDimensions(t *testing.T) {
	for _, dims := range [][]string{nil, {"completeness", "timeliness"}} {
		gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{Success: true}}
		s := NewService(gw, 0, nil, nil, nil)
		if _, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据",
			[]byte("a,b\n1,2\n"), "csv", dims); err != nil {
			t.Fatalf("合法维度 %v 不应报错: %v", dims, err)
		}
	}
}

// TestEvaluateUnstructuredRejectsStructuredDimension 非结构化端点拒绝结构化维度
//（用错评估类型也算未知维度），显式返回 InvalidDimensionError。
func TestEvaluateUnstructuredRejectsStructuredDimension(t *testing.T) {
	s := NewService(&fakeGateway{}, 0, nil, nil, nil)
	_, err := s.EvaluateUnstructured(context.Background(), 1, 10, "src", "hello",
		[]string{"completeness"})
	var dimErr *InvalidDimensionError
	if !errors.As(err, &dimErr) {
		t.Fatalf("结构化维度用于文本端点应被拒绝, got %T: %v", err, err)
	}
}
