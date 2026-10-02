package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxRefreshCandidates = 10000
const maxSourceAncestors = 128

func (s *Service) validateRefresh(ctx context.Context, p l.Principal, in d.AnalysisInput) error {
	if in.Refresh == nil {
		return nil
	}
	r := in.Refresh
	if r.Mode != "latest_published" || strings.TrimSpace(r.SourceID) == "" || len(r.SourceID) > 2048 {
		return d.ErrInvalid
	}
	switch r.SourceType {
	case "datasource", "web", "knowledge", "collection", "result", "evaluation":
	default:
		return d.ErrInvalid
	}
	anchor, _, err := s.input(ctx, p, in.AssetRefID, true)
	if err != nil {
		return err
	}
	match, err := s.matchesSource(ctx, p, anchor, *r)
	if err != nil {
		return err
	}
	if !match {
		return fmt.Errorf("%w: anchor does not belong to refresh source", d.ErrInvalid)
	}
	return nil
}
func (s *Service) matchesSource(ctx context.Context, p l.Principal, ref l.DataAssetRef, source d.RefreshSource) (bool, error) {
	pending := []l.DataAssetRef{ref}
	seen := map[string]bool{}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		node := pending[0]
		pending = pending[1:]
		if seen[node.RefID.String()] {
			continue
		}
		seen[node.RefID.String()] = true
		if len(seen) > maxSourceAncestors {
			return false, fmt.Errorf("%w: source ancestry exceeds %d refs", d.ErrInvalid, maxSourceAncestors)
		}
		if node.Provenance.SourceType == source.SourceType && node.Provenance.SourceID == source.SourceID {
			return true, nil
		}
		for _, parent := range node.Parents {
			if seen[parent.String()] {
				continue
			}
			ancestor, _, err := s.Assets.Resolve(ctx, p.TenantID, p.UserID, parent)
			if err != nil {
				return false, err
			}
			pending = append(pending, ancestor)
		}
	}
	return false, nil
}
func (s *Service) resolveSubscriptionInput(ctx context.Context, p l.Principal, in d.AnalysisInput) (string, error) {
	if in.Refresh == nil {
		return in.AssetRefID, nil
	}
	if err := s.validateRefresh(ctx, p, in); err != nil {
		return "", err
	}
	latest, _, err := s.input(ctx, p, in.AssetRefID, true)
	if err != nil {
		return "", err
	}
	cursor := ""
	for scanned := 0; ; {
		refs, err := s.Repo.PublishedRefs(ctx, p.TenantID, cursor, 100)
		if err != nil {
			return "", err
		}
		for _, raw := range refs {
			scanned++
			if scanned > maxRefreshCandidates {
				return "", fmt.Errorf("%w: refresh exceeds %d published candidates; narrow source history", d.ErrInvalid, maxRefreshCandidates)
			}
			id, err := l.ParseRefID(raw)
			if err != nil {
				return "", err
			}
			candidate, _, err := s.Assets.Resolve(ctx, p.TenantID, p.UserID, id)
			if errors.Is(err, l.ErrForbidden) || errors.Is(err, l.ErrExpired) || errors.Is(err, l.ErrNotFound) {
				continue
			}
			if err != nil {
				return "", err
			}
			if candidate.Kind != l.PublishedAsset || !candidate.Policy.Approved {
				continue
			}
			match, err := s.matchesSource(ctx, p, candidate, *in.Refresh)
			if errors.Is(err, l.ErrForbidden) || errors.Is(err, l.ErrExpired) || errors.Is(err, l.ErrNotFound) {
				continue
			}
			if err != nil {
				return "", err
			}
			if match && (candidate.CreatedAt.After(latest.CreatedAt) || (candidate.CreatedAt.Equal(latest.CreatedAt) && candidate.RefID.String() > latest.RefID.String())) {
				latest = candidate
			}
		}
		if len(refs) < 100 {
			return latest.RefID.String(), nil
		}
		cursor = refs[len(refs)-1]
	}
}
