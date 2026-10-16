package evaluation

import (
	d "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type versionMemory struct {
	d.DatasetVersionRepository
	v   *d.DatasetVersion
	key string
}

func (r *versionMemory) Put(_ context.Context, p l.Principal, key string, v *d.DatasetVersion, _ map[string]int) (*d.DatasetVersion, error) {
	if r.v != nil && r.key == key {
		if r.v.ManifestDigest != v.ManifestDigest {
			return nil, d.ErrVersionConflict
		}
		return r.v, nil
	}
	copy := *v
	copy.ID = uuid.NewString()
	copy.TenantID = p.TenantID
	copy.UserID = p.UserID
	copy.Version = 1
	r.v = &copy
	r.key = key
	return &copy, nil
}
func (r *versionMemory) Get(_ context.Context, p l.Principal, id string) (*d.DatasetVersion, error) {
	if r.v == nil || r.v.ID != id || r.v.TenantID != p.TenantID || r.v.UserID != p.UserID {
		return nil, d.ErrVersionNotFound
	}
	return r.v, nil
}

type versionInputs struct {
	rows map[string]*g.Record
	ref  l.DataAssetRef
	deny bool
}

func (a *versionInputs) Get(_ context.Context, p l.Principal, kind, id string) (*g.Record, error) {
	r := a.rows[id]
	if r == nil || p.TenantID != r.TenantID || r.Kind != kind {
		return nil, g.ErrNotFound
	}
	return r, nil
}
func (a *versionInputs) Resolve(_ context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if a.deny || tenant != a.ref.TenantID || id != a.ref.RefID {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	return a.ref, json.RawMessage(`{"kind":"text","text":"proof"}`), nil
}
func versionFixture(t *testing.T) (*VersionService, *versionMemory, *versionInputs, l.Principal, d.PublishDatasetVersion) {
	t.Helper()
	p := l.Principal{TenantID: 2, UserID: 3}
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	a := &versionInputs{rows: map[string]*g.Record{}, ref: l.DataAssetRef{RefID: ref, AssetRefID: ref.String(), TenantID: 2, CreatedAt: time.Now(), Provenance: l.Provenance{Digest: "digest"}}}
	task := g.AnnotationTask{Type: "qa", SourceRefID: ref.String()}
	item := g.AnnotationItem{Content: "source", Annotation: g.Annotation{QA: &g.QAPair{Question: "Question", ReferenceAnswer: "Answer"}}, ReviewStatus: "approved", ReviewedBy: 3}
	tr, _ := json.Marshal(task)
	ir, _ := json.Marshal(item)
	a.rows["task"] = &g.Record{Meta: g.Meta{ID: "task", TenantID: 2}, Kind: "annotation_task", Payload: tr}
	a.rows["item"] = &g.Record{Meta: g.Meta{ID: "item", TenantID: 2, Version: 3}, Kind: "annotation_item", ParentID: "task", Status: "approved", Payload: ir}
	r := &versionMemory{}
	v := NewVersionService(r, a, a, func(context.Context, l.Principal, g.EvaluationProfile) error { return nil })
	return v, r, a, p, d.PublishDatasetVersion{DatasetID: "reviewed", Name: "Reviewed", Profile: "qa", Samples: []d.SampleSelection{{TaskID: "task", ItemID: "item", ExpectedVersion: 3}}}
}
func TestDatasetVersionFreezesReviewedSamplesAndRechecksSources(t *testing.T) {
	v, r, a, p, in := versionFixture(t)
	out, e := v.Publish(context.Background(), p, in, "k")
	require.NoError(t, e)
	require.Len(t, out.QAPairs, 1)
	require.Equal(t, "Answer", out.QAPairs[0].ReferenceAnswer)
	replay, e := v.Publish(context.Background(), p, in, "k")
	require.NoError(t, e)
	require.Equal(t, out.ID, replay.ID)
	a.rows["item"].Status = "pending"
	_, e = v.Publish(context.Background(), p, in, "next")
	require.ErrorIs(t, e, d.ErrVersionConflict)
	old, e := v.Read(context.Background(), p, out.ID)
	require.NoError(t, e)
	require.Equal(t, "Answer", old.QAPairs[0].ReferenceAnswer)
	a.deny = true
	_, e = v.Read(context.Background(), p, out.ID)
	require.ErrorIs(t, e, d.ErrVersionForbidden)
	_, e = r.Get(context.Background(), l.Principal{TenantID: 2, UserID: 4}, out.ID)
	require.ErrorIs(t, e, d.ErrVersionNotFound)
}
func TestDatasetProfileRequiresExplicitReviewedMappings(t *testing.T) {
	v, _, _, p, _ := versionFixture(t)
	sample := d.ReviewedSample{AnnotationType: "classification", Annotation: g.Annotation{Labels: []string{"x"}}}
	_, e := v.sampleProfile(context.Background(), p, "qa", sample)
	require.Error(t, e)
	sample.AnnotationType = "qa"
	sample.Annotation = g.Annotation{QA: &g.QAPair{Question: "q", ReferenceAnswer: "a"}}
	for _, profile := range []string{"rag", "agent", "sql"} {
		_, e = v.sampleProfile(context.Background(), p, profile, sample)
		require.Error(t, e)
	}
	sample.Annotation.EvaluationProfile = &g.EvaluationProfile{Kind: "sql", DatasourceID: "d", Dialect: "mysql", GoldSQL: "DELETE FROM users"}
	_, e = v.sampleProfile(context.Background(), p, "sql", sample)
	require.Error(t, e)
	sample.Annotation.EvaluationProfile.GoldSQL = "SELECT 1"
	qa, e := v.sampleProfile(context.Background(), p, "sql", sample)
	require.NoError(t, e)
	require.Equal(t, "SELECT 1", qa.GoldSQL)
	v.AuthorizeProfile = func(context.Context, l.Principal, g.EvaluationProfile) error { return errors.New("denied") }
	_, e = v.sampleProfile(context.Background(), p, "sql", sample)
	require.ErrorIs(t, e, d.ErrVersionForbidden)
}
func TestDatasetSplitRejectsCrossBoundaryLeakage(t *testing.T) {
	a := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	b := a.AddDate(0, 1, 0)
	v := d.DatasetVersion{Split: d.DatasetSplit{Algorithm: "source_time_v1", TrainBefore: &a, ValidationBefore: &b}, Samples: []d.ReviewedSample{{Content: "a copied sample with the same answer", SourceTime: a.Add(-time.Hour), Group: "a"}, {Content: "a copied sample with the same answer!", SourceTime: b.Add(time.Hour), Group: "b"}}}
	require.Error(t, assignDatasetSplits(&v))
	v.Samples[1].Content = "completely unrelated material and distinct vocabulary"
	require.NoError(t, assignDatasetSplits(&v))
	require.Equal(t, "train", v.Samples[0].Split)
	require.Equal(t, "test", v.Samples[1].Split)
	v.Split = d.DatasetSplit{Algorithm: "source_hash_v1", Seed: "fixed", TrainPercent: 60, ValidationPercent: 20}
	v.Samples[1].Group = "a"
	require.NoError(t, assignDatasetSplits(&v))
	require.Equal(t, v.Samples[0].Split, v.Samples[1].Split)
}
func TestEvaluationSnapshotPinsTargetAndRejectsConfigDrift(t *testing.T) {
	v, _, assets, p, in := versionFixture(t)
	out, e := v.Publish(context.Background(), p, in, "k")
	require.NoError(t, e)
	target := `{"model":"synthetic-v1"}`
	s := NewSnapshotService(v, nil, func(context.Context, l.Principal, *EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(target), nil
	})
	frozen, e := s.Freeze(context.Background(), p, &EvaluationTaskConfig{DatasetID: in.DatasetID, Type: EvaluationTypeQA}, out.ID)
	require.NoError(t, e)
	_, e = s.Validate(context.Background(), p, frozen)
	require.NoError(t, e)
	target = `{"model":"changed"}`
	_, e = s.Validate(context.Background(), p, frozen)
	require.ErrorIs(t, e, d.ErrVersionConflict)
	_, e = s.ValidateRead(context.Background(), p, frozen)
	require.NoError(t, e, "historical results remain readable after target configuration changes")
	assets.deny = true
	_, e = s.ValidateRead(context.Background(), p, frozen)
	require.Error(t, e, "read still checks current source authorization")
}

func TestLegacySnapshotFilePathAndRemovalGuard(t *testing.T) {
	root := t.TempDir()
	old := DatasetDir
	DatasetDir = root
	t.Cleanup(func() { DatasetDir = old })
	require.NoError(t, os.Mkdir(filepath.Join(root, "packaged"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "packaged", MetaFileName), []byte(`{"id":"packaged","name":"QA","evaluation_type":"qa","qa_count":1}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "packaged", SamplesFileName), []byte("{\"question\":\"q\",\"reference_answer\":\"a\"}\n"), 0600))
	v, r, _, p, _ := versionFixture(t)
	snapshotter := NewSnapshotService(v, NewDatasetLoader(nil, nil), func(context.Context, l.Principal, *EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(`{"model":"fixed"}`), nil
	})
	frozen, e := snapshotter.Freeze(context.Background(), p, &EvaluationTaskConfig{DatasetID: "packaged", Type: EvaluationTypeQA}, "")
	require.NoError(t, e)
	require.Equal(t, "file:packaged", r.v.LegacySource)
	require.Error(t, snapshotter.authorizeLegacy(context.Background(), p, "../escape"))
	require.NoError(t, os.Remove(filepath.Join(root, "packaged", SamplesFileName)))
	_, e = snapshotter.Validate(context.Background(), p, frozen)
	require.ErrorIs(t, e, d.ErrVersionForbidden)
}

type versionListRepo struct {
	d.DatasetVersionRepository
	page d.VersionPage
	rows map[string]*d.DatasetVersion
	err  error
}

func (r *versionListRepo) List(context.Context, l.Principal, string, string, int) (*d.VersionPage, error) {
	out := r.page
	return &out, nil
}
func (r *versionListRepo) Get(_ context.Context, _ l.Principal, id string) (*d.DatasetVersion, error) {
	if r.err != nil {
		return nil, r.err
	}
	if row := r.rows[id]; row != nil {
		return row, nil
	}
	return nil, d.ErrVersionNotFound
}

type versionReadAssets struct{ err error }

func (a versionReadAssets) Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	return l.DataAssetRef{}, nil, a.err
}
func TestDatasetVersionListSkipsRevokedAndMissingWithoutLosingCursor(t *testing.T) {
	p := l.Principal{TenantID: 1, UserID: 2}
	revoked := d.DatasetVersion{ID: "revoked", SourceRefs: []string{l.RefID{AssetID: uuid.NewString(), Version: 1}.String()}}
	valid := d.DatasetVersion{ID: "valid", Profile: "qa", Name: "visible"}
	r := &versionListRepo{page: d.VersionPage{Items: []d.DatasetVersion{revoked, {ID: "gone"}, valid}, NextCursor: "scanned-cursor", HasMore: true}, rows: map[string]*d.DatasetVersion{"revoked": &revoked, "valid": &valid}}
	v := NewVersionService(r, nil, versionReadAssets{l.ErrForbidden}, nil)
	page, err := v.List(context.Background(), p, "", "", 3)
	require.NoError(t, err)
	require.Equal(t, []d.DatasetVersion{valid}, page.Items)
	require.Equal(t, "scanned-cursor", page.NextCursor)
	require.True(t, page.HasMore)
	v.Assets = versionReadAssets{l.ErrExpired}
	page, err = v.List(context.Background(), p, "", "", 3)
	require.NoError(t, err)
	require.Equal(t, []d.DatasetVersion{valid}, page.Items)
	r.page.Items = []d.DatasetVersion{revoked}
	page, err = v.List(context.Background(), p, "", "", 1)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.Equal(t, "scanned-cursor", page.NextCursor)
	require.True(t, page.HasMore)
	outage := errors.New("asset backend unavailable")
	v.Assets = versionReadAssets{outage}
	_, err = v.List(context.Background(), p, "", "", 1)
	require.ErrorIs(t, err, outage)
	r.err = errors.New("database unavailable")
	_, err = v.List(context.Background(), p, "", "", 1)
	require.ErrorIs(t, err, r.err)
}
