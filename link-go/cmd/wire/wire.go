//go:build wireinject
// +build wireinject

// Package main 提供依赖注入
package wire

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/google/wire"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	agent "link/internal/model/agent"
	domain_audit "link/internal/model/audit"
	appAccount "link/internal/service/account"
	auditsvc "link/internal/service/audit"
	agentuc "link/internal/service/agent"
	agentframework "link/internal/service/agent/framework"
	agenttools "link/internal/service/agent/tools"
	app_chat "link/internal/service/chat"
	app_evaluation "link/internal/service/evaluation"
	evalexecutor "link/internal/service/evaluation/executor"
	app_kb "link/internal/service/knowledge"
	raguc "link/internal/service/knowledge"
	ragpipeline "link/internal/service/knowledge/pipeline"

	"link/internal/handler"
	"link/internal/handler/middleware"
	"link/internal/handler/router"
	"link/internal/handler/web"
	evaluationcache "link/internal/infrastructure/cache/evaluation"
	rediscache "link/internal/infrastructure/cache/redis"
	"link/internal/infrastructure/config"
	infragraph "link/internal/infrastructure/graph"
	docreader "link/internal/infrastructure/grpc/docreader"
	qualitygrpc "link/internal/infrastructure/grpc/quality"
	infraid "link/internal/infrastructure/id"
	"link/internal/infrastructure/llm"
	llmchat "link/internal/infrastructure/llm/chat"
	linkembedding "link/internal/infrastructure/llm/embedding"
	"link/internal/infrastructure/queue"
	domain_types "link/internal/model/common"
	domain_conversation "link/internal/model/conversation"
	datasourcemodel "link/internal/model/datasource"
	domain_evaluation "link/internal/model/evaluation"
	domain_knowledge "link/internal/model/knowledge"
	domain_llm "link/internal/model/llm"
	domain_rag "link/internal/model/rag"
	qualitymodel "link/internal/model/quality"
	domain_task "link/internal/model/task"
	domain_tenant "link/internal/model/tenant"
	domain_user "link/internal/model/user"
	qualitysvc "link/internal/service/quality"
	datasourcesvc "link/internal/service/datasource"
	"link/internal/repository/milvus/retriever"
	"link/internal/repository/mysql"
	neo4jimpl "link/internal/repository/neo4j"
)

// ========================================
// Wire Injector - 注入器
// ========================================

//go:generate wire
func InitializeApp(db *gorm.DB) (*App, error) {
	wire.Build(
		// 配置
		ProvideJWTConfig,
		ProvideChatConfig,
		ProvideEvaluationConfig,
		ProvideRedisConfig,
		ProvideEmbeddingConfig,
		ProvideNeo4jConfig,

		// 仓储
		ProvideKnowledgeBaseRepository,
		ProvideKnowledgeRepository,
		ProvideChunkRepository,
		ProvideKnowledgeBaseSettingRepository,
		ProvideRetrievalSettingRepository,
		ProvideKnowledgeStatsQuerier,
		ProvideSessionRepository,
		ProvideMessageRepository,
		ProvideUserRepository,
		ProvideRefreshTokenRepository,
		ProvideTenantRepository,
		ProvideEvaluationTaskRepository,
		ProvideEvaluationResultRepository,
		ProvideDatasetRepository,
		ProvideModelRepository,
		ProvideTaskRepository,
		ProvideTaskQueue,
		ProvideGraphQueryRepository,

		// Neo4j
		ProvideNeo4jDriver,
		ProvideNeo4jGraphRepository,

		// 服务

		// RAG 组件
		ProvideEmbedder,
		ProvideVectorRetriever,
		ProvideVectorRepository,

		// 文档处理（解析→分块→向量化→图谱）
		ProvideDocReaderClient,
		ProvideIDGenerator,
		ProvideDocumentProcessorService,

		// LLM 组件
		ProvideLLMClient,
		ProvideModelFactory,
		ProvideRAGLLMChat,
		ProvideChatModel,

		// RAG 检索（供检索优化器使用）
		ProvideMilvusRetriever,
		ProvideRAGGraphRepository,
		ProvideRetriever,

		// 服务
		ProvideSessionService,
		ProvideMessageService,
		ProvideChatService,
		ProvideAccountService,
		ProvideKnowledgeBaseService,
		ProvideModelService,
		ProvideGraphService,
		ProvideEvaluationService,
		ProvideTaskService,

		// Agent 编排器
		ProvideAgentOrchestrator,

		// Agent Services
		ProvideExecuteService,
		ProvideResearchService,
		ProvideProgressService,
		ProvideConfigService,
		ProvideAgentPersistenceService,

		// Agent Registry
		ProvideAgentRegistry,

		// 评测 Worker
		ProvideRedisClient,
		ProvideEvaluationProgressCache,
		ProvideEvaluationQueue,
		ProvideEvaluationWorker,
		ProvideDatasetLoader,
		ProvideDatasetService,
		ProvideDatasetUseCase,

		// 请求审计（异步批量落库）
		ProvideAuditRepository,
		ProvideAuditWriter,
		ProvideAuditHandler,

		// 中间件
		ProvideAuthMiddleware,
		ProvideTenantMiddleware,
		ProvideCORSMiddleware,
		ProvideRecoveryMiddleware,
		ProvideLoggerMiddleware,
		ProvideTraceMiddleware,
		ProvideAuditMiddleware,

		// Handler
		ProvideAuthHandler,
		ProvideKnowledgeBaseHandler,
		ProvideSessionHandler,
		ProvideMessageHandler,
		ProvideChatHandler,
		ProvideTenantHandler,
		ProvideAgentHandler,
		ProvideRegistryAgentHandler,
		ProvideEvaluationHandler,
		ProvideGraphHandler,
		ProvideModelHandler,
		ProvideTaskHandler,
		ProvideRAGOptimizerHandler,
		ProvideGuardrailHandler,
		ProvideWebHandler,

		// 数据质量管理中心
		ProvidePythonGrpcConfig,
		ProvideQualityGateway,
		ProvideQualityCheckRecordRepository,
		ProvideQualityService,
		ProvideQualityHandler,

		// 外部数据源
		ProvideDataSourceRepository,
		ProvideDataSourceService,
		ProvideDataSourceHandler,

		// 路由
		ProvideRouter,

		// 中间件集合
		ProvideMiddlewares,

		// 应用
		ProvideApp,
	)
	return nil, nil
}

// ========================================
// Config Providers
// ========================================

func ProvideJWTConfig() *config.JWTConfig {
	return config.LoadJWTConfig()
}

func ProvideChatConfig() *config.ChatConfig {
	return config.LoadChatConfig()
}

func ProvideEvaluationConfig() *config.EvaluationConfig {
	return config.LoadEvaluationConfig()
}

func ProvideRedisConfig() *config.RedisConfig {
	return config.LoadRedisConfig()
}

func ProvideEmbeddingConfig() *config.EmbeddingConfig {
	return config.LoadEmbeddingConfig()
}

func ProvideNeo4jConfig() *config.Neo4jConfig {
	return config.LoadNeo4jConfig()
}

// ========================================
// Repository Providers
// ========================================

func ProvideKnowledgeBaseRepository(db *gorm.DB) domain_knowledge.KnowledgeBaseRepository {
	return mysql.NewKnowledgeBaseRepository(db, true)
}

func ProvideKnowledgeRepository(db *gorm.DB) domain_knowledge.KnowledgeRepository {
	return mysql.NewKnowledgeRepository(db)
}

func ProvideChunkRepository(db *gorm.DB) domain_knowledge.ChunkRepository {
	return mysql.NewChunkRepository(db)
}

func ProvideKnowledgeBaseSettingRepository(db *gorm.DB) domain_knowledge.KnowledgeBaseSettingRepository {
	return mysql.NewKnowledgeBaseSettingRepository(db)
}

func ProvideRetrievalSettingRepository(db *gorm.DB) domain_knowledge.RetrievalSettingRepository {
	return mysql.NewRetrievalSettingRepository(db)
}

func ProvideTagRepository(db *gorm.DB) domain_knowledge.TagRepository {
	return mysql.NewTagRepository(db)
}

func ProvideSessionRepository(db *gorm.DB) domain_conversation.SessionRepository {
	return mysql.NewSessionRepository(db)
}

func ProvideMessageRepository(db *gorm.DB) domain_conversation.MessageRepository {
	return mysql.NewMessageRepository(db)
}

func ProvideUserRepository(db *gorm.DB) domain_user.UserRepository {
	return mysql.NewUserRepository(db)
}

func ProvideRefreshTokenRepository(db *gorm.DB) domain_user.RefreshTokenRepository {
	return mysql.NewRefreshTokenRepository(db)
}

func ProvideTenantRepository(db *gorm.DB) domain_tenant.TenantRepository {
	return mysql.NewTenantRepository(db)
}

func ProvideTenantUserRepository(db *gorm.DB) domain_tenant.TenantUserRepository {
	return mysql.NewTenantUserRepository(db)
}

func ProvideGraphRepository(db *gorm.DB) domain_knowledge.GraphRepository {
	return mysql.NewGraphMetaRepository(db)
}

// ProvideGraphRepositoryWithFallback 返回可用的图谱仓储：优先使用 Neo4j，
// 当未配置 Neo4j（neo4jRepo 为 nil）时降级到基于 MySQL 的图谱存储，
// 从而保证 GraphService 在无 Neo4j 部署下依然可用。
func ProvideGraphRepositoryWithFallback(neo4jRepo domain_knowledge.GraphRepository, db *gorm.DB) domain_knowledge.GraphRepository {
	if neo4jRepo != nil {
		return neo4jRepo
	}
	return mysql.NewGraphMetaRepository(db)
}

func ProvideGraphQueryRepository(db *gorm.DB) domain_knowledge.GraphQueryRepository {
	return mysql.NewGraphQueryRepository(db, true)
}

// ========================================
// Service Providers
// ========================================

func ProvideSessionService(
	sessionRepo domain_conversation.SessionRepository,
	messageRepo domain_conversation.MessageRepository,
	retrievalSettingRepo domain_knowledge.RetrievalSettingRepository,
) *app_chat.SessionService {
	return app_chat.NewSessionService(sessionRepo, messageRepo, retrievalSettingRepo)
}

func ProvideMessageService(messageRepo domain_conversation.MessageRepository) *app_chat.MessageService {
	return app_chat.NewMessageService(messageRepo)
}

// ProvideLLMChatService 提供领域层的 LLMChatService 接口实现
func ProvideLLMChatService(chatConfig *config.ChatConfig) domain_conversation.LLMChatService {
	if chatConfig == nil || chatConfig.APIKey == "" {
		return nil
	}

	// 创建基础设施层的 Chat 实例
	chatConfigInfra := &llmchat.ChatConfig{
		Source:    domain_types.ModelSource(chatConfig.Provider),
		BaseURL:   chatConfig.BaseURL,
		ModelName: chatConfig.ModelName,
		APIKey:    chatConfig.APIKey,
		Provider:  chatConfig.Provider,
		ModelID:   fmt.Sprintf("chat_%d", 0),
	}

	chat, err := llmchat.NewChat(chatConfigInfra)
	if err != nil {
		return nil
	}

	// 创建适配器
	return llmchat.NewChatServiceAdapter(chat)
}

func ProvideAccountService(
	userRepo domain_user.UserRepository,
	refreshRepo domain_user.RefreshTokenRepository,
	jwtConfig *config.JWTConfig,
	tenantRepo domain_tenant.TenantRepository,
) *appAccount.AccountService {
	return appAccount.NewAccountService(userRepo, refreshRepo, jwtConfig, tenantRepo)
}

func ProvideKnowledgeBaseService(
	kbRepo domain_knowledge.KnowledgeBaseRepository,
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository,
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	statsQuerier domain_knowledge.KnowledgeStatsQuerier,
	vectorRepo domain_knowledge.VectorRepository,
	graphRepo domain_knowledge.GraphRepository,
) app_kb.KnowledgeBaseService {
	return app_kb.NewKnowledgeBaseService(kbRepo, kbSettingRepo, knowledgeRepo, chunkRepo, statsQuerier, vectorRepo, graphRepo)
}

// ProvideVectorRepository provides the vector repository
func ProvideVectorRepository(vectorRetriever *retriever.VectorRetriever) domain_knowledge.VectorRepository {
	return retriever.NewVectorRepositoryAdapter(vectorRetriever)
}

func ProvideKnowledgeStatsQuerier(db *gorm.DB) domain_knowledge.KnowledgeStatsQuerier {
	return mysql.NewKnowledgeStatsQuerier(db)
}

func ProvideKnowledgeService(
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository,
	txManager domain_knowledge.TransactionManager,
) app_kb.KnowledgeService {
	return app_kb.NewKnowledgeService(knowledgeRepo, chunkRepo, kbSettingRepo, txManager)
}

func ProvideTransactionManager(db *gorm.DB) domain_knowledge.TransactionManager {
	return mysql.NewTransactionManager(db)
}

func ProvideModelService(modelRepo domain_llm.ModelRepository, modelFactory domain_llm.ModelFactory) *app_chat.ModelService {
	return app_chat.NewModelService(modelRepo, modelFactory)
}

func ProvideChatService(
	llmClient domain_llm.ChatRepository,
	modelRepo domain_llm.ModelRepository,
	modelFactory domain_llm.ModelFactory,
) *app_chat.ChatService {
	return app_chat.NewChatService(llmClient, modelRepo, modelFactory)
}

// ProvideLLMClient provides the LLM client
func ProvideLLMClient(chatConfig *config.ChatConfig) domain_llm.ChatRepository {
	if chatConfig == nil || chatConfig.APIKey == "" {
		return nil
	}
	modelConfig := &domain_llm.ModelConfig{
		Provider:  domain_llm.Provider(chatConfig.Provider),
		BaseURL:   chatConfig.BaseURL,
		ModelName: chatConfig.ModelName,
		APIKey:    chatConfig.APIKey,
	}
	client, err := llm.NewChatRepository(modelConfig)
	if err != nil {
		return nil
	}
	return client
}

// ProvideModelRepository provides the model repository
func ProvideModelRepository(db *gorm.DB) domain_llm.ModelRepository {
	return mysql.NewModelRepository(db)
}

// ProvideModelFactory provides the model factory
func ProvideModelFactory() domain_llm.ModelFactory {
	return llm.NewModelFactory()
}

// ProvideChatModel 基于聊天配置创建 model.ChatModel，
// 供 RAG 检索优化（HyDE/查询改写）与安全护栏使用。
// 未配置或创建失败时返回 nil，对应功能降级。
func ProvideChatModel(chatConfig *config.ChatConfig) model.ChatModel {
	if chatConfig == nil || chatConfig.APIKey == "" {
		return nil
	}
	chatModel, err := llmchat.NewChatModel(context.Background(), &llmchat.ChatConfig{
		Source:    domain_types.ModelSource(chatConfig.Provider),
		BaseURL:   chatConfig.BaseURL,
		ModelName: chatConfig.ModelName,
		APIKey:    chatConfig.APIKey,
		Provider:  chatConfig.Provider,
		ModelID:   "chat_optimizer",
	})
	if err != nil {
		log.Printf("[ChatModel] 创建失败，RAG优化/护栏功能降级: %v", err)
		return nil
	}
	return chatModel
}

// ProvideNeo4jDriver provides the Neo4j driver
func ProvideNeo4jDriver(cfg *config.Neo4jConfig) (neo4j.DriverWithContext, error) {
	if cfg == nil || cfg.URI == "" {
		return nil, nil
	}
	return CreateDriver(context.Background(), Neo4jConfig{
		URI:      cfg.URI,
		Username: cfg.Username,
		Password: cfg.Password,
	})
}

// ProvideNeo4jGraphRepository provides the Neo4j graph repository
func ProvideNeo4jGraphRepository(driver neo4j.DriverWithContext, cfg *config.Neo4jConfig) (domain_knowledge.GraphRepository, error) {
	if driver == nil {
		return nil, nil
	}
	dbName := "neo4j"
	if cfg != nil && cfg.DatabaseName != "" {
		dbName = cfg.DatabaseName
	}
	return neo4jimpl.NewNeo4jRepositoryFromDriver(driver, dbName)
}

// ProvideGraphService provides the Graph service with Neo4j repository
func ProvideGraphService(
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	llmChat domain_rag.LLMChat,
) *app_kb.GraphService {
	if graphRepo == nil {
		return nil
	}
	// Create GraphService with basic dependencies
	// Note: llmChat is optional and only needed for extraction features
	// 抽取器(基础设施实现)在 Wire 层装配，service 仅依赖 GraphExtractor 接口
	return app_kb.NewGraphServiceWithQuery(graphRepo, graphQueryRepo, infragraph.NewLLMExtractor(llmChat))
}

func ProvideEvaluationService(
	datasetService app_evaluation.DatasetService,
	taskRepo domain_evaluation.EvaluationTaskRepository,
	resultRepo domain_evaluation.EvaluationResultRepository,
	progressCache *evaluationcache.ProgressCache,
	evalQueue *evaluationcache.EvaluationQueue,
	evalConfig *config.EvaluationConfig,
) *app_evaluation.Service {
	// 可用指标目录代理到 Python 注册表（唯一事实来源），端点来自评测配置带默认兜底
	pythonEndpoint := "http://localhost:8000"
	if evalConfig != nil && evalConfig.PythonEndpoint != "" {
		pythonEndpoint = evalConfig.PythonEndpoint
	}
	graderCatalog := app_evaluation.NewPythonEvaluationClient(pythonEndpoint)
	return app_evaluation.NewService(datasetService, taskRepo, resultRepo, progressCache, evalQueue, graderCatalog)
}

// ProvideDatasetLoader provides the dataset loader
func ProvideDatasetLoader(db *gorm.DB, datasetRepo domain_evaluation.DatasetRepository) *app_evaluation.DatasetLoader {
	return app_evaluation.NewDatasetLoader(db, datasetRepo)
}

// ProvideDatasetService provides the dataset service
func ProvideDatasetService(loader *app_evaluation.DatasetLoader) app_evaluation.DatasetService {
	return loader
}

// ProvideDocReaderClient 提供文档读取客户端
func ProvideDocReaderClient() (*docreader.Client, error) {
	// 使用默认的 gRPC 地址
	return docreader.NewClient("localhost:50051")
}

// ProvideIDGenerator 提供唯一 ID 生成器
func ProvideIDGenerator() infraid.IDGenerator {
	return infraid.NewIDGenerator()
}

// ProvideDocumentProcessorService 提供文档处理服务（解析→分块→向量化→图谱提取）
func ProvideDocumentProcessorService(
	kbRepo domain_knowledge.KnowledgeBaseRepository,
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	vectorRepo domain_knowledge.VectorRepository,
	graphRepo domain_knowledge.GraphRepository,
	docReaderClient *docreader.Client,
	embedder embedding.Embedder,
	llmClient domain_llm.ChatRepository,
	idGenerator infraid.IDGenerator,
) app_kb.DocumentProcessorService {
	return app_kb.NewDocumentProcessorService(
		kbRepo, knowledgeRepo, chunkRepo, vectorRepo, graphRepo,
		docReaderClient, embedder, llmClient, idGenerator,
	)
}

// ========================================
// Agent Use Case Providers
// ========================================

func ProvideAgentOrchestrator(registry *agentframework.SpecRegistry) agent.AgentExecutor {
	// 创建基于注册中心的 Agent 编排器：从声明式注册表按 agentID 取可运行实例（含别名）。
	orchestrator := agentframework.NewRegistryAgentOrchestrator(registry.GetInstance)
	return orchestrator
}

func ProvideExecuteService(executor agent.AgentExecutor) *agentuc.ExecuteService {
	return agentuc.NewExecuteService(executor)
}

func ProvideResearchService(executor agent.AgentExecutor) *agentuc.ResearchService {
	config := agent.DefaultDeepResearchConfig()
	return agentuc.NewResearchService(executor, config)
}

func ProvideProgressService() *agentuc.ProgressService {
	return agentuc.NewProgressService()
}

func ProvideConfigService() *agentuc.ConfigService {
	return agentuc.NewConfigService()
}

func ProvideAgentPersistenceService(
	sessionRepo domain_conversation.SessionRepository,
	messageRepo domain_conversation.MessageRepository,
) *agentuc.AgentPersistenceService {
	return agentuc.NewAgentPersistenceService(sessionRepo, messageRepo)
}

// ========================================
// Agent Registry Provider
// ========================================

func ProvideAgentRegistry() *agentframework.SpecRegistry {
	return agentframework.NewSpecRegistry()
}

// ========================================
// Evaluation Worker Providers
// ========================================

// ProvideRedisClient provides the Redis client.
// Redis 不可用时优雅降级（返回 nil），不阻断应用启动；
// 下游依赖（进度缓存、队列、任务队列、Worker）均能容忍 nil 客户端。
func ProvideRedisClient(cfg *config.RedisConfig) (*redis.Client, error) {
	if cfg == nil || cfg.Addr == "" {
		log.Printf("[Redis] 未配置地址，相关功能降级运行")
		return nil, nil
	}
	rc, err := rediscache.NewClient(cfg)
	if err != nil {
		log.Printf("[Redis] 连接失败，相关功能降级运行: %v", err)
		return nil, nil
	}
	return rc.GetClient(), nil
}

func ProvideEvaluationProgressCache(redisClient *redis.Client) *evaluationcache.ProgressCache {
	return evaluationcache.NewProgressCache(redisClient)
}

func ProvideEvaluationQueue(redisClient *redis.Client) *evaluationcache.EvaluationQueue {
	return evaluationcache.NewQueue(redisClient, 100)
}

// ProvideEvaluationWorker 装配评测 Worker：
// 队列/进度缓存经领域端口注入，QA 评测执行器使用已装配的 LLMChat，
// 指标计算通过 Python 评测服务客户端完成。
func ProvideEvaluationWorker(
	evalQueue *evaluationcache.EvaluationQueue,
	progressCache *evaluationcache.ProgressCache,
	llmChat domain_rag.LLMChat,
	taskRepo domain_evaluation.EvaluationTaskRepository,
	resultRepo domain_evaluation.EvaluationResultRepository,
	datasetLoader *app_evaluation.DatasetLoader,
	evalConfig *config.EvaluationConfig,
) *app_evaluation.EvaluationWorker {
	// 注册评测执行器（QA 评测仅需 LLMChat，已在上游装配）
	registry := evalexecutor.NewExecutorRegistry()
	if err := registry.Register(evalexecutor.NewQAExecutor(llmChat)); err != nil {
		log.Printf("[Worker] 注册 QA 执行器失败: %v", err)
	}

	// Python 评测服务客户端 + Worker 配置（来自评测配置，带默认值兜底）
	pythonEndpoint := "http://localhost:8000"
	maxConcurrent := 3
	maxRetries := 3
	if evalConfig != nil {
		if evalConfig.PythonEndpoint != "" {
			pythonEndpoint = evalConfig.PythonEndpoint
		}
		if evalConfig.MaxConcurrent > 0 {
			maxConcurrent = evalConfig.MaxConcurrent
		}
		if evalConfig.MaxRetries > 0 {
			maxRetries = evalConfig.MaxRetries
		}
	}
	pythonClient := app_evaluation.NewPythonEvaluationClient(pythonEndpoint)

	workerConfig := &app_evaluation.WorkerConfig{
		MaxConcurrent: maxConcurrent,
		PollTimeout:   5 * time.Second,
		MaxRetries:    maxRetries,
	}

	return app_evaluation.NewWorker(
		evalQueue,
		progressCache,
		pythonClient,
		datasetLoader,
		registry,
		taskRepo,
		resultRepo,
		workerConfig,
	)
}

func ProvideDatasetUseCase(
	datasetRepo domain_evaluation.DatasetRepository,
	loader *app_evaluation.DatasetLoader,
) *app_evaluation.DatasetManager {
	return app_evaluation.NewDatasetManager(datasetRepo, loader)
}

// ========================================
// Middleware Providers
// ========================================

func ProvideAuthMiddleware(accountService *appAccount.AccountService) *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(accountService)
}

func ProvideTenantMiddleware() *middleware.TenantMiddleware {
	return middleware.NewTenantMiddleware()
}

func ProvideCORSMiddleware() *middleware.CORSMiddleware {
	return middleware.NewCORSMiddleware()
}

func ProvideRecoveryMiddleware() *middleware.RecoveryMiddleware {
	return middleware.NewRecoveryMiddleware()
}

func ProvideLoggerMiddleware() *middleware.LoggerMiddleware {
	return middleware.NewLoggerMiddleware()
}

func ProvideTraceMiddleware() *middleware.TraceMiddleware {
	return middleware.NewTraceMiddleware()
}

func ProvideAuditMiddleware(writer *auditsvc.Writer) *middleware.AuditMiddleware {
	return middleware.NewAuditMiddleware(writer)
}

// ========================================
// 请求审计 Providers
// ========================================

func ProvideAuditRepository(db *gorm.DB) domain_audit.Repository {
	return mysql.NewAuditRepository(db)
}

// ProvideAuditWriter 提供审计异步批量写入器。
// 后台 flush goroutine 在构造时启动，App.Shutdown 负责优雅收尾。
func ProvideAuditWriter(repo domain_audit.Repository) *auditsvc.Writer {
	return auditsvc.NewWriter(repo, auditsvc.DefaultWriterConfig())
}

func ProvideAuditHandler(repo domain_audit.Repository) *handler.AuditHandler {
	return handler.NewAuditHandler(repo)
}

// ========================================
// Handler Providers
// ========================================

func ProvideAuthHandler(accountService *appAccount.AccountService) *handler.AuthHandler {
	return handler.NewAuthHandler(accountService)
}

func ProvideKnowledgeBaseHandler(
	kbService app_kb.KnowledgeBaseService,
	documentProcessor app_kb.DocumentProcessorService,
) *handler.KnowledgeBaseHandler {
	return handler.NewKnowledgeBaseHandler(kbService, documentProcessor)
}

func ProvideSessionHandler(
	sessionService *app_chat.SessionService,
	chatService *app_chat.ChatService,
) *handler.SessionHandler {
	return handler.NewSessionHandler(sessionService, chatService)
}

func ProvideMessageHandler(messageService *app_chat.MessageService) *handler.MessageHandler {
	return handler.NewMessageHandler(messageService)
}

func ProvideChatHandler(chatService *app_chat.ChatService) *handler.ChatHandler {
	return handler.NewChatHandler(chatService)
}

func ProvideTenantHandler(accountService *appAccount.AccountService) *handler.TenantHandler {
	return handler.NewTenantHandler(accountService)
}

func ProvideAgentHandler(
	executeUC *agentuc.ExecuteService,
	researchUC *agentuc.ResearchService,
	configUC *agentuc.ConfigService,
	progressUC *agentuc.ProgressService,
	persistenceSvc *agentuc.AgentPersistenceService,
) *handler.AgentHandler {
	return handler.NewAgentHandler(executeUC, researchUC, configUC, progressUC, persistenceSvc)
}

func ProvideEvaluationHandler(
	evalService *app_evaluation.Service,
	datasetManager *app_evaluation.DatasetManager,
	progressCache *evaluationcache.ProgressCache,
) *handler.EvaluationHandler {
	return handler.NewEvaluationHandler(evalService, datasetManager, progressCache)
}

func ProvideGraphHandler(graphService *app_kb.GraphService) *handler.GraphHandler {
	return handler.NewGraphHandler(graphService)
}

func ProvideModelHandler(
	modelService *app_chat.ModelService,
	chatService *app_chat.ChatService,
) *handler.ModelHandler {
	return handler.NewModelHandler(modelService, chatService)
}

func ProvideRegistryAgentHandler(registry *agentframework.SpecRegistry) *handler.RegistryAgentHandler {
	return handler.NewRegistryAgentHandler(registry)
}

func ProvideTaskHandler(taskService *app_evaluation.TaskService) *handler.TaskHandler {
	return handler.NewTaskHandler(taskService)
}

// ProvideRAGOptimizerHandler 装配 RAG 检索优化器。
// 未配置 ChatModel 时降级（handler 内部判空跳过优化）。
func ProvideRAGOptimizerHandler(chatModel model.ChatModel, baseRetriever domain_rag.Retriever) *handler.RAGOptimizerHandler {
	if chatModel == nil {
		return handler.NewRAGOptimizerHandler(nil)
	}
	optimizer := app_kb.NewOptimizer(chatModel, baseRetriever)
	return handler.NewRAGOptimizerHandler(optimizer)
}

// ProvideGuardrailHandler 装配内容安全护栏。
// 未配置 ChatModel 时降级（handler 内部判空放行）。
func ProvideGuardrailHandler(chatModel model.ChatModel) *handler.GuardrailHandler {
	if chatModel == nil {
		return handler.NewGuardrailHandler(nil)
	}
	svc := agenttools.NewGuardrailService(chatModel)
	impl, ok := svc.(*agenttools.GuardrailServiceImpl)
	if !ok {
		return handler.NewGuardrailHandler(nil)
	}
	return handler.NewGuardrailHandler(impl)
}

func ProvideWebHandler() *web.Handler {
	// TODO: Get the static files path from config
	return web.NewHandler("./static")
}

// ========================================
// Evaluation Repository Providers
// ========================================

func ProvideEvaluationTaskRepository(db *gorm.DB) domain_evaluation.EvaluationTaskRepository {
	return mysql.NewEvaluationTaskRepository(db)
}

func ProvideTaskService(
	taskRepo domain_task.TaskRepository,
	taskQueue domain_task.TaskQueue,
) *app_evaluation.TaskService {
	return app_evaluation.NewTaskService(taskRepo, taskQueue)
}

func ProvideTaskRepository(db *gorm.DB) domain_task.TaskRepository {
	return mysql.NewTaskRepository(db)
}

// ProvideTaskQueue 提供基于 Redis 的任务队列。
// Redis 不可用时返回 nil（任务调度功能降级，不阻断启动）。
func ProvideTaskQueue(redisClient *redis.Client) domain_task.TaskQueue {
	if redisClient == nil {
		return nil
	}
	return queue.NewRedisTaskQueue(redisClient)
}

func ProvideEvaluationResultRepository(db *gorm.DB) domain_evaluation.EvaluationResultRepository {
	return mysql.NewEvaluationResultRepository(db)
}

func ProvideDatasetRepository(db *gorm.DB) domain_evaluation.DatasetRepository {
	return mysql.NewDatasetRepository(db)
}

func ProvideChatRepository(chatConfig *config.ChatConfig) (domain_llm.ChatRepository, error) {
	// 如果配置了 API Key，创建真实的 ChatRepository
	if chatConfig.APIKey != "" {
		modelConfig := &domain_llm.ModelConfig{
			Provider:  domain_llm.Provider(chatConfig.Provider),
			BaseURL:   chatConfig.BaseURL,
			ModelName: chatConfig.ModelName,
			APIKey:    chatConfig.APIKey,
		}
		return llm.NewChatRepository(modelConfig)
	}
	// 否则返回 nil（评测服务会处理 nil 的情况）
	return nil, nil
}

// ========================================
// RAG Component Providers
// ========================================

func ProvideEmbedder(cfg *config.EmbeddingConfig) (embedding.Embedder, error) {
	return linkembedding.NewEmbedder(cfg)
}

func ProvideVectorRetriever(embedder embedding.Embedder) (*retriever.VectorRetriever, error) {
	return retriever.NewVectorRetriever(embedder)
}

func ProvideMilvusRetriever() ragpipeline.MilvusRetriever {
	// 创建 Milvus 向量检索器
	vectorRetriever, err := retriever.NewVectorRetriever(nil) // embedder 将从外部注入
	if err != nil {
		// 如果 Milvus 未初始化，返回 nil
		return nil
	}
	// 返回 Milvus 适配器，适配 MilvusRetriever 接口
	return retriever.NewMilvusAdapter(vectorRetriever)
}

func ProvideRAGGraphRepository() domain_rag.GraphRepository {
	// 暂时返回 nil，RAG 评估主要使用 Retriever 进行检索
	// 后续可以添加 Neo4j 适配器
	return nil
}

func ProvideRetriever(
	kbSettingRepo domain_knowledge.KnowledgeBaseSettingRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	embedder embedding.Embedder,
	milvusRetriever ragpipeline.MilvusRetriever,
	graphRepo domain_rag.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
) domain_rag.Retriever {
	// TODO: graph.GraphQueryRepository 和 rag.GraphQueryRepository 接口不兼容
	// 暂时传入 nil，待接口统一后再启用
	return ragpipeline.NewRetriever(kbSettingRepo, chunkRepo, embedder, milvusRetriever, graphRepo, nil)
}

func ProvideReranker() domain_rag.Reranker {
	return ragpipeline.NewReranker()
}

func ProvideRAGLLMChat(chatRepo domain_llm.ChatRepository) domain_rag.LLMChat {
	if chatRepo != nil {
		return ragpipeline.NewLLMChatAdapterWithRepo(chatRepo)
	}
	return ragpipeline.NewLLMChatAdapter(nil)
}

func ProvideRAGService(
	retriever domain_rag.Retriever,
	reranker domain_rag.Reranker,
	queryStrengthener domain_rag.QueryStrengthener,
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	llmChat domain_rag.LLMChat,
) *raguc.RAGService {
	return raguc.NewRAGService(retriever, reranker, queryStrengthener, graphRepo, graphQueryRepo, llmChat, infragraph.NewLLMExtractor(llmChat))
}

// ========================================
// Router Provider
// ========================================

func ProvideRouter(
	authHandler *handler.AuthHandler,
	knowledgeBaseHandler *handler.KnowledgeBaseHandler,
	sessionHandler *handler.SessionHandler,
	messageHandler *handler.MessageHandler,
	chatHandler *handler.ChatHandler,
	tenantHandler *handler.TenantHandler,
	agentHandler *handler.AgentHandler,
	registryAgentHandler *handler.RegistryAgentHandler,
	graphHandler *handler.GraphHandler,
	modelHandler *handler.ModelHandler,
	taskHandler *handler.TaskHandler,
	ragOptimizerHandler *handler.RAGOptimizerHandler,
	guardrailHandler *handler.GuardrailHandler,
	evaluationHandler *handler.EvaluationHandler,
	qualityHandler *handler.QualityHandler,
	dataSourceHandler *handler.DataSourceHandler,
	auditHandler *handler.AuditHandler,
	webHandler *web.Handler,
	authMiddleware *middleware.AuthMiddleware,
	tenantMiddleware *middleware.TenantMiddleware,
	auditMiddleware *middleware.AuditMiddleware,
) *router.Router {
	return router.NewRouter(
		authHandler,
		knowledgeBaseHandler,
		sessionHandler,
		messageHandler,
		chatHandler,
		tenantHandler,
		agentHandler,
		registryAgentHandler,
		graphHandler,
		modelHandler,
		taskHandler,
		ragOptimizerHandler,
		guardrailHandler,
		evaluationHandler,
		qualityHandler,
		dataSourceHandler,
		auditHandler,
		webHandler,
		authMiddleware,
		tenantMiddleware,
		auditMiddleware,
	)
}

// ========================================
// 数据质量管理中心 Providers
// ========================================

// ProvidePythonGrpcConfig 提供 Python gRPC 配置。
func ProvidePythonGrpcConfig() *config.PythonGrpcConfig {
	return config.LoadPythonGrpcConfig()
}

// ProvideQualityGateway 提供数据质量 gRPC 网关（Python 服务适配器）。
func ProvideQualityGateway(cfg *config.PythonGrpcConfig) qualitysvc.Gateway {
	target := "localhost:50051"
	timeout := 60 * time.Second
	if cfg != nil {
		if cfg.Target != "" {
			target = cfg.Target
		}
		if cfg.Timeout > 0 {
			timeout = time.Duration(cfg.Timeout) * time.Second
		}
	}
	return qualitygrpc.NewGateway(target, timeout)
}

// ProvideQualityCheckRecordRepository 提供质检记录仓储。
func ProvideQualityCheckRecordRepository(db *gorm.DB) qualitymodel.CheckRecordRepository {
	return mysql.NewQualityCheckRecordRepository(db)
}

// ProvideQualityService 提供数据质量应用服务。
func ProvideQualityService(gateway qualitysvc.Gateway, repo qualitymodel.CheckRecordRepository, idGen infraid.IDGenerator, cfg *config.PythonGrpcConfig) *qualitysvc.Service {
	timeout := 60 * time.Second
	if cfg != nil && cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	return qualitysvc.NewService(gateway, timeout, repo, idGen)
}

// ProvideQualityHandler 提供数据质量 HTTP 处理器。
func ProvideQualityHandler(service *qualitysvc.Service) *handler.QualityHandler {
	return handler.NewQualityHandler(service)
}

// ========================================
// 外部数据源 Providers
// ========================================

// ProvideDataSourceRepository 提供数据源仓储。
func ProvideDataSourceRepository(db *gorm.DB) datasourcemodel.Repository {
	return mysql.NewDataSourceRepository(db)
}

// ProvideDataSourceService 提供数据源应用服务（含凭证加密与受管连接池）。
// DATASOURCE_SECRET_KEY 未配置时 fail-fast：不允许明文/无密钥启动。
func ProvideDataSourceService(repo datasourcemodel.Repository, idGen infraid.IDGenerator) *datasourcesvc.Service {
	cipher, err := datasourcesvc.NewCipherFromEnv()
	if err != nil {
		panic(fmt.Sprintf("外部数据源加密初始化失败: %v（请配置 %s）", err, datasourcesvc.SecretKeyEnv))
	}
	cm := datasourcesvc.NewConnectionManager(repo, cipher, datasourcesvc.DefaultPoolOptions())
	return datasourcesvc.NewService(repo, cipher, cm, idGen)
}

// ProvideDataSourceHandler 提供数据源 HTTP 处理器。
func ProvideDataSourceHandler(service *datasourcesvc.Service) *handler.DataSourceHandler {
	return handler.NewDataSourceHandler(service)
}

// ========================================
// Middlewares Collection
// ========================================

// Middlewares 中间件集合
type Middlewares struct {
	Auth     *middleware.AuthMiddleware
	Tenant   *middleware.TenantMiddleware
	CORS     *middleware.CORSMiddleware
	Recovery *middleware.RecoveryMiddleware
	Logger   *middleware.LoggerMiddleware
	Trace    *middleware.TraceMiddleware
}

func ProvideMiddlewares(
	auth *middleware.AuthMiddleware,
	tenant *middleware.TenantMiddleware,
	cors *middleware.CORSMiddleware,
	recovery *middleware.RecoveryMiddleware,
	logger *middleware.LoggerMiddleware,
	trace *middleware.TraceMiddleware,
) *Middlewares {
	return &Middlewares{
		Auth:     auth,
		Tenant:   tenant,
		CORS:     cors,
		Recovery: recovery,
		Logger:   logger,
		Trace:    trace,
	}
}

// ========================================
// App Provider
// ========================================

// App 应用程序实例
type App struct {
	Router           *router.Router
	Middlewares      *Middlewares
	AgentRegistry    *agentframework.SpecRegistry
	ChatConfig       *config.ChatConfig
	EvaluationConfig *config.EvaluationConfig
	EvaluationWorker *app_evaluation.EvaluationWorker
	// 以下依赖暴露给组合根（main.go），用于把 Agent 工具（rag_query/graph_query/kb_list）
	// 接线到真实检索/图谱/知识库服务，替代此前工具层的 mock/未接线状态。
	Retriever               domain_rag.Retriever
	GraphService            *app_kb.GraphService
	KnowledgeBaseRepository domain_knowledge.KnowledgeBaseRepository
	// AuditWriter 暴露给组合根：优雅关闭时 flush 积压审计。
	AuditWriter *auditsvc.Writer
	// DataSourceService 暴露给组合根：把查询类工具的 database_id 路由
	// 接线到外部数据源 ConnectionManager（InitDatasourceTools）。
	DataSourceService *datasourcesvc.Service
	// AgentHandler 暴露给组合根：构造 ToolRegistry 后经 SetToolGateway 注入工具网关
	// （confirm-resume / UI 取数 / schema 查询），替代 tools 包级默认槽位。
	AgentHandler *handler.AgentHandler
}

func ProvideApp(
	r *router.Router,
	m *Middlewares,
	agentRegistry *agentframework.SpecRegistry,
	chatConfig *config.ChatConfig,
	evalConfig *config.EvaluationConfig,
	evalWorker *app_evaluation.EvaluationWorker,
	retriever domain_rag.Retriever,
	graphService *app_kb.GraphService,
	kbRepo domain_knowledge.KnowledgeBaseRepository,
	auditWriter *auditsvc.Writer,
	dataSourceService *datasourcesvc.Service,
	agentHandler *handler.AgentHandler,
) *App {
	return &App{
		Router:                  r,
		Middlewares:             m,
		AgentRegistry:           agentRegistry,
		ChatConfig:              chatConfig,
		EvaluationConfig:        evalConfig,
		EvaluationWorker:        evalWorker,
		Retriever:               retriever,
		GraphService:            graphService,
		KnowledgeBaseRepository: kbRepo,
		AuditWriter:             auditWriter,
		DataSourceService:       dataSourceService,
		AgentHandler:            agentHandler,
	}
}

// Shutdown 关闭应用程序：flush 审计写入器，确保积压审计落库不丢。
func (a *App) Shutdown(ctx context.Context) error {
	if a.AuditWriter != nil {
		return a.AuditWriter.Close(ctx)
	}
	return nil
}
