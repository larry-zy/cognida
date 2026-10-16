package mysql

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strings"
	"time"
)

type knowledgeLabelReceipt struct {
	ID                                                                      string `gorm:"primaryKey"`
	TenantID, UserID                                                        int64
	KnowledgeID, ResultID                                                   string
	ReviewVersion                                                           int
	SourceRefID, ContentDigest, SchemeDigest, IdempotencyKey, RequestDigest string
	LabelsJSON                                                              []byte `gorm:"column:labels_json"`
	Status                                                                  string
	CreatedAt                                                               time.Time
}

func (knowledgeLabelReceipt) TableName() string { return "knowledge_label_applications" }
func (r knowledgeLabelReceipt) domain() *d.KnowledgeLabelApplication {
	out := &d.KnowledgeLabelApplication{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, KnowledgeID: r.KnowledgeID, ResultID: r.ResultID, ReviewVersion: r.ReviewVersion, SourceRefID: r.SourceRefID, ContentDigest: r.ContentDigest, SchemeDigest: r.SchemeDigest, Status: r.Status, CreatedAt: r.CreatedAt}
	_ = json.Unmarshal(r.LabelsJSON, &out.Labels)
	return out
}

type knowledgeAppliedLabel struct {
	TenantID                                         int64
	KnowledgeID, Label, ApplicationID, ContentDigest string
}

func (knowledgeAppliedLabel) TableName() string { return "knowledge_applied_labels" }

type knowledgeLabelRepository struct{ db *gorm.DB }

func NewKnowledgeLabelRepository(db *gorm.DB) d.KnowledgeLabelRepository {
	return &knowledgeLabelRepository{db}
}
func (r *knowledgeLabelRepository) ApplyApproved(ctx context.Context, p l.Principal, in d.PreparedKnowledgeLabels) (*d.KnowledgeLabelApplication, error) {
	if p.Validate() != nil || in.ResultID == "" || in.KnowledgeID == "" || in.ExpectedVersion < 1 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 100 {
		return nil, d.ErrInvalid
	}
	labels, _ := json.Marshal(in.Labels)
	request, _ := json.Marshal(in)
	digest := d.LabelContentDigest(string(request))
	var out *d.KnowledgeLabelApplication
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var review governanceRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND kind='tagging_result' AND id=?", p.TenantID, in.ResultID).First(&review).Error; e != nil {
			return d.ErrNotFound
		}
		if review.Version != in.ExpectedVersion || review.Status != "approved" || d.LabelContentDigest(string(review.Payload)) != in.ReviewDigest {
			return d.ErrConflict
		}
		var doc KnowledgeModel
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=? AND deleted_at IS NULL AND enable_status='enabled'", p.TenantID, in.KnowledgeID).First(&doc).Error; e != nil {
			return d.ErrForbidden
		}
		var kb KnowledgeBaseModel
		if e := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("tenant_id=? AND id=? AND deleted_at IS NULL AND status=1 AND (is_public=1 OR user_id=?)", p.TenantID, doc.KnowledgeBaseID, p.UserID).First(&kb).Error; e != nil {
			return d.ErrForbidden
		}
		content, e := knowledgeLabelContent(tx, p.TenantID, in.KnowledgeID)
		if e != nil {
			return e
		}
		contentDigest := d.LabelContentDigest(content)
		if content == "" {
			return d.ErrConflict
		}
		if in.PublicationRefID == "" {
			if in.PublicationContentDigest != "" || in.PublicationChunkDigest != "" || contentDigest != d.LabelContentDigest(in.Content) {
				return d.ErrConflict
			}
		} else {
			// Publication evidence is constructed by the authorized lifecycle
			// resolver, never accepted from HTTP fields. Bind reviewed source
			// bytes to the ingestion snapshot and verify all current chunks still
			// match it, including parser boundaries and intentional overlap.
			if doc.ParseStatus != "completed" || len(in.PublicationContentDigest) != 64 || len(in.PublicationChunkDigest) != 64 ||
				in.PublicationContentDigest != d.LabelContentDigest(in.Content) || doc.FileHash != in.PublicationContentDigest ||
				doc.IngestedChunkDigest != in.PublicationChunkDigest || contentDigest != in.PublicationChunkDigest {
				return d.ErrConflict
			}
		}
		var prior knowledgeLabelReceipt
		e = tx.Where("tenant_id=? AND user_id=? AND idempotency_key=?", p.TenantID, p.UserID, in.IdempotencyKey).First(&prior).Error
		if e == nil {
			if prior.RequestDigest != digest {
				return d.ErrConflict
			}
			out = prior.domain()
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		e = tx.Where("tenant_id=? AND knowledge_id=? AND result_id=? AND review_version=?", p.TenantID, in.KnowledgeID, in.ResultID, in.ExpectedVersion).First(&prior).Error
		if e == nil {
			out = prior.domain()
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		row := knowledgeLabelReceipt{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, KnowledgeID: in.KnowledgeID, ResultID: in.ResultID, ReviewVersion: in.ExpectedVersion, SourceRefID: in.SourceRefID, ContentDigest: contentDigest, SchemeDigest: in.SchemeDigest, IdempotencyKey: in.IdempotencyKey, RequestDigest: digest, LabelsJSON: labels, Status: "applied", CreatedAt: time.Now().UTC()}
		if e = tx.Create(&row).Error; e != nil {
			return e
		}
		if e = tx.Where("tenant_id=? AND knowledge_id=?", p.TenantID, in.KnowledgeID).Delete(&knowledgeAppliedLabel{}).Error; e != nil {
			return e
		}
		for _, label := range in.Labels {
			if e = tx.Create(&knowledgeAppliedLabel{p.TenantID, in.KnowledgeID, label, row.ID, contentDigest}).Error; e != nil {
				return e
			}
		}
		out = row.domain()
		return nil
	})
	return out, err
}
func knowledgeLabelContent(tx *gorm.DB, tenant int64, id string) (string, error) {
	var chunks []ChunkModel
	if e := tx.Where("tenant_id=? AND knowledge_id=? AND is_enabled=1 AND deleted_at IS NULL", tenant, id).Order("chunk_index,id").Limit(10001).Find(&chunks).Error; e != nil {
		return "", e
	}
	if len(chunks) > 10000 {
		return "", d.ErrInvalid
	}
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(c.Content)
		b.WriteByte('\n')
		if b.Len() > 800000 {
			return "", d.ErrInvalid
		}
	}
	return b.String(), nil
}
func (r *knowledgeLabelRepository) active(ctx context.Context, tenant int64, ids []string) ([]knowledgeAppliedLabel, error) {
	if tenant <= 0 || len(ids) > 1000 {
		return nil, d.ErrInvalid
	}
	if len(ids) == 0 {
		return []knowledgeAppliedLabel{}, nil
	}
	var rows []knowledgeAppliedLabel
	// IDs are opaque bytes, not linguistic text. Legacy tables explicitly use
	// unicode_ci while newer migrated tables may inherit the operator's 0900
	// default; an unqualified text join fails before reading even an empty set.
	e := r.db.WithContext(ctx).Table("knowledge_applied_labels a").Select("a.*").Joins("JOIN knowledge_label_applications r ON BINARY r.id=BINARY a.application_id AND r.tenant_id=a.tenant_id").Joins("JOIN governance_records g ON BINARY g.id=BINARY r.result_id AND g.tenant_id=a.tenant_id AND g.version=r.review_version AND g.status='approved'").Joins("JOIN knowledge k ON BINARY k.id=BINARY a.knowledge_id AND k.tenant_id=a.tenant_id AND k.deleted_at IS NULL AND k.enable_status='enabled'").Joins("JOIN knowledge_bases b ON BINARY b.id=BINARY k.knowledge_base_id AND b.tenant_id=a.tenant_id AND b.deleted_at IS NULL AND b.status=1").Where("a.tenant_id=? AND a.knowledge_id IN ?", tenant, ids).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	valid := map[string]bool{}
	for _, row := range rows {
		if _, seen := valid[row.KnowledgeID]; !seen {
			content, err := knowledgeLabelContent(r.db.WithContext(ctx), tenant, row.KnowledgeID)
			if err != nil {
				return nil, err
			}
			valid[row.KnowledgeID] = d.LabelContentDigest(content) == row.ContentDigest
		}
	}
	out := []knowledgeAppliedLabel{}
	for _, row := range rows {
		if valid[row.KnowledgeID] {
			out = append(out, row)
		}
	}
	return out, nil
}
func (r *knowledgeLabelRepository) Labels(ctx context.Context, p l.Principal, id string) ([]string, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var count int64
	if e := r.db.WithContext(ctx).Table("knowledge k").Joins("JOIN knowledge_bases b ON BINARY b.id=BINARY k.knowledge_base_id AND b.tenant_id=k.tenant_id").Where("k.tenant_id=? AND k.id=? AND k.deleted_at IS NULL AND k.enable_status='enabled' AND b.deleted_at IS NULL AND b.status=1 AND (b.is_public=1 OR b.user_id=?)", p.TenantID, id, p.UserID).Count(&count).Error; e != nil {
		return nil, e
	}
	if count != 1 {
		return nil, d.ErrNotFound
	}
	rows, e := r.active(ctx, p.TenantID, []string{id})
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, v := range rows {
		out = append(out, v.Label)
	}
	sort.Strings(out)
	return out, nil
}
func (r *knowledgeLabelRepository) FilterKnowledgeLabels(ctx context.Context, tenant int64, ids, labels []string) (map[string]bool, error) {
	if len(labels) == 0 || len(labels) > 30 {
		return nil, d.ErrInvalid
	}
	rows, e := r.active(ctx, tenant, ids)
	if e != nil {
		return nil, e
	}
	matched := map[string]map[string]bool{}
	for _, row := range rows {
		if matched[row.KnowledgeID] == nil {
			matched[row.KnowledgeID] = map[string]bool{}
		}
		matched[row.KnowledgeID][row.Label] = true
	}
	out := map[string]bool{}
	for id, have := range matched {
		yes := true
		for _, label := range labels {
			yes = yes && have[label]
		}
		if yes {
			out[id] = true
		}
	}
	return out, nil
}
