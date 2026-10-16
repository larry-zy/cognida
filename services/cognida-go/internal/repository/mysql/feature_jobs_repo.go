package mysql

import (
	d "cognida/internal/model/governance"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type featureJobRow struct {
	ID                                    string `gorm:"primaryKey"`
	TenantID, UserID                      int64
	FeatureID                             string
	FeatureVersion                        int
	IdempotencyKey, RequestDigest, Status string
	Version                               int
	Body                                  []byte
	LeaseToken                            string
	LeaseUntil                            *time.Time
	CreatedAt, UpdatedAt                  time.Time
}

func (featureJobRow) TableName() string { return "feature_jobs" }

type featureWatermark struct {
	TenantID, UserID int64
	FeatureID        string
	FeatureVersion   int
	EventTime        *time.Time
	JobID            string
}

func (featureWatermark) TableName() string { return "feature_watermarks" }

type featureJobValueRow struct {
	TenantID, UserID        int64
	JobID                   string
	PartitionIndex          int
	FeatureID               string
	Version                 int
	EntityID                string
	ObservedAt, AvailableAt time.Time
	WithdrawnAt             *time.Time
	ValueExact              []byte
	SourceRefID             string
}

func (featureJobValueRow) TableName() string { return "feature_job_values" }
func decodeFeatureJob(row featureJobRow) (*d.FeatureJob, error) {
	var j d.FeatureJob
	if json.Unmarshal(row.Body, &j) != nil {
		return nil, d.ErrConflict
	}
	j.ID = row.ID
	j.TenantID = row.TenantID
	j.UserID = row.UserID
	j.Status = row.Status
	j.Version = row.Version
	j.LeaseToken = row.LeaseToken
	j.LeaseUntil = row.LeaseUntil
	j.CreatedAt = row.CreatedAt
	j.UpdatedAt = row.UpdatedAt
	return &j, nil
}
func updateFeatureJob(tx *gorm.DB, row *featureJobRow, j *d.FeatureJob) error {
	body, e := json.Marshal(j)
	if e != nil {
		return e
	}
	now, e := featureDatabaseNow(tx)
	if e != nil {
		return e
	}
	res := tx.Model(&featureJobRow{}).Where("id=? AND version=?", row.ID, row.Version).Updates(map[string]any{"body": body, "status": j.Status, "version": row.Version + 1, "lease_token": j.LeaseToken, "lease_until": j.LeaseUntil, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return d.ErrConflict
	}
	j.Version = row.Version + 1
	j.UpdatedAt = now
	row.Version++
	return nil
}
func jobWatermark(tx *gorm.DB, j *d.FeatureJob) (*featureWatermark, error) {
	wm := featureWatermark{TenantID: j.TenantID, UserID: j.UserID, FeatureID: j.Request.FeatureID, FeatureVersion: j.Request.FeatureVersion}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&wm).Error; e != nil {
		return nil, e
	}
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND feature_id=? AND feature_version=?", wm.TenantID, wm.UserID, wm.FeatureID, wm.FeatureVersion).Take(&wm).Error; e != nil {
		return nil, e
	}
	return &wm, nil
}
func (r *governanceRepository) CreateFeatureJob(ctx context.Context, j *d.FeatureJob, key string) (out *d.FeatureJob, err error) {
	if j.TenantID <= 0 || j.UserID <= 0 || key == "" || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	raw, _ := json.Marshal(struct {
		Request                       d.FeatureJobRequest
		Definition, Source, Operation string
		Deadline                      *time.Time
	}{j.Request, j.DefinitionDigest, j.SourceDigest, j.OperationID, j.Deadline})
	sum := hashFeatureRequest(raw)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A durable idempotent receipt remains readable after its execution deadline.
		var existing featureJobRow
		if e := tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", j.TenantID, j.UserID, key).Take(&existing).Error; e == nil {
			if existing.RequestDigest != sum {
				return d.ErrConflict
			}
			out, err = decodeFeatureJob(existing)
			return err
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		deadline, e := featureJobOperationFence(tx, j)
		if e != nil {
			return e
		}
		wm, e := jobWatermark(tx, j)
		if e != nil {
			return e
		}
		j.WatermarkBefore = wm.EventTime
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if deadline != nil && !deadline.After(now) {
			return d.ErrConflict
		}
		j.ID = uuid.NewString()
		j.Status = "queued"
		j.Version = 1
		j.CreatedAt = now
		j.UpdatedAt = now
		body, e := json.Marshal(j)
		if e != nil {
			return e
		}
		row := featureJobRow{ID: j.ID, TenantID: j.TenantID, UserID: j.UserID, FeatureID: j.Request.FeatureID, FeatureVersion: j.Request.FeatureVersion, IdempotencyKey: key, RequestDigest: sum, Status: j.Status, Version: 1, Body: body, CreatedAt: now, UpdatedAt: now}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		var old featureJobRow
		if e = tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", j.TenantID, j.UserID, key).Take(&old).Error; e != nil {
			return e
		}
		if old.RequestDigest != sum {
			return d.ErrConflict
		}
		out, e = decodeFeatureJob(old)
		return e
	})
	return
}
func (r *governanceRepository) GetFeatureJob(ctx context.Context, tenant, user int64, id string) (*d.FeatureJob, error) {
	var row featureJobRow
	e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, tenant, user).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return decodeFeatureJob(row)
}
func (r *governanceRepository) ListFeatureJobs(ctx context.Context, tenant, user int64, cursor string, limit int) ([]d.FeatureJob, string, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", tenant, user)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []featureJobRow
	if e := q.Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	out := []d.FeatureJob{}
	for _, row := range rows {
		j, e := decodeFeatureJob(row)
		if e != nil {
			return nil, "", e
		}
		out = append(out, *j)
	}
	return out, next, nil
}
func (r *governanceRepository) PendingFeatureJobs(ctx context.Context, limit int) ([]d.FeatureJob, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []featureJobRow
	if e := r.db.WithContext(ctx).Where("status IN ?", []string{"queued", "running", "cancel_requested"}).Order("updated_at").Limit(limit).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.FeatureJob{}
	for _, row := range rows {
		j, e := decodeFeatureJob(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *j)
	}
	return out, nil
}
func (r *governanceRepository) ClaimFeatureJob(ctx context.Context, tenant, user int64, id string) (out *d.FeatureJob, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, tenant, user).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if j.Status == "cancel_requested" {
			j.Status = "cancelled"
			j.LeaseToken = ""
			j.LeaseUntil = nil
			return updateFeatureJob(tx, &row, j)
		}
		if j.Status != "queued" && j.Status != "running" {
			return nil
		}
		if j.LeaseUntil != nil && j.LeaseUntil.After(now) {
			return nil
		}
		if j.Status == "running" && j.Embedding {
			all := len(j.Partitions) > 0
			for _, p := range j.Partitions {
				all = all && p.Status == "committed"
			}
			if !all {
				j.Status = "reconcile_required"
				j.Error = "模型调用在进程中断后无法核实；未自动重放付费调用。请核对供应商用量后显式创建新任务。"
				return updateFeatureJob(tx, &row, j)
			}
		}
		j.Status = "running"
		j.LeaseToken = uuid.NewString()
		until := now.Add(2 * time.Minute)
		j.LeaseUntil = &until
		if e = updateFeatureJob(tx, &row, j); e != nil {
			return e
		}
		out = j
		return nil
	})
	return
}
func (r *governanceRepository) CommitFeaturePartition(ctx context.Context, job *d.FeatureJob, part d.FeaturePartition, values []d.FeatureValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deadline, e := featureJobOperationFence(tx, job)
		if e != nil {
			return e
		}
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", job.ID, job.TenantID, job.UserID).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		if j.OperationID != job.OperationID || j.Status != "running" || j.LeaseToken != job.LeaseToken || part.Index < 0 || part.Index >= len(j.Partitions) || len(values) > 1000 || part.Count != len(values) || len(part.Digest) != 64 || part.OutputRefID == "" {
			return d.ErrConflict
		}
		if j.Partitions[part.Index].Status == "committed" {
			if j.Partitions[part.Index].Digest != part.Digest {
				return d.ErrConflict
			}
			*job = *j
			return nil
		}
		if _, e = jobWatermark(tx, j); e != nil {
			return e
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if j.LeaseUntil == nil || !j.LeaseUntil.After(now) ||
			(deadline != nil && !deadline.After(now)) || (j.Deadline != nil && !j.Deadline.After(now)) {
			return d.ErrConflict
		}
		for _, v := range values {
			if v.FeatureID != j.Request.FeatureID || v.Version != j.Request.FeatureVersion || v.SourceRefID != j.Request.SourceRefID || v.EntityID == "" || len(v.EntityID) > 128 || v.Timestamp.IsZero() || !json.Valid(v.Value) {
				return d.ErrInvalid
			}
			// Later full/backfill commits retain earlier observations. Query uses
			// availability time so historical as_of never sees the replacement.
			vr := featureJobValueRow{TenantID: j.TenantID, UserID: j.UserID, JobID: j.ID, PartitionIndex: part.Index, FeatureID: v.FeatureID, Version: v.Version, EntityID: v.EntityID, ObservedAt: v.Timestamp, AvailableAt: now, ValueExact: v.Value, SourceRefID: v.SourceRefID}
			if e = tx.Create(&vr).Error; e != nil {
				return e
			}
		}
		part.Status = "committed"
		part.AvailableAt = &now
		j.Partitions[part.Index] = part
		j.CommittedRows += len(values)
		until := now.Add(2 * time.Minute)
		j.LeaseUntil = &until
		if e = updateFeatureJob(tx, &row, j); e != nil {
			return e
		}
		*job = *j
		return nil
	})
}
func (r *governanceRepository) FinishFeatureJob(ctx context.Context, job *d.FeatureJob, status, message string, watermark *time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", job.ID, job.TenantID, job.UserID).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		if j.LeaseToken != job.LeaseToken || j.Status != "running" && j.Status != "cancel_requested" {
			return d.ErrConflict
		}
		if j.Status == "cancel_requested" {
			status = "cancelled"
		}
		if status == "completed" {
			for _, p := range j.Partitions {
				if p.Status != "committed" {
					return d.ErrConflict
				}
			}
			wm, e := jobWatermark(tx, j)
			if e != nil {
				return e
			}
			if j.Request.Mode != "backfill" && watermark != nil && (wm.EventTime == nil || watermark.After(*wm.EventTime)) {
				if e = tx.Model(&featureWatermark{}).Where("tenant_id=? AND user_id=? AND feature_id=? AND feature_version=?", j.TenantID, j.UserID, j.Request.FeatureID, j.Request.FeatureVersion).Updates(map[string]any{"event_time": watermark, "job_id": j.ID}).Error; e != nil {
					return e
				}
				wm.EventTime = watermark
			}
			j.WatermarkAfter = wm.EventTime
		}
		j.Status = status
		j.Error = message
		j.LeaseToken = ""
		j.LeaseUntil = nil
		if e = updateFeatureJob(tx, &row, j); e != nil {
			return e
		}
		*job = *j
		return nil
	})
}
func (r *governanceRepository) CancelFeatureJob(ctx context.Context, tenant, user int64, id string, version int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, tenant, user).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		if j.Version != version {
			return d.ErrConflict
		}
		switch j.Status {
		case "queued":
			j.Status = "cancelled"
		case "running":
			j.Status = "cancel_requested"
		default:
			return d.ErrConflict
		}
		return updateFeatureJob(tx, &row, j)
	})
}
func (r *governanceRepository) WithdrawFeatureJob(ctx context.Context, tenant, user int64, id string, version int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, tenant, user).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		if j.Version != version {
			return d.ErrConflict
		}
		if j.Status == "running" || j.Status == "queued" || j.Status == "cancel_requested" {
			return d.ErrConflict
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if e = tx.Model(&featureJobValueRow{}).Where("job_id=? AND withdrawn_at IS NULL", id).Update("withdrawn_at", now).Error; e != nil {
			return e
		}
		j.Status = "withdrawn"
		return updateFeatureJob(tx, &row, j)
	})
}
func (r *governanceRepository) FeatureQueryOwned(ctx context.Context, tenant, user int64, entities []string, features []d.FeatureValue, at time.Time) ([]d.FeatureValue, error) {
	out := []d.FeatureValue{}
	for _, f := range features {
		for _, entity := range entities {
			var rows []struct {
				FeatureID               string
				Version                 int
				EntityID                string
				ObservedAt, AvailableAt time.Time
				ValueExact              []byte
				SourceRefID             string
			}
			sql := `SELECT feature_id,version,entity_id,observed_at,available_at,value_exact,source_ref_id FROM (SELECT feature_id,version,entity_id,observed_at,available_at,COALESCE(value_exact,CAST(value AS CHAR)) value_exact,source_ref_id FROM feature_values WHERE tenant_id=? UNION ALL SELECT feature_id,version,entity_id,observed_at,available_at,value_exact,source_ref_id FROM feature_job_values WHERE tenant_id=? AND user_id=? AND (withdrawn_at IS NULL OR withdrawn_at>?)) v WHERE feature_id=? AND version=? AND entity_id=? AND observed_at<=? AND available_at<=? ORDER BY observed_at DESC,available_at DESC LIMIT 1`
			if e := r.db.WithContext(ctx).Raw(sql, tenant, tenant, user, at, f.FeatureID, f.Version, entity, at, at).Scan(&rows).Error; e != nil {
				return nil, e
			}
			for _, row := range rows {
				out = append(out, d.FeatureValue{FeatureID: row.FeatureID, Version: row.Version, EntityID: row.EntityID, Timestamp: row.ObservedAt, AvailableAt: row.AvailableAt, Value: row.ValueExact, ValueExact: string(row.ValueExact), SourceRefID: row.SourceRefID})
			}
		}
	}
	return out, nil
}

var _ d.FeatureJobRepository = (*governanceRepository)(nil)

func hashFeatureRequest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Lock the parent first, then check its deadline with the database clock after
// all partition/watermark locks are held. Context cancellation is not a commit fence.
func featureJobOperationFence(tx *gorm.DB, j *d.FeatureJob) (*time.Time, error) {
	deadline := j.Deadline
	if j.OperationID == "" {
		return deadline, nil
	}
	var op governanceRow
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND kind=?", j.OperationID, j.TenantID, j.UserID, d.GovernedKind).Take(&op).Error; e != nil {
		return nil, e
	}
	var state d.GovernedExecution
	if json.Unmarshal(op.Payload, &state) != nil || state.Invocation.Profile != d.FeatureProfile || state.Invocation.FeatureID != j.Request.FeatureID || state.Invocation.FeatureVersion != j.Request.FeatureVersion || state.Invocation.InputRef.String() != j.Request.SourceRefID {
		return nil, d.ErrConflict
	}
	if op.Status != "running" && op.Status != "queued" {
		return nil, d.ErrConflict
	}
	end := op.CreatedAt.Add(time.Duration(state.Invocation.TimeoutSeconds) * time.Second)
	if state.Invocation.Deadline.Before(end) {
		end = state.Invocation.Deadline
	}
	if deadline == nil || end.Before(*deadline) {
		deadline = &end
	}
	return deadline, nil
}
func (r *governanceRepository) FindFeatureJobByKey(ctx context.Context, t, u int64, key string) (*d.FeatureJob, error) {
	var row featureJobRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND idempotency_key=?", t, u, key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return decodeFeatureJob(row)
}

func (r *governanceRepository) RetryFeatureJob(ctx context.Context, t, u int64, id string, version int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row featureJobRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, t, u).Take(&row).Error; e != nil {
			return e
		}
		j, e := decodeFeatureJob(row)
		if e != nil {
			return e
		}
		if j.Version != version || j.Status != "failed" || j.Embedding || j.OperationID != "" {
			return d.ErrConflict
		}
		j.Status = "queued"
		j.Error = ""
		j.LeaseToken = ""
		j.LeaseUntil = nil
		return updateFeatureJob(tx, &row, j)
	})
}
