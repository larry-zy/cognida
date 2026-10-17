package governance

import (
	"bytes"
	d "cognida/internal/model/governance"
	"cognida/internal/model/job"
	l "cognida/internal/model/lifecycle"
	lifecycle "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type AssetService interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
	ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
}
type SourceValidator func(context.Context, l.Principal, d.SuggestionSource) error
type ApplySuggestion func(context.Context, l.Principal, d.Suggestion) (string, error)
type Service struct {
	labelSuggester   LabelSuggester
	knowledgeLabels  d.KnowledgeLabelRepository
	featureEmbedding *featureEmbeddingProvider
	driftRemediation DriftRemediation
	governedInputs   GovernedInputValidator
	Repo             d.Repository
	Assets           AssetService
	Compute          lifecycle.Compute
	Queue            job.Queue
	Authorize        lifecycle.Authorizer
	DiscoverSource   SuggestionDiscovery
	ValidateSource   SourceValidator
	Apply            ApplySuggestion
}

func NewService(repo d.Repository, assets AssetService, compute lifecycle.Compute, queue job.Queue, authorize lifecycle.Authorizer) *Service {
	return &Service{Repo: repo, Assets: assets, Compute: compute, Queue: queue, Authorize: authorize}
}
func (s *Service) authorize(ctx context.Context, p l.Principal, scope string) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	if s.Authorize == nil {
		return d.ErrForbidden
	}
	if err := s.Authorize(ctx, p, scope); err != nil {
		return d.ErrForbidden
	}
	return nil
}
func encode(v any) (json.RawMessage, error) { return json.Marshal(v) }
func (s *Service) create(ctx context.Context, p l.Principal, kind, parent, status, key string, value any, due *time.Time) (*d.Record, error) {
	if key == "" {
		key = uuid.NewString()
	}
	if err := s.authorize(ctx, p, "read"); err != nil {
		return nil, err
	}
	if len(key) > 100 {
		return nil, d.ErrInvalid
	}
	payload, e := encode(value)
	if e != nil {
		return nil, e
	}
	if len(payload) > 8<<20 {
		return nil, d.ErrInvalid
	}
	record, err := s.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: p.TenantID, UserID: p.UserID}, Kind: kind, ParentID: parent, Status: status, IdempotencyKey: fmt.Sprintf("%d:%s", p.UserID, key), Payload: payload, DueAt: due})
	if err != nil {
		return nil, err
	}
	if err = s.authorizeRecord(ctx, p, record); err != nil {
		return nil, err
	}
	return record, nil
}
func (s *Service) Get(ctx context.Context, p l.Principal, kind, id string) (*d.Record, error) {
	if err := s.authorize(ctx, p, "read"); err != nil {
		return nil, err
	}
	v, err := s.Repo.Get(ctx, p.TenantID, kind, id)
	if err != nil {
		return nil, err
	}
	if (kind == "notification" || kind == "subscription" || kind == "subscription_run") && v.UserID != p.UserID {
		return nil, d.ErrNotFound
	}
	if err := s.authorizeRecord(ctx, p, v); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, p l.Principal, f d.Filter) (*d.Page[d.Record], error) {
	if err := s.authorize(ctx, p, "read"); err != nil {
		return nil, err
	}
	if f.Kind == "notification" || f.Kind == "subscription" || f.Kind == "subscription_run" {
		f.UserID = p.UserID
	}
	if f.Kind == d.LabelStandardKind || f.Kind == d.LabelStandardVersionKind || f.Kind == d.SFTDatasetKind || f.Kind == d.SFTDatasetVersionKind {
		f.UserID = p.UserID
	}
	if f.Kind == "annotation_task" || f.Kind == "tagging_task" {
		f.GovernedOwnerID = p.UserID
	}
	if f.Kind == "suggestion" && s.authorize(ctx, p, "admin") != nil {
		f.UserID = p.UserID
	}
	page, err := s.Repo.List(ctx, p.TenantID, f)
	if err != nil {
		return nil, err
	}
	visible := make([]d.Record, 0, len(page.Items))
	for i := range page.Items {
		if err = s.authorizeRecord(ctx, p, &page.Items[i]); err != nil {
			if errors.Is(err, d.ErrForbidden) || errors.Is(err, d.ErrNotFound) || errors.Is(err, l.ErrForbidden) || errors.Is(err, l.ErrNotFound) || errors.Is(err, l.ErrExpired) {
				continue
			}
			return nil, err
		}
		visible = append(visible, page.Items[i])
	}
	// Pagination advances over the scanned records, including hidden ones.
	page.Items = visible
	return page, nil
}
func View(record *d.Record) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(record.Payload, &out); err != nil {
		return nil, err
	}
	if record.Kind == d.SFTDatasetKind || record.Kind == d.SFTDatasetVersionKind {
		delete(out, "rows")
		delete(out, "rows_exact")
	}
	if record.Kind == "feature" || record.Kind == "feature_version" {
		if exact, ok := out["definition_exact"].(string); ok && exact != "" {
			var definition any
			decoder := json.NewDecoder(strings.NewReader(exact))
			decoder.UseNumber()
			if err := decoder.Decode(&definition); err != nil {
				return nil, err
			}
			out["definition"] = definition
		}
		delete(out, "definition_exact")
	}
	meta, _ := json.Marshal(record.Meta)
	var fields map[string]any
	_ = json.Unmarshal(meta, &fields)
	for k, v := range fields {
		if k == "version" && record.Kind == "feature_version" {
			continue
		}
		out[k] = v
	}
	out["status"] = record.Status
	return out, nil
}
func (s *Service) update(ctx context.Context, record *d.Record, value any, status string, expected int) error {
	if expected != record.Version {
		return d.ErrConflict
	}
	payload, err := encode(value)
	if err != nil {
		return err
	}
	if len(payload) > 8<<20 {
		return d.ErrInvalid
	}
	record.Payload = payload
	record.Status = status
	return s.Repo.Update(ctx, record, expected)
}
func (s *Service) input(ctx context.Context, p l.Principal, id string, published bool) (l.DataAssetRef, l.Input, error) {
	refID, err := l.ParseRefID(id)
	if err != nil {
		return l.DataAssetRef{}, l.Input{}, d.ErrInvalid
	}
	ref, payload, err := s.Assets.Resolve(ctx, p.TenantID, p.UserID, refID)
	if err != nil {
		return ref, l.Input{}, err
	}
	if published && (ref.Kind != l.PublishedAsset || !ref.Policy.Approved) {
		return ref, l.Input{}, fmt.Errorf("%w: published asset required", d.ErrInvalid)
	}
	var input l.Input
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err = decoder.Decode(&input); err != nil {
		return ref, input, err
	}
	if err = input.Validate(); err != nil {
		return ref, input, err
	}
	return ref, input, nil
}
func texts(input l.Input) ([]string, error) {
	if input.Kind == "text" {
		return []string{input.Text}, nil
	}
	if len(input.Rows) > 1000 {
		return nil, d.ErrInvalid
	}
	out := []string{}
	for _, row := range input.Rows {
		parts := []string{}
		for i, v := range row {
			if v != nil {
				parts = append(parts, input.Columns[i]+": "+fmt.Sprint(v))
			}
		}
		text := strings.Join(parts, "\n")
		if strings.TrimSpace(text) == "" || len([]rune(text)) > 200000 {
			return nil, d.ErrInvalid
		}
		out = append(out, text)
	}
	return out, nil
}
