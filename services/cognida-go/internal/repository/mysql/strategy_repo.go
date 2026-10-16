package mysql

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type strategyHeadRow struct {
	TenantID, UserID     int64
	Slot, Kind           string
	Revision             int64
	NextVersion          int
	BaselineID, ActiveID string
}

func (strategyHeadRow) TableName() string { return "strategy_heads" }

type strategyVersionRow struct {
	ID               string
	TenantID, UserID int64
	Slot             string
	VersionNumber    int
	Body             []byte
	DisabledAt       *time.Time
	CreatedAt        time.Time
}

func (strategyVersionRow) TableName() string { return "strategy_versions" }

type strategyEventRow struct {
	ID                                               string
	TenantID, UserID                                 int64
	Slot, Kind, OperationKey, RequestHash, VersionID string
	Payload                                          []byte
	CreatedAt                                        time.Time
}

func (strategyEventRow) TableName() string { return "strategy_events" }

type strategyRepository struct{ db *gorm.DB }

func NewStrategyRepository(db *gorm.DB) d.Repository { return &strategyRepository{db} }
func strategyErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return e
}
func strategyScope(p l.Principal) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	return nil
}
func (r *strategyRepository) Head(ctx context.Context, p l.Principal, slot string) (*d.Head, error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	var row strategyHeadRow
	if e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=?", p.TenantID, p.UserID, slot).First(&row).Error; e != nil {
		return nil, strategyErr(e)
	}
	return &d.Head{TenantID: row.TenantID, UserID: row.UserID, Slot: row.Slot, Kind: row.Kind, Revision: row.Revision, NextVersion: row.NextVersion, BaselineID: row.BaselineID, ActiveID: row.ActiveID}, nil
}
func (r *strategyRepository) Version(ctx context.Context, p l.Principal, id string) (*d.Version, error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	var row strategyVersionRow
	if e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND id=?", p.TenantID, p.UserID, id).First(&row).Error; e != nil {
		return nil, strategyErr(e)
	}
	var v d.Version
	if e := json.Unmarshal(row.Body, &v); e != nil {
		return nil, e
	}
	v.DisabledAt = row.DisabledAt
	return &v, nil
}

type strategyTx struct {
	*strategyRepository
	p    l.Principal
	head *d.Head
}

func (t *strategyTx) Head() *d.Head { return t.head }
func (r *strategyRepository) InSlot(ctx context.Context, p l.Principal, slot, kind string, fn func(d.Tx) error) error {
	if e := strategyScope(p); e != nil {
		return e
	}
	if !d.ValidSlot(slot) {
		return d.ErrInvalid
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if kind != "" {
			row := strategyHeadRow{TenantID: p.TenantID, UserID: p.UserID, Slot: slot, Kind: kind, NextVersion: 1}
			if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
				return e
			}
		}
		var row strategyHeadRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND slot=?", p.TenantID, p.UserID, slot).First(&row).Error; e != nil {
			return strategyErr(e)
		}
		if kind != "" && row.Kind != kind {
			return d.ErrConflict
		}
		head := &d.Head{TenantID: row.TenantID, UserID: row.UserID, Slot: row.Slot, Kind: row.Kind, Revision: row.Revision, NextVersion: row.NextVersion, BaselineID: row.BaselineID, ActiveID: row.ActiveID}
		return fn(&strategyTx{&strategyRepository{tx}, p, head})
	})
}
func (t *strategyTx) Version(ctx context.Context, id string) (*d.Version, error) {
	v, e := t.strategyRepository.Version(ctx, t.p, id)
	if e == nil && v.Slot != t.head.Slot {
		return nil, d.ErrNotFound
	}
	return v, e
}
func (t *strategyTx) AppendVersion(ctx context.Context, v *d.Version) error {
	if v.Kind != t.head.Kind || d.ValidateBody(v.Kind, v.Body) != nil || !d.ValidGates(v.Gates) || len(v.EvidenceRefs) > 32 {
		return d.ErrInvalid
	}
	v.ID = uuid.NewString()
	v.TenantID = t.p.TenantID
	v.UserID = t.p.UserID
	v.Slot = t.head.Slot
	v.Number = t.head.NextVersion
	v.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	v.DisabledAt = nil
	v.Digest = d.Digest(struct {
		Kind     string
		Body     json.RawMessage
		Baseline string
		Sources  map[string]string
		Gates    d.Gates
	}{v.Kind, v.Body, v.BaselineID, v.SourceDigests, v.Gates})
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if e = t.db.WithContext(ctx).Create(&strategyVersionRow{ID: v.ID, TenantID: v.TenantID, UserID: v.UserID, Slot: v.Slot, VersionNumber: v.Number, Body: raw, CreatedAt: v.CreatedAt}).Error; e != nil {
		return e
	}
	t.head.NextVersion++
	return t.SetHead(ctx)
}
func (t *strategyTx) SetHead(ctx context.Context) error {
	h := t.head
	return t.db.WithContext(ctx).Model(&strategyHeadRow{}).Where("tenant_id=? AND user_id=? AND slot=?", t.p.TenantID, t.p.UserID, h.Slot).Updates(map[string]any{"revision": h.Revision, "next_version": h.NextVersion, "baseline_id": h.BaselineID, "active_id": h.ActiveID}).Error
}
func (t *strategyTx) DisableVersion(ctx context.Context, id string) error {
	v, e := t.Version(ctx, id)
	if e != nil {
		return e
	}
	if v.DisabledAt != nil {
		return nil
	}
	return t.db.WithContext(ctx).Model(&strategyVersionRow{}).Where("tenant_id=? AND user_id=? AND slot=? AND id=?", t.p.TenantID, t.p.UserID, t.head.Slot, id).Update("disabled_at", gorm.Expr("UTC_TIMESTAMP(3)")).Error
}
func strategyEvent(row strategyEventRow) *d.Event {
	return &d.Event{ID: row.ID, Kind: row.Kind, Key: row.OperationKey, Hash: row.RequestHash, VersionID: row.VersionID, Payload: row.Payload, CreatedAt: row.CreatedAt}
}
func (t *strategyTx) Event(ctx context.Context, kind, key string) (*d.Event, error) {
	var row strategyEventRow
	e := t.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=? AND kind=? AND operation_key=?", t.p.TenantID, t.p.UserID, t.head.Slot, kind, key).First(&row).Error
	if e != nil {
		return nil, strategyErr(e)
	}
	return strategyEvent(row), nil
}
func (t *strategyTx) EventByID(ctx context.Context, id string) (*d.Event, error) {
	var row strategyEventRow
	e := t.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=? AND id=?", t.p.TenantID, t.p.UserID, t.head.Slot, id).First(&row).Error
	if e != nil {
		return nil, strategyErr(e)
	}
	return strategyEvent(row), nil
}
func (t *strategyTx) AppendEvent(ctx context.Context, v *d.Event) error {
	if len(v.Payload) > 8<<20 || !json.Valid(v.Payload) || len(v.Hash) != 64 || !d.ValidKey(v.Key) {
		return d.ErrInvalid
	}
	if _, e := t.Version(ctx, v.VersionID); e != nil {
		return e
	}
	v.ID = uuid.NewString()
	v.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	return t.db.WithContext(ctx).Create(&strategyEventRow{ID: v.ID, TenantID: t.p.TenantID, UserID: t.p.UserID, Slot: t.head.Slot, Kind: v.Kind, OperationKey: v.Key, RequestHash: v.Hash, VersionID: v.VersionID, Payload: v.Payload, CreatedAt: v.CreatedAt}).Error
}
func (r *strategyRepository) Versions(ctx context.Context, p l.Principal, slot, cursor string, limit int) (*d.Page[d.Version], error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	limit = catalogLimit(limit)
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=?", p.TenantID, p.UserID, slot)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []strategyVersionRow
	if e := q.Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := &d.Page[d.Version]{Items: []d.Version{}}
	if len(rows) > limit {
		out.NextCursor = rows[limit-1].ID
		rows = rows[:limit]
	}
	for _, row := range rows {
		var v d.Version
		if e := json.Unmarshal(row.Body, &v); e != nil {
			return nil, e
		}
		v.DisabledAt = row.DisabledAt
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (r *strategyRepository) Events(ctx context.Context, p l.Principal, slot, kind, cursor string, limit int) (*d.Page[d.Event], error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	limit = catalogLimit(limit)
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=?", p.TenantID, p.UserID, slot)
	if kind != "" {
		q = q.Where("kind=?", kind)
	}
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []strategyEventRow
	if e := q.Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := &d.Page[d.Event]{Items: []d.Event{}}
	if len(rows) > limit {
		out.NextCursor = rows[limit-1].ID
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, *strategyEvent(row))
	}
	return out, nil
}

func (r *strategyRepository) ReadEvent(ctx context.Context, p l.Principal, slot, id string) (*d.Event, error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	var row strategyEventRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=? AND id=?", p.TenantID, p.UserID, slot, id).First(&row).Error
	if e != nil {
		return nil, strategyErr(e)
	}
	return strategyEvent(row), nil
}

func (r *strategyRepository) FindEvent(ctx context.Context, p l.Principal, slot, kind, key string) (*d.Event, error) {
	if e := strategyScope(p); e != nil {
		return nil, e
	}
	var row strategyEventRow
	if e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND slot=? AND kind=? AND operation_key=?", p.TenantID, p.UserID, slot, kind, key).Take(&row).Error; e != nil {
		return nil, strategyErr(e)
	}
	return strategyEvent(row), nil
}
