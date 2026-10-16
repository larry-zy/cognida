package feedback

import (
	a "cognida/internal/model/agent"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/rag"
	"context"
	"strconv"
)

func (s *Service) Prepare(ctx context.Context, tenant, kb string) error {
	if p, v, ok := d.FromContext(ctx); ok {
		if strconv.FormatInt(p.TenantID, 10) != tenant || (kb != "" && v.KnowledgeBaseID != kb) {
			return d.ErrForbidden
		}
		return s.CheckView(ctx, p, v)
	}
	return nil
}

// KnowledgeAccess also guards cached/direct document reads. Managed is true
// even when access is denied, so callers cannot fall back to a legacy read.
func (s *Service) KnowledgeAccess(ctx context.Context, p l.Principal, kb, id string) (managed, allowed bool, err error) {
	c, v, e := s.Repo.FindKnowledge(ctx, p.TenantID, id)
	if e != nil {
		return true, false, e
	}
	if c == nil {
		return false, false, nil
	}
	if c.KnowledgeBaseID != kb || c.Status == "invalidated" || c.Status == "withdrawn" {
		return true, false, nil
	}
	visible, e := s.Projection.Visible(ctx, p, kb, id, v != nil)
	if e != nil || !visible {
		return true, false, e
	}
	owner := l.Principal{TenantID: c.TenantID, UserID: c.UserID}
	if s.auth(ctx, owner, "read") != nil || s.refs(ctx, owner, c) != nil {
		return true, false, nil
	}
	if who, view, ok := d.FromContext(ctx); ok {
		if who != p || view.CaseID != c.ID || view.KnowledgeID != id {
			return true, false, nil
		}
		if e = s.CheckView(ctx, p, view); e != nil {
			return true, false, e
		}
		return true, true, nil
	}
	if v == nil {
		return true, c.ActiveVersionID == "", nil
	}
	if c.ActiveVersionID != v.ID || v.Status != "approved" {
		return true, false, nil
	}
	if _, _, _, e = s.candidate(ctx, owner, v.ID); e != nil {
		return true, false, nil
	}
	receipt, e := s.Projection.Lookup(ctx, owner, c, v)
	return true, e == nil && len(receipt) > 0 && d.Digest(receipt) == d.Digest(v.Projection), e
}
func (s *Service) Filter(ctx context.Context, tenant, kb string, docs []*rag.Document) ([]*rag.Document, error) {
	tenantID, e := strconv.ParseInt(tenant, 10, 64)
	if e != nil || tenantID < 1 {
		return nil, d.ErrForbidden
	}
	user, _ := a.GetUserID(ctx)
	p := l.Principal{TenantID: tenantID, UserID: user}
	if who, _, ok := d.FromContext(ctx); ok {
		p = who
	}
	if e = s.Prepare(ctx, tenant, kb); e != nil {
		return nil, e
	}
	out := []*rag.Document{}
	for _, doc := range docs {
		if doc == nil || doc.KnowledgeID == "" || doc.KnowledgeBaseID == "" {
			continue
		}
		if kb != "" && doc.KnowledgeBaseID != kb {
			continue
		}
		if _, view, ok := d.FromContext(ctx); ok && doc.KnowledgeID != view.KnowledgeID {
			continue
		}
		managed, allowed, e := s.KnowledgeAccess(ctx, p, doc.KnowledgeBaseID, doc.KnowledgeID)
		if e != nil {
			return nil, e
		}
		if !managed {
			allowed, e = s.Projection.Visible(ctx, p, doc.KnowledgeBaseID, doc.KnowledgeID, false)
			if e != nil {
				return nil, e
			}
		}
		if allowed && len(d.RequestedLabels(ctx)) > 0 {
			matches, err := s.FilterKnowledgeLabels(ctx, tenantID, []string{doc.KnowledgeID}, d.RequestedLabels(ctx))
			if err != nil {
				return nil, err
			}
			allowed = matches[doc.KnowledgeID]
		}
		if allowed {
			out = append(out, doc)
		}
	}
	return out, nil
}

// Applied candidate labels are read only through the same publication pointer or fixed evaluation view.
func (s *Service) FilterKnowledgeLabels(ctx context.Context, tenant int64, ids, required []string) (map[string]bool, error) {
	user, _ := a.GetUserID(ctx)
	p := l.Principal{TenantID: tenant, UserID: user}
	if who, _, ok := d.FromContext(ctx); ok {
		p = who
	}
	out := map[string]bool{}
	for _, id := range ids {
		c, v, e := s.Repo.FindKnowledge(ctx, tenant, id)
		if e != nil {
			return nil, e
		}
		var labels []string
		if c != nil {
			_, allowed, e := s.KnowledgeAccess(ctx, p, c.KnowledgeBaseID, id)
			if e != nil {
				return nil, e
			}
			if !allowed {
				continue
			}
			if v != nil {
				labels = v.Labels
			} else {
				labels = c.BaselineLabels
			}
		} else {
			labels, e = s.Projection.CurrentLabels(ctx, p, id)
			if e != nil {
				return nil, e
			}
		}
		set := map[string]bool{}
		for _, label := range labels {
			set[label] = true
		}
		matched := true
		for _, label := range required {
			matched = matched && set[label]
		}
		out[id] = matched
	}
	return out, nil
}
