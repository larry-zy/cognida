// Package evaluation 提供评测系统应用层服务
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	domeval "cognida/internal/model/evaluation"
	"cognida/internal/model/task"
)

// DatasetService 数据集服务接口
type DatasetService interface {
	GetDatasetInfo(ctx context.Context, datasetID string) (*DatasetMetadata, error)
	ListDatasets(ctx context.Context) ([]*DatasetMetadata, error)
}

// GraderCatalogClient 拉取按评测类型过滤的可用指标目录。
// 由 Python 注册表驱动（唯一事实来源），Go 侧仅做类型校验与代理。
type GraderCatalogClient interface {
	ListGraders(ctx context.Context, evalType string) (*GraderCatalog, error)
}

// Service 评测服务
type Service struct {
	datasetService DatasetService
	taskRepo       domeval.EvaluationTaskRepository
	resultRepo     domeval.EvaluationResultRepository
	progressCache  domeval.ProgressReader
	evalQueue      domeval.TaskEnqueuer
	graderCatalog  GraderCatalogClient
}

// NewService 创建评测服务
func NewService(
	datasetService DatasetService,
	taskRepo domeval.EvaluationTaskRepository,
	resultRepo domeval.EvaluationResultRepository,
	progressCache domeval.ProgressReader,
	evalQueue domeval.TaskEnqueuer,
	graderCatalog GraderCatalogClient,
) *Service {
	return &Service{
		datasetService: datasetService,
		taskRepo:       taskRepo,
		resultRepo:     resultRepo,
		progressCache:  progressCache,
		evalQueue:      evalQueue,
		graderCatalog:  graderCatalog,
	}
}

// ListAvailableGraders 返回某评测类型下的可用指标目录。
// eval_type 在 Go 侧先做校验并归一化(qa->llm)，再代理到 Python 注册表；
// 未知类型直接返回 ErrInvalidEvalType，不做静默全量回退。
func (s *Service) ListAvailableGraders(ctx context.Context, evalType string) (*GraderCatalog, error) {
	et := EvaluationType(evalType)
	if !et.IsValid() {
		return nil, fmt.Errorf("%w: %q", domeval.ErrInvalidEvalType, evalType)
	}
	if s.graderCatalog == nil {
		return nil, fmt.Errorf("grader catalog client not configured")
	}
	return s.graderCatalog.ListGraders(ctx, string(et.Normalize()))
}

// CreateEvaluation 创建评测任务
func (s *Service) CreateEvaluation(ctx context.Context, tenantID, userID int64, req *CreateEvaluationTaskRequest) (*task.Task, error) {
	// 验证配置
	config := &EvaluationTaskConfig{
		DatasetID:       req.DatasetID,
		Type:            req.Type,
		KnowledgeBaseID: req.KnowledgeBaseID,
		AgentID:         req.AgentID,
		ModelID:         req.ModelID,
		Config:          req.Config,
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", domeval.ErrInvalidConfig, err)
	}

	// 检查数据集是否存在
	datasetInfo, err := s.datasetService.GetDatasetInfo(ctx, config.DatasetID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domeval.ErrDatasetNotFound, config.DatasetID)
	}

	// 验证评测类型匹配（按归一化后比较：qa 与历史别名 llm 视为同一类型）
	if datasetInfo.EvalType != "" && datasetInfo.EvalType.Normalize() != config.Type.Normalize() {
		return nil, fmt.Errorf("%w: expected %s, got %s", domeval.ErrDatasetTypeMismatch, datasetInfo.EvalType, config.Type)
	}

	// 生成任务ID
	taskID := generateTaskID()

	// 创建领域任务实体
	evalTask := domeval.NewEvaluationTask(
		taskID,
		tenantID,
		userID,
		config.DatasetID,
		domeval.EvaluationType(config.Type),
		datasetInfo.QACount,
	)

	// 设置可选字段
	evalTask.KnowledgeBaseID = config.KnowledgeBaseID
	evalTask.AgentID = config.AgentID
	evalTask.ModelID = config.ModelID
	if config.Config != nil {
		configJSON, _ := json.Marshal(config.Config)
		evalTask.Config = json.RawMessage(configJSON)
	}

	// 保存任务到数据库
	if err := s.taskRepo.Create(ctx, evalTask); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// 构建返回的任务
	payload := map[string]interface{}{
		"dataset_id":      config.DatasetID,
		"evaluation_type": string(config.Type),
		"kb_id":           config.KnowledgeBaseID,
		"agent_id":        config.AgentID,
		"model_id":        config.ModelID,
		"config":          config.Config,
	}

	task := &task.Task{
		ID:         taskID,
		Type:       task.TaskTypeEvaluation,
		Payload:    payload,
		Status:     task.TaskStatusPending,
		MaxRetries: 3,
		TenantID:   tenantID,
		UserID:     userID,
	}

	// 将任务加入队列
	log.Printf("[Service] Enqueuing task %s to Redis queue", taskID)
	if s.evalQueue != nil {
		if err := s.evalQueue.Enqueue(ctx, taskID); err != nil {
			// 入队失败，回滚任务创建
			log.Printf("[Service] Failed to enqueue task %s: %v", taskID, err)
			_ = s.taskRepo.SoftDelete(ctx, taskID)
			return nil, fmt.Errorf("failed to enqueue task: %w", err)
		}
		log.Printf("[Service] Task %s successfully enqueued", taskID)
	} else {
		log.Printf("[Service] WARNING: evalQueue is nil, task %s not enqueued", taskID)
	}

	log.Printf("[Service] Created evaluation task: id=%s, type=%s, dataset=%s", task.ID, task.Type, config.DatasetID)
	return task, nil
}

// generateTaskID 生成任务ID
func generateTaskID() string {
	return fmt.Sprintf("eval-%s", uuid.New().String()[:8])
}

// GetEvaluationResult 获取评测结果
func (s *Service) GetEvaluationResult(ctx context.Context, taskID string) (*EvaluationResultDetail, error) {
	// 获取任务
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// 获取 QA 级别结果
	qaResults, err := s.resultRepo.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}

	// 尽力解析数据集名称（缺失时不影响详情返回）
	datasetName := ""
	if s.datasetService != nil && task.DatasetID != "" {
		if info, err := s.datasetService.GetDatasetInfo(ctx, task.DatasetID); err == nil && info != nil {
			datasetName = info.Name
		}
	}

	return &EvaluationResultDetail{
		TaskID:       task.ID,
		DatasetID:    task.DatasetID,
		DatasetName:  datasetName,
		Type:         task.Type,
		Status:       task.Status,
		Metrics:      task.Metrics,
		QAResults:    qaResults,
		TotalCount:   task.TotalCount,
		SuccessCount: task.SuccessCount,
		FailureCount: task.FailureCount,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}, nil
}

// EvaluationResultDetail 评测结果详情
type EvaluationResultDetail struct {
	TaskID       string                      `json:"task_id"`
	DatasetID    string                      `json:"dataset_id"`
	DatasetName  string                      `json:"dataset_name,omitempty"`
	Type         domeval.EvaluationType      `json:"type"`
	Status       domeval.TaskStatus          `json:"status"`
	Metrics      *domeval.TaskMetrics        `json:"metrics,omitempty"` // 任务级聚合指标
	QAResults    []*domeval.EvaluationResult `json:"qa_results"`
	TotalCount   int                         `json:"total_count"`
	SuccessCount int                         `json:"success_count"`
	FailureCount int                         `json:"failure_count"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

// ListEvaluationFilter 评测任务列表筛选条件（空字段表示不过滤）。
type ListEvaluationFilter struct {
	Status      string // running / failed / completed …
	EvalType    string // rag / qa / agent …
	TaskID      string // 任务 ID 模糊
	DatasetID   string // 数据集 ID 模糊
	DatasetName string // 数据集名称模糊（先解析为 dataset_id 列表再下推）
}

// ListEvaluationResults 列出评测结果。
// 过滤条件下推到仓储层由 DB 统一过滤+分页+返回真实 Total，
// 避免「先分页再内存过滤」导致的 Total 失真与跨页命中丢失。
func (s *Service) ListEvaluationResults(ctx context.Context, tenantID int64, page, pageSize int, f ListEvaluationFilter) (*EvaluationResultList, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 数据集名称字典：既用于列表回填 dataset_name，也用于名称筛选解析 ID
	nameByID := map[string]string{}
	if s.datasetService != nil {
		if all, err := s.datasetService.ListDatasets(ctx); err == nil {
			for _, ds := range all {
				if ds == nil {
					continue
				}
				nameByID[ds.ID] = ds.Name
			}
		}
	}

	filter := &domeval.TaskFilter{
		TenantID: &tenantID,
		Page:     page,
		PageSize: pageSize,
		TaskID:   strings.TrimSpace(f.TaskID),
	}
	if status := strings.TrimSpace(f.Status); status != "" {
		st := domeval.TaskStatus(status)
		filter.Status = &st
	}
	if evalType := strings.TrimSpace(f.EvalType); evalType != "" {
		et := domeval.EvaluationType(evalType)
		filter.Type = &et
	}
	if dsID := strings.TrimSpace(f.DatasetID); dsID != "" {
		filter.DatasetID = &dsID
	}
	if nameQ := strings.TrimSpace(f.DatasetName); nameQ != "" {
		needle := strings.ToLower(nameQ)
		ids := make([]string, 0)
		for id, name := range nameByID {
			if strings.Contains(strings.ToLower(name), needle) {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return &EvaluationResultList{
				Items: []*EvaluationTaskSummary{}, Total: 0,
				Page: page, PageSize: pageSize, TotalPages: 0,
			}, nil
		}
		filter.DatasetIDs = ids
	}

	tasks, total, err := s.taskRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*EvaluationTaskSummary, len(tasks))
	for i, task := range tasks {
		items[i] = &EvaluationTaskSummary{
			TaskID:       task.ID,
			DatasetID:    task.DatasetID,
			DatasetName:  nameByID[task.DatasetID],
			Type:         task.Type,
			Status:       task.Status,
			TotalCount:   task.TotalCount,
			SuccessCount: task.SuccessCount,
			FailureCount: task.FailureCount,
			CreatedAt:    task.CreatedAt,
			UpdatedAt:    task.UpdatedAt,
		}
	}

	return &EvaluationResultList{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (total + int64(pageSize) - 1) / int64(pageSize),
	}, nil
}

// EvaluationTaskSummary 评测任务摘要
type EvaluationTaskSummary struct {
	TaskID       string                 `json:"task_id"`
	DatasetID    string                 `json:"dataset_id"`
	DatasetName  string                 `json:"dataset_name,omitempty"`
	Type         domeval.EvaluationType `json:"type"`
	Status       domeval.TaskStatus     `json:"status"`
	TotalCount   int                    `json:"total_count"`
	SuccessCount int                    `json:"success_count"`
	FailureCount int                    `json:"failure_count"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// EvaluationResultList 评测结果列表
type EvaluationResultList struct {
	Items      []*EvaluationTaskSummary `json:"items"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int64                    `json:"total_pages"`
}

// ProgressInfo 进度信息
type ProgressInfo struct {
	TaskID     string `json:"task_id"`
	Stage      string `json:"stage"`
	Current    int    `json:"current"`
	Total      int    `json:"total"`
	Message    string `json:"message"`
	Percentage int    `json:"percentage"`
	RetryCount int    `json:"retry_count"`
	Error      string `json:"error,omitempty"`
}

// DeleteEvaluationTask 删除评测任务
func (s *Service) DeleteEvaluationTask(ctx context.Context, taskID string) error {
	// 先检查任务是否存在和状态
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}

	// 不能删除运行中的任务
	if task.Status == domeval.TaskStatusRunning {
		return fmt.Errorf("cannot delete running task")
	}

	// 软删除任务
	if err := s.taskRepo.SoftDelete(ctx, taskID); err != nil {
		return err
	}

	// 删除关联的 QA 结果
	return s.resultRepo.DeleteByTaskID(ctx, taskID)
}

// GetTaskProgress 获取任务进度
func (s *Service) GetTaskProgress(ctx context.Context, taskID string) (*ProgressInfo, error) {
	// 从 Redis 获取进度
	progress, err := s.progressCache.GetProgress(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	// 如果没有进度信息，尝试从数据库获取任务信息
	if progress == nil {
		task, err := s.taskRepo.FindByID(ctx, taskID)
		if err != nil {
			return nil, err
		}

		// 返回基于任务状态的进度信息
		return &ProgressInfo{
			TaskID:     taskID,
			Stage:      string(task.Status),
			Current:    task.SuccessCount + task.FailureCount,
			Total:      task.TotalCount,
			Percentage: calculatePercentage(task.SuccessCount+task.FailureCount, task.TotalCount),
			Message:    getStageMessage(task.Status),
		}, nil
	}

	return &ProgressInfo{
		TaskID:     taskID,
		Stage:      string(progress.Stage),
		Current:    progress.Current,
		Total:      progress.Total,
		Message:    progress.Message,
		Percentage: progress.Percentage,
		RetryCount: progress.RetryCount,
		Error:      progress.Error,
	}, nil
}

// calculatePercentage 计算百分比
func calculatePercentage(current, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(current) / float64(total) * 100)
}

// getStageMessage 获取阶段消息
func getStageMessage(status domeval.TaskStatus) string {
	switch status {
	case domeval.TaskStatusPending:
		return "Task is pending"
	case domeval.TaskStatusRunning:
		return "Task is running"
	case domeval.TaskStatusCompleted:
		return "Task completed"
	case domeval.TaskStatusFailed:
		return "Task failed"
	default:
		return "Unknown status"
	}
}

// GetDatasetList 获取数据集列表
func (s *Service) GetDatasetList(ctx context.Context) ([]*DatasetMetadata, error) {
	return s.datasetService.ListDatasets(ctx)
}

// GetDatasetInfo 获取数据集信息
func (s *Service) GetDatasetInfo(ctx context.Context, datasetID string) (*DatasetMetadata, error) {
	return s.datasetService.GetDatasetInfo(ctx, datasetID)
}

// GetQAResults 获取 QA 级别评测结果（分页）
func (s *Service) GetQAResults(ctx context.Context, taskID string, page, pageSize int) (*QAResultList, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 获取分页的 QA 结果
	results, total, err := s.resultRepo.FindByTaskIDWithPagination(ctx, taskID, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &QAResultList{
		Results:  results,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetQAResultsByCursor 游标（keyset）分页〔M5〕获取某任务的 QA 级评测结果。
// 与 GetQAResults 语义一致但走 keyset：以 cursor 为锚点取下一页，nextCursor 为空表示末页，不返回 total。
func (s *Service) GetQAResultsByCursor(ctx context.Context, taskID string, cursor string, pageSize int) (*QAResultCursorList, error) {
	results, nextCursor, err := s.resultRepo.FindByTaskIDByCursor(ctx, taskID, cursor, pageSize)
	if err != nil {
		return nil, err
	}

	return &QAResultCursorList{
		Results:    results,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		PageSize:   pageSize,
	}, nil
}

// QAResultList QA 结果列表
type QAResultList struct {
	Results  []*domeval.EvaluationResult `json:"results"`
	Total    int64                       `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}

// QAResultCursorList QA 结果游标分页列表〔M5〕。
type QAResultCursorList struct {
	Results    []*domeval.EvaluationResult `json:"results"`
	NextCursor string                      `json:"next_cursor"`
	HasMore    bool                        `json:"has_more"`
	PageSize   int                         `json:"page_size"`
}
