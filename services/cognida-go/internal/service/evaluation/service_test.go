// Package evaluation 评测应用服务测试
package evaluation

import (
	"context"
	"errors"
	"strings"
	"testing"

	evaluationcache "cognida/internal/infrastructure/cache/evaluation"
	domeval "cognida/internal/model/evaluation"
	domaintask "cognida/internal/model/task"
	"cognida/internal/pkg/pagination"
)

// Mock repositories for testing
type mockTaskRepo struct {
	tasks     map[string]*domeval.EvaluationTask
	createErr error
	findErr   error
	updateErr error
	deleteErr error
}

func newMockTaskRepo() *mockTaskRepo {
	return &mockTaskRepo{
		tasks: make(map[string]*domeval.EvaluationTask),
	}
}

func (m *mockTaskRepo) Create(ctx context.Context, task *domeval.EvaluationTask) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskRepo) FindByID(ctx context.Context, id string) (*domeval.EvaluationTask, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	task, ok := m.tasks[id]
	if !ok {
		return nil, domeval.ErrTaskNotFound
	}
	return task, nil
}

func (m *mockTaskRepo) UpdateStatus(ctx context.Context, id string, status domeval.TaskStatus) error {
	task, ok := m.tasks[id]
	if !ok {
		return domeval.ErrTaskNotFound
	}
	task.SetStatus(status)
	return m.updateErr
}

func (m *mockTaskRepo) UpdateProgress(ctx context.Context, id string, success, failure int) error {
	task, ok := m.tasks[id]
	if !ok {
		return domeval.ErrTaskNotFound
	}
	task.SuccessCount = success
	task.FailureCount = failure
	return m.updateErr
}

func (m *mockTaskRepo) UpdateError(ctx context.Context, id string, errorMsg string) error {
	task, ok := m.tasks[id]
	if !ok {
		return domeval.ErrTaskNotFound
	}
	task.SetError(errorMsg)
	return m.updateErr
}

func (m *mockTaskRepo) UpdateMetrics(ctx context.Context, id string, metrics *domeval.TaskMetrics) error {
	task, ok := m.tasks[id]
	if !ok {
		return domeval.ErrTaskNotFound
	}
	task.Metrics = metrics
	return m.updateErr
}

func (m *mockTaskRepo) SoftDelete(ctx context.Context, id string) error {
	_, ok := m.tasks[id]
	if !ok {
		return domeval.ErrTaskNotFound
	}
	delete(m.tasks, id) // Actually remove from map
	return m.deleteErr
}

func (m *mockTaskRepo) List(ctx context.Context, filter *domeval.TaskFilter) ([]*domeval.EvaluationTask, int64, error) {
	var result []*domeval.EvaluationTask
	for _, task := range m.tasks {
		// honor 下推的过滤条件（模拟 DB 层过滤），以便验证 status/type 过滤下推。
		if filter != nil {
			if filter.TenantID != nil && task.TenantID != *filter.TenantID {
				continue
			}
			if filter.Status != nil && task.Status != *filter.Status {
				continue
			}
			if filter.Type != nil && task.Type != *filter.Type {
				continue
			}
			if filter.TaskID != "" && !strings.Contains(task.ID, filter.TaskID) {
				continue
			}
			if filter.DatasetID != nil && *filter.DatasetID != "" && !strings.Contains(task.DatasetID, *filter.DatasetID) {
				continue
			}
			if len(filter.DatasetIDs) > 0 {
				ok := false
				for _, id := range filter.DatasetIDs {
					if task.DatasetID == id {
						ok = true
						break
					}
				}
				if !ok {
					continue
				}
			}
		}
		result = append(result, task)
	}
	return result, int64(len(result)), nil
}

func (m *mockTaskRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type mockResultRepo struct {
	results   map[string][]*domeval.EvaluationResult
	createErr error
	findErr   error
	deleteErr error
}

func newMockResultRepo() *mockResultRepo {
	return &mockResultRepo{
		results: make(map[string][]*domeval.EvaluationResult),
	}
}

func (m *mockResultRepo) Create(ctx context.Context, result *domeval.EvaluationResult) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.results[result.TaskID] = append(m.results[result.TaskID], result)
	return nil
}

func (m *mockResultRepo) CreateBatch(ctx context.Context, results []*domeval.EvaluationResult) error {
	if m.createErr != nil {
		return m.createErr
	}
	for _, result := range results {
		m.results[result.TaskID] = append(m.results[result.TaskID], result)
	}
	return nil
}

func (m *mockResultRepo) FindByTaskID(ctx context.Context, taskID string) ([]*domeval.EvaluationResult, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	results, ok := m.results[taskID]
	if !ok {
		return []*domeval.EvaluationResult{}, nil
	}
	return results, nil
}

func (m *mockResultRepo) FindByTaskIDWithPagination(ctx context.Context, taskID string, page, pageSize int) ([]*domeval.EvaluationResult, int64, error) {
	if m.findErr != nil {
		return nil, 0, m.findErr
	}
	results, ok := m.results[taskID]
	if !ok {
		return []*domeval.EvaluationResult{}, 0, nil
	}

	total := int64(len(results))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = len(results)
	}

	start := (page - 1) * pageSize
	if start >= len(results) {
		return []*domeval.EvaluationResult{}, total, nil
	}
	end := start + pageSize
	if end > len(results) {
		end = len(results)
	}
	return results[start:end], total, nil
}

func (m *mockResultRepo) FindByTaskIDByCursor(ctx context.Context, taskID string, cursor string, limit int) ([]*domeval.EvaluationResult, string, error) {
	if m.findErr != nil {
		return nil, "", m.findErr
	}
	rows := m.results[taskID]
	if limit <= 0 {
		limit = 20
	}
	start := 0
	if cur, err := pagination.Decode(cursor); err == nil && !cur.IsZero() {
		start = int(cur.ID)
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + limit
	next := ""
	if end < len(rows) {
		next = pagination.Cursor{ID: int64(end)}.Encode()
	} else {
		end = len(rows)
	}
	return rows[start:end], next, nil
}

func (m *mockResultRepo) DeleteByTaskID(ctx context.Context, taskID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.results, taskID)
	return nil
}

func (m *mockResultRepo) ReplaceByTaskID(ctx context.Context, taskID string, results []*domeval.EvaluationResult) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if m.createErr != nil {
		return m.createErr
	}
	delete(m.results, taskID)
	for _, result := range results {
		m.results[result.TaskID] = append(m.results[result.TaskID], result)
	}
	return nil
}

type mockDatasetLoader struct {
	datasets map[string]*DatasetMetadata
	listErr  error
	findErr  error
}

func newMockDatasetLoader() *mockDatasetLoader {
	return &mockDatasetLoader{
		datasets: make(map[string]*DatasetMetadata),
	}
}

func (m *mockDatasetLoader) GetDatasetInfo(ctx context.Context, datasetID string) (*DatasetMetadata, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	ds, ok := m.datasets[datasetID]
	if !ok {
		return nil, domeval.ErrDatasetNotFound
	}
	return ds, nil
}

func (m *mockDatasetLoader) ListDatasets(ctx context.Context) ([]*DatasetMetadata, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var result []*DatasetMetadata
	for _, ds := range m.datasets {
		result = append(result, ds)
	}
	return result, nil
}

func (m *mockDatasetLoader) Load(ctx context.Context, datasetID string) (*Dataset, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return &Dataset{
		Metadata: m.datasets[datasetID],
		QAPairs: []*QAPair{
			{
				Question:        "What is AI?",
				ReferenceAnswer: "Artificial Intelligence",
			},
		},
	}, nil
}

func TestService_CreateEvaluation(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add mock dataset
	dsService.datasets["ds-001"] = &DatasetMetadata{
		ID:       "ds-001",
		Name:     "Test Dataset",
		EvalType: EvaluationTypeRAG,
		QACount:  10,
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	req := &CreateEvaluationTaskRequest{
		DatasetID:       "ds-001",
		Type:            EvaluationTypeRAG,
		KnowledgeBaseID: "kb-001",
		ModelID:         "model-001", // Required for all types
	}

	task, err := service.CreateEvaluation(context.Background(), 1, 10, req)
	if err != nil {
		t.Fatalf("CreateEvaluation() error = %v", err)
	}

	if task == nil {
		t.Fatal("task should not be nil")
	}
	if task.Type != domaintask.TaskTypeEvaluation {
		t.Errorf("task.Type = %v, want %v", task.Type, domaintask.TaskTypeEvaluation)
	}
	if task.Status != domaintask.TaskStatusPending {
		t.Errorf("task.Status = %v, want %v", task.Status, domaintask.TaskStatusPending)
	}
	if task.TenantID != 1 {
		t.Errorf("task.TenantID = %v, want 1", task.TenantID)
	}
	if task.UserID != 10 {
		t.Errorf("task.UserID = %v, want 10", task.UserID)
	}

	// Verify task was saved to repo
	savedTask, err := taskRepo.FindByID(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if savedTask.ID != task.ID {
		t.Errorf("saved task ID = %v, want %v", savedTask.ID, task.ID)
	}
}

func TestService_CreateEvaluation_EmptyModelIDAllowedForRAG(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)
	dsService.datasets["ds-001"] = &DatasetMetadata{
		ID:       "ds-001",
		Name:     "Test Dataset",
		EvalType: EvaluationTypeRAG,
		QACount:  1,
	}
	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	req := &CreateEvaluationTaskRequest{
		DatasetID:       "ds-001",
		Type:            EvaluationTypeRAG,
		KnowledgeBaseID: "kb-001",
		ModelID:         "", // 前端「默认模型」
	}
	task, err := service.CreateEvaluation(context.Background(), 1, 10, req)
	if err != nil {
		t.Fatalf("empty model_id should use system default, got %v", err)
	}
	if task == nil {
		t.Fatal("task should not be nil")
	}
}

func TestService_CreateEvaluation_DatasetNotFound(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	req := &CreateEvaluationTaskRequest{
		DatasetID:       "nonexistent",
		Type:            EvaluationTypeRAG,
		KnowledgeBaseID: "kb-001",
		ModelID:         "model-001",
	}

	_, err := service.CreateEvaluation(context.Background(), 1, 10, req)
	if !errors.Is(err, domeval.ErrDatasetNotFound) {
		t.Errorf("error = %v, want ErrDatasetNotFound", err)
	}
}

func TestService_CreateEvaluation_TypeMismatch(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add dataset with RAG type
	dsService.datasets["ds-001"] = &DatasetMetadata{
		ID:       "ds-001",
		Name:     "RAG Dataset",
		EvalType: EvaluationTypeRAG,
		QACount:  10,
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	req := &CreateEvaluationTaskRequest{
		DatasetID: "ds-001",
		Type:      EvaluationTypeAgent, // Mismatch
		AgentID:   "agent-001",         // Required for Agent type
		ModelID:   "model-001",
	}

	_, err := service.CreateEvaluation(context.Background(), 1, 10, req)
	if !errors.Is(err, domeval.ErrDatasetTypeMismatch) {
		t.Errorf("error = %v, want ErrDatasetTypeMismatch", err)
	}
}

func TestService_GetEvaluationResult(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Create a mock task
	task := domeval.NewEvaluationTask("task-001", 1, 10, "ds-001", domeval.EvaluationTypeRAG, 10)
	task.SetStatus(domeval.TaskStatusCompleted)
	taskRepo.tasks["task-001"] = task

	// Add mock results
	resultRepo.results["task-001"] = []*domeval.EvaluationResult{
		{
			TaskID:          "task-001",
			Question:        "What is AI?",
			ReferenceAnswer: "AI is...",
			GeneratedAnswer: "Artificial Intelligence",
			Success:         true,
		},
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	result, err := service.GetEvaluationResult(context.Background(), "task-001")
	if err != nil {
		t.Fatalf("GetEvaluationResult() error = %v", err)
	}

	if result.TaskID != "task-001" {
		t.Errorf("TaskID = %v, want task-001", result.TaskID)
	}
	if result.Status != domeval.TaskStatusCompleted {
		t.Errorf("Status = %v, want %v", result.Status, domeval.TaskStatusCompleted)
	}
}

func TestService_GetEvaluationResult_TaskNotFound(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	_, err := service.GetEvaluationResult(context.Background(), "nonexistent")
	if !errors.Is(err, domeval.ErrTaskNotFound) {
		t.Errorf("error = %v, want ErrTaskNotFound", err)
	}
}

func TestService_ListEvaluationResults(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add mock tasks
	taskRepo.tasks["task-001"] = domeval.NewEvaluationTask("task-001", 1, 10, "ds-001", domeval.EvaluationTypeRAG, 10)
	taskRepo.tasks["task-002"] = domeval.NewEvaluationTask("task-002", 1, 10, "ds-002", domeval.EvaluationTypeAgent, 5)

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	result, err := service.ListEvaluationResults(context.Background(), 1, 1, 10, ListEvaluationFilter{})
	if err != nil {
		t.Fatalf("ListEvaluationResults() error = %v", err)
	}

	if len(result.Items) != 2 {
		t.Errorf("Items length = %v, want 2", len(result.Items))
	}
	if result.Total != 2 {
		t.Errorf("Total = %v, want 2", result.Total)
	}
	if result.Page != 1 {
		t.Errorf("Page = %v, want 1", result.Page)
	}
	if result.PageSize != 10 {
		t.Errorf("PageSize = %v, want 10", result.PageSize)
	}
}

// TestService_ListEvaluationResults_FilterPushdown 验证 status/type 过滤被下推到仓储层，
// 由过滤后的结果集决定 Items 与真实 Total（不再是「先分页再内存过滤」）。
func TestService_ListEvaluationResults_FilterPushdown(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	ragRunning := domeval.NewEvaluationTask("task-001", 1, 10, "ds-001", domeval.EvaluationTypeRAG, 10)
	ragRunning.SetStatus(domeval.TaskStatusRunning)
	agentCompleted := domeval.NewEvaluationTask("task-002", 1, 10, "ds-002", domeval.EvaluationTypeAgent, 5)
	agentCompleted.SetStatus(domeval.TaskStatusCompleted)
	ragCompleted := domeval.NewEvaluationTask("task-003", 1, 10, "ds-003", domeval.EvaluationTypeRAG, 8)
	ragCompleted.SetStatus(domeval.TaskStatusCompleted)
	taskRepo.tasks["task-001"] = ragRunning
	taskRepo.tasks["task-002"] = agentCompleted
	taskRepo.tasks["task-003"] = ragCompleted

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	// 仅按 status=completed 过滤 → 命中 task-002/task-003，Total=2
	byStatus, err := service.ListEvaluationResults(context.Background(), 1, 1, 10, ListEvaluationFilter{Status: "completed"})
	if err != nil {
		t.Fatalf("ListEvaluationResults(status) error = %v", err)
	}
	if byStatus.Total != 2 || len(byStatus.Items) != 2 {
		t.Errorf("status filter: Total=%d len=%d, want 2/2", byStatus.Total, len(byStatus.Items))
	}

	// status=completed + type=rag → 仅命中 task-003
	byBoth, err := service.ListEvaluationResults(context.Background(), 1, 1, 10, ListEvaluationFilter{Status: "completed", EvalType: "rag"})
	if err != nil {
		t.Fatalf("ListEvaluationResults(status+type) error = %v", err)
	}
	if byBoth.Total != 1 || len(byBoth.Items) != 1 {
		t.Fatalf("status+type filter: Total=%d len=%d, want 1/1", byBoth.Total, len(byBoth.Items))
	}
	if byBoth.Items[0].TaskID != "task-003" {
		t.Errorf("status+type filter: got %s, want task-003", byBoth.Items[0].TaskID)
	}
}

// TestService_GetQAResultsByCursor 验证游标（keyset）分页〔M5〕：分两页取满 3 条结果，
// 首页返回 nextCursor 且 HasMore=true，用该游标取下一页取回剩余结果并到达末页。
func TestService_GetQAResultsByCursor(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	resultRepo.results["task-001"] = []*domeval.EvaluationResult{
		{TaskID: "task-001", Question: "q1", Success: true},
		{TaskID: "task-001", Question: "q2", Success: true},
		{TaskID: "task-001", Question: "q3", Success: false},
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	// 第 1 页：pageSize=2 → 取回 2 条，仍有下一页。
	page1, err := service.GetQAResultsByCursor(context.Background(), "task-001", "", 2)
	if err != nil {
		t.Fatalf("GetQAResultsByCursor(page1) error = %v", err)
	}
	if len(page1.Results) != 2 {
		t.Fatalf("page1 Results len = %d, want 2", len(page1.Results))
	}
	if !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1 HasMore=%v NextCursor=%q, want has_more with cursor", page1.HasMore, page1.NextCursor)
	}

	// 第 2 页：携带上一页游标 → 取回剩余 1 条，到达末页。
	page2, err := service.GetQAResultsByCursor(context.Background(), "task-001", page1.NextCursor, 2)
	if err != nil {
		t.Fatalf("GetQAResultsByCursor(page2) error = %v", err)
	}
	if len(page2.Results) != 1 {
		t.Fatalf("page2 Results len = %d, want 1", len(page2.Results))
	}
	if page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("page2 HasMore=%v NextCursor=%q, want末页", page2.HasMore, page2.NextCursor)
	}
	if page2.Results[0].Question != "q3" {
		t.Errorf("page2 first question = %q, want q3", page2.Results[0].Question)
	}
}

func TestService_DeleteEvaluationTask(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add a completed task
	task := domeval.NewEvaluationTask("task-001", 1, 10, "ds-001", domeval.EvaluationTypeRAG, 10)
	task.SetStatus(domeval.TaskStatusCompleted)
	taskRepo.tasks["task-001"] = task

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	err := service.DeleteEvaluationTask(context.Background(), "task-001")
	if err != nil {
		t.Fatalf("DeleteEvaluationTask() error = %v", err)
	}

	// Task should be removed
	_, err = taskRepo.FindByID(context.Background(), "task-001")
	if !errors.Is(err, domeval.ErrTaskNotFound) {
		t.Error("task should be deleted")
	}
}

func TestService_DeleteEvaluationTask_RunningTask(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add a running task
	task := domeval.NewEvaluationTask("task-001", 1, 10, "ds-001", domeval.EvaluationTypeRAG, 10)
	task.SetStatus(domeval.TaskStatusRunning)
	taskRepo.tasks["task-001"] = task

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	err := service.DeleteEvaluationTask(context.Background(), "task-001")
	if err == nil {
		t.Error("should return error for running task")
	}
}

func TestService_GetDatasetList(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	// Add mock datasets
	dsService.datasets["ds-001"] = &DatasetMetadata{
		ID:       "ds-001",
		Name:     "Dataset 1",
		EvalType: EvaluationTypeRAG,
		QACount:  10,
	}
	dsService.datasets["ds-002"] = &DatasetMetadata{
		ID:       "ds-002",
		Name:     "Dataset 2",
		EvalType: EvaluationTypeAgent,
		QACount:  5,
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	datasets, err := service.GetDatasetList(context.Background())
	if err != nil {
		t.Fatalf("GetDatasetList() error = %v", err)
	}

	if len(datasets) != 2 {
		t.Errorf("datasets length = %v, want 2", len(datasets))
	}
}

func TestService_GetDatasetInfo(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	dsService.datasets["ds-001"] = &DatasetMetadata{
		ID:       "ds-001",
		Name:     "Test Dataset",
		EvalType: EvaluationTypeRAG,
		QACount:  10,
	}

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	info, err := service.GetDatasetInfo(context.Background(), "ds-001")
	if err != nil {
		t.Fatalf("GetDatasetInfo() error = %v", err)
	}

	if info.ID != "ds-001" {
		t.Errorf("ID = %v, want ds-001", info.ID)
	}
	if info.Name != "Test Dataset" {
		t.Errorf("Name = %v, want Test Dataset", info.Name)
	}
}
