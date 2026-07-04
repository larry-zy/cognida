// Package router 提供路由配置
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"link/internal/handler"
	"link/internal/handler/middleware"
	"link/internal/handler/web"
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
	chatHandler          *handler.ChatHandler
	tenantHandler        *handler.TenantHandler
	agentHandler         *handler.AgentHandler
	registryAgentHandler *handler.RegistryAgentHandler
	graphHandler         *handler.GraphHandler
	modelHandler         *handler.ModelHandler
	taskHandler          *handler.TaskHandler
	ragOptimizerHandler  *handler.RAGOptimizerHandler
	guardrailHandler     *handler.GuardrailHandler
	evaluationHandler    *handler.EvaluationHandler
	webHandler           *web.Handler
	authMiddleware       *middleware.AuthMiddleware
	tenantMiddleware     *middleware.TenantMiddleware
}

// NewRouter 创建路由器
func NewRouter(
	// Handler
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
	webHandler *web.Handler,
	// Middleware
	authMiddleware *middleware.AuthMiddleware,
	tenantMiddleware *middleware.TenantMiddleware,
) *Router {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(middleware.CORS())
	engine.Use(middleware.Recovery())
	engine.Use(middleware.Logger())
	engine.Use(middleware.Trace())

	return &Router{
		engine:               engine,
		authHandler:          authHandler,
		knowledgeBaseHandler: knowledgeBaseHandler,
		sessionHandler:       sessionHandler,
		messageHandler:       messageHandler,
		chatHandler:          chatHandler,
		tenantHandler:        tenantHandler,
		agentHandler:         agentHandler,
		registryAgentHandler: registryAgentHandler,
		graphHandler:         graphHandler,
		modelHandler:         modelHandler,
		taskHandler:          taskHandler,
		ragOptimizerHandler:  ragOptimizerHandler,
		guardrailHandler:     guardrailHandler,
		evaluationHandler:    evaluationHandler,
		webHandler:           webHandler,
		authMiddleware:       authMiddleware,
		tenantMiddleware:     tenantMiddleware,
	}
}

// Setup 设置路由
func (r *Router) Setup() {
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
		{
			r.setupUserRoutes(auth)
			r.setupTenantRoutes(auth)
			r.setupSessionRoutes(auth)
			r.setupMessageRoutes(auth)
			r.setupChatRoutes(auth)
			r.setupModelRoutes(auth)
			r.setupAgentRoutes(auth)
			r.setupEvaluationRoutes(auth)
		}

		// 需要认证 + 租户的路由
		tenant := api.Group("")
		tenant.Use(r.authMiddleware.Apply())
		tenant.Use(r.tenantMiddleware.Apply())
		{
			r.setupKBRoutes(tenant)
			r.setupGraphRoutes(tenant)

			// 知识库搜索（在 tenant 组内，但不需要 kb_id 参数）
			tenant.POST("/knowledge/search", r.knowledgeBaseHandler.SearchKnowledge)
		}

		// 检索优化路由
		r.setupRAGOptimizerRoutes(api)

		// 安全护栏路由
		r.setupGuardrailRoutes(api)
	}

	// 静态文件服务（生产环境）
	r.setupWebRoutes()
}

// setupAuthRoutes 设置认证路由
func (r *Router) setupAuthRoutes(api *gin.RouterGroup) {
	auth := api.Group("/auth")
	{
		auth.POST("/register", r.authHandler.Register)
		auth.POST("/login", r.authHandler.Login)
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

		// 知识库文档管理
		kbs.GET("/:id/knowledge", r.knowledgeBaseHandler.GetKnowledgeList)
		kbs.POST("/:id/knowledge/file", r.knowledgeBaseHandler.UploadKnowledgeFile)
		// pending 路由必须在 :kid 之前，因为 pending 会被当作 :kid 匹配
		kbs.GET("/:id/knowledge/pending", r.knowledgeBaseHandler.GetPendingKnowledgeList)
		kbs.GET("/:id/knowledge/pending/status", r.knowledgeBaseHandler.GetPendingKnowledgeList)
		kbs.GET("/:id/knowledge/:kid", r.knowledgeBaseHandler.GetKnowledgeDetail)
		kbs.GET("/:id/knowledge/:kid/status", r.knowledgeBaseHandler.GetKnowledgeStatus)
		kbs.DELETE("/:id/knowledge/:knowledge_id", r.knowledgeBaseHandler.DeleteKnowledge)

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

		// 聊天
		models.POST("/chat", r.modelHandler.Chat)
		models.POST("/chat/stream", r.modelHandler.ChatStream)
	}
}

// setupAgentRoutes 设置 Agent 路由
func (r *Router) setupAgentRoutes(api *gin.RouterGroup) {
	agent := api.Group("/agent")
	{
		// AgenticRAG 聊天（原有接口，已废弃）
		// agent.POST("/chat", r.agentHandler.Chat)
		// agent.POST("/chat/stream", r.agentHandler.ChatStream)

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
	}
}

// GetEngine 获取Gin引擎
func (r *Router) GetEngine() *gin.Engine {
	return r.engine
}

// Run 运行服务器
func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
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
			c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
			return
		}
		c.File("./dist/index.html")
	})
}
