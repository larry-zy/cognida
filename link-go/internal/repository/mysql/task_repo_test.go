// Package mysql provides TaskRepository unit tests
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"link/internal/model/task"
)

// ========================================
// Mock TaskRepository for Testing
// ========================================

// mockTaskRepository 内存实现的 TaskRepository，用于测试
type mockTaskRepository struct {
	tasks map[string]*task.Task
}

func newMockTaskRepository() *mockTaskRepository {
	return &mockTaskRepository{
		tasks: make(map[string]*task.Task),
	}
}

func (m *mockTaskRepository) Create(ctx context.Context, task *task.Task) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskRepository) FindByID(ctx context.Context, id string) (*task.Task, error) {
	task, ok := m.tasks[id]
	if !ok || task.DeletedAt != nil {
		return nil, assert.AnError
	}
	return task, nil
}

func (m *mockTaskRepository) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*task.Task, int64, error) {
	var result []*task.Task
	for _, t := range m.tasks {
		if t.TenantID == tenantID && t.DeletedAt == nil {
			result = append(result, t)
		}
	}
	total := int64(len(result))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start >= len(result) {
		return []*task.Task{}, total, nil
	}
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *mockTaskRepository) FindByType(ctx context.Context, tenantID int64, taskType string, status string, limit int) ([]*task.Task, error) {
	var result []*task.Task
	count := 0
	for _, t := range m.tasks {
		if t.TenantID == tenantID && t.Type == taskType && t.DeletedAt == nil {
			if status == "" || t.Status == status {
				result = append(result, t)
				count++
				if count >= limit {
					break
				}
			}
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindByStatus(ctx context.Context, status string, limit int) ([]*task.Task, error) {
	var result []*task.Task
	count := 0
	for _, t := range m.tasks {
		if t.Status == status && t.DeletedAt == nil {
			result = append(result, t)
			count++
			if count >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindByTargetID(ctx context.Context, tenantID int64, targetType, targetID string) ([]*task.Task, error) {
	var result []*task.Task
	for _, t := range m.tasks {
		if t.TenantID == tenantID && t.Type == targetType && t.TargetID == targetID && t.DeletedAt == nil {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindPendingTasks(ctx context.Context, taskType string, limit int) ([]*task.Task, error) {
	var result []*task.Task
	count := 0
	for _, t := range m.tasks {
		if t.Status == task.TaskStatusPending && t.DeletedAt == nil {
			if taskType == "" || t.Type == taskType {
				result = append(result, t)
				count++
				if count >= limit {
					break
				}
			}
		}
	}
	return result, nil
}

func (m *mockTaskRepository) FindByParentID(ctx context.Context, parentID string) ([]*task.Task, error) {
	var result []*task.Task
	for _, t := range m.tasks {
		if t.ParentID == parentID && t.DeletedAt == nil {
			result = append(result, t)
		}
	}
	return result, nil
}

func (m *mockTaskRepository) Update(ctx context.Context, task *task.Task) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockTaskRepository) UpdateStatus(ctx context.Context, id string, status string, errMsg string) error {
	task, ok := m.tasks[id]
	if !ok {
		return assert.AnError
	}
	task.Status = status
	task.ErrorMessage = errMsg
	return nil
}

func (m *mockTaskRepository) UpdateResult(ctx context.Context, id string, result map[string]interface{}) error {
	t, ok := m.tasks[id]
	if !ok {
		return assert.AnError
	}
	t.Result = result
	t.Status = task.TaskStatusCompleted
	return nil
}

func (m *mockTaskRepository) IncrementRetry(ctx context.Context, id string) error {
	task, ok := m.tasks[id]
	if !ok {
		return assert.AnError
	}
	task.RetryCount++
	return nil
}

func (m *mockTaskRepository) Delete(ctx context.Context, id string) error {
	task, ok := m.tasks[id]
	if !ok {
		return assert.AnError
	}
	now := time.Now()
	task.DeletedAt = &now
	return nil
}

func (m *mockTaskRepository) DeleteByTenantID(ctx context.Context, tenantID int64) error {
	for _, t := range m.tasks {
		if t.TenantID == tenantID {
			var dummy time.Time
			t.DeletedAt = &dummy
		}
	}
	return nil
}

func (m *mockTaskRepository) Exists(ctx context.Context, id string) (bool, error) {
	t, ok := m.tasks[id]
	return ok && t.DeletedAt == nil, nil
}

func (m *mockTaskRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	count := int64(0)
	for _, t := range m.tasks {
		if t.TenantID == tenantID && t.DeletedAt == nil {
			count++
		}
	}
	return count, nil
}

func (m *mockTaskRepository) CountByTypeAndStatus(ctx context.Context, tenantID int64, taskType, status string) (int64, error) {
	count := int64(0)
	for _, t := range m.tasks {
		if t.TenantID == tenantID && t.DeletedAt == nil {
			if (taskType == "" || t.Type == taskType) && (status == "" || t.Status == status) {
				count++
			}
		}
	}
	return count, nil
}

// ========================================
// Test Helpers
// ========================================

func createTestTask(id string) *task.Task {
	return &task.Task{
		ID:             id,
		TenantID:       1,
		UserID:         100,
		Type:           task.TaskTypeEvaluation,
		TargetID:       "kb-123",
		Status:         task.TaskStatusPending,
		Payload:        map[string]interface{}{"key": "value"},
		RetryCount:     0,
		MaxRetries:     3,
		TimeoutSeconds: 30,
	}
}

// ========================================
// Create Tests
// ========================================

func TestTaskRepository_Create(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("create task successfully", func(t *testing.T) {
		task := createTestTask("task-1")

		err := repo.Create(context.Background(), task)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(repo.tasks))
	})
}

// ========================================
// FindByID Tests
// ========================================

func TestTaskRepository_FindByID(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("find by id successfully", func(t *testing.T) {
		task := createTestTask("task-1")
		_ = repo.Create(context.Background(), task)

		found, err := repo.FindByID(context.Background(), "task-1")
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "task-1", found.ID)
	})

	t.Run("find by id not found", func(t *testing.T) {
		found, err := repo.FindByID(context.Background(), "non-existent")
		assert.Error(t, err)
		assert.Nil(t, found)
	})
}

// ========================================
// FindByTenantID Tests
// ========================================

func TestTaskRepository_FindByTenantID(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("find by tenant id", func(t *testing.T) {
		for i := 1; i <= 15; i++ {
			task := createTestTask(string(rune('a'+i)))
			task.TenantID = 1
			_ = repo.Create(context.Background(), task)
		}

		tasks, total, err := repo.FindByTenantID(context.Background(), 1, 1, 10)
		assert.NoError(t, err)
		assert.Len(t, tasks, 10)
		assert.Equal(t, int64(15), total)
	})
}

// ========================================
// FindByType Tests
// ========================================

func TestTaskRepository_FindByType(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("find by type with status filter", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			testTask := createTestTask("eval-" + string(rune('a'+i)))
			testTask.Type = task.TaskTypeEvaluation
			testTask.Status = task.TaskStatusPending
			_ = repo.Create(context.Background(), testTask)
		}

		tasks, err := repo.FindByType(context.Background(), 1, task.TaskTypeEvaluation, task.TaskStatusPending, 10)
		assert.NoError(t, err)
		assert.Len(t, tasks, 5)
	})
}

// ========================================
// FindPendingTasks Tests
// ========================================

func TestTaskRepository_FindPendingTasks(t *testing.T) {
	t.Run("find pending tasks", func(t *testing.T) {
		repo := newMockTaskRepository()
		for i := 0; i < 3; i++ {
			testTask := createTestTask("pending-" + string(rune('a'+i)))
			testTask.Status = task.TaskStatusPending
			_ = repo.Create(context.Background(), testTask)
		}

		testTask := createTestTask("processing-task")
		testTask.Status = task.TaskStatusProcessing
		_ = repo.Create(context.Background(), testTask)

		tasks, err := repo.FindPendingTasks(context.Background(), "", 10)
		assert.NoError(t, err)
		assert.Len(t, tasks, 3)
	})

	t.Run("find pending tasks by type", func(t *testing.T) {
		repo := newMockTaskRepository()
		evalTask := createTestTask("eval-pending")
		evalTask.Type = task.TaskTypeEvaluation
		evalTask.Status = task.TaskStatusPending
		_ = repo.Create(context.Background(), evalTask)

		docTask := createTestTask("doc-pending")
		docTask.Type = task.TaskTypeDocumentParse
		docTask.Status = task.TaskStatusPending
		_ = repo.Create(context.Background(), docTask)

		tasks, err := repo.FindPendingTasks(context.Background(), task.TaskTypeEvaluation, 10)
		assert.NoError(t, err)
		assert.Len(t, tasks, 1)
		assert.Equal(t, task.TaskTypeEvaluation, tasks[0].Type)
	})
}

// ========================================
// Update Tests
// ========================================

func TestTaskRepository_Update(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("update task", func(t *testing.T) {
		testTask := createTestTask("update-task")
		_ = repo.Create(context.Background(), testTask)

		testTask.Status = task.TaskStatusProcessing
		testTask.RetryCount = 1

		err := repo.Update(context.Background(), testTask)
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "update-task")
		assert.Equal(t, task.TaskStatusProcessing, found.Status)
		assert.Equal(t, 1, found.RetryCount)
	})
}

// ========================================
// UpdateStatus Tests
// ========================================

func TestTaskRepository_UpdateStatus(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("update status to processing", func(t *testing.T) {
		testTask := createTestTask("status-task")
		_ = repo.Create(context.Background(), testTask)

		err := repo.UpdateStatus(context.Background(), "status-task", task.TaskStatusProcessing, "")
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "status-task")
		assert.Equal(t, task.TaskStatusProcessing, found.Status)
	})

	t.Run("update status to failed with error message", func(t *testing.T) {
		testTask := createTestTask("failed-task")
		_ = repo.Create(context.Background(), testTask)

		errMsg := "processing failed"
		err := repo.UpdateStatus(context.Background(), "failed-task", task.TaskStatusFailed, errMsg)
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "failed-task")
		assert.Equal(t, task.TaskStatusFailed, found.Status)
		assert.Equal(t, errMsg, found.ErrorMessage)
	})
}

// ========================================
// UpdateResult Tests
// ========================================

func TestTaskRepository_UpdateResult(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("update task result", func(t *testing.T) {
		testTask := createTestTask("result-task")
		_ = repo.Create(context.Background(), testTask)

		result := map[string]interface{}{
			"score": 0.95,
			"metrics": map[string]interface{}{
				"precision": 0.9,
				"recall": 0.85,
			},
		}

		err := repo.UpdateResult(context.Background(), "result-task", result)
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "result-task")
		assert.Equal(t, task.TaskStatusCompleted, found.Status)
		assert.Equal(t, 0.95, found.Result["score"])
	})
}

// ========================================
// IncrementRetry Tests
// ========================================

func TestTaskRepository_IncrementRetry(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("increment retry count", func(t *testing.T) {
		task := createTestTask("retry-task")
		task.RetryCount = 0
		_ = repo.Create(context.Background(), task)

		err := repo.IncrementRetry(context.Background(), "retry-task")
		assert.NoError(t, err)

		found, _ := repo.FindByID(context.Background(), "retry-task")
		assert.Equal(t, 1, found.RetryCount)
	})
}

// ========================================
// Exists Tests
// ========================================

func TestTaskRepository_Exists(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("task exists", func(t *testing.T) {
		task := createTestTask("exists-task")
		_ = repo.Create(context.Background(), task)

		exists, err := repo.Exists(context.Background(), "exists-task")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("task does not exist", func(t *testing.T) {
		exists, err := repo.Exists(context.Background(), "non-existent")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

// ========================================
// Count Tests
// ========================================

func TestTaskRepository_CountByTenantID(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("count tasks by tenant", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			task := createTestTask("count-" + string(rune('a'+i)))
			task.TenantID = 1
			_ = repo.Create(context.Background(), task)
		}

		count, err := repo.CountByTenantID(context.Background(), 1)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)
	})
}

func TestTaskRepository_CountByTypeAndStatus(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("count by type and status", func(t *testing.T) {
		tenantID := int64(1)

		for i := 0; i < 3; i++ {
			testTask := createTestTask("count-eval-" + string(rune('a'+i)))
			testTask.TenantID = tenantID
			testTask.Type = task.TaskTypeEvaluation
			testTask.Status = task.TaskStatusPending
			_ = repo.Create(context.Background(), testTask)
		}

		docTask := createTestTask("count-doc")
		docTask.TenantID = tenantID
		docTask.Type = task.TaskTypeDocumentParse
		docTask.Status = task.TaskStatusPending
		_ = repo.Create(context.Background(), docTask)

		count, err := repo.CountByTypeAndStatus(context.Background(), tenantID, task.TaskTypeEvaluation, task.TaskStatusPending)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})
}

// ========================================
// FindByParentID Tests
// ========================================

func TestTaskRepository_FindByParentID(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("find child tasks by parent id", func(t *testing.T) {
		parentID := "parent-task"

		for i := 0; i < 3; i++ {
			task := createTestTask("child-" + string(rune('a'+i)))
			task.ParentID = parentID
			_ = repo.Create(context.Background(), task)
		}

		task := createTestTask("orphan-task")
		_ = repo.Create(context.Background(), task)

		children, err := repo.FindByParentID(context.Background(), parentID)
		assert.NoError(t, err)
		assert.Len(t, children, 3)

		for _, child := range children {
			assert.Equal(t, parentID, child.ParentID)
		}
	})
}

// ========================================
// FindByTargetID Tests
// ========================================

func TestTaskRepository_FindByTargetID(t *testing.T) {
	repo := newMockTaskRepository()

	t.Run("find tasks by target id", func(t *testing.T) {
		tenantID := int64(1)
		targetType := task.TaskTypeEvaluation
		targetID := "kb-target"

		for i := 0; i < 2; i++ {
			task := createTestTask("target-" + string(rune('a'+i)))
			task.TenantID = tenantID
			task.Type = targetType
			task.TargetID = targetID
			_ = repo.Create(context.Background(), task)
		}

		tasks, err := repo.FindByTargetID(context.Background(), tenantID, targetType, targetID)
		assert.NoError(t, err)
		assert.Len(t, tasks, 2)

		for _, task := range tasks {
			assert.Equal(t, targetID, task.TargetID)
		}
	})
}
