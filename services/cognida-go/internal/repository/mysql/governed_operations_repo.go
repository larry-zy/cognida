package mysql

import (
	audit "cognida/internal/model/agent/operations"
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type operationTargetRow struct {
	ID                   string `gorm:"primaryKey"`
	TenantID, UserID     int64
	Name                 string
	Version              int64
	ActiveVersionID      string
	CreatedAt, UpdatedAt time.Time
}

func (operationTargetRow) TableName() string { return "governed_operation_targets" }

type operationJobRow struct {
	ID                                                string `gorm:"primaryKey"`
	TenantID, UserID                                  int64
	TargetID, OperationKey, PayloadHash, Kind, Status string
	Version, Epoch, LeaseUntilUs, DeadlineUs          int64
	Body                                              []byte
	CreatedAt, UpdatedAt                              time.Time
}

func (operationJobRow) TableName() string { return "governed_operation_jobs" }

type operationVersionRow struct {
	ID                         string `gorm:"primaryKey"`
	TenantID, UserID           int64
	TargetID                   string
	Version                    int64
	OperationID, PhysicalTable string
	RowCount                   int64
	ContentDigest              string
	Body                       []byte
	CreatedAt                  time.Time
}

func (operationVersionRow) TableName() string { return "governed_operation_versions" }

type operationRepository struct{ db *gorm.DB }

func NewGovernedOperationRepository(db *gorm.DB) d.Repository { return &operationRepository{db} }
func operationScope(tx *gorm.DB, p l.Principal) *gorm.DB {
	return tx.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
}
func operationError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return e
}
func operationDecode(row operationJobRow) (*d.Job, error) {
	var out d.Job
	if json.Unmarshal(row.Body, &out) != nil {
		return nil, d.ErrConflict
	}
	out.ID = row.ID
	out.TenantID = row.TenantID
	out.UserID = row.UserID
	out.TargetID = row.TargetID
	out.Status = row.Status
	out.Version = row.Version
	out.Epoch = row.Epoch
	out.LeaseUntil = time.UnixMicro(row.LeaseUntilUs).UTC()
	out.Deadline = time.UnixMicro(row.DeadlineUs).UTC()
	out.CreatedAt = row.CreatedAt
	out.UpdatedAt = row.UpdatedAt
	if out.Plan.Hash != row.PayloadHash {
		return nil, d.ErrConflict
	}
	return &out, nil
}
func operationSave(tx *gorm.DB, row *operationJobRow, j *d.Job) error {
	now, e := featureDatabaseNow(tx)
	if e != nil {
		return e
	}
	j.Version = row.Version + 1
	j.UpdatedAt = now
	raw, e := json.Marshal(j)
	if e != nil {
		return e
	}
	result := tx.Model(&operationJobRow{}).Where("id=? AND version=?", row.ID, row.Version).Updates(map[string]any{"body": raw, "status": j.Status, "version": j.Version, "epoch": j.Epoch, "lease_until_us": j.LeaseUntil.UnixMicro(), "deadline_us": j.Deadline.UnixMicro(), "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	row.Version = j.Version
	return nil
}
func operationAudit(tx *gorm.DB, j *d.Job, action, status string) error {
	return NewOperationAuditRepository(tx).Record(tx.Statement.Context, &audit.OperationAudit{TenantID: j.TenantID, OperationType: action, Target: j.TargetID, SQLText: j.Plan.SQL, Params: fmt.Sprintf(`{"operation_id":%q,"plan_hash":%q,"user_id":%d}`, j.ID, j.Plan.Hash, j.UserID), IdempotencyKey: j.ID, Status: status, Result: j.Status})
}
func (r *operationRepository) Prepare(ctx context.Context, p l.Principal, plan d.Plan, key string) (out *d.Job, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if plan.Request.Validate() != nil || plan.After.Validate() != nil || plan.AfterDigest != d.Digest(plan.After) || len(plan.Hash) != 64 || len(key) != 64 {
		return nil, d.ErrInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		target := operationTargetRow{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Name: plan.Request.Target, CreatedAt: now, UpdatedAt: now}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&target).Error; e != nil {
			return e
		}
		target = operationTargetRow{} // Do not constrain an existing target with the discarded INSERT candidate ID.
		if e = operationScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("name=?", plan.Request.Target).Take(&target).Error; e != nil {
			return operationError(e)
		}
		var prior operationJobRow
		e = operationScope(tx, p).Where("operation_key=?", key).Take(&prior).Error
		if e == nil {
			if prior.PayloadHash != plan.Hash {
				return d.ErrConflict
			}
			out, e = operationDecode(prior)
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if target.Version != plan.Request.ExpectedVersion || target.ActiveVersionID != plan.BeforeVersionID {
			return d.ErrConflict
		}
		j := &d.Job{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, TargetID: target.ID, Status: "planned", Version: 1, Plan: plan, ExpiresAt: now.Add(15 * time.Minute), Deadline: now.Add(15 * time.Minute), CreatedAt: now, UpdatedAt: now}
		raw, _ := json.Marshal(j)
		row := operationJobRow{ID: j.ID, TenantID: p.TenantID, UserID: p.UserID, TargetID: target.ID, OperationKey: key, PayloadHash: plan.Hash, Kind: plan.Request.Action, Status: j.Status, Version: 1, DeadlineUs: j.Deadline.UnixMicro(), Body: raw, CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if e = operationAudit(tx, j, "governed_plan", audit.StatusPendingConfirm); e != nil {
			return e
		}
		out = j
		return nil
	})
	return
}
func (r *operationRepository) Get(ctx context.Context, p l.Principal, id string) (*d.Job, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row operationJobRow
	e := operationScope(r.db.WithContext(ctx), p).Where("id=?", id).Take(&row).Error
	if e != nil {
		return nil, operationError(e)
	}
	return operationDecode(row)
}
func (r *operationRepository) List(ctx context.Context, p l.Principal, cursor string) ([]d.Job, string, error) {
	if p.Validate() != nil {
		return nil, "", d.ErrForbidden
	}
	var rows []operationJobRow
	e := operationScope(r.db.WithContext(ctx), p).Where("id>?", cursor).Order("id").Limit(51).Find(&rows).Error
	out := []d.Job{}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = rows[49].ID
	}
	for _, row := range rows {
		j, x := operationDecode(row)
		if x != nil {
			return nil, "", x
		}
		out = append(out, *j)
	}
	return out, next, e
}
func targetView(v operationTargetRow) *d.Target {
	return &d.Target{ID: v.ID, Name: v.Name, Version: v.Version, ActiveVersionID: v.ActiveVersionID}
}
func (r *operationRepository) Targets(ctx context.Context, p l.Principal) ([]d.Target, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var rows []operationTargetRow
	if e := operationScope(r.db.WithContext(ctx), p).Order("name").Limit(101).Find(&rows).Error; e != nil {
		return nil, e
	}
	if len(rows) > 100 {
		return nil, d.ErrInvalid
	}
	out := []d.Target{}
	for _, row := range rows {
		out = append(out, *targetView(row))
	}
	return out, nil
}
func (r *operationRepository) Target(ctx context.Context, p l.Principal, name string) (*d.Target, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row operationTargetRow
	e := operationScope(r.db.WithContext(ctx), p).Where("name=?", name).Take(&row).Error
	return targetView(row), operationError(e)
}
func operationVersion(tx *gorm.DB, p l.Principal, id string) (*d.Version, error) {
	var row operationVersionRow
	if e := operationScope(tx, p).Where("id=?", id).Take(&row).Error; e != nil {
		return nil, operationError(e)
	}
	var v d.Version
	if json.Unmarshal(row.Body, &v) != nil {
		return nil, d.ErrConflict
	}
	v.PhysicalTable = row.PhysicalTable
	if v.ID != row.ID || v.OperationID != row.OperationID || v.Digest != row.ContentDigest || v.Table.Validate() != nil {
		return nil, d.ErrConflict
	}
	return &v, nil
}
func (r *operationRepository) ReadVersion(ctx context.Context, p l.Principal, name string, version int64) (*d.Version, error) {
	target, e := r.Target(ctx, p, name)
	if e != nil {
		return nil, e
	}
	if version == 0 {
		version = target.Version
	}
	var row operationVersionRow
	if e = operationScope(r.db.WithContext(ctx), p).Where("target_id=? AND version=?", target.ID, version).Take(&row).Error; e != nil {
		return nil, operationError(e)
	}
	v, e := operationVersion(r.db.WithContext(ctx), p, row.ID)
	if e != nil {
		return nil, e
	}
	actual, e := readOperationTable(r.db.WithContext(ctx), v.PhysicalTable, v.Table)
	if e != nil || d.Digest(actual) != v.Digest {
		if e != nil {
			return nil, e
		}
		return nil, d.ErrConflict
	}
	v.Table = actual
	return v, nil
}

// All state transitions and SQL commits lock target -> job. Cancellation and
// terminal receipts therefore serialize with the actual data mutation.
func (r *operationRepository) lock(ctx context.Context, p l.Principal, id string, fn func(*gorm.DB, *operationTargetRow, *operationJobRow, *d.Job) error) error {
	prior, e := r.Get(ctx, p, id)
	if e != nil {
		return e
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target operationTargetRow
		if e := operationScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", prior.TargetID).Take(&target).Error; e != nil {
			return operationError(e)
		}
		var row operationJobRow
		if e := operationScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", id).Take(&row).Error; e != nil {
			return operationError(e)
		}
		j, e := operationDecode(row)
		if e != nil {
			return e
		}
		return fn(tx, &target, &row, j)
	})
}
func (r *operationRepository) Confirm(ctx context.Context, p l.Principal, id string, in d.Confirmation) (out *d.Job, err error) {
	err = r.lock(ctx, p, id, func(tx *gorm.DB, target *operationTargetRow, row *operationJobRow, j *d.Job) error {
		if in.PlanHash != j.Plan.Hash || in.Action != j.Plan.Request.Action {
			return d.ErrConflict
		}
		if j.ConfirmedBy == p.UserID && (j.Status == "queued" || j.Status == "running" || j.Status == "completed") {
			out = j
			return nil
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if j.Status != "planned" || j.Version != in.ExpectedVersion || !j.ExpiresAt.After(now) || target.Version != j.Plan.Request.ExpectedVersion || target.ActiveVersionID != j.Plan.BeforeVersionID {
			return d.ErrConflict
		}
		j.Status = "queued"
		j.ConfirmedBy = p.UserID
		j.ConfirmedAt = &now
		j.Deadline = now.Add(120 * time.Second)
		if e = operationSave(tx, row, j); e != nil {
			return e
		}
		if e = operationAudit(tx, j, "governed_confirm", audit.StatusPendingConfirm); e != nil {
			return e
		}
		out = j
		return nil
	})
	return
}
func (r *operationRepository) Cancel(ctx context.Context, p l.Principal, id string) (out *d.Job, err error) {
	err = r.lock(ctx, p, id, func(tx *gorm.DB, _ *operationTargetRow, row *operationJobRow, j *d.Job) error {
		out = j
		if j.Terminal() {
			return nil
		}
		j.Status = "cancelled"
		j.Epoch++
		j.LeaseUntil = time.Time{}
		if e := operationSave(tx, row, j); e != nil {
			return e
		}
		return operationAudit(tx, j, "governed_cancel", audit.StatusSuccess)
	})
	return
}
func (r *operationRepository) Pending(ctx context.Context, limit int) ([]d.Job, error) {
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	var rows []operationJobRow
	e := r.db.WithContext(ctx).Where("status='queued' OR (status='running' AND lease_until_us<=CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED))").Order("created_at, id").Limit(limit).Find(&rows).Error
	out := []d.Job{}
	for _, row := range rows {
		j, x := operationDecode(row)
		if x != nil {
			return nil, x
		}
		out = append(out, *j)
	}
	return out, e
}
func (r *operationRepository) Claim(ctx context.Context, p l.Principal, id string) (out *d.Job, err error) {
	err = r.lock(ctx, p, id, func(tx *gorm.DB, _ *operationTargetRow, row *operationJobRow, j *d.Job) error {
		if j.Status != "queued" && j.Status != "running" {
			return nil
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if j.Status == "running" && j.LeaseUntil.After(now) {
			return nil
		}
		if !j.Deadline.After(now) {
			j.Status = "timed_out"
			j.Error = "execution deadline exceeded"
			return operationSave(tx, row, j)
		}
		j.Status = "running"
		j.Epoch++
		j.LeaseUntil = now.Add(120 * time.Second)
		if e = operationSave(tx, row, j); e != nil {
			return e
		}
		out = j
		return nil
	})
	return
}
func operationPhysical(id string) string { return "cognida_op_" + d.Digest(id)[:32] }
func ensureOperationTable(tx *gorm.DB, j *d.Job, name string) error {
	columns := []string{"`_row_id` BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY"}
	for _, c := range j.Plan.After.Columns {
		if !d.Identifier(c) {
			return d.ErrInvalid
		}
		columns = append(columns, d.Quote(c)+" LONGTEXT NULL")
	}
	if e := tx.Exec("CREATE TABLE IF NOT EXISTS " + d.Quote(name) + " (" + strings.Join(columns, ",") + ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='governed-operation:" + j.ID + "'").Error; e != nil {
		return e
	}
	var comment string
	if e := tx.Raw("SELECT TABLE_COMMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", name).Scan(&comment).Error; e != nil {
		return e
	}
	if comment != "governed-operation:"+j.ID {
		return d.ErrConflict
	}
	return nil
}
func readOperationTable(tx *gorm.DB, name string, shape d.Table) (d.Table, error) {
	if !strings.HasPrefix(name, "cognida_op_") || len(name) != 43 {
		return d.Table{}, d.ErrConflict
	}
	columns := []string{}
	for _, c := range shape.Columns {
		if !d.Identifier(c) {
			return d.Table{}, d.ErrInvalid
		}
		columns = append(columns, d.Quote(c))
	}
	rows, e := tx.Raw("SELECT " + strings.Join(columns, ",") + " FROM " + d.Quote(name) + " ORDER BY `_row_id` LIMIT 1001").Rows()
	if e != nil {
		return d.Table{}, e
	}
	defer rows.Close()
	out := d.Table{Columns: shape.Columns, Types: shape.Types, Rows: [][]d.Cell{}}
	for rows.Next() {
		cells := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range cells {
			dest[i] = &cells[i]
		}
		if e = rows.Scan(dest...); e != nil {
			return out, e
		}
		row := []d.Cell{}
		for i, c := range cells {
			if !c.Valid {
				row = append(row, d.Cell{Type: "null"})
			} else {
				row = append(row, d.Cell{Type: shape.Types[shape.Columns[i]], Text: c.String})
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if rows.Err() != nil {
		return out, rows.Err()
	}
	return out, out.Validate()
}
func cellArgs(cells []d.Cell) []any {
	args := make([]any, len(cells))
	for i, c := range cells {
		if c.Type != "null" {
			args[i] = c.Text
		}
	}
	return args
}
func insertOperationRows(tx *gorm.DB, name string, t d.Table) error {
	columns := []string{}
	marks := []string{}
	for _, c := range t.Columns {
		columns = append(columns, d.Quote(c))
		marks = append(marks, "?")
	}
	query := "INSERT INTO " + d.Quote(name) + " (" + strings.Join(columns, ",") + ") VALUES (" + strings.Join(marks, ",") + ")"
	for _, row := range t.Rows {
		if e := tx.Exec(query, cellArgs(row)...).Error; e != nil {
			return e
		}
	}
	return nil
}
func (r *operationRepository) Commit(ctx context.Context, p l.Principal, id string, epoch int64) (out *d.Job, err error) {
	j, e := r.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if j.Status == "completed" {
		return j, nil
	}
	if j.Status != "running" || j.Epoch != epoch {
		return nil, d.ErrConflict
	}
	physical := operationPhysical(id)
	if j.Plan.Request.Action != "rollback" {
		if e = ensureOperationTable(r.db.WithContext(ctx), j, physical); e != nil {
			return nil, e
		}
	}
	err = r.lock(ctx, p, id, func(tx *gorm.DB, target *operationTargetRow, row *operationJobRow, j *d.Job) error {
		if j.Status == "completed" {
			out = j
			return nil
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if j.Status != "running" || j.Epoch != epoch || !j.LeaseUntil.After(now) || !j.Deadline.After(now) || j.ConfirmedBy != p.UserID || target.Version != j.Plan.Request.ExpectedVersion || target.ActiveVersionID != j.Plan.BeforeVersionID {
			return d.ErrConflict
		}
		actual := j.Plan.After
		actualAffected := int64(len(actual.Rows))
		if j.Plan.Request.Action == "rollback" {
			prior, e := operationVersion(tx, p, j.Plan.RollbackVersionID)
			if e != nil {
				return e
			}
			if prior.TargetID != target.ID || prior.Version != j.Plan.Request.RollbackVersion || prior.Digest != j.Plan.AfterDigest {
				return d.ErrConflict
			}
			physical = prior.PhysicalTable
			actual, e = readOperationTable(tx, physical, prior.Table)
			if e != nil {
				return e
			}
		} else {
			var count int64
			if e = tx.Raw("SELECT COUNT(*) FROM " + d.Quote(physical)).Scan(&count).Error; e != nil {
				return e
			}
			if count != 0 {
				return d.ErrConflict
			}
			if j.Plan.Request.Action == "etl" {
				if e = insertOperationRows(tx, physical, j.Plan.After); e != nil {
					return e
				}
			} else {
				before, e := operationVersion(tx, p, j.Plan.BeforeVersionID)
				if e != nil {
					return e
				}
				if before.TargetID != target.ID || before.Digest != j.Plan.BeforeDigest {
					return d.ErrConflict
				}
				original, e := readOperationTable(tx, before.PhysicalTable, before.Table)
				if e != nil || d.Digest(original) != before.Digest {
					if e != nil {
						return e
					}
					return d.ErrConflict
				}
				expected, statement, params, affected, e := d.Compile(original, j.Plan.Request)
				if e != nil || statement != j.Plan.SQL || d.Digest(params) != d.Digest(j.Plan.Parameters) || d.Digest(expected) != j.Plan.AfterDigest || affected != j.Plan.EstimatedAffected {
					return d.ErrConflict
				}
				if e = insertOperationRows(tx, physical, original); e != nil {
					return e
				}
				result := tx.Exec(strings.ReplaceAll(statement, "$TARGET", d.Quote(physical)), cellArgs(params)...)
				if result.Error != nil {
					return result.Error
				}
				actualAffected = result.RowsAffected
			}
			actual, e = readOperationTable(tx, physical, j.Plan.After)
			if e != nil {
				return e
			}
		}
		if d.Digest(actual) != j.Plan.AfterDigest {
			return d.ErrConflict
		}
		now, e = featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if !j.LeaseUntil.After(now) || !j.Deadline.After(now) {
			return d.ErrConflict
		}
		v := d.Version{ID: uuid.NewString(), TargetID: target.ID, Version: target.Version + 1, OperationID: j.ID, PhysicalTable: physical, Table: actual, Digest: j.Plan.AfterDigest, SourceRefs: j.Plan.SourceRefs, CreatedAt: now}
		body, _ := json.Marshal(v)
		if e = tx.Create(&operationVersionRow{ID: v.ID, TenantID: p.TenantID, UserID: p.UserID, TargetID: target.ID, Version: v.Version, OperationID: j.ID, PhysicalTable: physical, RowCount: int64(len(actual.Rows)), ContentDigest: v.Digest, Body: body, CreatedAt: now}).Error; e != nil {
			return e
		}
		if e = tx.Model(target).Updates(map[string]any{"version": v.Version, "active_version_id": v.ID, "updated_at": now}).Error; e != nil {
			return e
		}
		j.Status = "completed"
		j.CommittedVersion = v.Version
		j.CommittedVersionID = v.ID
		j.AffectedRows = actualAffected
		j.LeaseUntil = time.Time{}
		if e = operationSave(tx, row, j); e != nil {
			return e
		}
		if e = operationAudit(tx, j, "governed_commit", audit.StatusSuccess); e != nil {
			return e
		}
		out = j
		return nil
	})
	return
}
func (r *operationRepository) Fail(ctx context.Context, p l.Principal, id string, epoch int64, message string) (out *d.Job, err error) {
	err = r.lock(ctx, p, id, func(tx *gorm.DB, _ *operationTargetRow, row *operationJobRow, j *d.Job) error {
		out = j
		if j.Terminal() {
			return nil
		}
		if j.Status != "running" || j.Epoch != epoch {
			return d.ErrConflict
		}
		j.Status = "failed"
		j.Error = message
		j.LeaseUntil = time.Time{}
		if e := operationSave(tx, row, j); e != nil {
			return e
		}
		return operationAudit(tx, j, "governed_failed", audit.StatusFailed)
	})
	return
}
func (r *operationRepository) AttachOutput(ctx context.Context, p l.Principal, id string, ref l.RefID) error {
	return r.lock(ctx, p, id, func(tx *gorm.DB, _ *operationTargetRow, row *operationJobRow, j *d.Job) error {
		if j.Status != "completed" || ref.Validate() != nil {
			return d.ErrConflict
		}
		if j.OutputRef != nil {
			if *j.OutputRef != ref {
				return d.ErrConflict
			}
			return nil
		}
		j.OutputRef = &ref
		return operationSave(tx, row, j)
	})
}

var _ d.Repository = (*operationRepository)(nil)

func (r *operationRepository) FindByKey(ctx context.Context, p l.Principal, key string) (*d.Job, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row operationJobRow
	e := operationScope(r.db.WithContext(ctx), p).Where("operation_key=?", key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return operationDecode(row)
}
