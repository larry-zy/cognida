package wire

import (
	"cognida/internal/config"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	storeinfra "cognida/internal/infrastructure/objectstore"
	agentmodel "cognida/internal/model/agent"
	apimodel "cognida/internal/model/apisource"
	auditmodel "cognida/internal/model/audit"
	collectionmodel "cognida/internal/model/collection"
	dsmodel "cognida/internal/model/datasource"
	operationmodel "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	lineagemodel "cognida/internal/model/lineage"
	llmmodel "cognida/internal/model/llm"
	"cognida/internal/model/objectstore"
	taskmodel "cognida/internal/model/taskcontrol"
	"cognida/internal/repository/mysql"
	graphrepo "cognida/internal/repository/neo4j"
	"cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/apisource"
	"cognida/internal/service/audit"
	"cognida/internal/service/catalog"
	"cognida/internal/service/collection"
	collectionfetcher "cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"cognida/internal/service/evaluation"
	"cognida/internal/service/feedback"
	"cognida/internal/service/governance"
	"cognida/internal/service/governedops"
	"cognida/internal/service/knowledge"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/lineage"
	memorysvc "cognida/internal/service/memory"
	"cognida/internal/service/queryjob"
	"cognida/internal/service/researchrun"
	"cognida/internal/service/sandbox"
	"cognida/internal/service/strategy"
	tasksvc "cognida/internal/service/taskcontrol"
	"cognida/internal/service/taskcontrol/governanceadapter"
	"cognida/internal/service/taskcontrol/lifecycleadapter"
	"cognida/internal/service/taskcontrol/sandboxadapter"
	"cognida/internal/service/taskcontrol/warehouseadapter"
	"cognida/internal/service/warehouse"
	"context"
	"encoding/json"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// FeatureServices is assembled with the same storage and service dependencies
// as the legacy endpoints, so the new console does not create parallel stores.
type FeatureServices struct {
	Feedback                *feedback.Service
	FeedbackWorker          *feedback.Worker
	FeedbackHandler         *handler.FeedbackHandler
	QueryJobs               *queryjob.Service
	QueryJobHandler         *handler.QueryJobHandler
	Strategies              *strategy.Service
	StrategyHandler         *handler.StrategyHandler
	TaskControl             *tasksvc.Service
	TaskControlHandler      *handler.TaskControlHandler
	TaskRuntime             *tasksvc.Runtime
	TaskRuntimeWorker       *tasksvc.RuntimeWorker
	Memory                  *memorysvc.Service
	MemoryHandler           *handler.MemoryHandler
	Catalog                 *catalog.Service
	Lineage                 *lineage.Service
	CatalogHandler          *handler.CatalogHandler
	LineageHandler          *handler.LineageHandler
	Lifecycle               *lifecycle.Service
	LifecycleHandler        *handler.LifecycleHandler
	LifecycleWorker         *lifecycle.Worker
	Governance              *governance.Service
	GovernanceHandler       *handler.GovernanceHandler
	GovernanceWorker        *governance.Worker
	CollectionSchedule      *collection.ScheduleDispatcher
	OfflineEvaluation       *evaluation.OfflineService
	OfflineDispatcher       *job.Dispatcher
	OfflineWorker           *job.Worker
	ResearchRuns            *researchrun.Service
	ResearchWorker          *researchrun.Worker
	ResearchRunHandler      *handler.ResearchRunHandler
	DatasetVersions         *evaluation.VersionService
	EvaluationSnapshots     *evaluation.SnapshotService
	evaluationSourceRead    evaluationSourceAuthorizer
	DatasetVersionHandler   *handler.DatasetVersionHandler
	FeatureOperationsWorker *governance.FeatureOperationsWorker
	SandboxRunner           *sandbox.DockerRunner
	SandboxHandler          *handler.SandboxHandler
	APISources              *apisource.Service
	APISourceHandler        *handler.APISourceHandler
	APIWorker               *job.Worker
	Warehouse               *warehouse.Service
	WarehouseHandler        *handler.WarehouseHandler
	WarehouseWorker         *warehouse.Worker
	Operations              *governedops.Service
	OperationHandler        *handler.GovernedOperationHandler
	OperationWorker         *job.Worker
	workers                 sync.WaitGroup
	cancel                  context.CancelFunc
	done                    chan struct{}
}

func ProvideObjectStore(cfg *config.Config) (objectstore.Store, error) {
	return storeinfra.NewFromConfig(cfg.ObjectStorage)
}

type featureClock struct{}

func (featureClock) Now() time.Time { return time.Now() }
func NewFeatureServices(db *gorm.DB, cfg *config.Config, source *datasource.Service, graph neo4j.DriverWithContext, auditWriter *audit.Writer, store objectstore.Store, redisClient *redis.Client, processor knowledge.DocumentProcessorService, fetcher collectionmodel.Fetcher, collections *collection.Service, collectionRepo collectionmodel.Repository, embedder embedding.Embedder, plannerClient llmmodel.LLMClient, agentRegistry *framework.SpecRegistry, datasetLoader *evaluation.DatasetLoader) (*FeatureServices, error) {
	repo := mysql.NewCatalogRepository(db)
	catalogService := catalog.NewService(repo, source.ConnectionManager(), source, catalog.AuditWriter{Writer: auditWriter})
	var lineageService *lineage.Service
	var lineageRepository lineagemodel.Repository
	if graph != nil {
		database := "neo4j"
		if cfg.Neo4j != nil && cfg.Neo4j.DatabaseName != "" {
			database = cfg.Neo4j.DatabaseName
		}
		lineageRepository = graphrepo.NewLineageRepository(graph, database)
		lineageService = lineage.NewService(lineageRepository, repo, mysql.NewSemanticRepository(db))
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
	originValidator := func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		if strings.HasPrefix(ref.ProducerID, "governed-operation:") && f.Operations != nil {
			return f.Operations.ValidateGovernedOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "api-ingestion:") && f.APISources != nil {
			return f.APISources.ValidateGovernedOrigin(ctx, p, ref)
		}
		if (strings.HasPrefix(ref.ProducerID, "warehouse-candidate:") || strings.HasPrefix(ref.ProducerID, "warehouse-approved:")) && f.Warehouse != nil {
			return f.Warehouse.ValidateGovernedOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "sandbox-governed:") && f.SandboxRunner != nil {
			return f.SandboxRunner.ValidateOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "access:") {
			return validateFeatureSourceOrigin(ctx, db, source, source.GuardResults(resultstore.NewRedisStore(redisClient)), p, ref, f.authorizeEvaluationSource, f.knowledgeFeedbackGuard)
		}
		if strings.HasPrefix(ref.ProducerID, "collection-governed:") {
			return collections.ValidateGovernedOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "research-governed:") && f.ResearchRuns != nil {
			return f.ResearchRuns.ValidateGovernedOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "evaluation-governed:") && f.OfflineEvaluation != nil {
			return f.OfflineEvaluation.ValidateGovernedOrigin(ctx, p, ref)
		}
		if f.Governance == nil {
			return l.ErrForbidden
		}
		if strings.HasPrefix(ref.ProducerID, "feature-materialization:") {
			return f.Governance.ValidateFeatureMaterializationOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "feature-job:") {
			return f.Governance.ValidateFeatureJobOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "governance-governed:") {
			return f.Governance.ValidateGovernedOrigin(ctx, p, ref)
		}
		return l.ErrForbidden
	}
	life, err := lifecycle.NewService(mysql.NewLifecycleRepository(db), store, job.NewRedisQueueWithPrefix(redisClient, "lifecycle:jobs"), lifecycle.Config{ValidateGovernedOrigin: originValidator, Audit: func(ctx context.Context, p l.Principal, action, resource string, details map[string]any) {
		if auditWriter == nil {
			return
		}
		body, err := json.Marshal(details)
		if err != nil {
			return
		}
		requestID, _ := agentmodel.GetRequestID(ctx)
		auditWriter.Enqueue(auditmodel.NewAuditLog(&p.TenantID, &p.UserID, "lifecycle", action).WithResource("lifecycle", &resource, nil).WithDetails(string(body)).WithRequest(&requestID, nil))
	}, Compute: compute, Authorize: authorize, Access: featureAccess(db, source, catalogService, fetcher, store, source.GuardResults(resultstore.NewRedisStore(redisClient)), f.authorizeEvaluationSource, f.knowledgeFeedbackGuard), Publish: featurePublication(processor, catalogService), ValidatePublication: featurePublicationValidator(db, catalogService)})
	if err != nil {
		return nil, err
	}
	f.Lifecycle = life
	f.LifecycleHandler = handler.NewLifecycleHandler(life)
	f.LifecycleWorker = lifecycle.NewWorker(life)
	apiQueue := job.NewRedisQueueWithPrefix(redisClient, "api-ingestion:jobs")
	cipher, err := datasource.NewCipherFromEnv()
	if err != nil {
		return nil, err
	}
	sourceRepo := mysql.NewDataSourceRepository(db)
	f.QueryJobs, err = queryjob.New(mysql.NewQueryJobRepository(db), source, func(ctx context.Context, p l.Principal, id string) (*dsmodel.DataSource, error) {
		if err := authorize(ctx, p, "read"); err != nil {
			return nil, err
		}
		return sourceRepo.Get(ctx, id, p.TenantID)
	}, cipher)
	if err != nil {
		return nil, err
	}
	f.QueryJobHandler = handler.NewQueryJobHandler(f.QueryJobs)
	source.WithDurableQueries(f.QueryJobs.Execute)
	f.APISources, err = apisource.New(mysql.NewAPISourceRepository(db), store, life, apiQueue, cipher, collectionfetcher.New(collectionfetcher.Options{}), apisource.Authorizer(authorize))
	if err != nil {
		return nil, err
	}
	apiDispatcher := job.NewDispatcher(apiQueue, job.RetryPolicy{MaxAttempts: 1})
	apiDispatcher.Register(apimodel.JobType, f.APISources, f.APISources)
	f.APIWorker = job.NewWorker(apiDispatcher, 2, time.Second)
	f.APISourceHandler = handler.NewAPISourceHandler(f.APISources)
	f.Warehouse = warehouse.NewService(mysql.NewWarehouseRepository(db), life, mysql.NewSemanticRepository(db), lineageRepository, authorize)
	f.WarehouseWorker = warehouse.NewWorker(f.Warehouse)
	f.WarehouseHandler = handler.NewWarehouseHandler(f.Warehouse)
	operationQueue := job.NewRedisQueueWithPrefix(redisClient, "operations:jobs")
	f.Operations, err = governedops.New(mysql.NewGovernedOperationRepository(db), life, operationQueue, governedops.Authorizer(authorize))
	if err != nil {
		return nil, err
	}
	operationDispatcher := job.NewDispatcher(operationQueue, job.RetryPolicy{MaxAttempts: 1})
	operationDispatcher.Register(operationmodel.JobType, f.Operations, f.Operations)
	f.OperationWorker = job.NewWorker(operationDispatcher, 2, time.Second)
	f.OperationHandler = handler.NewGovernedOperationHandler(f.Operations)
	if err := f.configureSandbox(db); err != nil {
		return nil, err
	}
	f.Governance = governance.NewService(mysql.NewGovernanceRepository(db), life, compute, job.NewRedisQueueWithPrefix(redisClient, "governance:jobs"), authorize)
	f.Governance.WithKnowledgeLabels(mysql.NewKnowledgeLabelRepository(db))
	if cfg.Chat != nil && plannerClient != nil {
		f.Governance.WithLabelSuggester(governance.NewLLMLabelSuggester(plannerClient, cfg.Chat.Provider+":"+cfg.Chat.ModelName))
	}
	if cfg.Embedding != nil && embedder != nil {
		modelID := cfg.Embedding.Provider + ":" + cfg.Embedding.Model
		f.Governance.SetFeatureEmbedding(modelID, cfg.Embedding.Revision, func(ctx context.Context, texts []string) ([][]float64, error) {
			return embedder.EmbedStrings(ctx, texts)
		})
	}
	f.FeatureOperationsWorker = governance.NewFeatureOperationsWorker(f.Governance)
	f.DatasetVersions = evaluation.NewVersionService(mysql.NewDatasetVersionRepository(db), f.Governance, life, datasetProfileAuthorizer(db, source, life, agentRegistry))
	f.EvaluationSnapshots = evaluation.NewSnapshotService(f.DatasetVersions, datasetLoader, evaluationTargetSnapshot(db, cfg, agentRegistry))
	f.DatasetVersionHandler = handler.NewDatasetVersionHandler(f.DatasetVersions)
	f.Governance.ValidateSource, f.Governance.Apply, f.Governance.DiscoverSource = NewSuggestionHooks(db, processor)
	f.GovernanceHandler = handler.NewGovernanceHandler(f.Governance)
	f.GovernanceWorker = governance.NewWorker(f.Governance)
	adapters := map[string]tasksvc.ExecutionAdapter{}
	capabilities := tasksvc.LifecycleCapabilities(lifecycle.Contracts())
	warehouseAdapter, err := warehouseadapter.New(f.Warehouse)
	if err != nil {
		return nil, err
	}
	adapters[warehouseAdapter.Capability().ID] = warehouseAdapter
	capabilities = append(capabilities, warehouseAdapter.Capability())
	if f.SandboxRunner != nil {
		adapter, err := sandboxadapter.New(f.SandboxRunner, life)
		if err != nil {
			return nil, err
		}
		adapters[adapter.Capability().ID] = adapter
		capabilities = append(capabilities, adapter.Capability())
	}
	domainAdapters, domainCapabilities, err := f.domainAdapters(db, cfg, redisClient, collections, compute, authorize)
	if err != nil {
		return nil, err
	}
	for id, adapter := range domainAdapters {
		adapters[id] = adapter
	}
	capabilities = append(capabilities, domainCapabilities...)
	for _, id := range []string{"governance.tagging", "governance.classification", "governance.named_entity", "governance.qa", "governance.feature-engineering"} {
		adapter, err := governanceadapter.New(f.Governance, life, id)
		if err != nil {
			return nil, err
		}
		adapters[id] = adapter
		capabilities = append(capabilities, adapter.Capability())
	}
	validator, err := tasksvc.NewValidator(capabilities, func(ctx context.Context, p taskmodel.Scope, scope string) error {
		return authorize(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, scope)
	}, func(ctx context.Context, p taskmodel.Scope, ref l.RefID) (l.Kind, error) {
		asset, _, err := life.Resolve(ctx, p.TenantID, p.UserID, ref)
		return asset.Kind, err
	})
	if err != nil {
		return nil, err
	}
	taskRepo := mysql.NewTaskControlRepository(db)
	f.TaskControl, err = tasksvc.NewService(taskRepo, validator)
	if err != nil {
		return nil, err
	}
	f.TaskControlHandler = handler.NewTaskControlHandler(f.TaskControl)
	f.Governance.SetDriftRemediation(func(ctx context.Context, p l.Principal, reportID, sourceRef, key string) (string, error) {
		ref, err := l.ParseRefID(sourceRef)
		if err != nil {
			return "", err
		}
		task, err := f.TaskControl.CreateDraft(ctx, taskmodel.Scope{TenantID: p.TenantID, UserID: p.UserID}, tasksvc.DraftInput{Goal: "核查漂移报告 " + reportID + " 并制定数据质量修复计划", InputRefs: []l.RefID{ref}, Budget: tasksvc.Budget{MaxNodes: 4, MaxConcurrency: 2, MaxAttempts: 4, MaxCalls: 40, TimeoutSeconds: 600}}, key, "")
		if err != nil {
			return "", err
		}
		return task.ID, nil
	})
	supervisor, err := tasksvc.NewSupervisor(f.TaskControl, tasksvc.NewLLMPlanner(plannerClient))
	if err != nil {
		return nil, err
	}
	f.TaskControlHandler.SetSupervisor(supervisor)
	for _, capability := range []string{"lifecycle.data-quality", "lifecycle.data-cleaning", "lifecycle.data-enrichment", "lifecycle.data-evaluation"} {
		adapter, err := lifecycleadapter.New(life, capability)
		if err != nil {
			return nil, err
		}
		adapters[capability] = adapter
	}
	// Disabling this flag blocks new starts. Existing executions still reconcile,
	// honour deadlines and propagate cancellation while workers are enabled.
	f.TaskRuntime, err = tasksvc.NewRuntime(f.TaskControl, taskRepo, adapters, os.Getenv("TASK_CONTROL_EXECUTION_ENABLED") != "false" && os.Getenv("BACKGROUND_WORKERS_ENABLED") != "false", func(ctx context.Context, p taskmodel.Scope, refs []l.RefID) error {
		return life.ValidateGovernedInputs(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, refs)
	})
	if err != nil {
		return nil, err
	}
	f.configureStrategies(db, authorize, compute)
	f.TaskControlHandler.SetRuntime(f.TaskRuntime)
	f.TaskRuntimeWorker = tasksvc.NewRuntimeWorker(f.TaskRuntime)
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
	f.TaskRuntimeWorker.Start()
	if f.OperationWorker != nil {
		if err := f.OperationWorker.Run(); err != nil {
			f.Stop()
			return err
		}
	}
	if f.APIWorker != nil {
		if err := f.APIWorker.Run(); err != nil {
			f.Stop()
			return err
		}
	}
	if f.OfflineWorker != nil {
		if err := f.OfflineWorker.Run(); err != nil {
			f.Stop()
			return err
		}
	}
	if f.ResearchWorker != nil {
		if err := f.ResearchWorker.Run(); err != nil {
			f.Stop()
			return err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.done = make(chan struct{})
	if f.FeedbackWorker != nil {
		f.workers.Add(1)
		go func() { defer f.workers.Done(); f.FeedbackWorker.Run(ctx) }()
	}
	if f.QueryJobs != nil {
		f.workers.Add(1)
		go func() { defer f.workers.Done(); f.QueryJobs.Start(ctx) }()
	}
	if f.FeatureOperationsWorker != nil {
		go f.FeatureOperationsWorker.Run(ctx)
	}
	if f.SandboxRunner != nil {
		f.workers.Add(1)
		go func() { defer f.workers.Done(); f.SandboxRunner.Run(ctx) }()
	}
	if f.WarehouseWorker != nil {
		f.workers.Add(1)
		go func() { defer f.workers.Done(); f.WarehouseWorker.Run(ctx) }()
	}
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
	if f.OperationWorker != nil {
		f.OperationWorker.Stop()
	}
	if f.APIWorker != nil {
		f.APIWorker.Stop()
	}
	if f.FeatureOperationsWorker != nil {
		f.FeatureOperationsWorker.Stop()
	}
	if f.ResearchWorker != nil {
		f.ResearchWorker.Stop()
	}
	if f.OfflineWorker != nil {
		f.OfflineWorker.Stop()
	}
	if f.TaskRuntimeWorker != nil {
		f.TaskRuntimeWorker.Stop()
	}
	if f.cancel != nil {
		f.cancel()
		<-f.done
		f.workers.Wait()
		f.cancel = nil
	}
	f.LifecycleWorker.Stop()
	f.GovernanceWorker.Stop()
}
