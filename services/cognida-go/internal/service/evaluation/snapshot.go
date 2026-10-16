package evaluation

import (
	d "cognida/internal/model/evaluation"
	f "cognida/internal/model/feedback"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	prompts "cognida/internal/prompt"
	lifecycle "cognida/internal/service/lifecycle"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type TargetSnapshotter func(context.Context, l.Principal, *EvaluationTaskConfig) (json.RawMessage, error)
type SnapshotService struct {
	FeedbackView    func(context.Context, l.Principal, f.View) error
	Strategies      *strategies.Service
	StrategyCompute lifecycle.Compute
	Versions        *VersionService
	Loader          *DatasetLoader
	Target          TargetSnapshotter
}

func NewSnapshotService(v *VersionService, loader *DatasetLoader, target TargetSnapshotter) *SnapshotService {
	s := &SnapshotService{Versions: v, Loader: loader, Target: target}
	if v != nil {
		v.LegacyAuthorizer = s.authorizeLegacy
	}
	return s
}
func (s *SnapshotService) Freeze(ctx context.Context, p l.Principal, c *EvaluationTaskConfig, versionID string, strategyIDs ...string) (*d.EvaluationSnapshot, error) {
	if p.Validate() != nil || s.Versions == nil || s.Target == nil {
		return nil, d.ErrVersionForbidden
	}
	var version *d.DatasetVersion
	var e error
	if versionID != "" {
		version, e = s.Versions.Read(ctx, p, versionID)
	} else {
		version, e = s.snapshotLegacy(ctx, p, c.DatasetID)
	}
	if e != nil {
		return nil, e
	}
	if version.DatasetID != c.DatasetID {
		return nil, d.ErrVersionConflict
	}
	for _, sample := range version.Samples {
		mapping := sample.Annotation.EvaluationProfile
		if mapping != nil && mapping.Kind == "sql" {
			if configString(c.Config, "datasource_id") != mapping.DatasourceID {
				return nil, versionError("evaluation datasource must match the reviewed SQL profile")
			}
		}
	}
	profile := string(c.Type.Normalize())
	if profile == "llm" {
		profile = "qa"
	}
	if version.Profile != profile || len(version.QAPairs) == 0 {
		return nil, d.ErrDatasetTypeMismatch
	}
	// Never evaluate train/validation partitions as if they were held-out samples.
	pairs := version.QAPairs
	if len(version.Samples) > 0 {
		pairs = nil
		for i, sample := range version.Samples {
			if sample.Split == "evaluation" || sample.Split == "test" {
				pairs = append(pairs, version.QAPairs[i])
			}
		}
	}
	if len(pairs) == 0 {
		return nil, versionError("no evaluation partition")
	}
	target, e := s.target(ctx, p, c)
	if e != nil {
		return nil, e
	}
	config, e := json.Marshal(c)
	if e != nil {
		return nil, e
	}
	out := &d.EvaluationSnapshot{DatasetVersionID: version.ID, DatasetManifest: version.ManifestDigest, Profile: version.Profile, QAPairs: pairs, Configuration: config, Target: target, GraderVersion: "cognida-evaluation-v1", CreatedAt: time.Now().UTC()}
	if len(strategyIDs) > 1 {
		return nil, d.ErrVersionInvalid
	}
	if len(strategyIDs) == 1 && strategyIDs[0] != "" {
		if s.Strategies == nil {
			return nil, d.ErrVersionForbidden
		}
		version, e := s.Strategies.Read(ctx, p, strategyIDs[0])
		if e != nil {
			return nil, e
		}
		out.Strategy, e = s.Strategies.PinEvaluation(ctx, p, strategyIDs[0], "evaluation:"+uuid.NewString(), evaluationInheritedPolicy(c, version.Kind))
		if e != nil {
			return nil, e
		}
		if e = validateStrategyProfile(out.Strategy, profile); e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *SnapshotService) target(ctx context.Context, p l.Principal, c *EvaluationTaskConfig) (json.RawMessage, error) {
	raw, e := s.Target(ctx, p, c)
	if e != nil {
		return nil, e
	}
	if len(raw) == 0 || len(raw) > 256<<10 || !json.Valid(raw) {
		return nil, d.ErrVersionInvalid
	}
	return json.Marshal(struct {
		Runtime      json.RawMessage `json:"runtime"`
		QAPrompt     string          `json:"qa_prompt"`
		RAGPrompt    string          `json:"rag_prompt"`
		AgentGraders []string        `json:"agent_graders"`
		SQLGraders   []string        `json:"sql_graders"`
	}{raw, prompts.MustGet("evaluation", "qa_system"), prompts.MustGet("evaluation", "rag_system_no_context") + "\n" + prompts.MustGet("evaluation", "rag_system_with_context"), agentGraders, sqlGraders})
}

// ValidateRead authorizes immutable results without requiring today's runtime configuration
// to equal the historical target. Resume/execution must use Validate instead.
func (s *SnapshotService) ValidateRead(ctx context.Context, p l.Principal, snapshot *d.EvaluationSnapshot) (*EvaluationTaskConfig, error) {
	if snapshot == nil || s == nil || s.Versions == nil {
		return nil, d.ErrVersionForbidden
	}
	if snapshot.FeedbackView != nil {
		if s.FeedbackView == nil {
			return nil, d.ErrVersionForbidden
		}
		if e := s.FeedbackView(ctx, p, *snapshot.FeedbackView); e != nil {
			// A deleted, expired or revoked source denies this historical view.
			// Keep unexpected storage/service errors distinct from authorization.
			if errors.Is(e, f.ErrForbidden) || errors.Is(e, f.ErrNotFound) || errors.Is(e, l.ErrForbidden) || errors.Is(e, l.ErrNotFound) || errors.Is(e, l.ErrExpired) {
				return nil, d.ErrVersionForbidden
			}
			return nil, e
		}
	}
	version, e := s.Versions.Read(ctx, p, snapshot.DatasetVersionID)
	if e != nil {
		return nil, e
	}
	if version.ManifestDigest != snapshot.DatasetManifest {
		return nil, d.ErrVersionConflict
	}
	if version.LegacySource != "" {
		if e = s.authorizeLegacy(ctx, p, version.LegacySource); e != nil {
			return nil, e
		}
	}
	if snapshot.Strategy != nil {
		if s.Strategies == nil {
			return nil, d.ErrVersionForbidden
		}
		if e := s.Strategies.ValidateSnapshotRead(ctx, p, snapshot.Strategy); e != nil {
			return nil, e
		}
	}
	var config EvaluationTaskConfig
	if json.Unmarshal(snapshot.Configuration, &config) != nil {
		return nil, d.ErrVersionInvalid
	}
	return &config, nil
}

func (s *SnapshotService) Validate(ctx context.Context, p l.Principal, snapshot *d.EvaluationSnapshot) (*EvaluationTaskConfig, error) {
	config, e := s.ValidateRead(ctx, p, snapshot)
	if e != nil {
		return nil, e
	}
	if s.Target == nil {
		return nil, d.ErrVersionForbidden
	}
	if snapshot.Strategy != nil {
		if s.Strategies == nil {
			return nil, d.ErrVersionForbidden
		}
		if e = s.Strategies.ValidateSnapshot(ctx, p, snapshot.Strategy); e != nil {
			return nil, e
		}
	}
	target, e := s.target(ctx, p, config)
	if e != nil {
		return nil, e
	}
	if !g.JSONEqual(target, snapshot.Target) {
		return nil, d.ErrVersionConflict
	}
	return config, nil
}

var legacyDatasetID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func (s *SnapshotService) authorizeLegacy(ctx context.Context, p l.Principal, id string) error {
	kind := ""
	if parts := strings.SplitN(id, ":", 2); len(parts) == 2 {
		kind, id = parts[0], parts[1]
		if kind != "database" && kind != "file" {
			return d.ErrVersionInvalid
		}
	}
	if s.Loader == nil || !legacyDatasetID.MatchString(id) {
		return d.ErrVersionInvalid
	}
	if s.Loader.datasetRepo != nil && kind != "file" {
		record, e := s.Loader.datasetRepo.FindByID(ctx, id)
		if e == nil {
			if record.TenantID != p.TenantID || record.UserID != p.UserID || record.DeletedAt != nil {
				return d.ErrVersionForbidden
			}
			return nil
		}
		if !errors.Is(e, d.ErrDatasetNotFound) {
			return e
		}
	}
	if kind == "database" {
		return d.ErrVersionForbidden
	}
	root, e := filepath.EvalSymlinks(DatasetDir)
	if e != nil {
		if kind == "" {
			return d.ErrVersionNotFound
		}
		return d.ErrVersionForbidden
	}
	path, e := filepath.EvalSymlinks(filepath.Join(root, id))
	if e != nil {
		if kind == "" {
			return d.ErrVersionNotFound
		}
		return d.ErrVersionForbidden
	}
	relative, e := filepath.Rel(root, path)
	if e != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return d.ErrVersionForbidden
	}
	for _, name := range []string{MetaFileName, SamplesFileName} {
		file, e := filepath.EvalSymlinks(filepath.Join(path, name))
		if e != nil {
			return d.ErrVersionForbidden
		}
		rel, e := filepath.Rel(root, file)
		if e != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return d.ErrVersionForbidden
		}
	}
	return nil
}
func (s *SnapshotService) snapshotLegacy(ctx context.Context, p l.Principal, id string) (*d.DatasetVersion, error) {
	if e := s.authorizeLegacy(ctx, p, id); e != nil {
		return nil, e
	}
	dataset, e := s.Loader.Load(ctx, id)
	if e != nil {
		return nil, e
	}
	if len(dataset.QAPairs) < 1 || len(dataset.QAPairs) > 1000 {
		return nil, d.ErrVersionInvalid
	}
	kind := "file"
	if s.Loader.datasetRepo != nil {
		if _, err := s.Loader.datasetRepo.FindByID(ctx, id); err == nil {
			kind = "database"
		}
	}
	profile := string(dataset.Metadata.EvalType.Normalize())
	if profile == "llm" {
		profile = "qa"
	}
	version := &d.DatasetVersion{DatasetID: id, Name: dataset.Metadata.Name, Profile: profile, LegacySource: kind + ":" + id, Split: d.DatasetSplit{Algorithm: "none", NearDuplicateThreshold: .85}, SourceRefs: []string{}}
	for _, qa := range dataset.QAPairs {
		raw, _ := json.Marshal(qa)
		var pair d.QAPair
		if json.Unmarshal(raw, &pair) != nil || strings.TrimSpace(pair.Question) == "" || strings.TrimSpace(pair.ReferenceAnswer) == "" {
			return nil, d.ErrVersionInvalid
		}
		if profile == "sql" {
			if pair.GoldSQL == "" {
				pair.GoldSQL = pair.ReferenceAnswer
			}
			if validateReadOnlySQL(pair.GoldSQL) != nil {
				return nil, d.ErrVersionInvalid
			}
		}
		version.QAPairs = append(version.QAPairs, pair)
	}
	raw, _ := json.Marshal(version)
	version.ManifestDigest = g.LabelContentDigest(string(raw))
	return s.Versions.Repo.Put(ctx, p, "legacy:"+version.ManifestDigest, version, nil)
}
func (s *Service) WithSnapshots(v *SnapshotService) *Service { s.snapshots = v; return s }
func (w *EvaluationWorker) WithSnapshots(v *SnapshotService) *EvaluationWorker {
	w.snapshots = v
	return w
}
func (s *Service) AuthorizeTask(ctx context.Context, p l.Principal, id string) error {
	task, e := s.taskRepo.FindByID(ctx, id)
	if e != nil {
		return e
	}
	if p.Validate() != nil || task.TenantID != p.TenantID || task.UserID != p.UserID {
		return d.ErrVersionForbidden
	}
	return s.authorizeTaskRead(ctx, task)
}
func (s *Service) authorizeTaskRead(ctx context.Context, task *d.EvaluationTask) error {
	if task.Type == d.OfflineType {
		if s.offlineReadAuthorizer == nil {
			return d.ErrOfflineForbidden
		}
		return s.offlineReadAuthorizer(ctx, task.ID)
	}
	if len(task.Snapshot) > 0 {
		var snapshot d.EvaluationSnapshot
		if json.Unmarshal(task.Snapshot, &snapshot) != nil {
			return d.ErrVersionInvalid
		}
		_, e := s.snapshots.ValidateRead(ctx, l.Principal{TenantID: task.TenantID, UserID: task.UserID}, &snapshot)
		return e
	}
	return nil
}

func (s *Service) AuthorizeDataset(ctx context.Context, p l.Principal, id string) error {
	if s == nil || s.snapshots == nil {
		return d.ErrVersionForbidden
	}
	return s.snapshots.authorizeLegacy(ctx, p, id)
}
func (s *Service) ListDatasetsFor(ctx context.Context, p l.Principal) ([]*DatasetMetadata, error) {
	if p.Validate() != nil {
		return nil, d.ErrVersionForbidden
	}
	all, e := s.datasetService.ListDatasets(ctx)
	if e != nil {
		return nil, e
	}
	out := []*DatasetMetadata{}
	for _, item := range all {
		if s.AuthorizeDataset(ctx, p, item.ID) == nil {
			out = append(out, item)
		}
	}
	return out, nil
}
