package mysql

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func governanceGovernedID(p l.Principal, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("governance-governed/%d/%d/%s", p.TenantID, p.UserID, key))).String()
}
func (r *governanceRepository) FindGoverned(ctx context.Context, p l.Principal, key string) (*d.Record, error) {
	if p.Validate() != nil || len(key) != 64 {
		return nil, d.ErrInvalid
	}
	var row governanceRow
	e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=? AND kind=?", governanceGovernedID(p, key), p.TenantID, p.UserID, d.GovernedKind).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return row.domain(), nil
}
func (r *governanceRepository) SubmitGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation, cancel bool) (out *d.Record, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if err = in.Validate(); err != nil {
		return
	}
	id := governanceGovernedID(p, in.OperationKey)
	domainID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(id+"/task")).String()
	kind := "tagging_task"
	if in.AnnotationType() != "" {
		kind = "annotation_task"
	}
	if in.Profile == d.FeatureProfile {
		kind = "feature_execution"
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		status := "queued"
		if cancel {
			status = "cancelled"
		}
		state := d.GovernedExecution{Invocation: in, DomainID: domainID, DomainKind: kind, OutputRefs: []l.RefID{}}
		raw, _ := json.Marshal(state)
		row := governanceRow{ID: id, TenantID: p.TenantID, UserID: p.UserID, Kind: d.GovernedKind, Status: status, IdempotencyKey: id, Version: 1, Payload: raw, CreatedAt: now, UpdatedAt: now}
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if inserted.Error != nil {
			return inserted.Error
		}
		var stored governanceRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND kind=?", id, p.TenantID, p.UserID, d.GovernedKind).First(&stored).Error; e != nil {
			return e
		}
		var existing d.GovernedExecution
		if json.Unmarshal(stored.Payload, &existing) != nil || !d.SameGoverned(existing.Invocation, in) {
			return d.ErrConflict
		}
		if inserted.RowsAffected == 1 {
			var task any
			if kind == "tagging_task" {
				task = d.TaggingTask{Name: "Governed tagging candidates", SourceRefID: in.InputRef.String(), Config: d.TaggingConfig{EnableTags: true, EnableClassification: true, LabelSchema: in.Schema()}, Status: "governed_queued"}
			} else if kind == "annotation_task" {
				task = d.AnnotationTask{Name: "Governed annotation candidates", Type: in.AnnotationType(), SourceRefID: in.InputRef.String(), Labels: in.Schema(), QAMapping: in.QAMapping, Status: "governed_queued"}
			} else {
				task = map[string]any{"feature_id": in.FeatureID, "version": in.FeatureVersion, "source_ref_id": in.InputRef.String(), "status": "governed_queued"}
			}
			payload, _ := json.Marshal(task)
			taskRow := governanceRow{ID: domainID, TenantID: p.TenantID, UserID: p.UserID, Kind: kind, ParentID: id, Status: "governed_queued", IdempotencyKey: id + ":task", Version: 1, Payload: payload, CreatedAt: now, UpdatedAt: now}
			if cancel {
				taskRow.Status = "cancelled"
			}
			if e := tx.Create(&taskRow).Error; e != nil {
				return e
			}
		}
		if cancel && (stored.Status == "queued" || stored.Status == "running") {
			nextStatus := "cancelled"
			if in.Profile == d.FeatureProfile && stored.Status == "running" {
				nextStatus = "cancel_requested"
			}
			if e := tx.Model(&stored).Updates(map[string]any{"status": nextStatus, "version": stored.Version + 1, "updated_at": now}).Error; e != nil {
				return e
			}
			stored.Status = nextStatus
			stored.Version++
			stored.UpdatedAt = now
			if e := tx.Model(&governanceRow{}).Where("id=? AND tenant_id=? AND user_id=?", domainID, p.TenantID, p.UserID).Updates(map[string]any{"status": nextStatus, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		out = stored.domain()
		return nil
	})
	return
}

// The operation row is the cancellation fence for each item commit and final
// manifest binding. Callbacks must perform no remote I/O while holding this lock.
func (r *governanceRepository) UpdateGoverned(ctx context.Context, p l.Principal, id string, fn func(*d.Record, *d.GovernedExecution, d.Repository) error) (out *d.Record, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row governanceRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND kind=?", id, p.TenantID, p.UserID, d.GovernedKind).First(&row).Error; e != nil {
			return e
		}
		out = row.domain()
		var state d.GovernedExecution
		if json.Unmarshal(out.Payload, &state) != nil {
			return d.ErrInvalid
		}
		repo := &governanceRepository{tx}
		if e := fn(out, &state, repo); e != nil {
			return e
		}
		raw, e := json.Marshal(state)
		if e != nil {
			return e
		}
		if out.Status == row.Status && d.JSONEqual(raw, row.Payload) {
			return nil
		}
		out.Payload = raw
		return repo.Update(ctx, out, row.Version)
	})
	return
}

var _ d.GovernedRepository = (*governanceRepository)(nil)
