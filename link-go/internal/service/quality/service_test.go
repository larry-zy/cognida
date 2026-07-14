package quality

import (
	"context"
	"errors"
	"strings"
	"testing"

	qualitypb "link/api/proto/quality"
)

// fakeGateway 记录最后一次评估请求，返回预置成功响应。
type fakeGateway struct {
	lastCSV    []byte
	lastConfig map[string]string
	resp       *qualitypb.EvaluateQualityResponse
	err        error

	// 清洗相关：记录最后一次清洗请求，返回预置响应
	lastCleanReq *qualitypb.CleanDataRequest
	cleanResp    *qualitypb.CleanDataResponse
	cleanErr     error
}

func (f *fakeGateway) EvaluateQuality(_ context.Context, req *qualitypb.EvaluateQualityRequest) (*qualitypb.EvaluateQualityResponse, error) {
	f.lastCSV = req.GetCsvData()
	f.lastConfig = req.GetConfig()
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeGateway) EvaluateUnstructuredQuality(context.Context, *qualitypb.EvaluateUnstructuredQualityRequest) (*qualitypb.EvaluateUnstructuredQualityResponse, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeGateway) CleanData(_ context.Context, req *qualitypb.CleanDataRequest) (*qualitypb.CleanDataResponse, error) {
	f.lastCleanReq = req
	if f.cleanErr != nil {
		return nil, f.cleanErr
	}
	if f.cleanResp != nil {
		return f.cleanResp, nil
	}
	// 默认：成功、空清洗结果、回显 csv_data 作为清洗后数据
	return &qualitypb.CleanDataResponse{
		Success:     true,
		Result:      &qualitypb.CleaningResult{},
		CleanedData: req.GetCsvData(),
	}, nil
}
func (f *fakeGateway) ListDimensions(context.Context, *qualitypb.ListDimensionsRequest) (*qualitypb.ListDimensionsResponse, error) {
	return nil, errors.New("not implemented")
}

// fakeSampler 模拟数据源抽样。
type fakeSampler struct {
	csv   []byte
	n     int
	err   error
	calls int
	// 记录入参
	gotID, gotTable string
	gotLimit        int
}

func (f *fakeSampler) SampleCSV(_ context.Context, _ int64, id, table string, limit int) ([]byte, int, error) {
	f.calls++
	f.gotID, f.gotTable, f.gotLimit = id, table, limit
	return f.csv, f.n, f.err
}

func TestEvaluateDatasourceNoSampler(t *testing.T) {
	// sampler 为 nil → 未启用，直接拒绝
	s := NewService(&fakeGateway{}, 0, nil, nil, nil)
	_, err := s.EvaluateDatasource(context.Background(), 1, 10, "ds1", "orders", 50, nil)
	if err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("nil sampler 应报未启用, got %v", err)
	}
}

func TestEvaluateDatasourceSamplerError(t *testing.T) {
	smp := &fakeSampler{err: errors.New("connect refused u:pw@host")}
	s := NewService(&fakeGateway{}, 0, nil, nil, smp)
	_, err := s.EvaluateDatasource(context.Background(), 1, 10, "ds1", "orders", 50, nil)
	if err == nil || !strings.Contains(err.Error(), "数据源抽样失败") {
		t.Fatalf("抽样错误应被包装, got %v", err)
	}
}

func TestEvaluateDatasourceEmptySample(t *testing.T) {
	smp := &fakeSampler{csv: []byte("id\n"), n: 0}
	s := NewService(&fakeGateway{}, 0, nil, nil, smp)
	_, err := s.EvaluateDatasource(context.Background(), 1, 10, "ds1", "orders", 50, nil)
	if err == nil || !strings.Contains(err.Error(), "无数据") {
		t.Fatalf("空样本应报无数据, got %v", err)
	}
}

func TestEvaluateStructuredInputRejected(t *testing.T) {
	// Python 成功处理但拒绝了畸形数据（success=false）→ 应归类为 InputRejectedError，
	// 便于 handler 返回 4xx 而非 5xx。
	gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{
		Success:      false,
		ErrorMessage: "Error tokenizing data. C error: Expected 4 fields in line 3, saw 5",
	}}
	s := NewService(gw, 0, nil, nil, nil)

	_, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据", []byte("a,b\n1,2,3\n"), "csv", nil)
	var inputErr *InputRejectedError
	if !errors.As(err, &inputErr) {
		t.Fatalf("success=false 应归类为 InputRejectedError, got %T: %v", err, err)
	}
	if !strings.Contains(inputErr.Error(), "结构化质量评估失败") ||
		!strings.Contains(inputErr.Error(), "tokenizing") {
		t.Errorf("错误信息应含操作名与 Python 原因: %q", inputErr.Error())
	}
}

func TestEvaluateStructuredTransportErrorNotInputRejected(t *testing.T) {
	// gRPC 传输层错误（Python 不可用）→ 不应归类为 InputRejectedError，保持 5xx。
	gw := &fakeGateway{err: errors.New("connection refused")}
	s := NewService(gw, 0, nil, nil, nil)

	_, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据", []byte("a,b\n1,2\n"), "csv", nil)
	var inputErr *InputRejectedError
	if errors.As(err, &inputErr) {
		t.Fatalf("传输错误不应归类为输入错误: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "结构化质量评估失败") {
		t.Fatalf("传输错误应被包装, got %v", err)
	}
}

func TestEvaluateStructuredJSONFormatSetsConfig(t *testing.T) {
	// format=json → 经 config["format"]=json 透传给 Python
	gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{Success: true}}
	s := NewService(gw, 0, nil, nil, nil)

	_, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据",
		[]byte(`[{"a":1,"b":2}]`), "json", nil)
	if err != nil {
		t.Fatalf("json 评估不应出错: %v", err)
	}
	if gw.lastConfig["format"] != "json" {
		t.Errorf("config[format] 应为 json, got %v", gw.lastConfig)
	}
}

func TestEvaluateStructuredCSVFormatNoConfig(t *testing.T) {
	// format=csv（默认）→ 不设置 config，避免污染
	gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{Success: true}}
	s := NewService(gw, 0, nil, nil, nil)

	_, err := s.EvaluateStructured(context.Background(), 1, 10, "粘贴数据",
		[]byte("a,b\n1,2\n"), "csv", nil)
	if err != nil {
		t.Fatalf("csv 评估不应出错: %v", err)
	}
	if _, ok := gw.lastConfig["format"]; ok {
		t.Errorf("csv 不应设置 config[format], got %v", gw.lastConfig)
	}
}

func TestEvaluateDatasourceHappyPath(t *testing.T) {
	gw := &fakeGateway{resp: &qualitypb.EvaluateQualityResponse{Success: true}}
	smp := &fakeSampler{csv: []byte("id,name\n1,a\n"), n: 1}
	// repo=nil：persist 容忍 nil 仓储，无需落库
	s := NewService(gw, 0, nil, nil, smp)

	report, err := s.EvaluateDatasource(context.Background(), 1, 10, "ds1", "orders", 50, []string{"completeness"})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if report == nil {
		t.Fatalf("应返回报告")
	}
	// 抽样入参透传正确
	if smp.gotID != "ds1" || smp.gotTable != "orders" || smp.gotLimit != 50 {
		t.Errorf("抽样入参不符: id=%q table=%q limit=%d", smp.gotID, smp.gotTable, smp.gotLimit)
	}
	// 抽到的 CSV 被推给 gateway（Python 不直连库）
	if string(gw.lastCSV) != "id,name\n1,a\n" {
		t.Errorf("gateway 收到的 CSV 不符: %q", gw.lastCSV)
	}
}

// TestCleanForwardsFormatToGateway 校验清洗入口把输入/导出格式经 config 透传给 Python，
// 且 CleanedFormat 正确回填——覆盖前端「格式切换」到 gRPC 请求的契约。
func TestCleanForwardsFormatToGateway(t *testing.T) {
	cases := []struct {
		name         string
		format       string
		outputFormat string
		wantFormat   string // config["format"] 期望值，"" 表示不应存在该键
		wantOutput   string // config["output_format"] 期望值
		wantCleaned  string // report.CleanedFormat 期望值
	}{
		{"csv 默认不设 format", "csv", "csv", "", "csv", "csv"},
		{"json 入 json 出", "json", "json", "json", "json", "json"},
		{"json 入 csv 出", "json", "csv", "json", "csv", "csv"},
		{"未指定导出跟随输入", "json", "", "json", "", "json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := &fakeGateway{}
			s := NewService(gw, 0, nil, nil, nil)

			report, err := s.Clean(context.Background(), 1, 10, "src",
				[]byte(`[{"id":1}]`), []string{"trim"}, tc.format, tc.outputFormat)
			if err != nil {
				t.Fatalf("Clean 失败: %v", err)
			}
			cfg := gw.lastCleanReq.GetConfig()
			if got := cfg["format"]; got != tc.wantFormat {
				t.Errorf("config[format]=%q, 期望 %q", got, tc.wantFormat)
			}
			if got := cfg["output_format"]; got != tc.wantOutput {
				t.Errorf("config[output_format]=%q, 期望 %q", got, tc.wantOutput)
			}
			if report.CleanedFormat != tc.wantCleaned {
				t.Errorf("CleanedFormat=%q, 期望 %q", report.CleanedFormat, tc.wantCleaned)
			}
			// cleaners 也应透传
			if len(gw.lastCleanReq.GetCleaners()) != 1 || gw.lastCleanReq.GetCleaners()[0] != "trim" {
				t.Errorf("cleaners 透传错误: %v", gw.lastCleanReq.GetCleaners())
			}
		})
	}
}
