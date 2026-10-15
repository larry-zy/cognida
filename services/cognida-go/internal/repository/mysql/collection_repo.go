package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"cognida/internal/model/collection"
)

var _ collection.Repository = (*CollectionRepository)(nil)

// CollectionRepository is the MySQL adapter for the isolated collection domain.
type CollectionRepository struct{ db *gorm.DB }

func NewCollectionRepository(db *gorm.DB) *CollectionRepository { return &CollectionRepository{db: db} }

func (r *CollectionRepository) CreateTask(ctx context.Context, task *collection.Task, sources []*collection.Source) error {
	if err := task.Validate(); err != nil {
		return err
	}
	if len(sources) == 0 || len(sources) > task.Limits.MaxSources {
		return collection.ErrInvalidSource
	}
	for _, source := range sources {
		if source.TaskID != task.ID {
			return collection.ErrInvalidSource
		}
		if err := source.Validate(); err != nil {
			return err
		}
	}
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(fromCollectionTask(task)).Error; err != nil {
			return repositoryError(err)
		}
		models := make([]*CollectionSourceModel, 0, len(sources))
		for _, source := range sources {
			models = append(models, fromCollectionSource(source))
		}
		if err := tx.Create(&models).Error; err != nil {
			return repositoryError(err)
		}
		return nil
	})
}

func (r *CollectionRepository) FindTask(ctx context.Context, tenantID int64, taskID string) (*collection.Task, error) {
	var model CollectionTaskModel
	err := r.scoped(ctx, tenantID).Where("id = ?", taskID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrTaskNotFound
	}
	if err != nil {
		return nil, repositoryError(err)
	}
	return model.ToDomain(), nil
}

func (r *CollectionRepository) ListTasks(ctx context.Context, tenantID int64, status collection.TaskStatus, limit int) ([]*collection.Task, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := r.scoped(ctx, tenantID).Order("created_at DESC").Limit(limit)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var models []*CollectionTaskModel
	if err := query.Find(&models).Error; err != nil {
		return nil, repositoryError(err)
	}
	tasks := make([]*collection.Task, 0, len(models))
	for _, model := range models {
		tasks = append(tasks, model.ToDomain())
	}
	return tasks, nil
}

func (r *CollectionRepository) FindTaskByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*collection.Task, error) {
	var model CollectionTaskModel
	err := r.scoped(ctx, tenantID).Where("idempotency_key = ?", key).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrTaskNotFound
	}
	if err != nil {
		return nil, repositoryError(err)
	}
	return model.ToDomain(), nil
}

func (r *CollectionRepository) ListSources(ctx context.Context, tenantID int64, taskID string) ([]*collection.Source, error) {
	var models []*CollectionSourceModel
	err := r.scoped(ctx, tenantID).Table((CollectionSourceModel{}).TableName()+" AS sources").
		Select("sources.*").Joins("JOIN collection_tasks AS tasks ON tasks.id = sources.task_id").
		Where("sources.task_id = ?", taskID).Order("sources.created_at ASC").Find(&models).Error
	if err != nil {
		return nil, repositoryError(err)
	}
	result := make([]*collection.Source, 0, len(models))
	for _, model := range models {
		result = append(result, model.ToDomain())
	}
	return result, nil
}

func (r *CollectionRepository) UpdateTask(ctx context.Context, task *collection.Task) error {
	query := r.scoped(ctx, task.TenantID).Model(&CollectionTaskModel{}).Where("id = ?", task.ID)
	if task.Status == collection.TaskStatusCancelled {
		query = query.Where("status IN ?", []collection.TaskStatus{collection.TaskStatusDraft, collection.TaskStatusRunning})
	}
	result := query.Updates(map[string]any{"status": string(task.Status), "schedule_enabled": task.Schedule.Enabled, "schedule_interval_seconds": task.Schedule.IntervalSeconds, "next_run_at": task.Schedule.NextRunAt, "last_scheduled_at": task.Schedule.LastScheduledAt, "error_message": task.ErrorMessage, "updated_at": task.UpdatedAt})
	if result.Error != nil {
		return repositoryError(result.Error)
	}
	if result.RowsAffected == 0 {
		if task.Status == collection.TaskStatusCancelled {
			return collection.ErrInvalidTransition
		}
		return collection.ErrTaskNotFound
	}
	return nil
}

func (r *CollectionRepository) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]*collection.Task, error) {
	return r.claimDueTasks(ctx, now, limit, "")
}

// taskID narrows integration probes to their own fixture; production claims all due tasks.
func (r *CollectionRepository) claimDueTasks(ctx context.Context, now time.Time, limit int, taskID string) ([]*collection.Task, error) {
	if limit <= 0 {
		return nil, collection.ErrInvalidTask
	}
	var claimed []*collection.Task
	err := r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		var models []*CollectionTaskModel
		scope := tx
		if taskID != "" {
			scope = scope.Where("id=?", taskID)
		}
		query := scope.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("schedule_enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now.UTC()).
			Where("status IN ?", []string{string(collection.TaskStatusDraft), string(collection.TaskStatusCompleted), string(collection.TaskStatusFailed)}).
			Order("next_run_at ASC").Limit(limit).Find(&models)
		if query.Error != nil {
			return repositoryError(query.Error)
		}
		for _, model := range models {
			if err := model.ToDomain().Schedule.Validate(); err != nil {
				return err
			}
			var active int64
			if err := tx.Model(&CollectionRunModel{}).Where("task_id=? AND tenant_id=? AND status=?", model.ID, model.TenantID, collection.RunStatusRunning).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				continue
			}
			run := &collection.Run{ID: uuid.NewString(), TaskID: model.ID, TenantID: model.TenantID, Status: collection.RunStatusRunning, StartedAt: now.UTC(), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
			if err := tx.Create(fromCollectionRun(run)).Error; err != nil {
				return repositoryError(err)
			}
			next := now.UTC().Add(time.Duration(model.ScheduleIntervalSeconds) * time.Second)
			if err := tx.Model(&CollectionTaskModel{}).Where("id = ? AND tenant_id = ?", model.ID, model.TenantID).
				Updates(map[string]any{"status": string(collection.TaskStatusRunning), "error_message": "", "next_run_at": next, "last_scheduled_at": now.UTC(), "updated_at": now.UTC()}).Error; err != nil {
				return repositoryError(err)
			}
			model.Status = string(collection.TaskStatusRunning)
			model.NextRunAt, model.LastScheduledAt, model.UpdatedAt = &next, timePtr(now.UTC()), now.UTC()
			claimed = append(claimed, model.ToDomain())
		}
		return nil
	})
	return claimed, err
}

func timePtr(value time.Time) *time.Time { return &value }

func (r *CollectionRepository) CreateRun(ctx context.Context, run *collection.Run) error {
	if run.TenantID <= 0 || run.ID == "" || run.TaskID == "" || run.Status != collection.RunStatusRunning {
		return collection.ErrInvalidTask
	}
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		var task CollectionTaskModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", run.TaskID, run.TenantID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return collection.ErrTaskNotFound
			}
			return repositoryError(err)
		}
		var count int64
		if err := tx.Model(&CollectionRunModel{}).Where("task_id = ? AND tenant_id = ? AND status = ?", run.TaskID, run.TenantID, collection.RunStatusRunning).Count(&count).Error; err != nil {
			return repositoryError(err)
		}
		if count > 0 {
			return collection.ErrActiveRunExists
		}
		taskEntity := task.ToDomain()
		if err := taskEntity.Start(); err != nil {
			return err
		}
		if err := tx.Model(&task).Updates(map[string]any{"status": string(taskEntity.Status), "error_message": "", "updated_at": run.UpdatedAt}).Error; err != nil {
			return repositoryError(err)
		}
		if err := tx.Create(fromCollectionRun(run)).Error; err != nil {
			return repositoryError(err)
		}
		return nil
	})
}

func (r *CollectionRepository) FindActiveRun(ctx context.Context, tenantID int64, taskID string) (*collection.Run, error) {
	var model CollectionRunModel
	err := r.scoped(ctx, tenantID).Where("task_id = ? AND status = ?", taskID, collection.RunStatusRunning).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrRunNotFound
	}
	if err != nil {
		return nil, repositoryError(err)
	}
	return model.ToDomain(), nil
}

func (r *CollectionRepository) FindLatestRun(ctx context.Context, tenantID int64, taskID string) (*collection.Run, error) {
	var model CollectionRunModel
	err := r.scoped(ctx, tenantID).Where("task_id = ?", taskID).Order("created_at DESC").First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrRunNotFound
	}
	if err != nil {
		return nil, repositoryError(err)
	}
	return model.ToDomain(), nil
}

// ListRunningRuns is internal worker recovery support. It deliberately does
// not expose an unscoped read on the collection domain's public repository.
func (r *CollectionRepository) ListRunningRuns(ctx context.Context, limit int) ([]*collection.Run, error) {
	if limit <= 0 {
		limit = 100
	}
	var models []*CollectionRunModel
	if err := r.getDB(ctx).Where("status = ?", collection.RunStatusRunning).Order("started_at ASC").Limit(limit).Find(&models).Error; err != nil {
		return nil, repositoryError(err)
	}
	runs := make([]*collection.Run, 0, len(models))
	for _, model := range models {
		runs = append(runs, model.ToDomain())
	}
	return runs, nil
}

func (r *CollectionRepository) UpdateRun(ctx context.Context, run *collection.Run) error {
	result := r.scoped(ctx, run.TenantID).Model(&CollectionRunModel{}).Where("id = ?", run.ID).Updates(map[string]any{"status": string(run.Status), "error_message": run.ErrorMessage, "completed_at": run.CompletedAt, "updated_at": run.UpdatedAt})
	if result.Error != nil {
		return repositoryError(result.Error)
	}
	if result.RowsAffected == 0 {
		return collection.ErrRunNotFound
	}
	return nil
}

func (r *CollectionRepository) CreateItem(ctx context.Context, item *collection.Item) error {
	if item.TenantID <= 0 || item.ID == "" || item.RunID == "" || item.TaskID == "" || item.SourceID == "" {
		return collection.ErrInvalidTask
	}
	if err := r.scoped(ctx, item.TenantID).Create(fromCollectionItem(item)).Error; err != nil {
		return repositoryError(err)
	}
	return nil
}

func (r *CollectionRepository) UpdateItem(ctx context.Context, item *collection.Item) error {
	result := r.scoped(ctx, item.TenantID).Model(&CollectionItemModel{}).Where("id = ? AND run_id = ?", item.ID, item.RunID).Updates(map[string]any{"canonical_url": item.CanonicalURL, "status": string(item.Status), "content_hash": item.ContentHash, "raw_object_key": item.RawObjectKey, "normalized_object_key": item.NormalizedObjectKey, "raw_hash": item.RawHash, "raw_size": item.RawSize, "normalized_size": item.NormalizedSize, "knowledge_id": item.KnowledgeID, "http_status": item.HTTPStatus, "content_type": item.ContentType, "error_reason": item.ErrorReason, "updated_at": item.UpdatedAt})
	if result.Error != nil {
		return repositoryError(result.Error)
	}
	if result.RowsAffected == 0 {
		return collection.ErrItemNotFound
	}
	return nil
}

func (r *CollectionRepository) ListItems(ctx context.Context, tenantID int64, runID string) ([]*collection.Item, error) {
	var models []*CollectionItemModel
	if err := r.scoped(ctx, tenantID).Where("run_id = ?", runID).Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, repositoryError(err)
	}
	result := make([]*collection.Item, 0, len(models))
	for _, model := range models {
		result = append(result, model.ToDomain())
	}
	return result, nil
}

func (r *CollectionRepository) FinishRun(ctx context.Context, run *collection.Run) error {
	if !run.Status.IsTerminal() {
		return collection.ErrInvalidTransition
	}
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		var task CollectionTaskModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ?", run.TaskID, run.TenantID).First(&task).Error; err != nil {
			return repositoryError(err)
		}
		var stored CollectionRunModel
		if err := tx.Where("id = ? AND task_id = ? AND tenant_id = ?", run.ID, run.TaskID, run.TenantID).First(&stored).Error; err != nil {
			return repositoryError(err)
		}
		if collection.RunStatus(stored.Status).IsTerminal() {
			return nil
		}
		status := run.Status
		if collection.TaskStatus(task.Status) == collection.TaskStatusCancelled {
			status = collection.RunStatusCancelled
		}
		if err := tx.Model(&stored).Updates(map[string]any{"status": string(status), "error_message": run.ErrorMessage, "completed_at": run.CompletedAt, "updated_at": run.UpdatedAt}).Error; err != nil {
			return repositoryError(err)
		}
		if collection.TaskStatus(task.Status) == collection.TaskStatusCancelled {
			return nil
		}
		return repositoryErrorOrNil(tx.Model(&task).Updates(map[string]any{"status": string(status), "error_message": run.ErrorMessage, "updated_at": run.UpdatedAt}).Error)
	})
}
func repositoryErrorOrNil(err error) error {
	if err == nil {
		return nil
	}
	return repositoryError(err)
}

func (r *CollectionRepository) FindAcceptedContent(ctx context.Context, tenantID int64, taskID, contentHash string) (*collection.Item, error) {
	var model CollectionItemModel
	err := r.getDB(ctx).Table("collection_items AS items").Select("items.*").
		Joins("JOIN collection_tasks AS tasks ON tasks.id = items.task_id AND tasks.tenant_id = items.tenant_id").
		Joins("JOIN knowledge AS documents ON documents.id = items.knowledge_id AND documents.tenant_id = items.tenant_id AND documents.knowledge_base_id = tasks.knowledge_base_id").
		Where("items.tenant_id = ? AND items.task_id = ? AND items.content_hash = ? AND items.status = ?", tenantID, taskID, contentHash, collection.ItemStatusAccepted).
		Where("documents.deleted_at IS NULL AND documents.parse_status = ? AND documents.enable_status = ?", "completed", "enabled").
		Order("items.created_at DESC").First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, collection.ErrItemNotFound
	}
	if err != nil {
		return nil, repositoryError(err)
	}
	return model.ToDomain(), nil
}

func (r *CollectionRepository) getDB(ctx context.Context) *gorm.DB { return DBFromContext(ctx, r.db) }
func (r *CollectionRepository) scoped(ctx context.Context, tenantID int64) *gorm.DB {
	return r.getDB(ctx).Where("tenant_id = ?", tenantID)
}
func repositoryError(err error) error { return fmt.Errorf("%w: %v", collection.ErrRepository, err) }
