package quality

import (
	"context"
	"fmt"
	"time"

	qualitypb "link/api/proto/quality"
	domain_quality "link/internal/model/quality"
)

// IDGenerator 生成质检记录 ID（与 infrastructure/id.IDGenerator 同构，便于复用注入）
type IDGenerator interface {
	Generate() string
}

// Service 数据质量应用服务。
// 通过 Gateway 端口调用 Python 质量服务完成评估/清洗，并把结果落库供历史回看。
type Service struct {
	gateway Gateway
	timeout time.Duration
	repo    domain_quality.CheckRecordRepository
	idGen   IDGenerator
}

// NewService 创建数据质量服务
func NewService(gateway Gateway, timeout time.Duration, repo domain_quality.CheckRecordRepository, idGen IDGenerator) *Service {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Service{gateway: gateway, timeout: timeout, repo: repo, idGen: idGen}
}

// ========================================
// 结构化数据评估
// ========================================

// EvaluateStructured 评估 CSV 结构化数据质量，并落库。
func (s *Service) EvaluateStructured(ctx context.Context, tenantID, userID int64, sourceName string, csv []byte, dimensions []string) (*StructuredReport, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.gateway.EvaluateQuality(callCtx, &qualitypb.EvaluateQualityRequest{
		Data:       &qualitypb.EvaluateQualityRequest_CsvData{CsvData: csv},
		Dimensions: dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("结构化质量评估失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, fmt.Errorf("结构化质量评估失败: %s", resp.GetErrorMessage())
	}

	report := mapStructuredReport(resp.GetReport())
	summary := fmt.Sprintf("%d 维度 / %d 项问题", len(report.Dimensions), len(report.Issues))
	s.persist(ctx, tenantID, userID, domain_quality.CheckTypeStructured, sourceName,
		report.OverallScore, report.RecordCount, summary, report)
	return report, nil
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
		return nil, fmt.Errorf("文本质量评估失败: %s", resp.GetErrorMessage())
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

// Clean 清洗 CSV 数据，返回清洗结果与清洗后的 CSV，并落库。
func (s *Service) Clean(ctx context.Context, tenantID, userID int64, sourceName string, csv []byte, cleaners []string) (*CleaningReport, error) {
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.gateway.CleanData(callCtx, &qualitypb.CleanDataRequest{
		Data:     &qualitypb.CleanDataRequest_CsvData{CsvData: csv},
		Cleaners: cleaners,
	})
	if err != nil {
		return nil, fmt.Errorf("数据清洗失败: %w", err)
	}
	if !resp.GetSuccess() {
		return nil, fmt.Errorf("数据清洗失败: %s", resp.GetErrorMessage())
	}

	report := mapCleaningReport(resp.GetResult())
	report.CleanedCSV = string(resp.GetCleanedData())
	summary := fmt.Sprintf("原 %d 行 → 清洗后 %d 行（移除 %d）",
		report.OriginalCount, report.CleanedCount, report.RemovedCount)
	// 清洗记录不含质量分，落库时 score 记 0，record_count 记原始行数。
	s.persist(ctx, tenantID, userID, domain_quality.CheckTypeCleaning, sourceName,
		0, report.OriginalCount, summary, report)
	return report, nil
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
