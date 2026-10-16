package wire

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cognida/internal/config"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	l "cognida/internal/model/lifecycle"
	t "cognida/internal/model/taskcontrol"
	"cognida/internal/repository/mysql"
	agent "cognida/internal/service/agent"
	"cognida/internal/service/collection"
	"cognida/internal/service/evaluation"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/researchrun"
	tc "cognida/internal/service/taskcontrol"
	"cognida/internal/service/taskcontrol/collectionadapter"
	"cognida/internal/service/taskcontrol/evaluationadapter"
	"cognida/internal/service/taskcontrol/researchadapter"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func (f *FeatureServices) domainAdapters(db *gorm.DB, cfg *config.Config, redisClient *redis.Client, collections *collection.Service, compute lifecycle.Compute, authorize lifecycle.Authorizer) (map[string]tc.ExecutionAdapter, []tc.Capability, error) {
	queue := job.NewRedisQueueWithPrefix(redisClient, "evaluation:offline:jobs")
	var err error
	f.OfflineEvaluation, err = evaluation.NewOfflineService(mysql.NewOfflineEvaluationRepository(db), f.Lifecycle, queue, evaluation.NewOfflineScorer(evaluation.NewPythonEvaluationClient(cfg.Evaluation.PythonEndpoint)))
	if err != nil {
		return nil, nil, err
	}
	f.OfflineDispatcher = job.NewDispatcher(queue, job.RetryPolicy{MaxAttempts: 1})
	f.OfflineDispatcher.Register(evaluation.OfflineJobType, f.OfflineEvaluation, f.OfflineEvaluation)
	f.OfflineWorker = job.NewWorker(f.OfflineDispatcher, 2, time.Second)
	f.ResearchRuns, err = researchrun.New(mysql.NewResearchRunRepository(db), researchRunAuthorizer(db, authorize), func(ctx context.Context, scope t.Scope, id string, report *agent.DeepResearchResponse) (l.RefID, error) {
		body, err := json.Marshal(report)
		if err != nil {
			return l.RefID{}, err
		}
		ref, err := f.Lifecycle.ImportDerived(ctx, l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}, l.Insight, "research-governed:"+id, nil, body, l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"})
		return ref.RefID, err
	})
	if err != nil {
		return nil, nil, err
	}
	f.ResearchWorker = researchrun.NewWorker(f.ResearchRuns)
	f.ResearchRunHandler = handler.NewResearchRunHandler(f.ResearchRuns)
	adapters, capabilities := map[string]tc.ExecutionAdapter{}, []tc.Capability{}
	add := func(cap tc.Capability, adapter tc.ExecutionAdapter) {
		adapters[cap.ID] = adapter
		capabilities = append(capabilities, cap)
	}
	collector, err := collectionadapter.New(collections, f.Lifecycle)
	if err != nil {
		return nil, nil, err
	}
	add(collector.Capability(), collector)
	scorer, err := evaluationadapter.New(f.OfflineEvaluation)
	if err != nil {
		return nil, nil, err
	}
	add(scorer.Capability(), scorer)
	researcher, err := researchadapter.New(f.ResearchRuns)
	if err != nil {
		return nil, nil, err
	}
	add(researchadapter.Capability(), researcher)
	return adapters, capabilities, nil
}

// Each report read rechecks the explicit KB scope and the exact cited chunks.
// A persisted report never grants access after membership/source revocation.
func researchRunAuthorizer(db *gorm.DB, authorize lifecycle.Authorizer) researchrun.Authorizer {
	return func(ctx context.Context, scope t.Scope, ids []string, report *agent.DeepResearchResponse) error {
		if err := authorize(ctx, l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}, "read"); err != nil {
			return t.ErrForbidden
		}
		if len(ids) > 50 {
			return t.ErrForbidden
		}
		allowed := map[string]bool{}
		for _, id := range ids {
			var count int64
			if db.WithContext(ctx).Table("knowledge_bases").Where("id = ? AND tenant_id = ? AND status <> 0 AND deleted_at IS NULL AND (user_id = ? OR is_public = ?)", id, scope.TenantID, scope.UserID, true).Count(&count).Error != nil || count != 1 || allowed[id] {
				return t.ErrForbidden
			}
			allowed[id] = true
		}
		if report == nil {
			return nil
		}
		if len(report.Sources) > 20 {
			return t.ErrForbidden
		}
		for _, source := range report.Sources {
			if source.Invalidated || len(source.ContentDigest) != 64 || source.AcquiredAt == "" {
				return t.ErrForbidden
			}
			if _, err := time.Parse(time.RFC3339Nano, source.AcquiredAt); err != nil {
				return t.ErrForbidden
			}
			if source.Type == "web" {
				u, err := url.Parse(source.URL)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || source.Excerpt == "" {
					return t.ErrForbidden
				}
				continue
			}
			if !allowed[source.KnowledgeBaseID] || source.KnowledgeID == "" || source.ChunkID == "" || source.Excerpt == "" || (source.Type != "rag" && source.Type != "graph") {
				return t.ErrForbidden
			}
			var rows []struct{ Content string }
			err := db.WithContext(ctx).Table("chunks c").Select("c.content").Joins("JOIN knowledge k ON k.id = c.knowledge_id AND k.tenant_id = c.tenant_id AND k.knowledge_base_id = c.knowledge_base_id").Where("c.id = ? AND c.tenant_id = ? AND c.knowledge_base_id = ? AND c.knowledge_id = ? AND c.is_enabled = ? AND c.deleted_at IS NULL AND k.deleted_at IS NULL AND k.enable_status = ? AND k.parse_status = ?", source.ChunkID, scope.TenantID, source.KnowledgeBaseID, source.KnowledgeID, true, "enabled", "completed").Limit(1).Scan(&rows).Error
			if err != nil || len(rows) != 1 || fmt.Sprintf("%x", sha256.Sum256([]byte(rows[0].Content))) != source.ContentDigest || !strings.Contains(rows[0].Content, strings.TrimSuffix(source.Excerpt, "…")) {
				return t.ErrForbidden
			}
		}
		return nil
	}
}
