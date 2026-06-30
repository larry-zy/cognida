// Package evaluation 提供数据集管理服务
package evaluation

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domeval "link/internal/model/evaluation"
)

// ========================================
// 数据集管理服务
// ========================================

// DatasetManager 数据集管理服务（提供 CRUD 功能）
type DatasetManager struct {
	datasetRepo domeval.DatasetRepository
	loader      *DatasetLoader
}

// NewDatasetManager 创建数据集管理服务
func NewDatasetManager(
	datasetRepo domeval.DatasetRepository,
	loader *DatasetLoader,
) *DatasetManager {
	return &DatasetManager{
		datasetRepo: datasetRepo,
		loader:      loader,
	}
}

// ========================================
// Request/Response DTOs
// ========================================

// CreateDatasetRequest 创建数据集请求
type CreateDatasetRequest struct {
	DatasetID      string                `json:"dataset_id" binding:"required"`
	Name           string                `json:"name" binding:"required"`
	Description    string                `json:"description"`
	EvaluationType EvaluationType        `json:"evaluation_type" binding:"required"`
	QAPairs        []*domeval.QAPair     `json:"qa_pairs"`
}

// Validate 验证请求
func (r *CreateDatasetRequest) Validate() error {
	if r.Name == "" {
		return domeval.ErrDatasetNameEmpty
	}
	if !r.EvaluationType.IsValid() {
		return domeval.ErrInvalidEvalType
	}
	return nil
}

// UpdateDatasetRequest 更新数据集请求
type UpdateDatasetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListSamplesRequest 列出样本请求
type ListSamplesRequest struct {
	Page     int `json:"page" form:"page"`
	PageSize int `json:"page_size" form:"page_size"`
}

// AddSamplesRequest 添加样本请求
type AddSamplesRequest struct {
	QAPairs []*domeval.QAPair `json:"qa_pairs" binding:"required"`
}

// Validate 验证请求
func (r *AddSamplesRequest) Validate() error {
	if len(r.QAPairs) == 0 {
		return fmt.Errorf("at least one qa_pair is required")
	}
	for i, qa := range r.QAPairs {
		if qa.Question == "" {
			return fmt.Errorf("qa_pairs[%d].question is required", i)
		}
		if qa.ReferenceAnswer == "" {
			return fmt.Errorf("qa_pairs[%d].reference_answer is required", i)
		}
	}
	return nil
}

// DatasetDetailResponse 数据集详情响应
type DatasetDetailResponse struct {
	DatasetID      string         `json:"dataset_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Type           domeval.DatasetType `json:"type"`
	EvaluationType EvaluationType `json:"evaluation_type"`
	QACount        int            `json:"qa_count"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

// SampleListResponse 样本列表响应
type SampleListResponse struct {
	Samples    []*domeval.DatasetRecord `json:"samples"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
}

// ========================================
// 用例方法
// ========================================

// CreateDataset 创建数据集
func (m *DatasetManager) CreateDataset(ctx context.Context, tenantID, userID int64, req *CreateDatasetRequest) (*DatasetDetailResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// 如果没有指定 dataset_id，生成一个
	datasetID := req.DatasetID
	if datasetID == "" {
		datasetID = generateDatasetID()
	}

	// 创建数据集实体
	dataset := domeval.NewDataset(datasetID, tenantID, userID, req.Name, domeval.EvaluationType(req.EvaluationType))
	if req.Description != "" {
		dataset.UpdateDescription(req.Description)
	}

	// 保存数据集
			if err := m.datasetRepo.Create(ctx, dataset); err != nil {
		return nil, err
	}

	// 如果有样本，添加样本
	if len(req.QAPairs) > 0 {
		records := make([]*domeval.DatasetRecord, len(req.QAPairs))
		for i, qa := range req.QAPairs {
			records[i] = domeval.NewDatasetRecord(datasetID, tenantID, qa.Question, qa.ReferenceAnswer)
			if len(qa.RelevantPIDs) > 0 {
				records[i].SetRelevantPIDs(qa.RelevantPIDs)
			}
			if qa.Context != "" {
				records[i].SetContext(qa.Context)
			}
		}

		if err := m.datasetRepo.AddSamples(ctx, datasetID, records); err != nil {
			return nil, fmt.Errorf("failed to add samples: %w", err)
		}
	}

	// 重新加载获取完整数据
	result, err := m.datasetRepo.FindByIDWithTenant(ctx, datasetID, tenantID)
	if err != nil {
		return nil, err
	}

	return toDatasetDetailResponse(result), nil
}

// GetDataset 获取数据集详情
func (m *DatasetManager) GetDataset(ctx context.Context, datasetID string, tenantID int64) (*DatasetDetailResponse, error) {
	dataset, err := m.datasetRepo.FindByIDWithTenant(ctx, datasetID, tenantID)
	if err != nil {
		return nil, err
	}

	return toDatasetDetailResponse(dataset), nil
}

// ListDatasets 列出数据集（合并数据库和文件系统）
func (m *DatasetManager) ListDatasets(ctx context.Context, tenantID int64, evalType *EvaluationType) ([]*DatasetMetadata, error) {
	// 使用 DatasetLoader 获取合并后的列表
	datasets, err := m.loader.ListDatasets(ctx)
	if err != nil {
		return nil, err
	}

	// 按评测类型过滤
	if evalType != nil {
		filtered := make([]*DatasetMetadata, 0)
		for _, ds := range datasets {
			if ds.EvalType == *evalType {
				filtered = append(filtered, ds)
			}
		}
		return filtered, nil
	}

	return datasets, nil
}

// UpdateDataset 更新数据集
func (m *DatasetManager) UpdateDataset(ctx context.Context, datasetID string, tenantID int64, req *UpdateDatasetRequest) (*DatasetDetailResponse, error) {
	// 获取现有数据集
	dataset, err := m.datasetRepo.FindByIDWithTenant(ctx, datasetID, tenantID)
	if err != nil {
		return nil, err
	}

	// 更新字段
	if req.Name != "" {
		dataset.UpdateName(req.Name)
	}
	if req.Description != "" {
		dataset.UpdateDescription(req.Description)
	}

	// 保存更新
	if err := m.datasetRepo.Update(ctx, dataset); err != nil {
		return nil, err
	}

	return toDatasetDetailResponse(dataset), nil
}

// DeleteDataset 删除数据集
func (m *DatasetManager) DeleteDataset(ctx context.Context, datasetID string, tenantID int64) error {
	return m.datasetRepo.SoftDelete(ctx, datasetID, tenantID)
}

// ListSamples 列出样本
func (m *DatasetManager) ListSamples(ctx context.Context, datasetID string, tenantID int64, req *ListSamplesRequest) (*SampleListResponse, error) {
	page := req.Page
	pageSize := req.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	samples, total, err := m.datasetRepo.ListSamples(ctx, datasetID, tenantID, page, pageSize)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &SampleListResponse{
		Samples:    samples,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// AddSamples 添加样本
func (m *DatasetManager) AddSamples(ctx context.Context, datasetID string, tenantID int64, req *AddSamplesRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	// 检查数据集是否存在
	_, err := m.datasetRepo.FindByIDWithTenant(ctx, datasetID, tenantID)
	if err != nil {
		return err
	}

	// 转换为记录
	records := make([]*domeval.DatasetRecord, len(req.QAPairs))
	for i, qa := range req.QAPairs {
		records[i] = domeval.NewDatasetRecord(datasetID, tenantID, qa.Question, qa.ReferenceAnswer)
		if len(qa.RelevantPIDs) > 0 {
			records[i].SetRelevantPIDs(qa.RelevantPIDs)
		}
		if qa.Context != "" {
			records[i].SetContext(qa.Context)
		}
	}

	return m.datasetRepo.AddSamples(ctx, datasetID, records)
}

// DeleteSample 删除样本
func (m *DatasetManager) DeleteSample(ctx context.Context, datasetID string, tenantID int64, sampleID int64) error {
	return m.datasetRepo.DeleteSample(ctx, datasetID, tenantID, sampleID)
}

// ========================================
// Helper Functions
// ========================================

// generateDatasetID 生成数据集ID
func generateDatasetID() string {
	return fmt.Sprintf("ds-%s", uuid.New().String()[:8])
}

// toDatasetDetailResponse 转换为详情响应
func toDatasetDetailResponse(dataset *domeval.Dataset) *DatasetDetailResponse {
	return &DatasetDetailResponse{
		DatasetID:      dataset.DatasetID,
		Name:           dataset.Name,
		Description:    dataset.Description,
		Type:           dataset.Type,
		EvaluationType: EvaluationType(dataset.EvaluationType),
		QACount:        dataset.QACount,
		CreatedAt:      dataset.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:      dataset.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ========================================
// DatasetService 接口实现
// ========================================

// 注意：DatasetService 接口由 DatasetLoader 实现，不需要在 DatasetManager 中实现
