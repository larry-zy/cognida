package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
)

func (s *Service) previewFeature(ctx context.Context, p l.Principal, f d.Feature) (*d.FeaturePreview, l.DataAssetRef, error) {
	if err := s.validateFeature(ctx, p, f); err != nil {
		return nil, l.DataAssetRef{}, err
	}
	ref, input, err := s.input(ctx, p, f.SourceRefID, false)
	if err != nil {
		return nil, ref, err
	}
	values, warnings, err := s.computeFeature(ctx, f, ref, input)
	if err != nil {
		return nil, ref, err
	}
	def, err := parseFeatureDefinition(f.Definition)
	if err != nil {
		return nil, ref, err
	}
	raw, _ := json.Marshal(def)
	if len(ref.Provenance.Digest) != 64 {
		return nil, ref, featureErr("missing_source_digest", "source must have a verified immutable content digest")
	}
	return &d.FeaturePreview{Values: values, TotalCount: len(values), Warnings: warnings, DefinitionDigest: featureHash(raw), SourceDigest: ref.Provenance.Digest}, ref, nil
}

// PreviewFeature computes the entire bounded input before limiting display rows;
// an error after the displayed prefix is never hidden. It performs no writes.
func (s *Service) PreviewFeature(ctx context.Context, p l.Principal, f d.Feature, limit int) (*d.FeaturePreview, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, featureErr("preview_limit", "preview limit must be 1..100")
	}
	preview, _, err := s.previewFeature(ctx, p, f)
	if err != nil {
		return nil, err
	}
	if len(preview.Values) > limit {
		preview.Values = preview.Values[:limit]
	}
	return preview, nil
}
func materializationRepository(repo d.Repository) (d.FeatureMaterializationRepository, error) {
	r, ok := repo.(d.FeatureMaterializationRepository)
	if !ok {
		return nil, fmt.Errorf("feature materialization repository is unavailable")
	}
	return r, nil
}

// LookupFeatureMaterialization checks the owner/key only, allowing cancellation
// reconciliation after source revocation. Callers must not expose its output
// without separately resolving current source and output permissions.
func (s *Service) LookupFeatureMaterialization(ctx context.Context, p l.Principal, key string) (*d.FeatureMaterialization, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	r, err := materializationRepository(s.Repo)
	if err != nil {
		return nil, err
	}
	receipt, err := r.FeatureReceipt(ctx, p.TenantID, p.UserID, key)
	if errors.Is(err, d.ErrNotFound) {
		return nil, nil
	}
	return receipt, err
}
func (s *Service) MaterializeFeature(ctx context.Context, p l.Principal, id string, version int, key string) (*d.FeatureMaterialization, error) {
	if strings.HasPrefix(key, "governed:") {
		return nil, d.ErrInvalid
	}
	return s.materializeFeature(ctx, p, id, version, key, "")
}

// MaterializeGovernedFeature is an internal trusted entrypoint. The repository
// verifies the persisted operation binding and cancellation fence atomically.
func (s *Service) MaterializeGovernedFeature(ctx context.Context, p l.Principal, id string, version int, operationID string) (*d.FeatureMaterialization, error) {
	if _, e := uuid.Parse(operationID); e != nil {
		return nil, d.ErrInvalid
	}
	return s.materializeFeature(ctx, p, id, version, "governed:"+operationID, operationID)
}
func (s *Service) materializeFeature(ctx context.Context, p l.Principal, id string, version int, key, operationID string) (*d.FeatureMaterialization, error) {
	if !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	f, err := s.featureVersion(ctx, p, id, version)
	if err != nil {
		return nil, err
	}
	def, err := parseFeatureDefinition(f.Definition)
	if err != nil {
		return nil, err
	}
	if def.Kind == "embedding" {
		return nil, featureErr("durable_job_required", "embedding materialization requires a durable feature job; unconfirmed paid calls are not replayed")
	}
	preview, ref, err := s.previewFeature(ctx, p, f)
	if err != nil {
		return nil, err
	}
	existing, err := s.LookupFeatureMaterialization(ctx, p, key)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.FeatureID != id || existing.Version != version || existing.SourceRefID != f.SourceRefID || existing.DefinitionDigest != preview.DefinitionDigest || existing.SourceDigest != preview.SourceDigest {
			return nil, d.ErrConflict
		}
		out, err := l.ParseRefID(existing.OutputRefID)
		if err != nil {
			return nil, d.ErrConflict
		}
		if _, _, err = s.Assets.Resolve(ctx, p.TenantID, p.UserID, out); err != nil {
			return nil, err
		}
		return existing, nil
	}
	r, err := materializationRepository(s.Repo)
	if err != nil {
		return nil, err
	}
	receiptID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("feature-materialization:%d:%d:%s", p.TenantID, p.UserID, key))).String()
	body, err := json.Marshal(map[string]any{"schema_version": 1, "kind": "feature_values", "feature_id": id, "version": version, "definition": json.RawMessage(f.Definition), "definition_digest": preview.DefinitionDigest, "source_ref_id": f.SourceRefID, "source_digest": preview.SourceDigest, "values": preview.Values, "count": preview.TotalCount, "warnings": preview.Warnings, "numeric_semantics": "decimal arithmetic; non-terminating division rounded to 18 fractional digits, half away from zero", "window_semantics": "per entity (event_time-window_seconds,event_time]"})
	if err != nil {
		return nil, err
	}
	output, err := s.Assets.ImportDerived(ctx, p, l.Result, "feature-materialization:"+receiptID, []l.RefID{ref.RefID}, body, ref.Policy)
	if err != nil {
		return nil, err
	}
	if _, _, err = s.Assets.Resolve(ctx, p.TenantID, p.UserID, ref.RefID); err != nil {
		return nil, err
	}
	receipt := d.FeatureMaterialization{ReceiptID: receiptID, FeatureID: id, Version: version, SourceRefID: f.SourceRefID, DefinitionDigest: preview.DefinitionDigest, SourceDigest: preview.SourceDigest, OutputRefID: output.RefID.String(), Count: preview.TotalCount}
	if operationID != "" {
		return r.FeatureMaterializeGoverned(ctx, p.TenantID, p.UserID, operationID, receipt, preview.Values)
	}
	return r.FeatureMaterialize(ctx, p.TenantID, p.UserID, key, receipt, preview.Values)
}

// Origin checks do not resolve the output again; Lifecycle checks its parents.
func (s *Service) ValidateFeatureMaterializationOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	if p.Validate() != nil || ref.TenantID != p.TenantID || ref.Kind != l.Result || !strings.HasPrefix(ref.ProducerID, "feature-materialization:") {
		return l.ErrForbidden
	}
	r, err := materializationRepository(s.Repo)
	if err != nil {
		return l.ErrForbidden
	}
	receipt, err := r.FeatureReceiptByID(ctx, p.TenantID, p.UserID, strings.TrimPrefix(ref.ProducerID, "feature-materialization:"))
	if err != nil || receipt.OutputRefID != ref.RefID.String() || len(ref.Parents) != 1 || ref.Parents[0].String() != receipt.SourceRefID {
		return l.ErrForbidden
	}
	return nil
}

func validFeatureKey(key string) bool {
	if len(key) < 1 || len(key) > 100 {
		return false
	}
	for i := range key {
		if key[i] < 33 || key[i] > 126 {
			return false
		}
	}
	return true
}
