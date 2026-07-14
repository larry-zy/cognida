package quality

import (
	"context"
	"fmt"
	"strings"
	"time"

	qualitypb "link/api/proto/quality"
	domain_quality "link/internal/model/quality"
)

// IDGenerator 生成质检记录 ID（与 infrastructure/id.IDGenerator 同构，便于复用注入）
type IDGenerator interface {
	Generate() string
}

// InputRejectedError 表示 Python 质量服务成功接收并处理了请求，但因输入数据本身
// 不合法/无法解析（如 CSV 格式错误、空数据）而拒绝。它与传输层错误（Python 服务
// 不可用、超时）区分：前者是客户端可修正的输入问题，handler 应返回 4xx 而非 5xx。
type InputRejectedError struct {
	Op  string // 操作名，如 "结构化质量评估"
	Msg string // Python 返回的错误信息
}

func (e *InputRejectedError) Error() string {
	if e.Msg == "" {
		return e.Op + "失败"
	}
	return e.Op + "失败: " + e.Msg
}

// Sampler 从外部数据源抽样取数（由 datasource.Service 实现）。
// Go 侧统一取数并推给 Python，Python 质量服务不直连任何外部数据库。
type Sampler interface {
	SampleCSV(ctx context.Context, tenantID int64, datasourceID, table string, limit int) ([]byte, int, error)
}

// Service 数据质量应用服务。
// 通过 Gateway 端口调用 Python 质量服务完成评估/清洗，并把结果落库供历史回看。
type Service struct {
	gateway Gateway
	timeout time.Duration
	repo    domain_quality.CheckRecordRepository
	idGen   IDGenerator
	sampler Sampler
}

// NewService 创建数据质量服务。sampler 可为 nil（未接入数据源时，数据源评估接口报错拒绝）。
func NewService(gateway Gateway, timeout time.Duration, repo domain_quality.CheckRecordRepository, idGen IDGenerator, sampler Sampler) *Service {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Service{gateway: gateway, timeout: timeout, repo: repo, idGen: idGen, sampler: sampler}
}

// ========================================
// 结构化数据评估
// ========================================

// EvaluateStructured 评估结构化数据质量，并落库。
// format 指定输入解析方式："csv"（默认）或 "json"（对象数组），经 config 透传给 Python。
func (s *Service) EvaluateStructured(ctx context.Context, tenantID, userID int64, sourceName string, data []byte, format string, dimensions []string) (*StructuredReport, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var config map[string]string
	if f := strings.ToLower(strings.TrimSpace(format)); f == "json" {
		config = map[string]string{"format": "json"}
	}
	resp, err := s.gateway.EvaluateQuality(callCtx, &qualitypb.EvaluateQualityRequest{
		Data:       &qualitypb.EvaluateQualityRequest_CsvData{CsvData: data},
		Dimensions: dimensions,
		Config:     config,
	})
	if err != nil {
		return nil, fmt.Errorf("结构化质量评估失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, &InputRejectedError{Op: "结构化质量评估", Msg: resp.GetErrorMessage()}
	}

	report := mapStructuredReport(resp.GetReport())
	summary := fmt.Sprintf("%d 维度 / %d 项问题", len(report.Dimensions), len(report.Issues))
	s.persist(ctx, tenantID, userID, domain_quality.CheckTypeStructured, sourceName,
		report.OverallScore, report.RecordCount, summary, report)
	return report, nil
}

// ========================================
// 数据源直评（Go 抽样 → Python 评估，Python 不直连数据库）
// ========================================

// EvaluateDatasource 从外部数据源指定表抽样若干行，评估其结构化数据质量并落库。
// 抽样与取数全在 Go 侧完成（受管连接池 + 只读 SELECT），仅把 CSV 样本推给 Python。
func (s *Service) EvaluateDatasource(ctx context.Context, tenantID, userID int64, datasourceID, table string, sampleSize int, dimensions []string) (*StructuredReport, error) {
	if s.sampler == nil {
		return nil, fmt.Errorf("数据源质量评估未启用：缺少数据源抽样能力")
	}
	csv, n, err := s.sampler.SampleCSV(ctx, tenantID, datasourceID, table, sampleSize)
	if err != nil {
		return nil, fmt.Errorf("数据源抽样失败: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("数据源抽样为空：表 %q 无数据", table)
	}
	// 以「数据源ID/表名」作为来源标识，便于历史回看定位
	sourceName := datasourceID + "/" + table
	// 数据源抽样固定输出 CSV
	return s.EvaluateStructured(ctx, tenantID, userID, sourceName, csv, "csv", dimensions)
}

// ========================================
// 非结构化文本质量
// ========================================

// EvaluateUnstructured 评估文本质量，并落库。
func (s *Service) EvaluateUnstructured(ctx context.Context, tenantID, userID int64, sourceName, text string, dimensions []string) (*UnstructuredReport, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.gateway.EvaluateUnstructuredQuality(callCtx, &qualitypb.EvaluateUnstructuredQualityRequest{
		Text:       text,
		Dimensions: dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("文本质量评估失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, &InputRejectedError{Op: "文本质量评估", Msg: resp.GetErrorMessage()}
	}

	report := mapUnstructuredReport(resp.GetReport())
	summary := fmt.Sprintf("%d 维度 / %d 项问题", len(report.Dimensions), len(report.Issues))
	s.persist(ctx, tenantID, userID, domain_quality.CheckTypeUnstructured, sourceName,
		report.OverallScore, 0, summary, report)
	return report, nil
}

// ========================================
// 数据清洗
// ========================================

// Clean 清洗数据，返回清洗结果与清洗后的数据，并落库。
// format 指定输入解析方式（csv/json，默认 csv）；outputFormat 指定导出格式
// （csv/json），为空时 Python 端跟随输入格式。二者均经 config 透传，无需改 proto。
func (s *Service) Clean(ctx context.Context, tenantID, userID int64, sourceName string, data []byte, cleaners []string, format, outputFormat string) (*CleaningReport, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	config := make(map[string]string)
	if f := strings.ToLower(strings.TrimSpace(format)); f == "json" {
		config["format"] = "json"
	}
	if of := strings.ToLower(strings.TrimSpace(outputFormat)); of == "json" || of == "csv" {
		config["output_format"] = of
	}
	if len(config) == 0 {
		config = nil
	}

	resp, err := s.gateway.CleanData(callCtx, &qualitypb.CleanDataRequest{
		Data:     &qualitypb.CleanDataRequest_CsvData{CsvData: data},
		Cleaners: cleaners,
		Config:   config,
	})
	if err != nil {
		return nil, fmt.Errorf("数据清洗失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, &InputRejectedError{Op: "数据清洗", Msg: resp.GetErrorMessage()}
	}

	report := mapCleaningReport(resp.GetResult())
	report.CleanedCSV = string(resp.GetCleanedData())
	// 导出格式：显式指定优先，否则跟随输入格式（与 Python 端缺省逻辑一致）
	report.CleanedFormat = resolveCleanedFormat(outputFormat, format)
	summary := fmt.Sprintf("原 %d 行 → 清洗后 %d 行（移除 %d）",
		report.OriginalCount, report.CleanedCount, report.RemovedCount)
	// 清洗记录不含质量分，落库时 score 记 0，record_count 记原始行数。
	s.persist(ctx, tenantID, userID, domain_quality.CheckTypeCleaning, sourceName,
		0, report.OriginalCount, summary, report)
	return report, nil
}

// resolveCleanedFormat 推导清洗结果的实际导出格式，供前端决定下载文件扩展名/MIME。
// 与 Python 端一致：显式 outputFormat 优先，其次跟随输入 format，默认 csv。
func resolveCleanedFormat(outputFormat, inputFormat string) string {
	if of := strings.ToLower(strings.TrimSpace(outputFormat)); of == "json" || of == "csv" {
		return of
	}
	if strings.ToLower(strings.TrimSpace(inputFormat)) == "json" {
		return "json"
	}
	return "csv"
}

// ========================================
// 维度列表
// ========================================

// ListDimensions 返回支持的质量维度说明。
func (s *Service) ListDimensions(ctx context.Context) ([]DimensionInfo, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.gateway.ListDimensions(callCtx, &qualitypb.ListDimensionsRequest{})
	if err != nil {
		return nil, fmt.Errorf("获取质量维度失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, fmt.Errorf("获取质量维度失败: %s", resp.GetErrorMessage())
	}
	out := make([]DimensionInfo, 0, len(resp.GetDimensions()))
	for _, d := range resp.GetDimensions() {
		out = append(out, DimensionInfo{
			Name:                 d.GetName(),
			Description:          d.GetDescription(),
			SupportsStructured:   d.GetSupportsStructured(),
			SupportsUnstructured: d.GetSupportsUnstructured(),
		})
	}
	return out, nil
}

// ========================================
// 历史记录
// ========================================

// ListRecords 分页查询历史质检记录。
func (s *Service) ListRecords(ctx context.Context, filter domain_quality.ListFilter) ([]*domain_quality.CheckRecord, int64, error) {
	return s.repo.List(ctx, filter)
}

// GetRecord 获取单条质检记录（含完整报告快照）。
func (s *Service) GetRecord(ctx context.Context, id string, tenantID int64) (*domain_quality.CheckRecord, error) {
	return s.repo.Get(ctx, id, tenantID)
}

// DeleteRecord 删除历史质检记录。
func (s *Service) DeleteRecord(ctx context.Context, id string, tenantID int64) error {
	return s.repo.Delete(ctx, id, tenantID)
}
