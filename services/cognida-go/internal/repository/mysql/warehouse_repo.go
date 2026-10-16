package mysql

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/warehouse"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type warehouseModelRow struct {
	ID                     string `gorm:"primaryKey"`
	TenantID, UserID       int64
	Version                int
	ActiveRunID, ActiveRef string
	Watermark              *time.Time
	IdempotencyKey         string
	CreatedAt              time.Time
}

func (warehouseModelRow) TableName() string { return "warehouse_models" }

type warehouseVersionRow struct {
	ModelID                string `gorm:"primaryKey"`
	Version                int    `gorm:"primaryKey"`
	Definition             string
	Digest, IdempotencyKey string
	CreatedAt              time.Time
}

func (warehouseVersionRow) TableName() string { return "warehouse_model_versions" }

type warehouseRunRow struct {
	ID                                          string `gorm:"primaryKey"`
	TenantID, UserID                            int64
	ModelID                                     string
	ModelVersion, Version                       int
	Status, IdempotencyKey, Digest, Body, Lease string
	LeaseUntil                                  *time.Time
	CreatedAt, UpdatedAt                        time.Time
}

func (warehouseRunRow) TableName() string { return "warehouse_runs" }

type warehouseEventRow struct {
	ID, RunID                 string
	TenantID, UserID, ActorID int64
	Action                    string
	CreatedAt                 time.Time
}

func (warehouseEventRow) TableName() string { return "warehouse_events" }

type warehouseRepository struct{ db *gorm.DB }

func NewWarehouseRepository(db *gorm.DB) d.Repository { return &warehouseRepository{db} }
func warehouseError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return e
}
func modelFrom(h warehouseModelRow, v warehouseVersionRow) (*d.Model, error) {
	out := &d.Model{ID: h.ID, TenantID: h.TenantID, UserID: h.UserID, Version: v.Version, Digest: v.Digest, ActiveRunID: h.ActiveRunID, ActiveRef: h.ActiveRef, Watermark: h.Watermark, CreatedAt: v.CreatedAt}
	e := json.Unmarshal([]byte(v.Definition), &out.Definition)
	return out, e
}
func (r *warehouseRepository) SaveModel(ctx context.Context, m *d.Model, expected int, key string) (out *d.Model, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		var head warehouseModelRow
		if m.ID == "" {
			head = warehouseModelRow{ID: uuid.NewString(), TenantID: m.TenantID, UserID: m.UserID, Version: 1, IdempotencyKey: key, CreatedAt: now}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&head).Error; e != nil {
				return e
			}
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND idempotency_key=?", m.TenantID, m.UserID, key).Take(&head).Error; e != nil {
				return e
			}
		} else {
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", m.ID, m.TenantID, m.UserID).Take(&head).Error; e != nil {
				return warehouseError(e)
			}
		}
		var old warehouseVersionRow
		if e = tx.Where("model_id=? AND idempotency_key=?", head.ID, key).Take(&old).Error; e == nil {
			if old.Digest != m.Digest {
				return d.ErrConflict
			}
			out, e = modelFrom(head, old)
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		version := 1
		if m.ID != "" {
			if head.Version != expected {
				return d.ErrConflict
			}
			version = expected + 1
			if e = tx.Model(&head).Update("version", version).Error; e != nil {
				return e
			}
		}
		raw, e := json.Marshal(m.Definition)
		if e != nil {
			return e
		}
		v := warehouseVersionRow{ModelID: head.ID, Version: version, Definition: string(raw), Digest: m.Digest, IdempotencyKey: key, CreatedAt: now}
		if e = tx.Create(&v).Error; e != nil {
			return e
		}
		out, e = modelFrom(head, v)
		return e
	})
	return
}
func (r *warehouseRepository) GetModel(ctx context.Context, p l.Principal, id string, version int) (*d.Model, error) {
	var head warehouseModelRow
	if e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&head).Error; e != nil {
		return nil, warehouseError(e)
	}
	if version == 0 {
		version = head.Version
	}
	var v warehouseVersionRow
	if e := r.db.WithContext(ctx).Where("model_id=? AND version=?", id, version).Take(&v).Error; e != nil {
		return nil, warehouseError(e)
	}
	return modelFrom(head, v)
}
func (r *warehouseRepository) ListModels(ctx context.Context, p l.Principal, cursor string) ([]d.Model, string, error) {
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var heads []warehouseModelRow
	if e := q.Order("id").Limit(51).Find(&heads).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(heads) > 50 {
		heads = heads[:50]
		next = heads[49].ID
	}
	out := []d.Model{}
	for _, h := range heads {
		m, e := r.GetModel(ctx, p, h.ID, h.Version)
		if e != nil {
			return nil, "", e
		}
		out = append(out, *m)
	}
	return out, next, nil
}
func decodeWarehouse(row warehouseRunRow) (*d.Run, error) {
	var out d.Run
	if e := json.Unmarshal([]byte(row.Body), &out); e != nil {
		return nil, e
	}
	out.ID = row.ID
	out.TenantID = row.TenantID
	out.UserID = row.UserID
	out.Status = row.Status
	out.Version = row.Version
	out.Lease = row.Lease
	out.LeaseUntil = row.LeaseUntil
	out.CreatedAt = row.CreatedAt
	out.UpdatedAt = row.UpdatedAt
	return &out, nil
}
func saveWarehouse(tx *gorm.DB, row *warehouseRunRow, run *d.Run) error {
	now, e := featureDatabaseNow(tx)
	if e != nil {
		return e
	}
	body, e := json.Marshal(run)
	if e != nil {
		return e
	}
	res := tx.Model(&warehouseRunRow{}).Where("id=? AND version=?", row.ID, row.Version).Updates(map[string]any{"body": string(body), "status": run.Status, "lease": run.Lease, "lease_until": run.LeaseUntil, "version": row.Version + 1, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return d.ErrConflict
	}
	run.Version = row.Version + 1
	run.UpdatedAt = now
	return nil
}
func (r *warehouseRepository) CreateRun(ctx context.Context, run *d.Run, key string) (out *d.Run, err error) {
	hash := hashFeatureRequest(mustWarehouseJSON([]any{run.Request, run.DefinitionDigest, run.SourceDigests, run.BaseRunID, run.Deadline}))
	if run.CommandDigest != "" {
		hash = run.CommandDigest
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head warehouseModelRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", run.Request.ModelID, run.TenantID, run.UserID).Take(&head).Error; e != nil {
			return warehouseError(e)
		}
		var existing warehouseRunRow
		if e := tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", run.TenantID, run.UserID, key).Take(&existing).Error; e == nil {
			if existing.Digest != hash {
				return d.ErrConflict
			}
			out, e = decodeWarehouse(existing)
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if run.Status != "cancelled" && (head.Version != run.Request.ModelVersion || head.ActiveRunID != run.BaseRunID) {
			return d.ErrConflict
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if run.Status != "cancelled" && run.Deadline != nil && !run.Deadline.After(now) {
			return d.ErrConflict
		}
		run.ID = uuid.NewString()
		run.Version = 1
		if run.Status != "cancelled" {
			run.Status = "queued"
		}
		if run.Deadline == nil {
			end := now.Add(120 * time.Second)
			run.Deadline = &end
		}
		run.CreatedAt = now
		run.UpdatedAt = now
		row := warehouseRunRow{ID: run.ID, TenantID: run.TenantID, UserID: run.UserID, ModelID: run.Request.ModelID, ModelVersion: run.Request.ModelVersion, Version: 1, Status: run.Status, IdempotencyKey: key, Digest: hash, Body: string(mustWarehouseJSON(run)), CreatedAt: now, UpdatedAt: now}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		out = run
		return nil
	})
	return
}
func mustWarehouseJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
func (r *warehouseRepository) GetRun(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	var row warehouseRunRow
	if e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
		return nil, warehouseError(e)
	}
	return decodeWarehouse(row)
}
func (r *warehouseRepository) ListRuns(ctx context.Context, p l.Principal, model, cursor string) ([]d.Run, string, error) {
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
	if model != "" {
		q = q.Where("model_id=?", model)
	}
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []warehouseRunRow
	if e := q.Order("id").Limit(51).Find(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = rows[49].ID
	}
	out := []d.Run{}
	for _, v := range rows {
		j, e := decodeWarehouse(v)
		if e != nil {
			return nil, "", e
		}
		out = append(out, *j)
	}
	return out, next, nil
}
func (r *warehouseRepository) Pending(ctx context.Context) ([]d.Run, error) {
	var rows []warehouseRunRow
	if e := r.db.WithContext(ctx).Where("status IN ?", []string{"queued", "running", "cancel_requested"}).Order("updated_at").Limit(20).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.Run{}
	for _, v := range rows {
		j, e := decodeWarehouse(v)
		if e != nil {
			return nil, e
		}
		out = append(out, *j)
	}
	return out, nil
}
func (r *warehouseRepository) Claim(ctx context.Context, p l.Principal, id string) (out *d.Run, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row warehouseRunRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
			return warehouseError(e)
		}
		run, e := decodeWarehouse(row)
		if e != nil {
			return e
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if run.Status == "cancel_requested" {
			run.Status = "cancelled"
			run.Lease = ""
			run.LeaseUntil = nil
			return saveWarehouse(tx, &row, run)
		}
		if run.Status != "queued" && run.Status != "running" {
			return nil
		}
		if run.LeaseUntil != nil && run.LeaseUntil.After(now) {
			return nil
		}
		if run.Deadline != nil && !run.Deadline.After(now) {
			run.Status = "failed"
			run.Error = "执行预算截止，保留已提交候选分区，活动版本未改变"
			return saveWarehouse(tx, &row, run)
		}
		run.Status = "running"
		run.Lease = uuid.NewString()
		until := now.Add(2 * time.Minute)
		run.LeaseUntil = &until
		if e = saveWarehouse(tx, &row, run); e != nil {
			return e
		}
		out = run
		return nil
	})
	return
}
func (r *warehouseRepository) Update(ctx context.Context, run *d.Run, mutate func(*d.Run) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row warehouseRunRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", run.ID, run.TenantID, run.UserID).Take(&row).Error; e != nil {
			return warehouseError(e)
		}
		stored, e := decodeWarehouse(row)
		if e != nil {
			return e
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if stored.Status != "running" || stored.Lease == "" || stored.Lease != run.Lease || stored.LeaseUntil == nil || !stored.LeaseUntil.After(now) {
			return d.ErrConflict
		}
		previousPartitions := len(stored.Partitions)
		if e = mutate(stored); e != nil {
			return e
		}
		if stored.Status != "failed" && stored.Deadline != nil && !stored.Deadline.After(now) {
			return d.ErrConflict
		}
		for i := previousPartitions; i < len(stored.Partitions); i++ {
			stored.Partitions[i].CommittedAt = now
		}
		until := now.Add(2 * time.Minute)
		stored.LeaseUntil = &until
		if stored.Status != "running" {
			stored.Lease = ""
			stored.LeaseUntil = nil
		}
		if e = saveWarehouse(tx, &row, stored); e != nil {
			return e
		}
		*run = *stored
		return nil
	})
}
func (r *warehouseRepository) Cancel(ctx context.Context, p l.Principal, id string, version int) (out *d.Run, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row warehouseRunRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
			return warehouseError(e)
		}
		v, e := decodeWarehouse(row)
		if e != nil {
			return e
		}
		if v.Version != version {
			return d.ErrConflict
		}
		switch v.Status {
		case "queued", "awaiting_approval":
			v.Status = "cancelled"
		case "running":
			v.Status = "cancel_requested"
		default:
			out = v
			return nil
		}
		if e = saveWarehouse(tx, &row, v); e != nil {
			return e
		}
		out = v
		return nil
	})
	return
}
func (r *warehouseRepository) Approve(ctx context.Context, p l.Principal, id string, version int, ref string) (out *d.Run, err error) {
	// Lock order is model -> run. Candidate/definition/base changes invalidate approval.
	candidate, e := r.GetRun(ctx, p, id)
	if e != nil {
		return nil, e
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head warehouseModelRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", candidate.Request.ModelID, p.TenantID, p.UserID).Take(&head).Error; e != nil {
			return warehouseError(e)
		}
		var row warehouseRunRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
			return e
		}
		v, e := decodeWarehouse(row)
		if e != nil {
			return e
		}
		if v.Status == "approved" && v.PublishedRef == ref {
			out = v
			return nil
		}
		if v.Version != version || v.Status != "awaiting_approval" || head.Version != v.Request.ModelVersion || head.ActiveRunID != v.BaseRunID || v.OutputRef == "" || v.FactRef == "" || !v.LineageRegistered {
			return d.ErrConflict
		}
		v.Status = "approved"
		v.PublishedRef = ref
		if e = saveWarehouse(tx, &row, v); e != nil {
			return e
		}
		if e = tx.Model(&head).Updates(map[string]any{"active_run_id": id, "active_ref": ref, "watermark": v.Watermark}).Error; e != nil {
			return e
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if e = tx.Create(&warehouseEventRow{ID: uuid.NewString(), RunID: id, TenantID: p.TenantID, UserID: p.UserID, ActorID: p.UserID, Action: "approve_switch", CreatedAt: now}).Error; e != nil {
			return e
		}
		out = v
		return nil
	})
	return
}
func (r *warehouseRepository) Events(ctx context.Context, p l.Principal, id string) ([]d.Event, error) {
	if _, e := r.GetRun(ctx, p, id); e != nil {
		return nil, e
	}
	var rows []warehouseEventRow
	if e := r.db.WithContext(ctx).Where("run_id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Order("created_at").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.Event{}
	for _, v := range rows {
		out = append(out, d.Event{ID: v.ID, RunID: v.RunID, Action: v.Action, ActorID: v.ActorID, CreatedAt: v.CreatedAt})
	}
	return out, nil
}
func (r *warehouseRepository) Impact(ctx context.Context, p l.Principal, refs []string) ([]d.Impact, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	// Bounded inventories cover registered references; unknown external consumers
	// and inventories that exceed the bound remain explicit, never "no impact".
	out := []d.Impact{{Kind: "external_consumers", Unknown: true, Name: "未登记的外部依赖未知"}}
	type item struct{ ID, Kind, Name, Body, Ref string }
	inventory := []item{}
	bounded := func(kind string, n int) int {
		if n > 1000 {
			out = append(out, d.Impact{Kind: kind, Unknown: true, Name: "该类依赖超过 1000 条，影响清单不完整"})
			return 1000
		}
		return n
	}
	var governance []governanceRow
	if e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID).Order("id").Limit(1001).Find(&governance).Error; e != nil {
		return nil, e
	}
	for _, v := range governance[:bounded("governance_inventory", len(governance))] {
		inventory = append(inventory, item{ID: v.ID, Kind: v.Kind, Body: string(v.Payload)})
	}
	var models []struct{ ID, Definition, ActiveRef string }
	if e := r.db.WithContext(ctx).Table("warehouse_models h").Select("h.id,v.definition,h.active_ref").Joins("JOIN warehouse_model_versions v ON v.model_id=h.id AND v.version=h.version").Where("h.tenant_id=? AND h.user_id=?", p.TenantID, p.UserID).Order("h.id").Limit(1001).Scan(&models).Error; e != nil {
		return nil, e
	}
	for _, v := range models[:bounded("model_inventory", len(models))] {
		inventory = append(inventory, item{ID: v.ID, Kind: "warehouse_model", Body: v.Definition, Ref: v.ActiveRef})
	}
	var datasets []datasetVersionRow
	if e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID).Order("id").Limit(1001).Find(&datasets).Error; e != nil {
		return nil, e
	}
	for _, v := range datasets[:bounded("dataset_inventory", len(datasets))] {
		inventory = append(inventory, item{ID: v.ID, Kind: "dataset_version", Body: v.Body})
	}
	var assets []struct {
		AssetID string
		Version int
		Body    string
	}
	if e := r.db.WithContext(ctx).Table("lifecycle_assets").Select("asset_id,version,body").Where("tenant_id=?", p.TenantID).Order("asset_id,version").Limit(1001).Scan(&assets).Error; e != nil {
		return nil, e
	}
	for _, v := range assets[:bounded("asset_inventory", len(assets))] {
		var ref l.DataAssetRef
		if e := json.Unmarshal([]byte(v.Body), &ref); e != nil {
			out = append(out, d.Impact{Kind: "unreadable_manifest", Unknown: true, Name: "存在无法解析的资产依赖"})
			continue
		}
		if ref.Provenance.SourceOwnerID != 0 && ref.Provenance.SourceOwnerID != p.UserID {
			continue
		}
		parents, _ := json.Marshal(ref.Parents)
		inventory = append(inventory, item{ID: fmt.Sprintf("%s:%d", v.AssetID, v.Version), Kind: "asset:" + string(ref.Kind), Body: string(parents), Ref: ref.RefID.String()})
	}
	known := map[string]bool{}
	for _, ref := range refs {
		known[ref] = true
	}
	seen := map[string]bool{}
	semantics := map[string]bool{}
	for pass := 0; pass < 16; pass++ {
		changed := false
		for _, v := range inventory {
			identity := v.Kind + ":" + v.ID
			if seen[identity] {
				continue
			}
			var payload any
			decoder := json.NewDecoder(strings.NewReader(v.Body))
			decoder.UseNumber()
			if decoder.Decode(&payload) != nil {
				out = append(out, d.Impact{Kind: "unreadable_manifest", Unknown: true, Name: "存在无法解析的受管依赖"})
				seen[identity] = true
				continue
			}
			matched := false
			for ref := range known {
				if warehouseContains(payload, ref) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			seen[identity] = true
			changed = true
			out = append(out, d.Impact{Kind: v.Kind, ID: v.ID, Name: v.Name})
			if v.Ref != "" {
				known[v.Ref] = true
			}
			if v.Kind == "warehouse_model" {
				var def d.Definition
				if json.Unmarshal([]byte(v.Body), &def) == nil && def.SemanticModelID != "" {
					semantics[def.SemanticModelID] = true
				}
			}
		}
		if !changed {
			break
		}
		if pass == 15 {
			out = append(out, d.Impact{Kind: "depth_limit", Unknown: true, Name: "依赖超过 16 层，影响清单不完整"})
		}
	}
	// Semantic metrics are fixed by explicitly linked model IDs; no SQL parsing
	// or guessed physical-table matches can invent a dependency.
	for id := range semantics {
		var model SemanticModelModel
		if e := r.db.WithContext(ctx).Where("tenant_id=? AND id=? AND deleted_at IS NULL", p.TenantID, id).Take(&model).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				out = append(out, d.Impact{Kind: "semantic_model", Unknown: true, Name: "已关联语义模型当前不可解析"})
				continue
			}
			return nil, e
		}
		out = append(out, d.Impact{Kind: "semantic_model", ID: id, Name: model.Name})
		var metrics []SemanticMetricModel
		if e := r.db.WithContext(ctx).Where("model_id=?", id).Order("id").Limit(1001).Find(&metrics).Error; e != nil {
			return nil, e
		}
		for _, v := range metrics[:bounded("metric_inventory", len(metrics))] {
			out = append(out, d.Impact{Kind: "metric", ID: v.ID, Name: v.Name})
		}
	}
	return out, nil
}
func warehouseContains(v any, ref string) bool {
	switch x := v.(type) {
	case string:
		return x == ref
	case []any:
		for _, y := range x {
			if warehouseContains(y, ref) {
				return true
			}
		}
	case map[string]any:
		if id, ok := x["asset_id"].(string); ok {
			if version, ok := x["version"].(json.Number); ok && id+":"+version.String() == ref {
				return true
			}
		}
		for _, y := range x {
			if warehouseContains(y, ref) {
				return true
			}
		}
	}
	return false
}

var _ d.Repository = (*warehouseRepository)(nil)

func (r *warehouseRepository) FindRunByKey(ctx context.Context, p l.Principal, key string) (*d.Run, error) {
	var row warehouseRunRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND idempotency_key=?", p.TenantID, p.UserID, key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return decodeWarehouse(row)
}
