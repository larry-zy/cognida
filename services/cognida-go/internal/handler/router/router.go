// Package router 提供路由配置
package router

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"cognida/internal/handler"
	"cognida/internal/handler/middleware"
	"cognida/internal/handler/web"
)

// ========================================
// Router 路由器
// ========================================

// Router 路由器
type Router struct {
	engine               *gin.Engine
	authHandler          *handler.AuthHandler
	knowledgeBaseHandler *handler.KnowledgeBaseHandler
	sessionHandler       *handler.SessionHandler
	messageHandler       *handler.MessageHandler
	tenantHandler        *handler.TenantHandler
	agentHandler         *handler.AgentHandler
	registryAgentHandler *handler.RegistryAgentHandler
	graphHandler         *handler.GraphHandler
	modelHandler         *handler.ModelHandler
	taskHandler          *handler.TaskHandler
	ragOptimizerHandler  *handler.RAGOptimizerHandler
	guardrailHandler     *handler.GuardrailHandler
	evaluationHandler    *handler.EvaluationHandler
	qualityHandler       *handler.QualityHandler
	dataSourceHandler    *handler.DataSourceHandler
	semanticHandler      *handler.SemanticHandler
	auditHandler         *handler.AuditHandler
	traceHandler         *handler.TraceHandler
	webHandler           *web.Handler
	authMiddleware       *middleware.AuthMiddleware
	tenantMiddleware     *middleware.TenantMiddleware
	auditMiddleware      *middleware.AuditMiddleware
	rateLimiter          *middleware.RateLimiter
	idempotency          *middleware.Idempotency

	mu  sync.Mutex   // 保护 srv 在 Run/Shutdown 间的并发访问
	srv *http.Server // 由 Run 构造并持有，供 Shutdown 优雅关闭
}

// NewRouter 创建路由器
func NewRouter(
	// Handler
	authHandler *handler.AuthHandler,
	knowledgeBaseHandler *handler.KnowledgeBaseHandler,
	sessionHandler *handler.SessionHandler,
	messageHandler *handler.MessageHandler,
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
	semanticHandler *handler.SemanticHandler,
	auditHandler *handler.AuditHandler,
	traceHandler *handler.TraceHandler,
	webHandler *web.Handler,
	// Middleware
	authMiddleware *middleware.AuthMiddleware,
	tenantMiddleware *middleware.TenantMiddleware,
	auditMiddleware *middleware.AuditMiddleware,
) *Router {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(middleware.CORS())
	engine.Use(middleware.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.Trace())
	// 审计须在 Trace 之后：复用其注入的 request_id 为每个业务请求留痕（异步落库）。
	engine.Use(auditMiddleware.Apply())

	return &Router{
		engine:               engine,
		authHandler:          authHandler,
		knowledgeBaseHandler: knowledgeBaseHandler,
		sessionHandler:       sessionHandler,
		messageHandler:       messageHandler,
		tenantHandler:        tenantHandler,
		agentHandler:         agentHandler,
		registryAgentHandler: registryAgentHandler,
		graphHandler:         graphHandler,
		modelHandler:         modelHandler,
		taskHandler:          taskHandler,
		ragOptimizerHandler:  ragOptimizerHandler,
		guardrailHandler:     guardrailHandler,
		evaluationHandler:    evaluationHandler,
		qualityHandler:       qualityHandler,
		dataSourceHandler:    dataSourceHandler,
		semanticHandler:      semanticHandler,
		auditHandler:         auditHandler,
		traceHandler:         traceHandler,
		webHandler:           webHandler,
		authMiddleware:       authMiddleware,
		tenantMiddleware:     tenantMiddleware,
		auditMiddleware:      auditMiddleware,
	}
}

// Setup 设置路由。
//
// 限流器〔INF-2〕由组合根（cmd/server）构建后注入：router 属 handler 层，不应直接依赖
// infrastructure/cache/redis 拿 *redis.Client（Clean Architecture 依赖方向），故 Redis 令牌桶
// 的装配上移到 main，本层只消费 *middleware.RateLimiter。
func (r *Router) Setup(rateLimiter *middleware.RateLimiter, idempotency *middleware.Idempotency) {
	// 全局兜底限流对每个请求生效（/health 与静态资源除外，见 Global 内部跳过逻辑）。
	// 优先 Redis 令牌桶（多实例共享配额），Redis 未就绪时降级为进程内令牌桶。
	r.rateLimiter = rateLimiter
	r.idempotency = idempotency
	r.engine.Use(r.rateLimiter.Global())

	// 健康检查
	r.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "服务器运行正常",
			"version": "3.0.0-clean-arch",
		})
	})

	// API 路由组
	api := r.engine.Group("/api/v1")
	{
		// 公开路由
		r.setupAuthRoutes(api)

		// 需要认证的路由
		auth := api.Group("")
		auth.Use(r.authMiddleware.Apply())
		// 幂等中间件〔M6〕：对写方法且携带 Idempotency-Key 的请求做请求级去重（重放缓存响应）。
		auth.Use(r.idempotency.Apply())
		{
			r.setupUserRoutes(auth)
			r.setupTenantRoutes(auth)
			r.setupSessionRoutes(auth)
			r.setupMessageRoutes(auth)
			r.setupChatRoutes(auth)
			r.setupModelRoutes(auth)
			r.setupAgentRoutes(auth)
			r.setupEvaluationRoutes(auth)
			r.setupQualityRoutes(auth)
			r.setupDataSourceRoutes(auth)
			r.setupSemanticRoutes(auth)
			r.setupAuditRoutes(auth)
			r.setupTraceRoutes(auth)

			// 安全护栏路由：需要认证但不依赖租户上下文
			r.setupGuardrailRoutes(auth)
		}

		// 需要认证 + 租户的路由
		tenant := api.Group("")
		tenant.Use(r.authMiddleware.Apply())
		tenant.Use(r.tenantMiddleware.Apply())
		tenant.Use(r.idempotency.Apply()) // 幂等去重〔M6〕（tenant 组，去重键含 tenant_id）
		{
			r.setupKBRoutes(tenant)
			r.setupGraphRoutes(tenant)

			// 知识库搜索（在 tenant 组内，但不需要 kb_id 参数）
			tenant.POST("/knowledge/search", r.knowledgeBaseHandler.SearchKnowledge)

			// 检索优化路由：handler 读取 tenant_id，必须在租户组内
			r.setupRAGOptimizerRoutes(tenant)
		}
	}

	// 静态文件服务（生产环境）
	r.setupWebRoutes()
}

// setupAuthRoutes 设置认证路由
func (r *Router) setupAuthRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		// 登录/注册单独收紧限流〔INF-2〕：专防账号暴力破解。
		login := r.rateLimiter.Login()
		auth.POST("/register", login, r.authHandler.Register)
		auth.POST("/login", login, r.authHandler.Login)
		auth.POST("/refresh", r.authHandler.RefreshToken)
		auth.POST("/logout", r.authHandler.Logout)
	}
}

// setupUserRoutes 设置用户路由
func (r *Router) setupUserRoutes(api *gin.RouterGroup) {
	user := api.Group("/user")
	{
		user.GET("/profile", r.authHandler.GetProfile)
	}
}

// setupTenantRoutes 设置租户路由
func (r *Router) setupTenantRoutes(api *gin.RouterGroup) {
	tenants := api.Group("/tenants")
	{
		tenants.POST("", r.tenantHandler.CreateTenant)
		tenants.GET("", r.tenantHandler.ListTenants)
		tenants.GET("/:id", r.tenantHandler.GetTenant)
		tenants.PUT("/:id", r.tenantHandler.UpdateTenant)
		tenants.DELETE("/:id", r.tenantHandler.DeleteTenant)
		tenants.GET("/:id/stats", r.tenantHandler.GetTenantStats)
	}
}

// setupSessionRoutes 设置会话路由
func (r *Router) setupSessionRoutes(api *gin.RouterGroup) {
	sessions := api.Group("/sessions")
	{
		sessions.POST("", r.sessionHandler.CreateSession)
		sessions.GET("", r.sessionHandler.ListSessions)
		sessions.GET("/:id", r.sessionHandler.GetSession)
		sessions.GET("/:id/detail", r.sessionHandler.GetSessionDetail)
		sessions.PUT("/:id", r.sessionHandler.UpdateSession)
		sessions.DELETE("/:id", r.sessionHandler.DeleteSession)
		sessions.POST("/:id/archive", r.sessionHandler.ArchiveSession)
		sessions.POST("/:id/activate", r.sessionHandler.ActivateSession)
	}
}

// setupMessageRoutes 设置消息路由
func (r *Router) setupMessageRoutes(api *gin.RouterGroup) {
	messages := api.Group("/messages")
	{
		messages.POST("", r.messageHandler.CreateMessage)
		messages.GET("", r.messageHandler.ListMessages)
		messages.GET("/:id", r.messageHandler.GetMessage)
		messages.PUT("/:id", r.messageHandler.UpdateMessage)
		messages.DELETE("/:id", r.messageHandler.DeleteMessage)
	}
}

// setupChatRoutes 设置聊天路由（已废弃，使用 Agent 路由代替）
func (r *Router) setupChatRoutes(api *gin.RouterGroup) {
	// 已废弃：原始 /api/v1/chat 已迁移到 /api/v1/agent/knowledge/stream
	// 保留此方法以保持路由结构兼容
}

// setupKBRoutes 设置知识库路由
func (r *Router) setupKBRoutes(api *gin.RouterGroup) {
	kbs := api.Group("/knowledge-bases")
	{
		// 知识库CRUD
		kbs.POST("", r.knowledgeBaseHandler.CreateKnowledgeBase)
		kbs.GET("", r.knowledgeBaseHandler.ListKnowledgeBases)
		kbs.GET("/:id", r.knowledgeBaseHandler.GetKnowledgeBase)
		kbs.PUT("/:id", r.knowledgeBaseHandler.UpdateKnowledgeBase)
		kbs.DELETE("/:id", r.knowledgeBaseHandler.DeleteKnowledgeBase)
		kbs.GET("/:id/stats", r.knowledgeBaseHandler.GetStats)
		// 为历史文档补建知识图谱（复用已存分块，幂等）
		kbs.POST("/:id/graph/rebuild", r.knowledgeBaseHandler.RebuildGraph)

		// 知识库文档管理
		kbs.GET("/:id/knowledge", r.knowledgeBaseHandler.GetKnowledgeList)
		kbs.POST("/:id/knowledge/file", r.knowledgeBaseHandler.UploadKnowledgeFile)
		// 上传前预检：文件是否已存在（防止重传相同文件）
		kbs.POST("/:id/knowledge/check", r.knowledgeBaseHandler.CheckKnowledgeFile)
		// pending 路由必须在 :kid 之前，因为 pending 会被当作 :kid 匹配
		kbs.GET("/:id/knowledge/pending", r.knowledgeBaseHandler.GetPendingKnowledgeList)
		kbs.GET("/:id/knowledge/pending/status", r.knowledgeBaseHandler.GetPendingKnowledgeList)
		kbs.GET("/:id/knowledge/:kid", r.knowledgeBaseHandler.GetKnowledgeDetail)
		kbs.GET("/:id/knowledge/:kid/status", r.knowledgeBaseHandler.GetKnowledgeStatus)
		kbs.DELETE("/:id/knowledge/:knowledge_id", r.knowledgeBaseHandler.DeleteKnowledge)
		// 文档启用/停用（MySQL 权威 + 检索后过滤）：停用后不再被检索命中
		kbs.PUT("/:id/knowledge/:knowledge_id/enable", r.knowledgeBaseHandler.EnableKnowledge)
		kbs.PUT("/:id/knowledge/:knowledge_id/disable", r.knowledgeBaseHandler.DisableKnowledge)

		// 分块管理
		kbs.GET("/:id/chunks", r.knowledgeBaseHandler.GetChunks)
		kbs.GET("/:id/chunks/:cid", r.knowledgeBaseHandler.GetChunkDetail)
		kbs.PUT("/:id/chunks/:cid", r.knowledgeBaseHandler.UpdateChunk)
		kbs.DELETE("/:id/chunks/:cid", r.knowledgeBaseHandler.DeleteChunk)
		// 知识图谱路由（需要图谱服务）
		if r.graphHandler != nil {
			kbs.GET("/:id/graph", r.graphHandler.GetGraph)
			kbs.POST("/:id/graph/search", r.graphHandler.SearchNode)
			kbs.GET("/:id/graph/nodes/:nodeId", r.graphHandler.GetNodeDetail)
			kbs.POST("/:id/graph/nodes", r.graphHandler.AddNode)
			kbs.PUT("/:id/graph/nodes/:nodeId", r.graphHandler.UpdateNode)
			kbs.DELETE("/:id/graph/nodes/:nodeId", r.graphHandler.DeleteNode)
			kbs.POST("/:id/graph/relations", r.graphHandler.AddRelation)
			kbs.PUT("/:id/graph/relations/:relationId", r.graphHandler.UpdateRelation)
			kbs.DELETE("/:id/graph/relations/:relationId", r.graphHandler.DeleteRelation)
			kbs.GET("/:id/graph/relation-types", r.graphHandler.GetRelationTypes)
			kbs.DELETE("/:id/graph", r.graphHandler.DeleteGraph)
		}
	}
}

// setupModelRoutes 设置模型路由
func (r *Router) setupModelRoutes(api *gin.RouterGroup) {
	models := api.Group("/models")
	{
		models.GET("", r.modelHandler.ListModels)
		models.GET("/:id", r.modelHandler.GetModel)
		models.GET("/default", r.modelHandler.GetDefaultModel)
		models.POST("", r.modelHandler.CreateModel)
		models.PUT("/:id", r.modelHandler.UpdateModel)
		models.DELETE("/:id", r.modelHandler.DeleteModel)
		models.GET("/info", r.modelHandler.GetModelInfo)

		// 聊天（昂贵端点限流〔INF-2〕：直连 LLM）
		expensive := r.rateLimiter.Expensive()
		models.POST("/chat", expensive, r.modelHandler.Chat)
		models.POST("/chat/stream", expensive, r.modelHandler.ChatStream)
	}
}

// setupAgentRoutes 设置 Agent 路由
func (r *Router) setupAgentRoutes(api *gin.RouterGroup) {
	agent := api.Group("/agent")
	// 昂贵端点限流〔INF-2〕：Agent 流式对话/Text2SQL 等直连 LLM，防并发打爆与成本失控。
	agent.Use(r.rateLimiter.Expensive())
	{
		// Knowledge Chat（知识库对话，带持久化）
		agent.POST("/knowledge/stream", r.agentHandler.KnowledgeStream)

		// Text2SQL 接口
		agent.POST("/text2sql/stream", r.agentHandler.Text2SQLStream)
		agent.GET("/text2sql/schema", r.agentHandler.GetDatabaseSchema)

		// 配置
		agent.GET("/config", r.agentHandler.GetConfig)
		agent.POST("/config", r.agentHandler.UpdateConfig)

		// 生成式 UI 交互回调：按 surface 绑定状态分页取数（Pagination/Filter）
		agent.GET("/ui/surfaces/:surface/page", r.agentHandler.GetUISurfacePage)

		// 危险操作人机确认 resume：确认卡片携 pending_action_id + token 恢复执行
		agent.POST("/operations/confirm", r.agentHandler.ConfirmOperation)

		// 进度
		agent.GET("/progress/:session_id", r.agentHandler.GetProgress)

		// 相似度
		agent.POST("/similarity", r.agentHandler.CalculateSimilarity)
	}

	// Agent 注册中心路由
	if r.registryAgentHandler != nil {
		registry := api.Group("/agents")
		{
			// Agent 发现
			registry.GET("", r.registryAgentHandler.ListAgents)        // 列出所有 Agent
			registry.GET("/search", r.registryAgentHandler.FindAgents) // 搜索 Agent
			registry.GET("/:id", r.registryAgentHandler.GetAgent)      // 获取 Agent 详情

			// Agent 执行
			registry.POST("/:id/chat", r.registryAgentHandler.ChatExecuteByID) // 通过 ID 执行
			registry.POST("/chat/:name", r.registryAgentHandler.ChatExecute)   // 通过名称执行
		}
	}
}

// setupGraphRoutes 设置图谱路由
func (r *Router) setupGraphRoutes(api *gin.RouterGroup) {
	graph := api.Group("/graph")
	{
		// 统计
		graph.GET("/stats", r.graphHandler.GetStats)
		graph.GET("/stats/relations", r.graphHandler.GetRelationStats)

		// 节点管理
		graph.POST("/node", r.graphHandler.AddNode)

		// 关系管理
		graph.POST("/relation", r.graphHandler.AddRelation)
	}
}

// setupRAGOptimizerRoutes 设置检索优化路由
func (r *Router) setupRAGOptimizerRoutes(api *gin.RouterGroup) {
	rag := api.Group("/rag")
	// 昂贵端点限流〔INF-2〕：检索优化各路径含多次 LLM 调用。
	rag.Use(r.rateLimiter.Expensive())
	{
		// HyDE 检索
		rag.POST("/hyde", r.ragOptimizerHandler.HyDERetrieve)

		// 查询重写
		rag.POST("/query/rewrite", r.ragOptimizerHandler.RewriteQuery)

		// 多跳检索
		rag.POST("/multi-hop", r.ragOptimizerHandler.MultiHopRetrieve)

		// 综合优化检索
		rag.POST("/optimized", r.ragOptimizerHandler.OptimizedRetrieve)

		// 配置管理
		rag.GET("/config/hyde", r.ragOptimizerHandler.GetHyDEConfig)
		rag.GET("/config/rewrite", r.ragOptimizerHandler.GetRewriteConfig)
		rag.GET("/config/multi-hop", r.ragOptimizerHandler.GetMultiHopConfig)
		rag.GET("/config/optimizer", r.ragOptimizerHandler.GetOptimizerConfig)
	}
}

// setupGuardrailRoutes 设置安全护栏路由
func (r *Router) setupGuardrailRoutes(api *gin.RouterGroup) {
	guardrail := api.Group("/guardrail")
	{
		// 输入检查
		guardrail.POST("/input/check", r.guardrailHandler.CheckInput)
		guardrail.POST("/input/sanitize", r.guardrailHandler.SanitizeInput)

		// 输出检查
		guardrail.POST("/output/check", r.guardrailHandler.CheckOutput)
		guardrail.POST("/output/sanitize", r.guardrailHandler.SanitizeOutput)

		// 越狱检测
		guardrail.POST("/jailbreak/check", r.guardrailHandler.CheckJailbreak)

		// 综合检查
		guardrail.POST("/full-check", r.guardrailHandler.FullInputCheck)
		guardrail.POST("/check-both", r.guardrailHandler.CheckInputAndOutput)

		// 快捷方法
		guardrail.POST("/quick-check", r.guardrailHandler.QuickCheck)
		guardrail.POST("/quick-sanitize", r.guardrailHandler.QuickSanitize)
		guardrail.POST("/is-jailbreak", r.guardrailHandler.IsJailbreak)

		// 配置和建议
		guardrail.GET("/config/default", r.guardrailHandler.GetDefaultConfig)
		guardrail.POST("/recommendation", r.guardrailHandler.GetRecommendation)
	}
}

// setupEvaluationRoutes 设置评测路由
func (r *Router) setupEvaluationRoutes(api *gin.RouterGroup) {
	eval := api.Group("/evaluation")
	{
		// 数据集管理
		datasets := eval.Group("/datasets")
		{
			datasets.GET("", r.evaluationHandler.ListDatasets)
			datasets.POST("", r.evaluationHandler.CreateDataset)
			datasets.GET("/:id", r.evaluationHandler.GetDatasetDetail)
			datasets.PUT("/:id", r.evaluationHandler.UpdateDataset)
			datasets.DELETE("/:id", r.evaluationHandler.DeleteDataset)

			// 样本管理
			datasets.GET("/:id/samples", r.evaluationHandler.ListSamples)
			datasets.POST("/:id/samples", r.evaluationHandler.AddSamples)
			datasets.DELETE("/:id/samples/:sample_id", r.evaluationHandler.DeleteSample)
		}

		// 评测任务
		tasks := eval.Group("/tasks")
		{
			tasks.GET("", r.evaluationHandler.ListTasks)
			tasks.POST("", r.evaluationHandler.CreateTask)
			tasks.GET("/:id", r.evaluationHandler.GetTask)
			tasks.DELETE("/:id", r.evaluationHandler.DeleteTask)
			tasks.GET("/:id/stream", r.evaluationHandler.StreamTask)
		}

		// 评测结果
		results := eval.Group("/results")
		{
			results.GET("", r.evaluationHandler.ListResults)
			results.GET("/:task_id", r.evaluationHandler.GetResults)
			results.GET("/:task_id/qa", r.evaluationHandler.GetQAResults)
		}

		// 可用指标目录（注册表驱动，按评测类型过滤）
		eval.GET("/graders", r.evaluationHandler.ListGraders)
	}
}

// setupQualityRoutes 设置数据质量管理中心路由
func (r *Router) setupQualityRoutes(api *gin.RouterGroup) {
	quality := api.Group("/quality")
	{
		// 评估与清洗
		quality.POST("/evaluate/structured", r.qualityHandler.EvaluateStructured)
		quality.POST("/evaluate/unstructured", r.qualityHandler.EvaluateUnstructured)
		quality.POST("/evaluate/datasource", r.qualityHandler.EvaluateDatasource)
		quality.POST("/clean", r.qualityHandler.CleanData)
		quality.GET("/dimensions", r.qualityHandler.ListDimensions)

		// 历史质检记录
		quality.GET("/records", r.qualityHandler.ListRecords)
		quality.GET("/records/:id", r.qualityHandler.GetRecord)
		quality.DELETE("/records/:id", r.qualityHandler.DeleteRecord)
	}
}

// setupDataSourceRoutes 设置外部数据源管理路由
func (r *Router) setupDataSourceRoutes(api *gin.RouterGroup) {
	ds := api.Group("/datasources")
	{
		// CRUD
		ds.GET("", r.dataSourceHandler.List)
		ds.POST("", r.dataSourceHandler.Create)
		ds.GET("/:id", r.dataSourceHandler.Get)
		ds.PUT("/:id", r.dataSourceHandler.Update)
		ds.DELETE("/:id", r.dataSourceHandler.Delete)

		// 测试连接（表单配置或已存 id，临时连接不进缓存）
		ds.POST("/test", r.dataSourceHandler.TestConnection)

		// schema 探查
		ds.GET("/:id/tables", r.dataSourceHandler.ListTables)
		ds.GET("/:id/tables/:table", r.dataSourceHandler.DescribeTable)
	}
}

// setupSemanticRoutes 设置指标语义层「建模」路由（受治理查询的进料线）。
func (r *Router) setupSemanticRoutes(api *gin.RouterGroup) {
	if r.semanticHandler == nil {
		return
	}
	sm := api.Group("/semantic-models")
	{
		sm.GET("", r.semanticHandler.List)
		sm.POST("", r.semanticHandler.Create)
		sm.GET("/:id", r.semanticHandler.Get)
		sm.PUT("/:id", r.semanticHandler.Update)
		sm.POST("/:id/publish", r.semanticHandler.Publish)
		sm.POST("/:id/deprecate", r.semanticHandler.Deprecate)
		// 画像↔ValueMap 一致性诊断（只读）：检出死映射/覆盖缺口。
		sm.GET("/:id/valuemap-diagnosis", r.semanticHandler.DiagnoseValueMaps)
	}
	// 覆盖率埋点只读视图（独立路径，避免与 /semantic-models/:id 的静态/参数段冲突）。
	api.GET("/semantic-coverage", r.semanticHandler.Coverage)
}

// setupAuditRoutes 设置请求审计查询路由（只读）
func (r *Router) setupAuditRoutes(api *gin.RouterGroup) {
	if r.auditHandler == nil {
		return
	}
	audit := api.Group("/audit-logs")
	{
		audit.GET("", r.auditHandler.ListAuditLogs)
		audit.GET("/stats", r.auditHandler.GetAuditStats)
		audit.GET("/:id", r.auditHandler.GetAuditLog)
	}
}

// setupTraceRoutes 设置调用链追踪查询路由（只读）
func (r *Router) setupTraceRoutes(api *gin.RouterGroup) {
	if r.traceHandler == nil {
		return
	}
	traces := api.Group("/traces")
	{
		traces.GET("", r.traceHandler.ListTraces)
		traces.GET("/:traceID", r.traceHandler.GetTrace)
	}
}

// GetEngine 获取Gin引擎
func (r *Router) GetEngine() *gin.Engine {
	return r.engine
}

// Run 运行服务器。
//
// 〔INF-7〕显式构造 http.Server 并设超时，收敛 Slowloris/慢连接 DoS 面：
//   - ReadHeaderTimeout：慢速发送请求头即被切断——Slowloris 的核心防护；
//   - IdleTimeout：keep-alive 空闲连接回收，防连接耗尽；
//   - MaxHeaderBytes：限制超大请求头。
//
// ReadTimeout/WriteTimeout 有意留 0（不限）：本服务含大文件上传（长时读体）与
// SSE 流式端点（长时写体），硬超时会误伤二者；Slowloris 已由 ReadHeaderTimeout 覆盖。
func (r *Router) Run(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           r.engine,
		ReadHeaderTimeout: time.Duration(envIntDefault("HTTP_READ_HEADER_TIMEOUT_SEC", 15)) * time.Second,
		IdleTimeout:       time.Duration(envIntDefault("HTTP_IDLE_TIMEOUT_SEC", 120)) * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	r.mu.Lock()
	r.srv = srv
	r.mu.Unlock()

	// ListenAndServe 在 Shutdown 后返回 ErrServerClosed，属正常收尾，向上层折叠为 nil。
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown 优雅关闭 HTTP 服务器：停止接收新连接并等待在途请求在 ctx 截止前完成。
// 在 Run 尚未构造 srv（服务器还没起来）时为空操作。
func (r *Router) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	srv := r.srv
	r.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// envIntDefault 读取正整型环境变量，缺省/非法时回退默认值。
func envIntDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

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

// setupWebRoutes 设置前端静态文件路由
func (r *Router) setupWebRoutes() {
	// 静态资源（assets）
	r.engine.Static("/assets", "./dist/assets")

	// SPA fallback：所有未匹配的路由返回 index.html
	r.engine.NoRoute(func(c *gin.Context) {
		// 跳过 API 路径
		path := c.Request.URL.Path
		if len(path) >= 4 && path[:4] == "/api" {
			// 与统一封套一致（含 request_id），避免 API 404 走异形 body（〔M7〕）。
			c.JSON(http.StatusNotFound, gin.H{
				"code":       http.StatusNotFound,
				"message":    "Not Found",
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.File("./dist/index.html")
	})
}
