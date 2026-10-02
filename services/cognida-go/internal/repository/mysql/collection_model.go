package mysql

import (
	"time"

	"cognida/internal/model/collection"
)

type CollectionTaskModel struct {
	ID                      string     `gorm:"column:id;type:varchar(64);primaryKey"`
	TenantID                int64      `gorm:"column:tenant_id"`
	UserID                  int64      `gorm:"column:user_id"`
	KnowledgeBaseID         string     `gorm:"column:knowledge_base_id;type:varchar(64)"`
	IdempotencyKey          string     `gorm:"column:idempotency_key;type:varchar(128)"`
	Status                  string     `gorm:"column:status;type:varchar(16)"`
	MaxSources              int        `gorm:"column:max_sources"`
	MaxResponseBytes        int64      `gorm:"column:max_response_bytes"`
	ScheduleEnabled         bool       `gorm:"column:schedule_enabled"`
	ScheduleIntervalSeconds int64      `gorm:"column:schedule_interval_seconds"`
	NextRunAt               *time.Time `gorm:"column:next_run_at;type:datetime(3)"`
	LastScheduledAt         *time.Time `gorm:"column:last_scheduled_at;type:datetime(3)"`
	ErrorMessage            string     `gorm:"column:error_message;type:text"`
	CreatedAt               time.Time  `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt               time.Time  `gorm:"column:updated_at;type:datetime(3)"`
}

func (CollectionTaskModel) TableName() string { return "collection_tasks" }
func (m *CollectionTaskModel) ToDomain() *collection.Task {
	return &collection.Task{ID: m.ID, TenantID: m.TenantID, UserID: m.UserID, KnowledgeBaseID: m.KnowledgeBaseID, IdempotencyKey: m.IdempotencyKey, Status: collection.TaskStatus(m.Status), Limits: collection.Limits{MaxSources: m.MaxSources, MaxResponseBytes: m.MaxResponseBytes}, Schedule: collection.Schedule{Enabled: m.ScheduleEnabled, IntervalSeconds: m.ScheduleIntervalSeconds, NextRunAt: m.NextRunAt, LastScheduledAt: m.LastScheduledAt}, ErrorMessage: m.ErrorMessage, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func fromCollectionTask(t *collection.Task) *CollectionTaskModel {
	return &CollectionTaskModel{ID: t.ID, TenantID: t.TenantID, UserID: t.UserID, KnowledgeBaseID: t.KnowledgeBaseID, IdempotencyKey: t.IdempotencyKey, Status: string(t.Status), MaxSources: t.Limits.MaxSources, MaxResponseBytes: t.Limits.MaxResponseBytes, ScheduleEnabled: t.Schedule.Enabled, ScheduleIntervalSeconds: t.Schedule.IntervalSeconds, NextRunAt: t.Schedule.NextRunAt, LastScheduledAt: t.Schedule.LastScheduledAt, ErrorMessage: t.ErrorMessage, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

type CollectionSourceModel struct {
	ID        string    `gorm:"column:id;type:varchar(64);primaryKey"`
	TaskID    string    `gorm:"column:task_id;type:varchar(64)"`
	URL       string    `gorm:"column:url;type:text"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime(3)"`
}

func (CollectionSourceModel) TableName() string { return "collection_sources" }
func (m *CollectionSourceModel) ToDomain() *collection.Source {
	return &collection.Source{ID: m.ID, TaskID: m.TaskID, URL: m.URL, CreatedAt: m.CreatedAt}
}
func fromCollectionSource(s *collection.Source) *CollectionSourceModel {
	return &CollectionSourceModel{ID: s.ID, TaskID: s.TaskID, URL: s.URL, CreatedAt: s.CreatedAt}
}

type CollectionRunModel struct {
	ID           string     `gorm:"column:id;type:varchar(64);primaryKey"`
	TaskID       string     `gorm:"column:task_id;type:varchar(64)"`
	TenantID     int64      `gorm:"column:tenant_id"`
	Status       string     `gorm:"column:status;type:varchar(16)"`
	ErrorMessage string     `gorm:"column:error_message;type:text"`
	StartedAt    time.Time  `gorm:"column:started_at;type:datetime(3)"`
	CompletedAt  *time.Time `gorm:"column:completed_at;type:datetime(3)"`
	CreatedAt    time.Time  `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;type:datetime(3)"`
}

func (CollectionRunModel) TableName() string { return "collection_runs" }
func (m *CollectionRunModel) ToDomain() *collection.Run {
	return &collection.Run{ID: m.ID, TaskID: m.TaskID, TenantID: m.TenantID, Status: collection.RunStatus(m.Status), ErrorMessage: m.ErrorMessage, StartedAt: m.StartedAt, CompletedAt: m.CompletedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func fromCollectionRun(r *collection.Run) *CollectionRunModel {
	return &CollectionRunModel{ID: r.ID, TaskID: r.TaskID, TenantID: r.TenantID, Status: string(r.Status), ErrorMessage: r.ErrorMessage, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

type CollectionItemModel struct {
	ID           string    `gorm:"column:id;type:varchar(64);primaryKey"`
	TenantID     int64     `gorm:"column:tenant_id"`
	TaskID       string    `gorm:"column:task_id;type:varchar(64)"`
	RunID        string    `gorm:"column:run_id;type:varchar(64)"`
	SourceID     string    `gorm:"column:source_id;type:varchar(64)"`
	SourceURL    string    `gorm:"column:source_url;type:text"`
	CanonicalURL string    `gorm:"column:canonical_url;type:text"`
	Status       string    `gorm:"column:status;type:varchar(16)"`
	ContentHash  string    `gorm:"column:content_hash;type:char(64)"`
	KnowledgeID  string    `gorm:"column:knowledge_id;type:varchar(64)"`
	HTTPStatus   int       `gorm:"column:http_status"`
	ContentType  string    `gorm:"column:content_type;type:varchar(255)"`
	ErrorReason  string    `gorm:"column:error_reason;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:datetime(3)"`
}

func (CollectionItemModel) TableName() string { return "collection_items" }
func (m *CollectionItemModel) ToDomain() *collection.Item {
	return &collection.Item{ID: m.ID, TenantID: m.TenantID, TaskID: m.TaskID, RunID: m.RunID, SourceID: m.SourceID, SourceURL: m.SourceURL, CanonicalURL: m.CanonicalURL, Status: collection.ItemStatus(m.Status), ContentHash: m.ContentHash, KnowledgeID: m.KnowledgeID, HTTPStatus: m.HTTPStatus, ContentType: m.ContentType, ErrorReason: m.ErrorReason, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func fromCollectionItem(i *collection.Item) *CollectionItemModel {
	return &CollectionItemModel{ID: i.ID, TenantID: i.TenantID, TaskID: i.TaskID, RunID: i.RunID, SourceID: i.SourceID, SourceURL: i.SourceURL, CanonicalURL: i.CanonicalURL, Status: string(i.Status), ContentHash: i.ContentHash, KnowledgeID: i.KnowledgeID, HTTPStatus: i.HTTPStatus, ContentType: i.ContentType, ErrorReason: i.ErrorReason, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt}
}
