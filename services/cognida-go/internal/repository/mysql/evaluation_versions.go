package mysql

import (
	d "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"time"
)

type datasetVersionHead struct {
	TenantID, UserID int64
	DatasetID        string
	CurrentVersion   int
}

func (datasetVersionHead) TableName() string { return "evaluation_dataset_heads" }

type datasetVersionRow struct {
	ID                                                  string
	TenantID, UserID                                    int64
	DatasetID                                           string
	Version                                             int
	IdempotencyKey, RequestDigest, ManifestDigest, Body string
	CreatedAt                                           time.Time
}

func (datasetVersionRow) TableName() string { return "evaluation_dataset_versions" }
func (r datasetVersionRow) domain() (*d.DatasetVersion, error) {
	var out d.DatasetVersion
	e := json.Unmarshal([]byte(r.Body), &out)
	return &out, e
}

type datasetVersionRepository struct{ db *gorm.DB }

func NewDatasetVersionRepository(db *gorm.DB) d.DatasetVersionRepository {
	return &datasetVersionRepository{db}
}
func (r *datasetVersionRepository) Put(ctx context.Context, p l.Principal, key string, in *d.DatasetVersion, revisions map[string]int) (*d.DatasetVersion, error) {
	if p.Validate() != nil {
		return nil, d.ErrVersionForbidden
	}
	if in == nil || in.DatasetID == "" || len(in.DatasetID) > 100 || key == "" || len(key) > 100 || len(revisions) > 1000 {
		return nil, d.ErrVersionInvalid
	}
	body, _ := json.Marshal(in)
	digest := g.LabelContentDigest(string(body))
	var out *d.DatasetVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		head := datasetVersionHead{TenantID: p.TenantID, UserID: p.UserID, DatasetID: in.DatasetID}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&head).Error; e != nil {
			return e
		}
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND dataset_id=?", p.TenantID, p.UserID, in.DatasetID).First(&head).Error; e != nil {
			return e
		}
		var prior datasetVersionRow
		e := tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", p.TenantID, p.UserID, key).First(&prior).Error
		if e == nil {
			if prior.RequestDigest != digest {
				return d.ErrVersionConflict
			}
			out, e = prior.domain()
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		ids := make([]string, 0, len(revisions))
		for id := range revisions {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			var row governanceRow
			if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("tenant_id=? AND kind='annotation_item' AND id=?", p.TenantID, id).First(&row).Error; e != nil {
				return d.ErrVersionConflict
			}
			if row.Version != revisions[id] || row.Status != "approved" {
				return d.ErrVersionConflict
			}
		}
		copy := *in
		copy.ID = uuid.NewString()
		copy.TenantID = p.TenantID
		copy.UserID = p.UserID
		copy.Version = head.CurrentVersion + 1
		copy.CreatedAt = time.Now().UTC()
		body, e := json.Marshal(copy)
		if e != nil {
			return e
		}
		if len(body) > 16<<20 {
			return d.ErrVersionInvalid
		}
		row := datasetVersionRow{ID: copy.ID, TenantID: p.TenantID, UserID: p.UserID, DatasetID: copy.DatasetID, Version: copy.Version, IdempotencyKey: key, RequestDigest: digest, ManifestDigest: copy.ManifestDigest, Body: string(body), CreatedAt: copy.CreatedAt}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if e = tx.Model(&datasetVersionHead{}).Where("tenant_id=? AND user_id=? AND dataset_id=?", p.TenantID, p.UserID, in.DatasetID).Update("current_version", copy.Version).Error; e != nil {
			return e
		}
		out = &copy
		return nil
	})
	return out, err
}
func (r *datasetVersionRepository) Get(ctx context.Context, p l.Principal, id string) (*d.DatasetVersion, error) {
	if p.Validate() != nil {
		return nil, d.ErrVersionForbidden
	}
	var row datasetVersionRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND id=?", p.TenantID, p.UserID, id).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrVersionNotFound
	}
	if e != nil {
		return nil, e
	}
	return row.domain()
}
func (r *datasetVersionRepository) List(ctx context.Context, p l.Principal, dataset, cursor string, limit int) (*d.VersionPage, error) {
	if p.Validate() != nil {
		return nil, d.ErrVersionForbidden
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
	if dataset != "" {
		q = q.Where("dataset_id=?", dataset)
	}
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []datasetVersionRow
	if e := q.Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := &d.VersionPage{Items: []d.DatasetVersion{}, HasMore: len(rows) > limit}
	if out.HasMore {
		rows = rows[:limit]
		out.NextCursor = rows[limit-1].ID
	}
	for _, row := range rows {
		v, e := row.domain()
		if e != nil {
			return nil, e
		}
		out.Items = append(out.Items, *v)
	}
	return out, nil
}
