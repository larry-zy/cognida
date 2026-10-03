package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	d "cognida/internal/model/lifecycle"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LifecycleRepository struct{ db *gorm.DB }

func NewLifecycleRepository(db *gorm.DB) *LifecycleRepository { return &LifecycleRepository{db: db} }

var _ d.Repository = (*LifecycleRepository)(nil)

type lifecycleRecord struct {
	ID             string `gorm:"column:id"`
	TenantID       int64  `gorm:"column:tenant_id"`
	RunID          string `gorm:"column:run_id"`
	Status         string `gorm:"column:status"`
	IdempotencyKey string `gorm:"column:idempotency_key"`
	Body           []byte `gorm:"column:body;type:json"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (r *LifecycleRepository) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&LifecycleRepository{db: tx}) })
}
func (r *LifecycleRepository) dbctx(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }
func lifecycleErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return err
}
func encodeBody(v any) ([]byte, error) { return json.Marshal(v) }
func (r *LifecycleRepository) PutAsset(ctx context.Context, ref d.DataAssetRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	body, err := encodeBody(ref)
	if err != nil {
		return err
	}
	return r.dbctx(ctx).Table("lifecycle_assets").Create(map[string]any{"asset_id": ref.AssetID, "version": ref.Version, "tenant_id": ref.TenantID, "object_key": ref.ObjectKey, "body": string(body), "created_at": ref.CreatedAt}).Error
}
func (r *LifecycleRepository) FindAsset(ctx context.Context, tenant int64, id d.RefID) (d.DataAssetRef, error) {
	var row struct {
		Body      []byte
		ObjectKey string
	}
	err := r.dbctx(ctx).Table("lifecycle_assets").Where("tenant_id = ? AND asset_id = ? AND version = ?", tenant, id.AssetID, id.Version).Take(&row).Error
	if err != nil {
		return d.DataAssetRef{}, lifecycleErr(err)
	}
	var ref d.DataAssetRef
	if err = json.Unmarshal(row.Body, &ref); err != nil {
		return ref, err
	}
	ref.ObjectKey = row.ObjectKey
	return ref, nil
}
func (r *LifecycleRepository) SaveDefinition(ctx context.Context, definition d.PipelineDefinition) error {
	body, err := encodeBody(definition)
	if err != nil {
		return err
	}
	if e := r.dbctx(ctx).Exec("INSERT IGNORE INTO lifecycle_definitions (id,version,body,created_at) VALUES (?,?,?,?)", definition.ID, definition.Version, string(body), time.Now().UTC()).Error; e != nil {
		return e
	}
	var row struct{ Body []byte }
	if e := r.dbctx(ctx).Table("lifecycle_definitions").Where("id=? AND version=?", definition.ID, definition.Version).Take(&row).Error; e != nil {
		return e
	}
	var stored d.PipelineDefinition
	if e := json.Unmarshal(row.Body, &stored); e != nil {
		return e
	}
	encoded, _ := json.Marshal(stored)
	if string(encoded) != string(body) {
		return d.ErrConflict
	}
	return nil
}
func (r *LifecycleRepository) FindDefinition(ctx context.Context, id string, version int) (d.PipelineDefinition, error) {
	var row struct{ Body []byte }
	if err := r.dbctx(ctx).Table("lifecycle_definitions").Where("id=? AND version=?", id, version).Take(&row).Error; err != nil {
		return d.PipelineDefinition{}, lifecycleErr(err)
	}
	var definition d.PipelineDefinition
	if err := json.Unmarshal(row.Body, &definition); err != nil {
		return definition, fmt.Errorf("%w: invalid persisted pipeline definition", d.ErrInvalid)
	}
	if definition.ID != id || definition.Version != version {
		return definition, d.ErrConflict
	}
	return definition, nil
}
func runRow(run *d.PipelineRun) (lifecycleRecord, error) {
	body, err := encodeBody(run)
	return lifecycleRecord{ID: run.ID, TenantID: run.TenantID, Status: run.Status, IdempotencyKey: run.IdempotencyKey, Body: body, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt}, err
}
func stageRow(stage *d.StageRun) (lifecycleRecord, error) {
	body, err := encodeBody(stage)
	return lifecycleRecord{ID: stage.ID, TenantID: stage.TenantID, RunID: stage.RunID, Status: stage.Status, IdempotencyKey: stage.IdempotencyKey, Body: body, CreatedAt: stage.CreatedAt, UpdatedAt: stage.UpdatedAt}, err
}
func (r *LifecycleRepository) CreateRun(ctx context.Context, run *d.PipelineRun, stages []d.StageRun) error {
	return r.Transaction(ctx, func(repo d.Repository) error {
		tx := repo.(*LifecycleRepository)
		row, err := runRow(run)
		if err != nil {
			return err
		}
		if err = tx.dbctx(ctx).Table("lifecycle_runs").Omit("run_id").Create(&row).Error; err != nil {
			return err
		}
		for i := range stages {
			if err = tx.CreateStage(ctx, &stages[i]); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *LifecycleRepository) find(ctx context.Context, table string, tenant int64, field, value string, lock bool) (lifecycleRecord, error) {
	var row lifecycleRecord
	q := r.dbctx(ctx).Table(table).Where("tenant_id = ?", tenant).Where(field+" = ?", value)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Take(&row).Error
	return row, lifecycleErr(err)
}
func decodeRun(row lifecycleRecord, err error) (*d.PipelineRun, error) {
	if err != nil {
		return nil, err
	}
	var run d.PipelineRun
	if err = json.Unmarshal(row.Body, &run); err != nil {
		return nil, err
	}
	run.IdempotencyKey = row.IdempotencyKey
	return &run, nil
}
func decodeStage(row lifecycleRecord, err error) (*d.StageRun, error) {
	if err != nil {
		return nil, err
	}
	var stage d.StageRun
	if err = json.Unmarshal(row.Body, &stage); err != nil {
		return nil, err
	}
	stage.IdempotencyKey = row.IdempotencyKey
	return &stage, nil
}
func (r *LifecycleRepository) FindRun(ctx context.Context, tenant int64, id string, lock bool) (*d.PipelineRun, error) {
	return decodeRun(r.find(ctx, "lifecycle_runs", tenant, "id", id, lock))
}
func (r *LifecycleRepository) FindRunByKey(ctx context.Context, tenant int64, key string) (*d.PipelineRun, error) {
	return decodeRun(r.find(ctx, "lifecycle_runs", tenant, "idempotency_key", key, false))
}
func (r *LifecycleRepository) FindStage(ctx context.Context, tenant int64, id string, lock bool) (*d.StageRun, error) {
	return decodeStage(r.find(ctx, "lifecycle_stages", tenant, "id", id, lock))
}
func (r *LifecycleRepository) FindStageByKey(ctx context.Context, tenant int64, key string) (*d.StageRun, error) {
	return decodeStage(r.find(ctx, "lifecycle_stages", tenant, "idempotency_key", key, false))
}
func limitN(n int) int {
	if n < 1 || n > 100 {
		return 100
	}
	return n
}
func (r *LifecycleRepository) ListRuns(ctx context.Context, tenant int64, n int) ([]d.PipelineRun, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_runs").Where("tenant_id = ?", tenant).Order("created_at DESC").Limit(limitN(n)).Find(&rows).Error
	out := make([]d.PipelineRun, 0, len(rows))
	for _, row := range rows {
		run, e := decodeRun(row, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, *run)
	}
	return out, err
}
func (r *LifecycleRepository) UpdateRun(ctx context.Context, run *d.PipelineRun) error {
	row, err := runRow(run)
	if err != nil {
		return err
	}
	return r.update(ctx, "lifecycle_runs", row)
}
func (r *LifecycleRepository) update(ctx context.Context, table string, row lifecycleRecord) error {
	result := r.dbctx(ctx).Table(table).Where("tenant_id = ? AND id = ?", row.TenantID, row.ID).Updates(map[string]any{"status": row.Status, "body": string(row.Body), "updated_at": row.UpdatedAt})
	return result.Error
}
func (r *LifecycleRepository) CreateStage(ctx context.Context, stage *d.StageRun) error {
	row, err := stageRow(stage)
	if err != nil {
		return err
	}
	return r.dbctx(ctx).Table("lifecycle_stages").Create(&row).Error
}
func (r *LifecycleRepository) UpdateStage(ctx context.Context, stage *d.StageRun) error {
	row, err := stageRow(stage)
	if err != nil {
		return err
	}
	return r.update(ctx, "lifecycle_stages", row)
}
func stagesFromRows(rows []lifecycleRecord, err error) ([]d.StageRun, error) {
	if err != nil {
		return nil, err
	}
	out := make([]d.StageRun, 0, len(rows))
	for _, row := range rows {
		s, e := decodeStage(row, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, *s)
	}
	return out, nil
}
func (r *LifecycleRepository) ListStages(ctx context.Context, tenant int64, runID string) ([]d.StageRun, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_stages").Where("tenant_id = ? AND run_id = ?", tenant, runID).Order("created_at ASC,id ASC").Find(&rows).Error
	return stagesFromRows(rows, err)
}

// Run controllers lock all stages in a stable order before inspecting them.
// This is a current read in InnoDB, so a concurrent stage update cannot be
// overwritten using an earlier repeatable-read snapshot.
func (r *LifecycleRepository) LockStages(ctx context.Context, tenant int64, runID string) ([]d.StageRun, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_stages").Where("tenant_id=? AND run_id=?", tenant, runID).Order("id ASC").Clauses(clause.Locking{Strength: "UPDATE"}).Find(&rows).Error
	return stagesFromRows(rows, err)
}
func (r *LifecycleRepository) ListInvocations(ctx context.Context, tenant int64, n int) ([]d.StageRun, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_stages").Where("tenant_id = ? AND run_id = ''", tenant).Order("created_at DESC").Limit(limitN(n)).Find(&rows).Error
	return stagesFromRows(rows, err)
}
func (r *LifecycleRepository) PendingStages(ctx context.Context, n int) ([]d.StageRun, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_stages").Where("status IN ?", []string{d.StatusQueued, d.StatusRunning}).Order("updated_at ASC").Limit(n).Find(&rows).Error
	return stagesFromRows(rows, err)
}
func (r *LifecycleRepository) CreateApproval(ctx context.Context, a *d.Approval) error {
	body, err := encodeBody(a)
	if err != nil {
		return err
	}
	return r.dbctx(ctx).Table("lifecycle_approvals").Create(map[string]any{"id": a.ID, "tenant_id": a.TenantID, "run_id": a.RunID, "stage_id": a.StageID, "status": a.Status, "body": string(body), "expires_at": a.ExpiresAt, "created_at": a.CreatedAt}).Error
}
func (r *LifecycleRepository) FindApproval(ctx context.Context, tenant int64, id string, lock bool) (*d.Approval, error) {
	row, err := r.find(ctx, "lifecycle_approvals", tenant, "id", id, lock)
	if err != nil {
		return nil, err
	}
	var a d.Approval
	err = json.Unmarshal(row.Body, &a)
	return &a, err
}
func (r *LifecycleRepository) ListApprovals(ctx context.Context, tenant int64, run string) ([]d.Approval, error) {
	var rows []lifecycleRecord
	err := r.dbctx(ctx).Table("lifecycle_approvals").Where("tenant_id = ? AND run_id = ?", tenant, run).Find(&rows).Error
	out := make([]d.Approval, 0, len(rows))
	for _, row := range rows {
		var a d.Approval
		if e := json.Unmarshal(row.Body, &a); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, err
}
func (r *LifecycleRepository) UpdateApproval(ctx context.Context, a *d.Approval) error {
	body, err := encodeBody(a)
	if err != nil {
		return err
	}
	return r.dbctx(ctx).Table("lifecycle_approvals").Where("id = ? AND tenant_id = ?", a.ID, a.TenantID).Updates(map[string]any{"status": a.Status, "body": string(body)}).Error
}
func (r *LifecycleRepository) AppendEvent(ctx context.Context, event *d.Event) error {
	return r.dbctx(ctx).Table("lifecycle_events").Create(event).Error
}
func (r *LifecycleRepository) Events(ctx context.Context, tenant int64, run string, after int64) ([]d.Event, error) {
	var events []d.Event
	err := r.dbctx(ctx).Table("lifecycle_events").Where("tenant_id = ? AND run_id = ? AND id > ?", tenant, run, after).Order("id ASC").Limit(200).Find(&events).Error
	return events, err
}

// UUID cursors are stable even when a running record changes status. Metadata
// lists never resolve or duplicate raw content.
func (r *LifecycleRepository) ListAssets(ctx context.Context, tenant int64, kind d.Kind, cursor string, n int) ([]d.DataAssetRef, error) {
	var rows []struct {
		Body      []byte
		ObjectKey string
	}
	q := r.dbctx(ctx).Table("lifecycle_assets").Where("tenant_id = ?", tenant)
	if cursor != "" {
		ref, err := d.ParseRefID(cursor)
		if err != nil {
			return nil, err
		}
		q = q.Where("asset_id > ? OR (asset_id = ? AND version > ?)", ref.AssetID, ref.AssetID, ref.Version)
	}
	if kind != "" {
		q = q.Where("JSON_UNQUOTE(JSON_EXTRACT(body, '$.kind')) = ?", kind)
	}
	e := q.Order("asset_id ASC,version ASC").Limit(n).Find(&rows).Error
	out := make([]d.DataAssetRef, 0, len(rows))
	for _, row := range rows {
		var ref d.DataAssetRef
		if e := json.Unmarshal(row.Body, &ref); e != nil {
			return nil, e
		}
		ref.ObjectKey = row.ObjectKey
		out = append(out, ref)
	}
	return out, e
}
func (r *LifecycleRepository) ListRunsPage(ctx context.Context, tenant int64, cursor string, n int) ([]d.PipelineRun, error) {
	var rows []lifecycleRecord
	e := r.dbctx(ctx).Table("lifecycle_runs").Where("tenant_id = ? AND id > ?", tenant, cursor).Order("id ASC").Limit(n).Find(&rows).Error
	out := make([]d.PipelineRun, 0, len(rows))
	for _, row := range rows {
		v, e := decodeRun(row, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, e
}
func (r *LifecycleRepository) ListInvocationsPage(ctx context.Context, tenant int64, cursor string, n int) ([]d.StageRun, error) {
	var rows []lifecycleRecord
	e := r.dbctx(ctx).Table("lifecycle_stages").Where("tenant_id = ? AND run_id = '' AND id > ?", tenant, cursor).Order("id ASC").Limit(n).Find(&rows).Error
	return stagesFromRows(rows, e)
}
func (r *LifecycleRepository) RecoverableRuns(ctx context.Context, n int) ([]d.PipelineRun, error) {
	var rows []lifecycleRecord
	e := r.dbctx(ctx).Table("lifecycle_runs").Where("status IN ?", []string{d.StatusRunning, d.StatusApproval}).Order("updated_at ASC").Limit(n).Find(&rows).Error
	out := make([]d.PipelineRun, 0, len(rows))
	for _, row := range rows {
		v, e := decodeRun(row, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, e
}
func (r *LifecycleRepository) ExpiredApprovals(ctx context.Context, now time.Time, n int) ([]d.Approval, error) {
	var rows []lifecycleRecord
	e := r.dbctx(ctx).Table("lifecycle_approvals").Where("status = 'pending' AND expires_at <= ?", now).Order("expires_at ASC").Limit(n).Find(&rows).Error
	out := make([]d.Approval, 0, len(rows))
	for _, row := range rows {
		var a d.Approval
		if e := json.Unmarshal(row.Body, &a); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, e
}
