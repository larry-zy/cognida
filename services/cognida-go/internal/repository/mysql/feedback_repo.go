package mysql

import (
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type feedbackCaseRow struct {
	ID                                                                                string `gorm:"primaryKey"`
	TenantID, UserID                                                                  int64
	Version                                                                           int
	Status, IdempotencyKey, RequestDigest, BaselineKnowledgeID, ActiveVersionID, Body string
	CreatedAt, UpdatedAt                                                              time.Time
}

func (feedbackCaseRow) TableName() string { return "quality_feedback_cases" }

type feedbackPublicationRow struct {
	ID                                                       string `gorm:"primaryKey"`
	CaseID                                                   string
	TenantID, UserID                                         int64
	Version                                                  int
	Status, KnowledgeID, IdempotencyKey, RequestDigest, Body string
	CreatedAt, UpdatedAt                                     time.Time
}

func (feedbackPublicationRow) TableName() string { return "quality_feedback_publications" }

type feedbackEventRow d.Event

func (feedbackEventRow) TableName() string { return "quality_feedback_events" }

type feedbackRepository struct{ db *gorm.DB }

func NewFeedbackRepository(db *gorm.DB) d.Repository { return &feedbackRepository{db} }
func feedbackErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return e
}
func feedbackCase(row feedbackCaseRow) (*d.Case, error) {
	var c d.Case
	if e := json.Unmarshal([]byte(row.Body), &c); e != nil {
		return nil, e
	}
	c.ID = row.ID
	c.Version = row.Version
	c.Status = row.Status
	c.ActiveVersionID = row.ActiveVersionID
	c.CreatedAt = row.CreatedAt
	c.UpdatedAt = row.UpdatedAt
	return &c, nil
}
func feedbackPublication(row feedbackPublicationRow) (*d.Publication, error) {
	var p d.Publication
	if e := json.Unmarshal([]byte(row.Body), &p); e != nil {
		return nil, e
	}
	p.ID = row.ID
	p.Version = row.Version
	p.Status = row.Status
	p.CreatedAt = row.CreatedAt
	p.UpdatedAt = row.UpdatedAt
	return &p, nil
}
func feedbackEvent(tx *gorm.DB, e d.Event, now time.Time) error {
	e.ID = uuid.NewString()
	e.CreatedAt = now
	return tx.Create((*feedbackEventRow)(&e)).Error
}
func (r *feedbackRepository) CreateCase(ctx context.Context, c *d.Case, key, digest string) (out *d.Case, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		c.Version = 1
		c.CreatedAt = now
		c.UpdatedAt = now
		raw, e := json.Marshal(c)
		if e != nil {
			return e
		}
		row := feedbackCaseRow{ID: c.ID, TenantID: c.TenantID, UserID: c.UserID, Version: 1, Status: c.Status, IdempotencyKey: key, RequestDigest: digest, BaselineKnowledgeID: c.BaselineKnowledgeID, Body: string(raw), CreatedAt: now, UpdatedAt: now}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return result.Error
		}
		if e = tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", c.TenantID, c.UserID, key).Take(&row).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return d.ErrConflict
			}
			return e
		}
		if row.RequestDigest != digest {
			return d.ErrConflict
		}
		out, e = feedbackCase(row)
		if e != nil {
			return e
		}
		if result.RowsAffected == 1 {
			return feedbackEvent(tx, d.Event{CaseID: c.ID, Action: "created", ActorID: c.UserID, Detail: "固定来源、验证门槛和返工预算已登记"}, now)
		}
		return nil
	})
	return
}
func (r *feedbackRepository) GetCase(ctx context.Context, p l.Principal, id string) (*d.Case, error) {
	var row feedbackCaseRow
	if e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
		return nil, feedbackErr(e)
	}
	return feedbackCase(row)
}
func (r *feedbackRepository) ListCases(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Case, string, error) {
	return r.list(ctx, r.db.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID), cursor, limit)
}
func (r *feedbackRepository) ScanCases(ctx context.Context, cursor string, limit int) ([]d.Case, string, error) {
	return r.list(ctx, r.db, cursor, limit)
}
func (r *feedbackRepository) list(ctx context.Context, q *gorm.DB, cursor string, limit int) ([]d.Case, string, error) {
	var rows []feedbackCaseRow
	q = q.WithContext(ctx).Where("id>?", cursor).Order("id").Limit(limit + 1)
	if e := q.Find(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	out := []d.Case{}
	for _, row := range rows {
		c, e := feedbackCase(row)
		if e != nil {
			return nil, "", e
		}
		out = append(out, *c)
	}
	return out, next, nil
}
func (r *feedbackRepository) SaveCase(ctx context.Context, c *d.Case, expected int, event d.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row feedbackCaseRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", c.ID, c.TenantID, c.UserID).Take(&row).Error; e != nil {
			return feedbackErr(e)
		}
		if row.Version != expected || row.ActiveVersionID != c.ActiveVersionID {
			return d.ErrConflict
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		c.Version = expected + 1
		c.UpdatedAt = now
		raw, e := json.Marshal(c)
		if e != nil {
			return e
		}
		if e = tx.Model(&row).Updates(map[string]any{"version": c.Version, "status": c.Status, "body": string(raw), "updated_at": now}).Error; e != nil {
			return e
		}
		event.CaseID = c.ID
		return feedbackEvent(tx, event, now)
	})
}
func (r *feedbackRepository) CreatePublication(ctx context.Context, p *d.Publication, key, digest string) (out *d.Publication, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent feedbackCaseRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", p.CaseID, p.TenantID, p.UserID).Take(&parent).Error; e != nil {
			return feedbackErr(e)
		}
		if parent.Status == "invalidated" || parent.Status == "withdrawn" {
			return d.ErrConflict
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		p.Version = 1
		p.CreatedAt = now
		p.UpdatedAt = now
		raw, e := json.Marshal(p)
		if e != nil {
			return e
		}
		row := feedbackPublicationRow{ID: p.ID, CaseID: p.CaseID, TenantID: p.TenantID, UserID: p.UserID, Version: 1, Status: p.Status, KnowledgeID: p.KnowledgeID, IdempotencyKey: key, RequestDigest: digest, Body: string(raw), CreatedAt: now, UpdatedAt: now}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return result.Error
		}
		if e = tx.Where("case_id=? AND idempotency_key=?", p.CaseID, key).Take(&row).Error; e != nil {
			return e
		}
		if row.RequestDigest != digest {
			return d.ErrConflict
		}
		out, e = feedbackPublication(row)
		if e != nil {
			return e
		}
		if result.RowsAffected == 1 {
			return feedbackEvent(tx, d.Event{CaseID: p.CaseID, PublicationID: p.ID, ActorID: p.UserID, Action: "candidate_created", Detail: "候选仅用于隔离验证；尚未激活"}, now)
		}
		return nil
	})
	return
}
func (r *feedbackRepository) GetPublication(ctx context.Context, p l.Principal, id string) (*d.Publication, error) {
	var row feedbackPublicationRow
	if e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Take(&row).Error; e != nil {
		return nil, feedbackErr(e)
	}
	return feedbackPublication(row)
}
func (r *feedbackRepository) ListPublications(ctx context.Context, p l.Principal, id string) ([]d.Publication, error) {
	var rows []feedbackPublicationRow
	if e := r.db.WithContext(ctx).Where("case_id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Order("created_at,id").Limit(100).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.Publication{}
	for _, row := range rows {
		v, e := feedbackPublication(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, nil
}
func (r *feedbackRepository) SavePublication(ctx context.Context, p *d.Publication, expected int, event d.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row feedbackPublicationRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", p.ID, p.TenantID, p.UserID).Take(&row).Error; e != nil {
			return feedbackErr(e)
		}
		if row.Version != expected {
			return d.ErrConflict
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		p.Version = expected + 1
		p.UpdatedAt = now
		raw, e := json.Marshal(p)
		if e != nil {
			return e
		}
		if e = tx.Model(&row).Updates(map[string]any{"version": p.Version, "status": p.Status, "body": string(raw), "updated_at": now}).Error; e != nil {
			return e
		}
		event.CaseID = p.CaseID
		event.PublicationID = p.ID
		return feedbackEvent(tx, event, now)
	})
}
func (r *feedbackRepository) Commit(ctx context.Context, p l.Principal, caseID, pubID string, caseVersion, pubVersion int, digest, action string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var head feedbackCaseRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", caseID, p.TenantID, p.UserID).Take(&head).Error; e != nil {
			return feedbackErr(e)
		}
		target := pubID
		status := "active"
		if action == "withdraw" {
			target = ""
			status = "withdrawn"
		}
		if action == "rollback_baseline" {
			target = ""
		}
		if head.ActiveVersionID == target && head.Status == status {
			return nil
		}
		if head.Version != caseVersion || head.Status == "invalidated" {
			return d.ErrConflict
		}
		if target != "" {
			var row feedbackPublicationRow
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND case_id=? AND tenant_id=? AND user_id=?", target, caseID, p.TenantID, p.UserID).Take(&row).Error; e != nil {
				return feedbackErr(e)
			}
			v, e := feedbackPublication(row)
			if e != nil {
				return e
			}
			if v.Version != pubVersion || v.Status != "approved" || v.ValidationDigest != digest || len(v.Projection) == 0 {
				return d.ErrGate
			}
			// The document's enabled bit and authoritative activity pointer commit together.
			result := tx.Model(&KnowledgeModel{}).Where("id=? AND tenant_id=? AND user_id=? AND deleted_at IS NULL AND parse_status=? AND file_hash=?", v.KnowledgeID, p.TenantID, p.UserID, "completed", v.ContentDigest).Update("enable_status", "enabled")
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return d.ErrGate
			}
		}
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if e = tx.Model(&head).Updates(map[string]any{"active_version_id": target, "status": status, "version": head.Version + 1, "updated_at": now}).Error; e != nil {
			return e
		}
		return feedbackEvent(tx, d.Event{CaseID: caseID, PublicationID: pubID, Action: action, ActorID: p.UserID, Detail: "活动指针已事务提交；每次读取按当前指针与来源重新授权"}, now)
	})
}
func (r *feedbackRepository) Events(ctx context.Context, p l.Principal, id string) ([]d.Event, error) {
	if _, e := r.GetCase(ctx, p, id); e != nil {
		return nil, e
	}
	var rows []feedbackEventRow
	if e := r.db.WithContext(ctx).Where("case_id=?", id).Order("created_at,id").Limit(300).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.Event{}
	for _, v := range rows {
		out = append(out, d.Event(v))
	}
	return out, nil
}
func (r *feedbackRepository) FindKnowledge(ctx context.Context, tenant int64, id string) (*d.Case, *d.Publication, error) {
	var row feedbackCaseRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND baseline_knowledge_id=?", tenant, id).Take(&row).Error
	if e == nil {
		c, e := feedbackCase(row)
		return c, nil, e
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil, e
	}
	var pub feedbackPublicationRow
	e = r.db.WithContext(ctx).Where("tenant_id=? AND knowledge_id=?", tenant, id).Take(&pub).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if e != nil {
		return nil, nil, e
	}
	p, e := feedbackPublication(pub)
	if e != nil {
		return nil, nil, e
	}
	c, e := r.GetCase(ctx, l.Principal{TenantID: tenant, UserID: pub.UserID}, pub.CaseID)
	return c, p, e
}
