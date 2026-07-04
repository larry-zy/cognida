// Package evaluation 评测应用服务测试
package evaluation

import (
	"context"
	"errors"
	"testing"

	domaintask "link/internal/model/task"
	domeval "link/internal/model/evaluation"
	evaluationcache "link/internal/infrastructure/cache/evaluation"
)

// Mock repositories for testing
type mockTaskRepo struct {
	tasks      map[string]*domeval.EvaluationTask
	createErr  error
	findErr    error
	updateErr  error
	deleteErr  error
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
		result = append(result, task)
	}
	return result, int64(len(result)), nil
}

func (m *mockTaskRepo) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type mockResultRepo struct {
	results      map[string][]*domeval.EvaluationResult
	createErr    error
	findErr      error
	deleteErr    error
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

func (m *mockResultRepo) DeleteByTaskID(ctx context.Context, taskID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.results, taskID)
	return nil
}

type mockDatasetLoader struct {
	datasets  map[string]*DatasetMetadata
	listErr   error
	findErr   error
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
		DatasetID: "ds-001",
		Type:      EvaluationTypeRAG,
		KnowledgeBaseID:      "kb-001",
		ModelID:   "model-001", // Required for all types
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

func TestService_CreateEvaluation_DatasetNotFound(t *testing.T) {
	taskRepo := newMockTaskRepo()
	resultRepo := newMockResultRepo()
	dsService := newMockDatasetLoader()
	progressCache := evaluationcache.NewProgressCache(nil)

	service := NewService(dsService, taskRepo, resultRepo, progressCache, nil, nil)

	req := &CreateEvaluationTaskRequest{
		DatasetID: "nonexistent",
		Type:      EvaluationTypeRAG,
		KnowledgeBaseID:      "kb-001",
		ModelID:   "model-001",
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
		AgentID:   "agent-001", // Required for Agent type
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

	result, err := service.ListEvaluationResults(context.Background(), 1, 1, 10)
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
