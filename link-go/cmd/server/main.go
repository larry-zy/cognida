// Package main 是应用程序的入口点
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"link/cmd/wire"

	rediscache "link/internal/infrastructure/cache/redis"
	"link/internal/infrastructure/config"
	llmchat "link/internal/infrastructure/llm/chat"
	"link/internal/infrastructure/mcp"
	obstel "link/internal/infrastructure/observability"
	agenttelemetry "link/internal/service/agent/telemetry"
	"link/internal/repository/milvus"
	"link/internal/repository/mysql"
	neo4jrepo "link/internal/repository/neo4j"
	agentadapters "link/internal/service/agent/adapters"
	experiencesvc "link/internal/service/agent/experience"
	"link/internal/service/agent/genui"
	agentinit "link/internal/service/agent/initializer"
	"link/internal/service/agent/pendingaction"
	"link/internal/service/agent/resultstore"
	"link/internal/service/agent/uibinding"
	"link/internal/service/agent/semanticcache"
	skilltools "link/internal/service/agent/skills/tools"
	"link/internal/service/agent/termgrounding"
	ragtool "link/internal/service/agent/tools"

	neo4jsdk "github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/joho/godotenv"
)

// loadEnvFile 从多个可能的路径加载 .env 文件
func loadEnvFile() {
	// 获取可执行文件所在目录
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execDir := filepath.Dir(execPath)

	// 尝试多个路径
	paths := []string{
		".env",                               // 当前工作目录
		filepath.Join(execDir, ".env"),       // 可执行文件所在目录
		filepath.Join(execDir, "..", ".env"), // 可执行文件的上级目录
	}

	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			log.Printf("✅ 已加载环境配置: %s", path)
			return
		}
	}
}

func main() {
	// 加载 .env 文件（从多个可能的路径尝试）
	loadEnvFile()

	// 加载配置
	cfg := config.LoadConfig()

	// JWT 密钥强校验：缺失/占位符/过短一律拒绝启动（fail-closed），
	// 杜绝弱密钥进入生产被离线爆破后自签合法 token。
	// DEV_MODE 下降级为告警放行——dev 认证本就在 middleware 处被整体绕过
	// （见 handler/middleware/auth.go），弱密钥在此无实际暴露面，不该阻塞本地启动。
	if err := cfg.JWT.Validate(); err != nil {
		if os.Getenv("DEV_MODE") == "true" {
			log.Printf("⚠️  [SECURITY] JWT_SECRET 校验未通过（DEV_MODE 放行）: %v", err)
		} else {
			log.Fatalf("❌ %v", err)
		}
	}

	// 初始化数据库
	log.Println("🔧 初始化数据库...")
	db, err := mysql.InitGORMDatabase(cfg.Database, "info")
	if err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}
	sqlDB, _ := db.DB()

	// 初始化 Milvus（如果配置了）
	if cfg.Milvus != nil && cfg.Milvus.Host != "" {
		log.Println("🔧 初始化 Milvus...")
		if err := milvus.InitMilvus(cfg.Milvus); err != nil {
			log.Printf("⚠️  Milvus 初始化失败: %v", err)
		}
		defer func() {
			if err := milvus.CloseMilvus(); err != nil {
				log.Printf("❌ Milvus 关闭失败: %v", err)
			}
		}()
	}

	// 初始化 Neo4j（如果配置了）
	var neo4jDriver interface{}
	if cfg.Neo4j != nil && cfg.Neo4j.URI != "" {
		ctx := context.Background()
		neo4jCfg := wire.Neo4jConfig{
			URI:      cfg.Neo4j.URI,
			Username: cfg.Neo4j.Username,
			Password: cfg.Neo4j.Password,
		}
		neo4jDriver, err = wire.CreateDriver(ctx, neo4jCfg)
		if err != nil {
			log.Printf("⚠️  Neo4j 连接失败: %v", err)
		} else {
			defer func() {
				if d, ok := neo4jDriver.(interface{ Close(context.Context) error }); ok {
					d.Close(context.Background())
					log.Println("🔌 关闭 Neo4j driver...")
				}
			}()
		}
	}

	// 使用 Wire 初始化应用
	log.Println("🔧 初始化应用程序...")
	app, err := wire.InitializeApp(db)
	if err != nil {
		log.Fatalf("❌ 应用初始化失败: %v", err)
	}

	// 会话经验自动沉淀 Worker（在 Agent 初始化块内用同一 toolModel 装配后启动）。
	var experienceWorker *experiencesvc.Worker

	// ==================== 调用链追踪（自建 in-app trace）====================
	// AGENT_TRACING_ENABLED=true 时：注册真实 TracerProvider（否则默认 no-op provider
	// 会丢弃所有 span），并把「全部 agent」经 SpecRegistry 装饰钩子统一叠加遥测中间件，
	// 使 agent.chat/工具调用/子代理委派/编排的 span 经 MySQL exporter 落到 trace_spans。
	var traceShutdown func(context.Context) error
	if os.Getenv("AGENT_TRACING_ENABLED") == "true" {
		shutdown, err := obstel.InitTracing(mysql.NewTraceRepository(db))
		if err != nil {
			log.Printf("⚠️  调用链追踪初始化失败: %v", err)
		} else {
			traceShutdown = shutdown
			if app.AgentRegistry != nil {
				app.AgentRegistry.SetWrap(agenttelemetry.WrapAgent)
			}
			log.Println("✅ 调用链追踪已启用（TracerProvider + MySQL exporter + 全 agent 遥测）")
		}
	}

	// 初始化 Agent 注册中心
	if app.AgentRegistry != nil && app.ChatConfig != nil && app.ChatConfig.APIKey != "" {
		log.Println("🔧 初始化 Agent 注册中心...")
		// 注入消息仓储：Data Agent 据此从 messages 表回放会话历史，具备跨轮对话记忆。
		// Initializer 待 ToolRegistry 构造完成后再创建（工具注册表经构造期显式注入）。
		msgRepo := mysql.NewMessageRepository(db)

		// 创建 ToolModel
		ctx := context.Background()
		toolModelConfig := &llmchat.ChatConfig{
			Source:    "remote",
			APIKey:    app.ChatConfig.APIKey,
			BaseURL:   app.ChatConfig.BaseURL,
			ModelName: app.ChatConfig.ModelName,
			Provider:  app.ChatConfig.Provider,
		}
		toolModel, err := llmchat.NewToolCallingChatModel(ctx, toolModelConfig)
		if err != nil {
			log.Printf("⚠️  创建 ToolModel 失败: %v", err)
		} else {
			// ==================== 装配会话经验自动沉淀 Worker ====================
			// 复用 toolModel 作蒸馏 LLM、db 建仓储、GraphService 的图谱仓储写经验节点。
			// 总开关 EXPERIENCE_DISTILL_ENABLED 默认关；关闭时仍装配但不启动。
			expCfg := experiencesvc.ConfigFromEnv()
			if expCfg.Enabled {
				var graphSink *experiencesvc.GraphSink
				if app.GraphService != nil {
					graphSink = experiencesvc.NewGraphSink(app.GraphService.GetGraphRepo())
				}
				experienceWorker = experiencesvc.NewWorker(
					expCfg,
					mysql.NewExperienceRepository(db),
					msgRepo,
					experiencesvc.NewSummarizer(toolModel),
					graphSink,
					experiencesvc.NewSkillSink(experiencesvc.SkillDirFromEnv()),
				)
			}

			// ==================== 装配工具依赖（ToolDeps）====================
			// 依赖在构造期显式注入 NewToolRegistry，替代旧的包级 Init* 逐一全局注入。

			// Result Store（data-by-reference）：完整结果集落库、回灌 LLM 只给信封。
			// Redis 可用则用 Redis 后端；否则降级为进程内内存后端（单实例可用）。
			var rs resultstore.Store
			if rediscache.Client != nil {
				rs = resultstore.NewRedisStore(rediscache.Client)
				log.Println("✅ Result Store 使用 Redis 后端")
			} else {
				rs = resultstore.NewMemoryStore()
				log.Println("⚠️  Redis 未配置，Result Store 降级为进程内内存后端")
			}

			// 初始化 UI 交互绑定存储（surface ↔ result_id + token，会话 TTL）：
			// 支撑 render_ui 的 Filter/Pagination 等组件回调路由。经 ToolDeps.UIBinding
			// 显式注入（去 uibinding 包级单例），render_ui 与 handler 回调路由共用同一实例。
			var uiBindingStore uibinding.Store
			if rediscache.Client != nil {
				uiBindingStore = uibinding.NewRedisStore(rediscache.Client)
				log.Println("✅ UI 交互绑定存储使用 Redis 后端")
			} else {
				uiBindingStore = uibinding.NewMemoryStore()
				log.Println("⚠️  Redis 未配置，UI 交互绑定存储降级为进程内内存后端")
			}

			// 受信查询缓存（Verified/Golden Query）：键含语义模型版本，版本变更即失效。
			// 复用 Result Store 的 Redis 客户端；Redis 不可用则降级为进程内内存缓存。
			var sc semanticcache.Cache
			if rediscache.Client != nil {
				sc = semanticcache.NewRedisCache(rediscache.Client)
			} else {
				sc = semanticcache.NewMemoryCache()
			}

			// 待确认操作存储（sql_mutate / etl_run / data_export 的人工确认闭环）：
			// Redis 可用则 Redis，否则进程内内存。
			var pendingStore pendingaction.Store
			if rediscache.Client != nil {
				pendingStore = pendingaction.NewRedisStore(rediscache.Client)
				log.Println("✅ 待确认操作存储使用 Redis 后端")
			} else {
				pendingStore = pendingaction.NewMemoryStore()
				log.Println("⚠️  Redis 未配置，待确认操作存储降级为进程内内存后端")
			}

			deps := ragtool.ToolDeps{
				SQLDB:       db,
				ResultStore: rs,
				UIBinding:   uiBindingStore,
				// 指标语义层（NL2Semantics）：语义模型仓储 + 受信查询缓存 + 覆盖埋点
				SemanticRepo:  mysql.NewSemanticRepository(db),
				SemanticCache: sc,
				CoverageSink:  mysql.NewSemanticCoverageRepository(db),
				// 知识库列表工具（kb_list）仓储；nil 时工具报告未接线
				KnowledgeBaseRepo: app.KnowledgeBaseRepository,
				// Metaso 搜索客户端（web_search / search_multi）；API Key 缺失时调用报错
				MetasoClient: ragtool.NewMetasoClient(config.LoadSearchConfig()),
				// 操作工具（sql_mutate / etl_run / data_export）：审计仓储走 MySQL
				Operation: ragtool.OperationConfig{
					DB:      db,
					Audit:   mysql.NewOperationAuditRepository(db),
					Pending: pendingStore,
					// 红线原始业务表：Agent 禁止直接修改的系统核心表
					RedlineTables: []string{
						"tenants", "tenant_users", "users", "refresh_tokens", "sessions",
						"audit_logs", "agent_operation_audit",
					},
				},
			}

			// 外部数据源路由：查询类工具 database_id 非空时经 ConnectionManager
			// 路由到已注册外部数据源；无效 id 显式报错不回落业务库。
			if app.DataSourceService != nil {
				deps.DatasourceProvider = app.DataSourceService
				log.Println("✅ 外部数据源工具路由已注入")
			}

			// 术语接地：模型内同义词接地始终可用；Neo4j 可用时叠加知识图谱/血缘增强。
			// 未连接 Neo4j 时端口为 nil，接地退化为仅模型内同义词。
			if driver, ok := neo4jDriver.(neo4jsdk.DriverWithContext); ok {
				dbName := ""
				if cfg.Neo4j != nil {
					dbName = cfg.Neo4j.DatabaseName
				}
				if graphRepo, gerr := neo4jrepo.NewNeo4jRepositoryFromDriver(driver, dbName); gerr != nil {
					log.Printf("⚠️  术语接地图谱端口初始化失败，退化为仅模型内同义词: %v", gerr)
				} else {
					deps.TermGrounding = termgrounding.NewGraphAdapter(graphRepo, "")
					log.Println("✅ 术语接地已叠加知识图谱增强")
				}
			} else {
				log.Println("ℹ️  未连接 Neo4j，术语接地仅用模型内同义词")
			}

			// 数据分析工具：注入 MCP 调用器（与 skill 同源端点）；失败时不注入（调用返回非致命错误）
			skillCfg := config.LoadSkillConfig()
			mcpClient, mcpErr := mcp.NewMCPClient(&mcp.Config{
				Endpoint: skillCfg.Endpoint,
				Timeout:  time.Duration(skillCfg.Timeout) * time.Second,
				CacheTTL: time.Duration(skillCfg.CacheTTL) * time.Second,
			})
			if mcpErr != nil {
				log.Printf("⚠️  data_analysis MCP 调用器初始化失败: %v", mcpErr)
			} else {
				deps.DataAnalysisInvoker = mcpClient
				log.Println("✅ 数据分析工具 MCP 调用器已注入")
			}

			// 知识库检索/图谱工具：将已接线的领域服务经适配器注入工具层。
			// 检索范围与图谱开关由会话 ctx 强制（用户在入口选定），工具/Agent 不再自行选库。
			// 仅在服务就绪时赋值接口字段，避免 typed-nil 绕过工具内 nil 判断。
			if app.Retriever != nil {
				deps.RAGService = agentadapters.NewRAGRetrieverAdapter(app.Retriever, app.KnowledgeBaseRepository)
				log.Println("✅ RAG 检索工具（rag_query）已接线真实检索器")
			} else {
				log.Println("⚠️  检索器未就绪，rag_query 工具未接线")
			}
			if app.GraphService != nil {
				deps.GraphService = agentadapters.NewGraphSearchAdapter(app.GraphService, app.KnowledgeBaseRepository)
				log.Println("✅ 图谱检索工具（graph_query）已接线真实图谱服务")
			} else {
				log.Println("⚠️  图谱服务未就绪，graph_query 工具未接线")
			}

			// ==================== 构造工具注册表并注入默认槽位 ====================
			reg, regErr := ragtool.NewToolRegistry(deps)
			if regErr != nil {
				log.Printf("⚠️  工具注册表构造失败，跳过 Agent 初始化: %v", regErr)
			} else {
				log.Printf("✅ 工具注册表构造完成（%d 个工具，依赖显式注入）", reg.Size())

				// 经组合根把工具注册表注入 handler 网关（confirm-resume / UI 取数 /
				// schema 查询），替代 tools 包级默认槽位（Decision A：窄接口 + setter）。
				if app.AgentHandler != nil {
					app.AgentHandler.SetToolGateway(reg)
					log.Println("✅ Agent handler 工具网关已注入")
				}

				// Skill 系统工具（skill_list/skill_invoke/skill_match）：在注册表构造后
				// 显式注册（同名覆盖内置工具发现版），替代旧的 init() 导入副作用。
				if err := skilltools.RegisterSkillTools(reg); err != nil {
					log.Printf("⚠️  Skill 工具注册失败: %v", err)
				}

				// 注入生成式 UI 的 LLM：Text2SQL 取数+分析后，由 Go 端用它定制布局
				// （Level 2）；未注入时降级为确定性模板（Level 1）。
				genui.SetModel(toolModel)
				log.Println("✅ 生成式 UI（GenUI）已启用 LLM 定制布局")

				// 工具注册表就绪后构造 Initializer（工具注册表构造期显式注入，
				// 替代 tools 包级 GetTool/GetToolsByGroup 全局查询）。
				initializer := agentinit.NewInitializer(app.AgentRegistry, reg, msgRepo).
					WithEmbedder(app.Embedder)

				// 初始化所有 Agents
				if err := initializer.Initialize(ctx, toolModel); err != nil {
					log.Printf("⚠️  Agent 初始化失败: %v", err)
				} else {
					log.Println("✅ Agent 注册中心初始化完成")
				}
			}
		}
	}

	// 启动评测 Worker（如果启用）
	if app.EvaluationConfig != nil && app.EvaluationConfig.WorkerEnabled && app.EvaluationWorker != nil {
		log.Println("🔧 启动评测 Worker...")
		go func() {
			if err := app.EvaluationWorker.Run(); err != nil {
				log.Printf("❌ 评测 Worker 错误: %v", err)
			}
		}()
		log.Println("✅ 评测 Worker 已启动")
	}

	// 启动会话经验自动沉淀 Worker（EXPERIENCE_DISTILL_ENABLED=true 时装配）
	if experienceWorker != nil {
		log.Println("🔧 启动会话经验沉淀 Worker...")
		if err := experienceWorker.Run(); err != nil {
			log.Printf("❌ 经验沉淀 Worker 启动失败: %v", err)
		} else {
			log.Println("✅ 会话经验沉淀 Worker 已启动")
		}
	}

	// 启动外部数据源健康检查（默认关闭；DATASOURCE_HEALTHCHECK_ENABLED=true 开启）
	if os.Getenv("DATASOURCE_HEALTHCHECK_ENABLED") == "true" && app.DataSourceService != nil {
		interval := 5 * time.Minute
		if v := os.Getenv("DATASOURCE_HEALTHCHECK_INTERVAL"); v != "" {
			if d, perr := time.ParseDuration(v); perr == nil && d > 0 {
				interval = d
			} else {
				log.Printf("⚠️  DATASOURCE_HEALTHCHECK_INTERVAL=%q 非法，回落 %s", v, interval)
			}
		}
		log.Printf("🔧 启动数据源健康检查（间隔 %s）...", interval)
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			runCheck := func() {
				hcCtx, cancel := context.WithTimeout(context.Background(), interval)
				defer cancel()
				report, err := app.DataSourceService.CheckHealthOnce(hcCtx)
				if err != nil {
					log.Printf("❌ 数据源健康检查失败: %v", err)
					return
				}
				log.Printf("💓 数据源健康检查：检查 %d / 正常 %d / 跳过 %d",
					report.Checked, report.Healthy, report.Skipped)
			}
			runCheck() // 启动即跑一次
			for range ticker.C {
				runCheck()
			}
		}()
		log.Println("✅ 数据源健康检查已启动")
	}

	// 设置路由
	app.Router.Setup()

	// 优雅关闭
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("🛑 收到关闭信号...")

		// 关闭服务器
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := app.Shutdown(ctx); err != nil {
			log.Printf("❌ 应用关闭失败: %v", err)
		}

		// 停止经验沉淀 Worker（等待在途蒸馏完成）
		if experienceWorker != nil {
			experienceWorker.Stop()
		}

		// 关闭追踪：flush 批量队列里未导出的 span，避免退出丢尾。
		if traceShutdown != nil {
			if err := traceShutdown(ctx); err != nil {
				log.Printf("❌ 调用链追踪关闭失败: %v", err)
			}
		}

		// 关闭数据库
		if err := sqlDB.Close(); err != nil {
			log.Printf("❌ 数据库关闭失败: %v", err)
		}
		if err := mysql.CloseDatabase(); err != nil {
			log.Printf("❌ GORM 数据库关闭失败: %v", err)
		}

		log.Println("✅ 应用已安全关闭")
		os.Exit(0)
	}()

	// 启动服务器
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("🚀 服务器启动中... 监听地址: %s", addr)
	if err := app.Router.Run(addr); err != nil {
		log.Fatalf("❌ 服务器启动失败: %v", err)
	}
}
