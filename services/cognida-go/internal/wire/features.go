package wire

import (
	"cognida/internal/config"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	storeinfra "cognida/internal/infrastructure/objectstore"
	agentmodel "cognida/internal/model/agent"
	auditmodel "cognida/internal/model/audit"
	collectionmodel "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	llmmodel "cognida/internal/model/llm"
	"cognida/internal/model/objectstore"
	taskmodel "cognida/internal/model/taskcontrol"
	"cognida/internal/repository/mysql"
	graphrepo "cognida/internal/repository/neo4j"
	"cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/audit"
	"cognida/internal/service/catalog"
	"cognida/internal/service/collection"
	"cognida/internal/service/datasource"
	"cognida/internal/service/governance"
	"cognida/internal/service/knowledge"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/lineage"
	memorysvc "cognida/internal/service/memory"
	tasksvc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"log"
	"os"
	"time"
)

// FeatureServices is assembled with the same storage and service dependencies
// as the legacy endpoints, so the new console does not create parallel stores.
type FeatureServices struct {
	TaskControl        *tasksvc.Service
	TaskControlHandler *handler.TaskControlHandler
	Memory             *memorysvc.Service
	MemoryHandler      *handler.MemoryHandler
	Catalog            *catalog.Service
	Lineage            *lineage.Service
	CatalogHandler     *handler.CatalogHandler
	LineageHandler     *handler.LineageHandler
	Lifecycle          *lifecycle.Service
	LifecycleHandler   *handler.LifecycleHandler
	LifecycleWorker    *lifecycle.Worker
	Governance         *governance.Service
	GovernanceHandler  *handler.GovernanceHandler
	GovernanceWorker   *governance.Worker
	CollectionSchedule *collection.ScheduleDispatcher
	cancel             context.CancelFunc
	done               chan struct{}
}

func ProvideObjectStore(cfg *config.Config) (objectstore.Store, error) {
	return storeinfra.NewFromConfig(cfg.ObjectStorage)
}

type featureClock struct{}

func (featureClock) Now() time.Time { return time.Now() }
func NewFeatureServices(db *gorm.DB, cfg *config.Config, source *datasource.Service, graph neo4j.DriverWithContext, auditWriter *audit.Writer, store objectstore.Store, redisClient *redis.Client, processor knowledge.DocumentProcessorService, fetcher collectionmodel.Fetcher, collections *collection.Service, collectionRepo collectionmodel.Repository, embedder embedding.Embedder, plannerClient llmmodel.LLMClient) (*FeatureServices, error) {
	repo := mysql.NewCatalogRepository(db)
	catalogService := catalog.NewService(repo, source.ConnectionManager(), source, catalog.AuditWriter{Writer: auditWriter})
	var lineageService *lineage.Service
	if graph != nil {
		database := "neo4j"
		if cfg.Neo4j != nil && cfg.Neo4j.DatabaseName != "" {
			database = cfg.Neo4j.DatabaseName
		}
		lineageService = lineage.NewService(graphrepo.NewLineageRepository(graph, database), repo, mysql.NewSemanticRepository(db))
	}
	f := &FeatureServices{Catalog: catalogService, Lineage: lineageService, CatalogHandler: handler.NewCatalogHandler(catalogService)}
	embeddingModelID := ""
	if cfg.Embedding != nil && embedder != nil {
		embeddingModelID = cfg.Embedding.Provider + ":" + cfg.Embedding.Model + ":" + cfg.Embedding.BaseURL + ":v1"
	}
	f.Memory = memorysvc.NewService(mysql.NewLongTermMemoryRepository(db), embedder, embeddingModelID)
	f.MemoryHandler = handler.NewMemoryHandler(f.Memory)
	if lineageService != nil {
		f.LineageHandler = handler.NewLineageHandler(lineageService)
	}
	compute := lifecycle.NewHTTPCompute(cfg.Evaluation.PythonEndpoint, os.Getenv("INTERNAL_HTTP_AUTH_TOKEN"))
	authorize := featureAuthorizer(db)
	f.CatalogHandler.Authorize = func(ctx context.Context, tenant, user int64, scope string) error {
		return authorize(ctx, l.Principal{TenantID: tenant, UserID: user}, scope)
	}
	life, err := lifecycle.NewService(mysql.NewLifecycleRepository(db), store, job.NewRedisQueueWithPrefix(redisClient, "lifecycle:jobs"), lifecycle.Config{Audit: func(ctx context.Context, p l.Principal, action, resource string, details map[string]any) {
		if auditWriter == nil {
			return
		}
		body, err := json.Marshal(details)
		if err != nil {
			return
		}
		requestID, _ := agentmodel.GetRequestID(ctx)
		auditWriter.Enqueue(auditmodel.NewAuditLog(&p.TenantID, &p.UserID, "lifecycle", action).WithResource("lifecycle", &resource, nil).WithDetails(string(body)).WithRequest(&requestID, nil))
	}, Compute: compute, Authorize: authorize, Access: featureAccess(db, source, catalogService, fetcher, store, resultstore.NewRedisStore(redisClient)), Publish: featurePublication(processor, catalogService), ValidatePublication: featurePublicationValidator(db, catalogService)})
	if err != nil {
		return nil, err
	}
	f.Lifecycle = life
	f.LifecycleHandler = handler.NewLifecycleHandler(life)
	f.LifecycleWorker = lifecycle.NewWorker(life)
	validator, err := tasksvc.NewValidator(tasksvc.LifecycleCapabilities(lifecycle.Contracts()), func(ctx context.Context, p taskmodel.Scope, scope string) error {
		return authorize(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, scope)
	}, func(ctx context.Context, p taskmodel.Scope, ref l.RefID) (l.Kind, error) {
		asset, _, err := life.Resolve(ctx, p.TenantID, p.UserID, ref)
		return asset.Kind, err
	})
	if err != nil {
		return nil, err
	}
	f.TaskControl, err = tasksvc.NewService(mysql.NewTaskControlRepository(db), validator)
	if err != nil {
		return nil, err
	}
	f.TaskControlHandler = handler.NewTaskControlHandler(f.TaskControl)
	supervisor, err := tasksvc.NewSupervisor(f.TaskControl, tasksvc.NewLLMPlanner(plannerClient))
	if err != nil {
		return nil, err
	}
	f.TaskControlHandler.SetSupervisor(supervisor)
	f.Governance = governance.NewService(mysql.NewGovernanceRepository(db), life, compute, job.NewRedisQueueWithPrefix(redisClient, "governance:jobs"), authorize)
	f.Governance.ValidateSource, f.Governance.Apply, f.Governance.DiscoverSource = NewSuggestionHooks(db, processor)
	f.GovernanceHandler = handler.NewGovernanceHandler(f.Governance)
	f.GovernanceWorker = governance.NewWorker(f.Governance)
	f.CollectionSchedule = collection.NewScheduleDispatcher(collectionRepo, collections, featureClock{}, 100)
	return f, nil
}
func (f *FeatureServices) RegisterAgents(ctx context.Context, registry framework.AgentInstanceRegistry) error {
	return f.Lifecycle.RegisterAgents(ctx, registry, framework.NewCollaborationRegistry())
}
func (f *FeatureServices) Start() error {
	if err := f.LifecycleWorker.Run(); err != nil {
		return err
	}
	if err := f.GovernanceWorker.Run(); err != nil {
		f.LifecycleWorker.Stop()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.done = make(chan struct{})
	go func() {
		defer close(f.done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			tickCtx, stop := context.WithTimeout(ctx, 30*time.Second)
			err := f.CollectionSchedule.Tick(tickCtx)
			stop()
			if err != nil && ctx.Err() == nil {
				log.Printf("[Collection] schedule dispatch failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
func (f *FeatureServices) Stop() {
	if f.cancel != nil {
		f.cancel()
		<-f.done
		f.cancel = nil
	}
	f.LifecycleWorker.Stop()
	f.GovernanceWorker.Stop()
}
