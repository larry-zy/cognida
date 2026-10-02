package mysql

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type governanceRow struct {
	ID                                     string `gorm:"primaryKey"`
	TenantID, UserID                       int64
	Kind, ParentID, Status, IdempotencyKey string
	Version                                int
	Payload                                []byte
	DueAt                                  *time.Time
	CreatedAt, UpdatedAt                   time.Time
}

func (governanceRow) TableName() string { return "governance_records" }
func (row governanceRow) domain() *d.Record {
	return &d.Record{Meta: d.Meta{ID: row.ID, TenantID: row.TenantID, UserID: row.UserID, Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, Kind: row.Kind, ParentID: row.ParentID, Status: row.Status, IdempotencyKey: row.IdempotencyKey, Payload: row.Payload, DueAt: row.DueAt}
}
func governanceModel(v *d.Record) governanceRow {
	return governanceRow{ID: v.ID, TenantID: v.TenantID, UserID: v.UserID, Kind: v.Kind, ParentID: v.ParentID, Status: v.Status, IdempotencyKey: v.IdempotencyKey, Version: v.Version, Payload: v.Payload, DueAt: v.DueAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

type governanceRepository struct{ db *gorm.DB }

func NewGovernanceRepository(db *gorm.DB) d.Repository { return &governanceRepository{db} }
func (r *governanceRepository) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&governanceRepository{tx}) })
}
func (r *governanceRepository) Create(ctx context.Context, v *d.Record) (*d.Record, error) {
	if v.ID == "" {
		v.ID = uuid.NewString()
	}
	if v.IdempotencyKey == "" {
		v.IdempotencyKey = v.ID
	}
	v.Version = 1
	v.CreatedAt = time.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	row := governanceModel(v)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		var prev governanceRow
		if e := r.db.WithContext(ctx).Where("tenant_id=? AND kind=? AND idempotency_key=?", v.TenantID, v.Kind, v.IdempotencyKey).First(&prev).Error; e == nil {
			return prev.domain(), nil
		}
		return nil, err
	}
	return row.domain(), nil
}
func (r *governanceRepository) Get(ctx context.Context, tenant int64, kind, id string) (*d.Record, error) {
	var row governanceRow
	err := r.db.WithContext(ctx).Where("tenant_id=? AND kind=? AND id=?", tenant, kind, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.domain(), nil
}
func (r *governanceRepository) List(ctx context.Context, tenant int64, f d.Filter) (*d.Page[d.Record], error) {
	q := r.db.WithContext(ctx).Where("tenant_id=? AND kind=?", tenant, f.Kind)
	if f.ParentID != "" {
		q = q.Where("parent_id=?", f.ParentID)
	}
	if f.Status != "" {
		q = q.Where("status=?", f.Status)
	}
	if f.UserID > 0 {
		q = q.Where("user_id=?", f.UserID)
	}
	if f.Cursor != "" {
		q = q.Where("id>?", f.Cursor)
	}
	limit := catalogLimit(f.Limit)
	var rows []governanceRow
	if err := q.Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &d.Page[d.Record]{Items: []d.Record{}, HasMore: len(rows) > limit}
	if out.HasMore {
		rows = rows[:limit]
		out.NextCursor = rows[limit-1].ID
	}
	for _, v := range rows {
		out.Items = append(out.Items, *v.domain())
	}
	return out, nil
}
func (r *governanceRepository) Update(ctx context.Context, v *d.Record, version int) error {
	result := r.db.WithContext(ctx).Model(&governanceRow{}).Where("id=? AND tenant_id=? AND kind=? AND version=?", v.ID, v.TenantID, v.Kind, version).Updates(map[string]any{"payload": []byte(v.Payload), "status": v.Status, "due_at": v.DueAt, "version": version + 1, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	v.Version = version + 1
	v.UpdatedAt = time.Now().UTC()
	return nil
}
func (r *governanceRepository) Due(ctx context.Context, now time.Time, limit int) ([]d.Record, error) {
	var rows []governanceRow
	err := r.db.WithContext(ctx).Where("kind='subscription' AND status='enabled' AND due_at<=?", now).Order("due_at").Limit(catalogLimit(limit)).Find(&rows).Error
	out := []d.Record{}
	for _, row := range rows {
		out = append(out, *row.domain())
	}
	return out, err
}
func (r *governanceRepository) Pending(ctx context.Context, limit int) ([]d.Record, error) {
	var rows []governanceRow
	err := r.db.WithContext(ctx).Where("kind IN ? AND status IN ?", []string{"subscription_run", "tagging_task"}, []string{"queued", "running"}).Order("updated_at").Limit(catalogLimit(limit)).Find(&rows).Error
	out := []d.Record{}
	for _, row := range rows {
		out = append(out, *row.domain())
	}
	return out, err
}

type featureValueRow struct {
	TenantID    int64     `gorm:"primaryKey"`
	FeatureID   string    `gorm:"primaryKey"`
	Version     int       `gorm:"primaryKey"`
	EntityID    string    `gorm:"primaryKey"`
	ObservedAt  time.Time `gorm:"primaryKey"`
	Value       []byte
	SourceRefID string
}

func (featureValueRow) TableName() string { return "feature_values" }
func (r *governanceRepository) FeatureWrite(ctx context.Context, tenant int64, values []d.FeatureValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, v := range values {
			row := featureValueRow{TenantID: tenant, FeatureID: v.FeatureID, Version: v.Version, EntityID: v.EntityID, ObservedAt: v.Timestamp.UTC().Truncate(time.Microsecond), Value: v.Value, SourceRefID: v.SourceRefID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var existing featureValueRow
			if err := tx.Where("tenant_id=? AND feature_id=? AND version=? AND entity_id=? AND observed_at=?", tenant, v.FeatureID, v.Version, v.EntityID, v.Timestamp.UTC().Truncate(time.Microsecond)).First(&existing).Error; err != nil {
				return err
			}
			if !d.JSONEqual(existing.Value, v.Value) || existing.SourceRefID != v.SourceRefID {
				return d.ErrConflict
			}
		}
		return nil
	})
}
func (r *governanceRepository) FeatureQuery(ctx context.Context, tenant int64, entities []string, features []d.FeatureValue, at time.Time) ([]d.FeatureValue, error) {
	out := []d.FeatureValue{}
	for _, feature := range features {
		for _, entity := range entities {
			var row featureValueRow
			err := r.db.WithContext(ctx).Where("tenant_id=? AND feature_id=? AND version=? AND entity_id=? AND observed_at<=?", tenant, feature.FeatureID, feature.Version, entity, at).Order("observed_at DESC").First(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out = append(out, d.FeatureValue{FeatureID: row.FeatureID, Version: row.Version, EntityID: row.EntityID, Timestamp: row.ObservedAt, Value: row.Value, SourceRefID: row.SourceRefID})
		}
	}
	return out, nil
}

// PublishedRefs uses a compound cursor so versions sharing an asset ID cannot
// disappear at page boundaries. Policy enforcement remains in Assets.Resolve.
func (r *governanceRepository) PublishedRefs(ctx context.Context, tenant int64, cursor string, limit int) ([]string, error) {
	q := r.db.WithContext(ctx).Table("lifecycle_assets").Where("tenant_id=? AND JSON_UNQUOTE(JSON_EXTRACT(body,'$.kind'))=? AND JSON_EXTRACT(body,'$.policy.approved')=true", tenant, l.PublishedAsset)
	if cursor != "" {
		id, err := l.ParseRefID(cursor)
		if err != nil {
			return nil, d.ErrInvalid
		}
		q = q.Where("(asset_id>? OR (asset_id=? AND version>?))", id.AssetID, id.AssetID, id.Version)
	}
	var rows []struct {
		AssetID string
		Version int
	}
	if err := q.Select("asset_id,version").Order("asset_id ASC,version ASC").Limit(catalogLimit(limit)).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, (l.RefID{AssetID: r.AssetID, Version: r.Version}).String())
	}
	return out, nil
}
