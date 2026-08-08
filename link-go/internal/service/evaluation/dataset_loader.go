// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	domeval "link/internal/model/evaluation"
	"link/internal/model/task"
)

const (
	// MetaFileName 元数据文件名
	MetaFileName = "meta.json"
	// SamplesFileName 样本文件名
	SamplesFileName = "samples.jsonl"
	// defaultDatasetDir 数据集根目录的跨平台默认值（相对当前工作目录）。
	defaultDatasetDir = "datasets"
)

// DatasetDir 数据集根目录。可用环境变量 EVAL_DATASET_DIR 覆盖；未设置时取相对路径 datasets/。
// 原实现硬编码 "D:/link/datasets"（Windows 绝对路径），在 macOS/Linux 上永远不存在，
// 文件系统回退加载必失败。
var DatasetDir = func() string {
	if dir := os.Getenv("EVAL_DATASET_DIR"); dir != "" {
		return dir
	}
	return defaultDatasetDir
}()

// DatasetLoader 数据集加载器
type DatasetLoader struct {
	db             *gorm.DB
	datasetRepo domeval.DatasetRepository
}

// NewDatasetLoader 创建数据集加载器
func NewDatasetLoader(db *gorm.DB, datasetRepo domeval.DatasetRepository) *DatasetLoader {
	return &DatasetLoader{
		db:             db,
		datasetRepo:    datasetRepo,
	}
}

// Load 加载数据集（混合模式：数据库优先，回退到文件系统）
func (l *DatasetLoader) Load(ctx context.Context, datasetID string) (*Dataset, error) {
	// 优先从数据库加载
	if l.datasetRepo != nil {
		ds, err := l.LoadFromDB(ctx, datasetID)
		if err == nil {
			return ds, nil
		}
		// 如果不是"未找到"错误，直接返回
		if !errors.Is(err, domeval.ErrDatasetNotFound) {
			return nil, err
		}
	}

	// 数据库不存在，尝试从文件系统加载
	return l.LoadFromFile(ctx, datasetID)
}

// LoadFromFile 从文件系统加载数据集
func (l *DatasetLoader) LoadFromFile(ctx context.Context, datasetID string) (*Dataset, error) {
	dirPath := filepath.Join(DatasetDir, datasetID)

	// 检查目录是否存在
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("dataset not found: %s", datasetID)
	}

	// 读取元数据
	metaPath := filepath.Join(dirPath, MetaFileName)
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta.json: %w", err)
	}

	var metadata DatasetMetadata
	if err := json.Unmarshal(metaData, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse meta.json: %w", err)
	}

	// 读取样本数据
	samplesPath := filepath.Join(dirPath, SamplesFileName)
	samplesFile, err := os.Open(samplesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open samples.jsonl: %w", err)
	}
	defer samplesFile.Close()

	qaPairs := make([]*QAPair, 0)
	decoder := json.NewDecoder(samplesFile)
	for decoder.More() {
		var qa QAPair
		if err := decoder.Decode(&qa); err != nil {
			return nil, fmt.Errorf("failed to parse sample: %w", err)
		}
		qaPairs = append(qaPairs, &qa)
	}

	// 验证数据集格式
	if err := l.validateDataset(&Dataset{
		Metadata: &metadata,
		QAPairs:  qaPairs,
	}); err != nil {
		return nil, fmt.Errorf("dataset validation failed: %w", err)
	}

	return &Dataset{
		Metadata: &metadata,
		QAPairs:  qaPairs,
	}, nil
}

// LoadFromDB 从数据库加载数据集
func (l *DatasetLoader) LoadFromDB(ctx context.Context, datasetID string) (*Dataset, error) {
	if l.datasetRepo == nil {
		return nil, fmt.Errorf("dataset repository not available")
	}

	// 加载数据集元数据
	datasetEntity, err := l.datasetRepo.FindByID(ctx, datasetID)
	if err != nil {
		return nil, err
	}

	// 加载样本数据
	page, pageSize := 1, 10000 // 最多加载 10000 条样本
	var allQAPairs []*QAPair

	for {
		records, total, err := l.datasetRepo.ListSamples(ctx, datasetID, 0, page, pageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to load samples: %w", err)
		}

		// 转换为 QAPair（含 Agent 期望标注，供 tool_selection/tool_order/trajectory 计算；
		// 缺失会让 Agent 指标误判为 0，故必须一并回填）
		for _, record := range records {
			allQAPairs = append(allQAPairs, &QAPair{
				Question:        record.Question,
				ReferenceAnswer: record.ReferenceAnswer,
				RelevantPIDs:    record.RelevantPIDs,
				Context:         record.Context,
				ExpectedTools:   record.ExpectedTools,
				ExpectedSteps:   record.ExpectedSteps,
			})
		}

		// 检查是否已加载完所有样本
		if int64(len(allQAPairs)) >= total {
			break
		}
		page++
	}

	// 构建返回的 Dataset
	return &Dataset{
		Metadata: &DatasetMetadata{
			ID:          datasetEntity.DatasetID,
			Name:        datasetEntity.Name,
			Description: datasetEntity.Description,
				Type:        DatasetType(datasetEntity.Type),
				EvalType:    EvaluationType(datasetEntity.EvaluationType),
			QACount:     datasetEntity.QACount,
			ModifiedAt:  datasetEntity.UpdatedAt,
		},
		QAPairs: allQAPairs,
	}, nil
}

// validateDataset 验证数据集格式
func (l *DatasetLoader) validateDataset(ds *Dataset) error {
	if ds.Metadata == nil {
		return fmt.Errorf("metadata is required")
	}

	if ds.Metadata.ID == "" {
		return fmt.Errorf("dataset_id is required")
	}

	if len(ds.QAPairs) == 0 {
		return fmt.Errorf("at least one QA pair is required")
	}

	for i, qa := range ds.QAPairs {
		if qa.Question == "" {
			return fmt.Errorf("question at index %d is empty", i)
		}
		if qa.ReferenceAnswer == "" {
			return fmt.Errorf("reference_answer at index %d is empty", i)
		}
	}

	return nil
}

// ListDatasets 列出可用的数据集（合并数据库和文件系统）
func (l *DatasetLoader) ListDatasets(ctx context.Context) ([]*DatasetMetadata, error) {
	// 用于去重的 map（dataset_id -> DatasetMetadata）
	datasetMap := make(map[string]*DatasetMetadata)

	// 获取数据库数据集
	if l.datasetRepo != nil {
		filter := &domeval.DatasetFilter{
			Page:     1,
			PageSize: 1000, // 最多返回 1000 个数据集
		}
		dbDatasets, _, err := l.datasetRepo.List(ctx, filter)
		if err == nil {
			for _, ds := range dbDatasets {
				datasetMap[ds.DatasetID] = &DatasetMetadata{
					ID:          ds.DatasetID,
					Name:        ds.Name,
					Description: ds.Description,
					Type:        DatasetType(ds.Type),
					EvalType:    EvaluationType(ds.EvaluationType),
					QACount:     ds.QACount,
					ModifiedAt:  ds.UpdatedAt,
				}
			}
		}
	}

	// 获取文件系统数据集
	fileDatasets, err := l.scanFileDatasets()
	if err == nil {
		for _, ds := range fileDatasets {
			// 仅当数据库中不存在时才添加文件系统数据集
			if _, exists := datasetMap[ds.ID]; !exists {
				datasetMap[ds.ID] = ds
			}
		}
	}

	// 转换为切片
	result := make([]*DatasetMetadata, 0, len(datasetMap))
	for _, ds := range datasetMap {
		result = append(result, ds)
	}

	return result, nil
}

// scanFileDatasets 扫描文件系统数据集
func (l *DatasetLoader) scanFileDatasets() ([]*DatasetMetadata, error) {
	// 检查数据集目录是否存在
	if _, err := os.Stat(DatasetDir); os.IsNotExist(err) {
		return []*DatasetMetadata{}, nil
	}

	entries, err := os.ReadDir(DatasetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset directory: %w", err)
	}

	datasets := make([]*DatasetMetadata, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		datasetID := entry.Name()
		metadata, err := l.loadDatasetMetadata(datasetID)
		if err != nil {
			// 跳过无效的数据集
			continue
		}

		metadata.Type = DatasetTypeFile
		datasets = append(datasets, metadata)
	}

	return datasets, nil
}

// loadDatasetMetadata 加载数据集元数据
func (l *DatasetLoader) loadDatasetMetadata(datasetID string) (*DatasetMetadata, error) {
	metaPath := filepath.Join(DatasetDir, datasetID, MetaFileName)
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var metadata DatasetMetadata
	if err := json.Unmarshal(metaData, &metadata); err != nil {
		return nil, err
	}

	metadata.ID = datasetID
	return &metadata, nil
}

// GetDatasetInfo 获取数据集信息
func (l *DatasetLoader) GetDatasetInfo(ctx context.Context, datasetID string) (*DatasetMetadata, error) {
	// 先尝试文件系统
	metaPath := filepath.Join(DatasetDir, datasetID, MetaFileName)
	if _, err := os.Stat(metaPath); err == nil {
		return l.loadDatasetMetadata(datasetID)
	}

	// 回退数据库：seed 脚本与前端上传的数据集只落库，无文件系统副本，
	// 必须从库里取元信息，否则创建评测任务时永远 dataset not found。
	if l.datasetRepo != nil {
		ds, err := l.datasetRepo.FindByID(ctx, datasetID)
		if err != nil {
			return nil, err
		}
		return &DatasetMetadata{
			ID:          ds.DatasetID,
			Name:        ds.Name,
			Description: ds.Description,
			Type:        DatasetType(ds.Type),
			EvalType:    EvaluationType(ds.EvaluationType),
			QACount:     ds.QACount,
			ModifiedAt:  ds.UpdatedAt,
		}, nil
	}

	return nil, fmt.Errorf("dataset not found: %s", datasetID)
}

// CreateDataset 创建数据库数据集
func (l *DatasetLoader) CreateDataset(ctx context.Context, tenantID int64, datasetID string, metadata *DatasetMetadata, qaPairs []*QAPair) error {
	if l.datasetRepo == nil {
		return fmt.Errorf("dataset repository not available")
	}

	// 创建数据集实体
	dataset := domeval.NewDataset(datasetID, tenantID, 0, metadata.Name, domeval.EvaluationType(metadata.EvalType))
	if metadata.Description != "" {
		dataset.UpdateDescription(metadata.Description)
	}

	// 创建数据集
	if err := l.datasetRepo.Create(ctx, dataset); err != nil {
		return fmt.Errorf("failed to create dataset: %w", err)
	}

	// 如果有样本，添加样本
	if len(qaPairs) > 0 {
		records := make([]*domeval.DatasetRecord, len(qaPairs))
		for i, qa := range qaPairs {
			records[i] = domeval.NewDatasetRecord(datasetID, tenantID, qa.Question, qa.ReferenceAnswer)
			if len(qa.RelevantPIDs) > 0 {
				records[i].SetRelevantPIDs(qa.RelevantPIDs)
			}
			if qa.Context != "" {
				records[i].SetContext(qa.Context)
			}
		}

		if err := l.datasetRepo.AddSamples(ctx, datasetID, records); err != nil {
			return fmt.Errorf("failed to add samples: %w", err)
		}
	}

	return nil
}

// CreateEvaluationTask 创建评测任务
func (l *DatasetLoader) CreateEvaluationTask(ctx context.Context, req *CreateEvaluationTaskRequest) (*task.Task, error) {
	config := &EvaluationTaskConfig{
		DatasetID: req.DatasetID,
		Type:      req.Type,
		KnowledgeBaseID:      req.KnowledgeBaseID,
		AgentID:   req.AgentID,
		ModelID:   req.ModelID,
		Config:    req.Config,
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	payload := map[string]interface{}{
		"dataset_id":     config.DatasetID,
		"evaluation_type": string(config.Type),
		"kb_id":          config.KnowledgeBaseID,
		"agent_id":       config.AgentID,
		"model_id":       config.ModelID,
		"config":         config.Config,
	}

	task := &task.Task{
		Type:       task.TaskTypeEvaluation,
		Payload:    payload,
		Status:     task.TaskStatusPending,
		MaxRetries: 3,
	}

	return task, nil
}

// CreateEvaluationTaskRequest 创建评测任务请求
type CreateEvaluationTaskRequest struct {
	DatasetID string                 `json:"dataset_id" binding:"required"`
	Type      EvaluationType         `json:"type" binding:"required"`
	KnowledgeBaseID      string                 `json:"kb_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	ModelID   string                 `json:"model_id,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

// Validate 验证评测配置
func (c *EvaluationTaskConfig) Validate() error {
	if c.DatasetID == "" {
		return fmt.Errorf("dataset_id is required")
	}

	if !c.Type.IsValid() {
		return fmt.Errorf("invalid evaluation_type: %s", c.Type)
	}

	// Agent 类型必须有 agent_id
	if c.Type == EvaluationTypeAgent && c.AgentID == "" {
		return fmt.Errorf("agent_id is required for agent evaluation")
	}

	// RAG 类型必须有 kb_id
	if c.Type == EvaluationTypeRAG && c.KnowledgeBaseID == "" {
		return fmt.Errorf("kb_id is required for RAG evaluation")
	}

	// model_id：QA/RAG 用它选择被测生成模型；Agent 类型的被测对象是运行中的 Agent，
	// 模型由 Agent 自身配置内嵌，故 agent 类型不强制 model_id。
	if c.Type != EvaluationTypeAgent && c.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}

	return nil
}

// NewDatasetFile 创建新的文件系统数据集
func (l *DatasetLoader) NewDatasetFile(datasetID, name, description string, evalType EvaluationType, qaPairs []*QAPair) error {
	dirPath := filepath.Join(DatasetDir, datasetID)

	// 创建目录
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create dataset directory: %w", err)
	}

	// 创建元数据
	metadata := DatasetMetadata{
		ID:          datasetID,
		Name:        name,
		Description: description,
		Type:        DatasetTypeFile,
		EvalType:    evalType,
		QACount:     len(qaPairs),
		ModifiedAt:  time.Now(),
	}

	metaData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaPath := filepath.Join(dirPath, MetaFileName)
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// 写入样本
	samplesPath := filepath.Join(dirPath, SamplesFileName)
	samplesFile, err := os.Create(samplesPath)
	if err != nil {
		return fmt.Errorf("failed to create samples file: %w", err)
	}
	defer samplesFile.Close()

	encoder := json.NewEncoder(samplesFile)
	for _, qa := range qaPairs {
		if err := encoder.Encode(qa); err != nil {
			return fmt.Errorf("failed to encode sample: %w", err)
		}
	}

	return nil
}
