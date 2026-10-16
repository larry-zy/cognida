package evaluation

import (
	d "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ReviewedAnnotationReader interface {
	Get(context.Context, l.Principal, string, string) (*g.Record, error)
}
type VersionAssets interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
}
type ProfileAuthorizer func(context.Context, l.Principal, g.EvaluationProfile) error
type VersionService struct {
	Repo             d.DatasetVersionRepository
	Annotations      ReviewedAnnotationReader
	Assets           VersionAssets
	AuthorizeProfile ProfileAuthorizer
	LegacyAuthorizer func(context.Context, l.Principal, string) error
}

func NewVersionService(repo d.DatasetVersionRepository, annotations ReviewedAnnotationReader, assets VersionAssets, authorize ProfileAuthorizer) *VersionService {
	return &VersionService{Repo: repo, Annotations: annotations, Assets: assets, AuthorizeProfile: authorize}
}
func (v *VersionService) Publish(ctx context.Context, p l.Principal, in d.PublishDatasetVersion, key string) (*d.DatasetVersion, error) {
	if p.Validate() != nil {
		return nil, d.ErrVersionForbidden
	}
	if len(in.Samples) < 1 || len(in.Samples) > 1000 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 128 || in.DatasetID == "" || len(in.DatasetID) > 100 || key == "" || len(key) > 100 || v.Annotations == nil || v.Assets == nil {
		return nil, d.ErrVersionInvalid
	}
	out := &d.DatasetVersion{DatasetID: in.DatasetID, Name: in.Name, Profile: in.Profile, Split: in.Split, Samples: []d.ReviewedSample{}, QAPairs: []d.QAPair{}, SourceRefs: []string{}}
	revisions := map[string]int{}
	sources := map[string]bool{}
	for _, selected := range in.Samples {
		if selected.ExpectedVersion < 1 || revisions[selected.ItemID] != 0 {
			return nil, d.ErrVersionInvalid
		}
		taskRow, e := v.Annotations.Get(ctx, p, "annotation_task", selected.TaskID)
		if e != nil {
			return nil, d.ErrVersionForbidden
		}
		itemRow, e := v.Annotations.Get(ctx, p, "annotation_item", selected.ItemID)
		if e != nil {
			return nil, d.ErrVersionForbidden
		}
		if itemRow.ParentID != taskRow.ID || itemRow.Status != "approved" || itemRow.Version != selected.ExpectedVersion {
			return nil, d.ErrVersionConflict
		}
		var task g.AnnotationTask
		var item g.AnnotationItem
		if json.Unmarshal(taskRow.Payload, &task) != nil || json.Unmarshal(itemRow.Payload, &item) != nil || item.ReviewStatus != "approved" || item.ReviewedBy <= 0 {
			return nil, d.ErrVersionInvalid
		}
		ref, e := l.ParseRefID(task.SourceRefID)
		if e != nil {
			return nil, d.ErrVersionInvalid
		}
		source, _, e := v.Assets.Resolve(ctx, p.TenantID, p.UserID, ref)
		if e != nil {
			return nil, d.ErrVersionForbidden
		}
		sample := d.ReviewedSample{ID: itemRow.ID, TaskID: taskRow.ID, AnnotationType: task.Type, ReviewVersion: itemRow.Version, ReviewedBy: item.ReviewedBy, Content: item.Content, Annotation: item.Annotation, LabelSchema: task.Labels, SourceRef: task.SourceRefID, SourceDigest: source.Provenance.Digest, Group: task.SourceRefID, SourceTime: source.CreatedAt}
		qa, e := v.sampleProfile(ctx, p, in.Profile, sample)
		if e != nil {
			return nil, e
		}
		if qa != nil {
			out.QAPairs = append(out.QAPairs, *qa)
		}
		raw, _ := json.Marshal(sample)
		sample.SampleHash = g.LabelContentDigest(string(raw))
		out.Samples = append(out.Samples, sample)
		revisions[itemRow.ID] = itemRow.Version
		sources[task.SourceRefID] = true
		if item.Annotation.EvaluationProfile != nil {
			for _, id := range item.Annotation.EvaluationProfile.SourceRefs {
				sources[id] = true
			}
		}
	}
	sort.Slice(out.Samples, func(i, j int) bool { return out.Samples[i].ID < out.Samples[j].ID })
	out.QAPairs = nil
	for _, sample := range out.Samples {
		qa, e := v.sampleProfile(ctx, p, in.Profile, sample)
		if e != nil {
			return nil, e
		}
		if qa != nil {
			out.QAPairs = append(out.QAPairs, *qa)
		}
	}
	for id := range sources {
		out.SourceRefs = append(out.SourceRefs, id)
	}
	sort.Strings(out.SourceRefs)
	if e := assignDatasetSplits(out); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(out)
	out.ManifestDigest = g.LabelContentDigest(string(raw))
	return v.Repo.Put(ctx, p, key, out, revisions)
}
func (v *VersionService) sampleProfile(ctx context.Context, p l.Principal, profile string, s d.ReviewedSample) (*d.QAPair, error) {
	switch profile {
	case "classification", "named_entity", "relation":
		if profile != s.AnnotationType || s.Annotation.QA != nil || s.Annotation.EvaluationProfile != nil {
			return nil, d.ErrVersionInvalid
		}
		return nil, nil
	case "qa", "rag", "agent", "sql":
	default:
		return nil, d.ErrVersionInvalid
	}
	if s.AnnotationType != "qa" || s.Annotation.QA == nil || strings.TrimSpace(s.Annotation.QA.Question) == "" || strings.TrimSpace(s.Annotation.QA.ReferenceAnswer) == "" {
		return nil, d.ErrVersionInvalid
	}
	qa := &d.QAPair{Question: s.Annotation.QA.Question, ReferenceAnswer: s.Annotation.QA.ReferenceAnswer}
	mapping := s.Annotation.EvaluationProfile
	if profile == "qa" {
		if mapping != nil && mapping.Kind != "qa" {
			return nil, d.ErrVersionInvalid
		}
		return qa, nil
	}
	if mapping == nil || mapping.Kind != profile {
		return nil, d.ErrVersionInvalid
	}
	switch profile {
	case "rag":
		if len(mapping.SourceRefs) == 0 || len(mapping.RelevantChunkIDs) == 0 {
			return nil, d.ErrVersionInvalid
		}
		qa.RelevantPIDs = append([]string{}, mapping.RelevantChunkIDs...)
	case "agent":
		if len(mapping.ExpectedTools) == 0 || len(mapping.ExpectedSteps) == 0 {
			return nil, d.ErrVersionInvalid
		}
		qa.ExpectedTools = append([]string{}, mapping.ExpectedTools...)
		qa.ExpectedSteps = append([]string{}, mapping.ExpectedSteps...)
	case "sql":
		if mapping.DatasourceID == "" || mapping.Dialect == "" || validateReadOnlySQL(mapping.GoldSQL) != nil {
			return nil, d.ErrVersionInvalid
		}
		first := strings.ToUpper(strings.Fields(mapping.GoldSQL)[0])
		if first != "SELECT" && first != "WITH" {
			return nil, d.ErrVersionInvalid
		}
		qa.GoldSQL = mapping.GoldSQL
	}
	for _, id := range mapping.SourceRefs {
		ref, e := l.ParseRefID(id)
		if e != nil {
			return nil, d.ErrVersionInvalid
		}
		if _, _, e = v.Assets.Resolve(ctx, p.TenantID, p.UserID, ref); e != nil {
			if errors.Is(e, l.ErrForbidden) || errors.Is(e, l.ErrNotFound) || errors.Is(e, l.ErrExpired) {
				return nil, d.ErrVersionForbidden
			}
			return nil, e
		}
	}
	if v.AuthorizeProfile == nil {
		return nil, d.ErrVersionForbidden
	}
	if e := v.AuthorizeProfile(ctx, p, *mapping); e != nil {
		return nil, d.ErrVersionForbidden
	}
	return qa, nil
}
func (v *VersionService) Read(ctx context.Context, p l.Principal, id string) (*d.DatasetVersion, error) {
	out, e := v.Repo.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if out.LegacySource != "" {
		if v.LegacyAuthorizer == nil {
			return nil, d.ErrVersionForbidden
		}
		if err := v.LegacyAuthorizer(ctx, p, out.LegacySource); err != nil {
			return nil, err
		}
	}
	if v.Assets == nil && len(out.SourceRefs) > 0 {
		return nil, d.ErrVersionForbidden
	}
	for _, source := range out.SourceRefs {
		ref, e := l.ParseRefID(source)
		if e != nil {
			return nil, d.ErrVersionInvalid
		}
		if _, _, e = v.Assets.Resolve(ctx, p.TenantID, p.UserID, ref); e != nil {
			if errors.Is(e, l.ErrForbidden) || errors.Is(e, l.ErrNotFound) || errors.Is(e, l.ErrExpired) {
				return nil, d.ErrVersionForbidden
			}
			return nil, e
		}
	}
	for _, sample := range out.Samples {
		if _, e = v.sampleProfile(ctx, p, out.Profile, sample); e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (v *VersionService) List(ctx context.Context, p l.Principal, dataset, cursor string, limit int) (*d.VersionPage, error) {
	page, e := v.Repo.List(ctx, p, dataset, cursor, limit)
	if e != nil {
		return nil, e
	}
	visible := make([]d.DatasetVersion, 0, len(page.Items))
	for i := range page.Items {
		item, e := v.Read(ctx, p, page.Items[i].ID)
		if e != nil {
			if errors.Is(e, d.ErrVersionForbidden) || errors.Is(e, d.ErrVersionNotFound) || errors.Is(e, l.ErrForbidden) || errors.Is(e, l.ErrNotFound) || errors.Is(e, l.ErrExpired) {
				continue
			}
			return nil, e
		}
		visible = append(visible, *item)
	}
	// Preserve the repository cursor even for an entirely hidden page, so callers
	// can advance without exposing revoked manifests or getting stuck on them.
	page.Items = visible
	return page, nil
}
func versionError(message string) error { return fmt.Errorf("%w: %s", d.ErrVersionInvalid, message) }
