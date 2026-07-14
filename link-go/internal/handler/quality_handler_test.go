// Package handler 数据质量清洗处理器测试——钉死前端契约到 gRPC 请求的完整链路：
// JSON body 的 format（输入格式）与 query 的 output_format/cleaners（导出格式/清洗器）
// 必须正确到达 service.Clean 并透传给 Python gateway。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	qualitypb "link/api/proto/quality"
	app_quality "link/internal/service/quality"
)

// fakeQualityGateway 实现 quality.Gateway，仅捕获清洗请求并回显数据，
// 其余 RPC 不参与本测试。
type fakeQualityGateway struct {
	lastCleanReq *qualitypb.CleanDataRequest
}

func (f *fakeQualityGateway) EvaluateQuality(context.Context, *qualitypb.EvaluateQualityRequest) (*qualitypb.EvaluateQualityResponse, error) {
	return &qualitypb.EvaluateQualityResponse{Success: true}, nil
}
func (f *fakeQualityGateway) EvaluateUnstructuredQuality(context.Context, *qualitypb.EvaluateUnstructuredQualityRequest) (*qualitypb.EvaluateUnstructuredQualityResponse, error) {
	return &qualitypb.EvaluateUnstructuredQualityResponse{Success: true}, nil
}
func (f *fakeQualityGateway) CleanData(_ context.Context, req *qualitypb.CleanDataRequest) (*qualitypb.CleanDataResponse, error) {
	f.lastCleanReq = req
	return &qualitypb.CleanDataResponse{
		Success:     true,
		Result:      &qualitypb.CleaningResult{},
		CleanedData: req.GetCsvData(),
	}, nil
}
func (f *fakeQualityGateway) ListDimensions(context.Context, *qualitypb.ListDimensionsRequest) (*qualitypb.ListDimensionsResponse, error) {
	return &qualitypb.ListDimensionsResponse{}, nil
}

func newCleanRouter(gw app_quality.Gateway) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := app_quality.NewService(gw, 0, nil, nil, nil) // repo=nil：persist 容忍 nil
	h := NewQualityHandler(svc)
	r := gin.New()
	r.POST("/api/v1/quality/clean", h.CleanData)
	return r
}

// TestCleanData_JSONBodyFormatAndQueryReachGateway 覆盖前端主路径：
// JSON body 携带 format=json（输入格式），query 携带 output_format=csv、cleaners=trim,dedup。
// 断言这些参数经 service 正确进入 gateway 的 CleanDataRequest.Config / Cleaners，
// 且响应回填 cleaned_format=csv（导出格式优先）。
func TestCleanData_JSONBodyFormatAndQueryReachGateway(t *testing.T) {
	gw := &fakeQualityGateway{}
	r := newCleanRouter(gw)

	body := `{"csv_data":"[{\"name\":\" Alice \"}]","source_name":"src","format":"json"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/quality/clean?output_format=csv&cleaners=trim,dedup",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// gateway 收到的请求应携带输入/导出格式与清洗器
	if assert.NotNil(t, gw.lastCleanReq, "gateway 应收到清洗请求") {
		cfg := gw.lastCleanReq.GetConfig()
		assert.Equal(t, "json", cfg["format"], "输入格式应透传 format=json")
		assert.Equal(t, "csv", cfg["output_format"], "导出格式应透传 output_format=csv")
		assert.Equal(t, []string{"trim", "dedup"}, gw.lastCleanReq.GetCleaners())
	}

	// 响应体的 cleaned_format 应等于导出格式 csv
	var resp struct {
		Data struct {
			CleanedFormat string `json:"cleaned_format"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "csv", resp.Data.CleanedFormat)
}

// TestCleanData_CSVDefaultNoFormatConfig 覆盖默认 CSV 路径：
// 不带 format、不带 output_format → gateway 请求不应污染 format/output_format 键，
// cleaned_format 回退 csv。
func TestCleanData_CSVDefaultNoFormatConfig(t *testing.T) {
	gw := &fakeQualityGateway{}
	r := newCleanRouter(gw)

	body := `{"csv_data":"name\n Alice \n Alice \n"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quality/clean",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	if assert.NotNil(t, gw.lastCleanReq) {
		cfg := gw.lastCleanReq.GetConfig()
		_, hasFmt := cfg["format"]
		_, hasOut := cfg["output_format"]
		assert.False(t, hasFmt, "CSV 默认不应设置 config[format]")
		assert.False(t, hasOut, "未指定导出不应设置 config[output_format]")
	}
	var resp struct {
		Data struct {
			CleanedFormat string `json:"cleaned_format"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "csv", resp.Data.CleanedFormat)
}
