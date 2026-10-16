package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
)

// Derived text and feature values retain their source asset's read policy.
// Resolve is repeated on every read so membership/policy revocation is effective.
func (s *Service) authorizeRecord(ctx context.Context, p l.Principal, record *d.Record) error {
	switch record.Kind {
	case d.SFTDatasetKind, d.SFTDatasetVersionKind:
		return s.authorizeSFT(ctx, p, record)
	case d.LabelStandardKind, d.LabelStandardVersionKind:
		if record.UserID != p.UserID {
			return d.ErrNotFound
		}
		return nil
	case d.GovernedKind:
		if record.UserID != p.UserID {
			return d.ErrForbidden
		}
		return nil
	case "suggestion":
		if record.UserID != p.UserID {
			if err := s.authorize(ctx, p, "admin"); err != nil {
				return d.ErrForbidden
			}
		}
		var in d.Suggestion
		if err := json.Unmarshal(record.Payload, &in); err != nil {
			return err
		}
		if s.ValidateSource == nil {
			return d.ErrForbidden
		}
		if err := s.ValidateSource(ctx, p, in.Source); err != nil {
			return err
		}
		for _, raw := range in.EvidenceRefIDs {
			id, err := l.ParseRefID(raw)
			if err != nil {
				return d.ErrInvalid
			}
			if s.Assets == nil {
				return d.ErrForbidden
			}
			if _, _, err = s.Assets.Resolve(ctx, p.TenantID, p.UserID, id); err != nil {
				return recordSourceError(err)
			}
		}

	case "annotation_item", "tagging_result":
		parentKind := "annotation_task"
		if record.Kind == "tagging_result" {
			parentKind = "tagging_task"
		}
		parent, err := s.Repo.Get(ctx, p.TenantID, parentKind, record.ParentID)
		if err != nil {
			return err
		}
		return s.authorizeRecord(ctx, p, parent)
	case "annotation_task", "tagging_task", "feature", "feature_version":
		if record.ParentID != "" && (record.Kind == "annotation_task" || record.Kind == "tagging_task") {
			op, err := s.Repo.Get(ctx, p.TenantID, d.GovernedKind, record.ParentID)
			if err != nil {
				return recordSourceError(err)
			}
			if op.UserID != p.UserID {
				return d.ErrForbidden
			}
		}
		var source struct {
			SourceRefID string `json:"source_ref_id"`
		}
		if err := json.Unmarshal(record.Payload, &source); err != nil {
			return err
		}
		id, err := l.ParseRefID(source.SourceRefID)
		if err != nil {
			return d.ErrInvalid
		}
		if s.Assets == nil {
			return d.ErrForbidden
		}
		if _, _, err = s.Assets.Resolve(ctx, p.TenantID, p.UserID, id); err != nil {
			return recordSourceError(err)
		}
	}
	return nil
}

func recordSourceError(err error) error {
	if errors.Is(err, l.ErrForbidden) || errors.Is(err, l.ErrNotFound) || errors.Is(err, l.ErrExpired) || errors.Is(err, d.ErrForbidden) || errors.Is(err, d.ErrNotFound) {
		return d.ErrForbidden
	}
	return err
}
