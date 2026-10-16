package wire

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"cognida/internal/config"
	e "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/framework"
	"cognida/internal/service/datasource"
	"cognida/internal/service/evaluation"
	"cognida/internal/service/lifecycle"
	"gorm.io/gorm"
)

func datasetProfileAuthorizer(db *gorm.DB, source *datasource.Service, life *lifecycle.Service, registry *framework.SpecRegistry) evaluation.ProfileAuthorizer {
	return func(ctx context.Context, p l.Principal, profile g.EvaluationProfile) error {
		if err := featureAuthorizer(db)(ctx, p, "read"); err != nil {
			return e.ErrVersionForbidden
		}
		switch profile.Kind {
		case "rag":
			allowed := map[string]bool{}
			visited := map[string]bool{}
			var visit func(l.RefID, int) error
			visit = func(id l.RefID, depth int) error {
				if depth > 16 || len(visited) >= 128 {
					return e.ErrVersionInvalid
				}
				if visited[id.String()] {
					return nil
				}
				visited[id.String()] = true
				ref, _, err := life.Resolve(ctx, p.TenantID, p.UserID, id)
				if err != nil {
					return e.ErrVersionForbidden
				}
				if ref.Provenance.SourceType == "knowledge" {
					allowed[ref.Provenance.SourceID] = true
				}
				for _, parent := range ref.Parents {
					if err := visit(parent, depth+1); err != nil {
						return err
					}
				}
				return nil
			}
			for _, value := range profile.SourceRefs {
				id, err := l.ParseRefID(value)
				if err != nil {
					return e.ErrVersionInvalid
				}
				if err = visit(id, 0); err != nil {
					return err
				}
			}
			for _, id := range profile.RelevantChunkIDs {
				chunk, err := mysql.NewChunkRepository(db).FindByID(ctx, id)
				if err != nil || chunk == nil || chunk.TenantID != p.TenantID || chunk.DeletedAt != nil || !chunk.IsEnabled || !allowed[chunk.KnowledgeID] {
					return e.ErrVersionForbidden
				}
			}
		case "sql":
			ds, err := source.Get(ctx, p.TenantID, profile.DatasourceID)
			if err != nil || ds.Type != profile.Dialect {
				return e.ErrVersionForbidden
			}
			return datasource.ValidateReadQuery(profile.GoldSQL)
		case "agent":
			defs, err := registry.List(ctx)
			if err != nil {
				return err
			}
			tools := map[string]bool{}
			for _, def := range defs {
				for _, name := range strings.Split(def.Metadata["tools"], ",") {
					tools[name] = true
				}
			}
			for _, name := range profile.ExpectedTools {
				if !tools[name] {
					return e.ErrVersionInvalid
				}
			}
		default:
			return e.ErrVersionInvalid
		}
		return nil
	}
}

var runtimeDigestOnce sync.Once
var runtimeDigest string

func executableDigest() string {
	runtimeDigestOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			return
		}
		file, err := os.Open(path)
		if err != nil {
			return
		}
		defer file.Close()
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err == nil {
			runtimeDigest = fmt.Sprintf("%x", hash.Sum(nil))
		}
	})
	return runtimeDigest
}
func evaluationTargetSnapshot(db *gorm.DB, cfg *config.Config, registry *framework.SpecRegistry) evaluation.TargetSnapshotter {
	return func(ctx context.Context, p l.Principal, in *evaluation.EvaluationTaskConfig) (json.RawMessage, error) {
		if err := featureAuthorizer(db)(ctx, p, "read"); err != nil {
			return nil, e.ErrVersionForbidden
		}
		if cfg.Chat == nil || cfg.Chat.ModelName == "" || executableDigest() == "" {
			return nil, e.ErrVersionInvalid
		}
		// The current QA/RAG executors use the configured model. Reject an
		// unsupported model override rather than labeling a different model's run.
		if in.ModelID != "" && in.ModelID != cfg.Chat.ModelName {
			return nil, fmt.Errorf("%w: evaluation uses configured model %s", e.ErrVersionInvalid, cfg.Chat.ModelName)
		}
		identity := map[string]any{"runtime_digest": executableDigest(), "provider": cfg.Chat.Provider, "model": cfg.Chat.ModelName, "endpoint_digest": fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.Chat.BaseURL))), "python_endpoint": cfg.Evaluation.PythonEndpoint, "default_timeout": cfg.Evaluation.DefaultTimeout, "agent_timeout": cfg.Evaluation.AgentTimeout, "grader_version": "cognida-evaluation-v1"}
		if in.KnowledgeBaseID != "" {
			kb, err := mysql.NewKnowledgeBaseRepository(db, true).FindByIDForTenant(ctx, in.KnowledgeBaseID, p.TenantID)
			if err != nil || kb == nil || !kb.IsActive() || (!kb.IsPublic && kb.UserID != p.UserID) {
				return nil, e.ErrVersionForbidden
			}
			settings, err := mysql.NewKnowledgeBaseSettingRepository(db).FindByKnowledgeBaseID(ctx, kb.ID)
			if err != nil {
				return nil, err
			}
			raw, err := json.Marshal(settings)
			if err != nil {
				return nil, err
			}
			identity["retrieval_settings_digest"] = fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		if in.AgentID != "" {
			def, err := registry.Get(ctx, in.AgentID)
			if err != nil || def == nil {
				return nil, e.ErrVersionInvalid
			}
			identity["agent_type"] = def.Type
			identity["agent_metadata"] = def.Metadata
			identity["agent_config_digest"] = fmt.Sprintf("%x", sha256.Sum256([]byte(def.ConfigJSON)))
		}
		return json.Marshal(identity)
	}
}
