package mysql

import (
	d "cognida/internal/model/apisource"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type apiCredentialRow struct {
	ID                             string
	TenantID, UserID               int64
	Name, Origin, Kind, Ciphertext string
	Enabled                        bool
	CreatedAt                      time.Time
}

func (apiCredentialRow) TableName() string { return "api_source_credentials" }

type apiSourceRow struct {
	ID                   string
	TenantID, UserID     int64
	Name                 string
	Version              int
	Enabled              bool
	Config               json.RawMessage
	Watermark, ActiveRun string
	CreatedAt, UpdatedAt time.Time
}

func (apiSourceRow) TableName() string { return "api_sources" }

type apiVersionRow struct {
	SourceID         string
	Version          int
	TenantID, UserID int64
	Body             json.RawMessage
}

func (apiVersionRow) TableName() string { return "api_source_versions" }

type apiRunRow struct {
	ID                                          string
	TenantID, UserID                            int64
	SourceID, OperationKey, RequestHash, Status string
	Version, Epoch, LeaseUntilUS                int64
	Body                                        json.RawMessage
	CreatedAt, UpdatedAt                        time.Time
}

func (apiRunRow) TableName() string { return "api_ingestion_runs" }

type apiRunState struct {
	Public                               d.Run `json:"public"`
	CursorCiphertext, SnapshotCiphertext string
	Records                              []string
	Keys                                 map[string]string
	Pages                                []apiStoredPage
	Epoch                                int64
	LeaseUntil                           time.Time
}
type apiStoredPage struct {
	Page      d.PageReceipt
	ObjectKey string
}
type apiSourceRepository struct{ db *gorm.DB }

func NewAPISourceRepository(db *gorm.DB) d.Repository { return &apiSourceRepository{db} }
func apiScope(db *gorm.DB, p l.Principal) *gorm.DB {
	return db.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
}
func apiError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	if errors.Is(e, gorm.ErrDuplicatedKey) {
		return d.ErrConflict
	}
	return e
}
func apiCredential(v apiCredentialRow) *d.Credential {
	return &d.Credential{ID: v.ID, TenantID: v.TenantID, UserID: v.UserID, Name: v.Name, Origin: v.Origin, Kind: v.Kind, Ciphertext: v.Ciphertext, Enabled: v.Enabled, CreatedAt: v.CreatedAt}
}
func (r *apiSourceRepository) CreateCredential(ctx context.Context, p l.Principal, in d.Credential) (*d.Credential, error) {
	if p.Validate() != nil || in.Ciphertext == "" {
		return nil, d.ErrForbidden
	}
	if _, e := uuid.Parse(in.ID); e != nil {
		return nil, d.ErrInvalid
	}
	row := apiCredentialRow{ID: in.ID, TenantID: p.TenantID, UserID: p.UserID, Name: in.Name, Origin: in.Origin, Kind: in.Kind, Ciphertext: in.Ciphertext, Enabled: true, CreatedAt: time.Now().UTC()}
	e := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	if e != nil {
		return nil, apiError(e)
	}
	return r.Credential(ctx, p, in.ID)
}
func (r *apiSourceRepository) Credential(ctx context.Context, p l.Principal, id string) (*d.Credential, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row apiCredentialRow
	e := apiScope(r.db.WithContext(ctx), p).Where("id=?", id).First(&row).Error
	return apiCredential(row), apiError(e)
}
func (r *apiSourceRepository) DisableCredential(ctx context.Context, p l.Principal, id string) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	result := apiScope(r.db.WithContext(ctx).Model(&apiCredentialRow{}), p).Where("id=?", id).Update("enabled", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if _, e := r.Credential(ctx, p, id); e != nil {
			return e
		}
	}
	return nil
}
func (r *apiSourceRepository) ListCredentials(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Credential, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []apiCredentialRow
	e := apiScope(r.db.WithContext(ctx), p).Where("id>?", cursor).Order("id").Limit(limit).Find(&rows).Error
	out := []d.Credential{}
	for _, row := range rows {
		out = append(out, *apiCredential(row))
	}
	return out, e
}
func apiSource(row apiSourceRow) (*d.Source, error) {
	out := &d.Source{ID: row.ID, TenantID: row.TenantID, UserID: row.UserID, Name: row.Name, Version: row.Version, Enabled: row.Enabled, Watermark: row.Watermark, ActiveRun: row.ActiveRun, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	e := json.Unmarshal(row.Config, &out.Config)
	return out, e
}
func apiSourceVersion(tx *gorm.DB, row apiSourceRow) error {
	source, e := apiSource(row)
	if e != nil {
		return e
	}
	source.ActiveRun = ""
	raw, e := json.Marshal(source)
	if e != nil {
		return e
	}
	return tx.Create(&apiVersionRow{SourceID: row.ID, Version: row.Version, TenantID: row.TenantID, UserID: row.UserID, Body: raw}).Error
}
func (r *apiSourceRepository) PutSource(ctx context.Context, p l.Principal, id string, in d.SourceInput) (out *d.Source, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if in.Config.Validate() != nil {
		return nil, d.ErrInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		config, _ := json.Marshal(in.Config)
		row := apiSourceRow{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Name: in.Name, Version: 1, Enabled: true, Config: config, CreatedAt: now, UpdatedAt: now}
		if id == "" {
			return d.ErrInvalid
		}
		if in.ExpectedVersion > 0 {
			if e := apiScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", id).First(&row).Error; e != nil {
				return apiError(e)
			}
			if row.Version != in.ExpectedVersion || row.ActiveRun != "" {
				return d.ErrConflict
			}
			row.Version++
			row.Config = config
			row.Name = in.Name
			row.Watermark = ""
			row.UpdatedAt = now
			if e := tx.Save(&row).Error; e != nil {
				return e
			}
		} else {
			row.ID = id
			inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			if inserted.Error != nil {
				return apiError(inserted.Error)
			}
			if inserted.RowsAffected == 0 {
				if e := apiScope(tx, p).Where("id=?", id).First(&row).Error; e != nil {
					return apiError(e)
				}
				existing, e := apiSource(row)
				if e != nil {
					return e
				}
				a, _ := json.Marshal(existing.Config)
				b, _ := json.Marshal(in.Config)
				if existing.Name != in.Name || string(a) != string(b) {
					return d.ErrConflict
				}
				out = existing
				return nil
			}
		}
		if e := apiSourceVersion(tx, row); e != nil {
			return e
		}
		out, err = apiSource(row)
		return err
	})
	return
}
func (r *apiSourceRepository) Source(ctx context.Context, p l.Principal, id string, version int) (*d.Source, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row apiSourceRow
	e := apiScope(r.db.WithContext(ctx), p).Where("id=?", id).First(&row).Error
	if e != nil {
		return nil, apiError(e)
	}
	if version == 0 || version == row.Version {
		return apiSource(row)
	}
	var fixed apiVersionRow
	e = apiScope(r.db.WithContext(ctx), p).Where("source_id=? AND version=?", id, version).First(&fixed).Error
	if e != nil {
		return nil, apiError(e)
	}
	var source d.Source
	if e = json.Unmarshal(fixed.Body, &source); e != nil {
		return nil, e
	}
	source.TenantID = p.TenantID
	source.UserID = p.UserID
	source.Enabled = row.Enabled
	source.Watermark = row.Watermark
	source.ActiveRun = row.ActiveRun
	return &source, nil
}
func (r *apiSourceRepository) ListSources(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Source, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []apiSourceRow
	e := apiScope(r.db.WithContext(ctx), p).Where("id>?", cursor).Order("id").Limit(limit).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := []d.Source{}
	for _, row := range rows {
		source, e := apiSource(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *source)
	}
	return out, nil
}
func (r *apiSourceRepository) SetEnabled(ctx context.Context, p l.Principal, id string, version int, enabled bool) (*d.Source, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	result := apiScope(r.db.WithContext(ctx).Model(&apiSourceRow{}), p).Where("id=? AND version=?", id, version).Updates(map[string]any{"enabled": enabled, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, d.ErrConflict
	}
	return r.Source(ctx, p, id, 0)
}
func apiEncode(run *d.Run) (apiRunRow, error) {
	pages := make([]apiStoredPage, len(run.Pages))
	for i, page := range run.Pages {
		pages[i] = apiStoredPage{page, page.ObjectKey}
	}
	records := make([]string, len(run.Records))
	for i, raw := range run.Records {
		records[i] = string(raw)
	}
	body, e := json.Marshal(apiRunState{Public: *run, CursorCiphertext: run.CursorCiphertext, SnapshotCiphertext: run.SnapshotCiphertext, Records: records, Keys: run.Keys, Pages: pages, Epoch: run.Epoch, LeaseUntil: run.LeaseUntil})
	lease := int64(0)
	if !run.LeaseUntil.IsZero() {
		lease = run.LeaseUntil.UnixMicro()
	}
	return apiRunRow{ID: run.ID, TenantID: run.TenantID, UserID: run.UserID, SourceID: run.SourceID, OperationKey: run.OperationKey, RequestHash: run.RequestHash, Status: run.Status, Version: run.Version, Epoch: run.Epoch, LeaseUntilUS: lease, Body: body, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt}, e
}
func apiDecode(row apiRunRow) (*d.Run, error) {
	var state apiRunState
	if e := json.Unmarshal(row.Body, &state); e != nil {
		return nil, e
	}
	out := state.Public
	if out.ID != row.ID || out.Status != row.Status || out.Version != row.Version || state.Epoch != row.Epoch || len(state.Pages) != len(out.Pages) {
		return nil, d.ErrConflict
	}
	out.TenantID = row.TenantID
	out.UserID = row.UserID
	out.OperationKey = row.OperationKey
	out.RequestHash = row.RequestHash
	out.Epoch = state.Epoch
	out.LeaseUntil = state.LeaseUntil
	out.Records = make([]json.RawMessage, len(state.Records))
	for i, raw := range state.Records {
		if !json.Valid([]byte(raw)) {
			return nil, d.ErrConflict
		}
		out.Records[i] = json.RawMessage(raw)
	}
	out.Keys = state.Keys
	out.CursorCiphertext = state.CursorCiphertext
	out.SnapshotCiphertext = state.SnapshotCiphertext
	for i, page := range state.Pages {
		out.Pages[i].ObjectKey = page.ObjectKey
	}
	return &out, nil
}
func apiFindRun(tx *gorm.DB, p l.Principal, id string, lock bool) (*d.Run, error) {
	query := apiScope(tx, p).Where("id=?", id)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row apiRunRow
	if e := query.First(&row).Error; e != nil {
		return nil, apiError(e)
	}
	return apiDecode(row)
}
func apiSaveRun(tx *gorm.DB, run *d.Run) error {
	old := run.Version
	run.Version++
	run.UpdatedAt = time.Now().UTC()
	row, e := apiEncode(run)
	if e != nil {
		return e
	}
	result := apiScope(tx.Model(&apiRunRow{}), l.Principal{TenantID: run.TenantID, UserID: run.UserID}).Where("id=? AND version=?", run.ID, old).Select("*").Updates(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func (r *apiSourceRepository) GetRun(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	return apiFindRun(r.db.WithContext(ctx), p, id, false)
}
func (r *apiSourceRepository) Start(ctx context.Context, p l.Principal, sourceID string, version int, key, hash string) (out *d.Run, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if len(key) != 64 || len(hash) != 64 || version < 1 {
		return nil, d.ErrInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head apiSourceRow
		if e := apiScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", sourceID).First(&head).Error; e != nil {
			return apiError(e)
		}
		var prior apiRunRow
		e := apiScope(tx, p).Where("operation_key=?", key).First(&prior).Error
		if e == nil {
			if prior.RequestHash != hash || prior.SourceID != sourceID {
				return d.ErrConflict
			}
			out, e = apiDecode(prior)
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !head.Enabled || head.Version != version || head.ActiveRun != "" {
			return d.ErrConflict
		}
		source, e := apiSource(head)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		out = &d.Run{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, SourceID: sourceID, SourceVersion: version, Version: 1, OperationKey: key, RequestHash: hash, Status: "queued", Config: source.Config, Pages: []d.PageReceipt{}, Records: []json.RawMessage{}, Keys: map[string]string{}, OutputRefs: []l.RefID{}, WatermarkBefore: head.Watermark, WatermarkAfter: head.Watermark, Consistency: "best_effort_pages; no cross-page snapshot guarantee", Deadline: now.Add(time.Duration(source.Config.Limits.TimeoutSeconds) * time.Second), CreatedAt: now, UpdatedAt: now}
		row, e := apiEncode(out)
		if e != nil {
			return e
		}
		if e = tx.Create(&row).Error; e != nil {
			return apiError(e)
		}
		return tx.Model(&apiSourceRow{}).Where("id=?", head.ID).Update("active_run", out.ID).Error
	})
	return
}
func (r *apiSourceRepository) Pending(ctx context.Context, limit int) ([]d.Run, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var rows []apiRunRow
	e := r.db.WithContext(ctx).Where("status IN ?", []string{"queued", "running"}).Order("created_at").Limit(limit).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := []d.Run{}
	for _, row := range rows {
		run, e := apiDecode(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *run)
	}
	return out, nil
}
func (r *apiSourceRepository) mutate(ctx context.Context, p l.Principal, id string, fn func(*gorm.DB, *apiSourceRow, *d.Run) error) (out *d.Run, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	first, e := r.GetRun(ctx, p, id)
	if e != nil {
		return nil, e
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source apiSourceRow
		if e := apiScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", first.SourceID).First(&source).Error; e != nil {
			return apiError(e)
		}
		out, e = apiFindRun(tx, p, id, true)
		if e != nil {
			return e
		}
		return fn(tx, &source, out)
	})
	return
}
func apiOwned(run *d.Run, epoch int64) bool {
	return run.Status == "running" && run.Epoch == epoch && time.Now().Before(run.LeaseUntil)
}
func (r *apiSourceRepository) Claim(ctx context.Context, p l.Principal, id string, lease time.Duration) (*d.Run, error) {
	if lease < time.Second || lease > time.Minute {
		return nil, d.ErrInvalid
	}
	return r.mutate(ctx, p, id, func(tx *gorm.DB, source *apiSourceRow, run *d.Run) error {
		if run.Terminal() {
			return nil
		}
		if run.LeaseUntil.After(time.Now()) {
			return d.ErrConflict
		}
		if run.Status == "running" && run.Error != "pages_complete" && len(run.Pages) > 0 && run.SnapshotCiphertext == "" {
			run.Error = "interrupted source does not support snapshot continuity"
		}
		run.Status = "running"
		run.Epoch++
		run.LeaseUntil = time.Now().Add(lease)
		return apiSaveRun(tx, run)
	})
}
func (r *apiSourceRepository) ReserveRequest(ctx context.Context, p l.Principal, id string, epoch int64) (*d.Run, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, _ *apiSourceRow, run *d.Run) error {
		if !apiOwned(run, epoch) || !run.Deadline.After(time.Now()) {
			return d.ErrConflict
		}
		if run.Requests >= run.Config.Limits.MaxRequests {
			return d.ErrInvalid
		}
		run.Requests++
		run.LeaseUntil = time.Now().Add(30 * time.Second)
		return apiSaveRun(tx, run)
	})
}
func (r *apiSourceRepository) Checkpoint(ctx context.Context, p l.Principal, id string, epoch int64, in d.Checkpoint) (*d.Run, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, _ *apiSourceRow, run *d.Run) error {
		if !apiOwned(run, epoch) || len(run.Pages) != in.ExpectedIndex || !run.Deadline.After(time.Now()) {
			return d.ErrConflict
		}
		if len(in.Records) > run.Config.Limits.MaxRecords || run.Bytes+in.Page.Bytes > run.Config.Limits.MaxBytes || len(run.Pages) >= run.Config.Limits.MaxPages {
			return d.ErrInvalid
		}
		run.Pages = append(run.Pages, in.Page)
		run.Records = in.Records
		run.RecordCount = len(in.Records)
		run.Keys = in.Keys
		run.DuplicateCount = in.DuplicateCount
		run.DeletedCount = in.DeletedCount
		run.Bytes += in.Page.Bytes
		run.CursorCiphertext = in.CursorCiphertext
		run.SnapshotCiphertext = in.SnapshotCiphertext
		run.SnapshotDigest = in.SnapshotDigest
		run.WatermarkAfter = in.WatermarkAfter
		run.LeaseUntil = time.Now().Add(30 * time.Second)
		if in.SnapshotDigest != "" {
			run.Consistency = "provider_snapshot_token; guarantee supplied by source"
		}
		if in.Complete {
			run.Error = "pages_complete"
		}
		return apiSaveRun(tx, run)
	})
}
func (r *apiSourceRepository) Finish(ctx context.Context, p l.Principal, id string, epoch int64, status, message string, retryable bool, refs []l.RefID) (*d.Run, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, source *apiSourceRow, run *d.Run) error {
		if run.Terminal() {
			return nil
		}
		if !apiOwned(run, epoch) {
			return d.ErrConflict
		}
		if status != "completed" && status != "partial" && status != "failed" && status != "timed_out" {
			return d.ErrInvalid
		}
		if status == "completed" && (run.Error != "pages_complete" || !run.Deadline.After(time.Now()) || !source.Enabled || (len(refs) != 2 && !(run.RecordCount == 0 && len(refs) == 1))) {
			return d.ErrConflict
		}
		if status == "completed" {
			var credential apiCredentialRow
			if e := apiScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=? AND enabled=?", run.Config.CredentialID, true).First(&credential).Error; e != nil {
				return d.ErrForbidden
			}
		}
		run.Status = status
		run.Error = message
		run.Retryable = retryable
		run.OutputRefs = refs
		run.LeaseUntil = time.Time{}
		if status == "completed" {
			if source.Watermark != run.WatermarkBefore {
				return d.ErrConflict
			}
			source.Watermark = run.WatermarkAfter
		}
		if source.ActiveRun == run.ID {
			source.ActiveRun = ""
		}
		if e := tx.Save(source).Error; e != nil {
			return e
		}
		return apiSaveRun(tx, run)
	})
}
func (r *apiSourceRepository) Cancel(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, source *apiSourceRow, run *d.Run) error {
		if run.Terminal() {
			return nil
		}
		run.Status = "cancelled"
		run.Error = "cancelled; already checkpointed pages retained"
		run.Retryable = false
		run.Epoch++
		run.LeaseUntil = time.Time{}
		if source.ActiveRun == run.ID {
			source.ActiveRun = ""
			if e := tx.Save(source).Error; e != nil {
				return e
			}
		}
		return apiSaveRun(tx, run)
	})
}
func (r *apiSourceRepository) Resume(ctx context.Context, p l.Principal, id string) (*d.Run, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, source *apiSourceRow, run *d.Run) error {
		if run.Status != "partial" || !run.Retryable || !run.Deadline.After(time.Now()) || !source.Enabled || source.ActiveRun != "" || source.Version != run.SourceVersion || source.Watermark != run.WatermarkBefore || len(run.Pages) > 0 && run.SnapshotCiphertext == "" {
			return d.ErrConflict
		}
		run.Status = "queued"
		run.Error = ""
		run.OutputRefs = nil
		run.Retryable = false
		source.ActiveRun = run.ID
		if e := tx.Save(source).Error; e != nil {
			return e
		}
		return apiSaveRun(tx, run)
	})
}
