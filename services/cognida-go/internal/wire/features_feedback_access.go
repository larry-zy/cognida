package wire

import (
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"context"
	"gorm.io/gorm"
	"strings"
)

type knowledgeFeedbackGuard func(context.Context, l.Principal, string, string) error

func (f *FeatureServices) knowledgeFeedbackGuard(ctx context.Context, p l.Principal, kb, id string) error {
	if f.Feedback == nil {
		return l.ErrForbidden
	}
	managed, allowed, e := f.Feedback.KnowledgeAccess(ctx, p, kb, id)
	if e != nil {
		return e
	}
	if managed && !allowed {
		return l.ErrForbidden
	}
	return nil
}
func knowledgeFeedbackImport(ctx context.Context, db *gorm.DB, p l.Principal, kb, id string, guards ...knowledgeFeedbackGuard) (string, error) {
	c, v, e := mysql.NewFeedbackRepository(db).FindKnowledge(ctx, p.TenantID, id)
	if e != nil {
		return "", e
	}
	if c == nil {
		return "", nil
	}
	if len(guards) != 1 || guards[0] == nil {
		return "", l.ErrForbidden
	}
	if e = guards[0](ctx, p, kb, id); e != nil {
		return "", e
	}
	if v != nil {
		return "feedback:" + v.ID, nil
	}
	return "feedback-baseline:" + c.ID, nil
}
func knowledgeFeedbackOrigin(ctx context.Context, db *gorm.DB, p l.Principal, kb, id, marker string, guards ...knowledgeFeedbackGuard) error {
	c, v, e := mysql.NewFeedbackRepository(db).FindKnowledge(ctx, p.TenantID, id)
	if e != nil {
		return e
	}
	if c == nil {
		if strings.HasPrefix(marker, "feedback") {
			return l.ErrForbidden
		}
		return nil
	}
	// Original baseline evidence predates registration and remains governed by
	// the original source authorization/digest, avoiding recursive read-gate checks.
	if v == nil && marker == "" {
		return nil
	}
	if v != nil && marker != "" && marker != "feedback:"+v.ID {
		return l.ErrForbidden
	}
	if v == nil && marker != "feedback-baseline:"+c.ID {
		return l.ErrForbidden
	}
	if len(guards) != 1 || guards[0] == nil {
		return l.ErrForbidden
	}
	return guards[0](ctx, p, kb, id)
}
