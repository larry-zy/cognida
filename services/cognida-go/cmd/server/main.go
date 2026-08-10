// Package main 是应用程序的入口点
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cognida/internal/wire"

	"cognida/internal/config"
	"cognida/internal/handler/middleware"
	cachesvc "cognida/internal/infrastructure/cache"
	rediscache "cognida/internal/infrastructure/cache/redis"
	llmchat "cognida/internal/infrastructure/llm/chat"
	"cognida/internal/infrastructure/mcp"
	obstel "cognida/internal/infrastructure/observability"
	domaincache "cognida/internal/model/cache"
	"cognida/internal/repository/milvus"
	milvusretriever "cognida/internal/repository/milvus/retriever"
	"cognida/internal/repository/mysql"
	neo4jrepo "cognida/internal/repository/neo4j"
	redisrepo "cognida/internal/repository/redis"
	agentadapters "cognida/internal/service/agent/adapters"
	experiencesvc "cognida/internal/service/agent/experience"
	agentinit "cognida/internal/service/agent/initializer"
	"cognida/internal/service/agent/pendingaction"
	dataagent "cognida/internal/service/agent/presets/data_agent"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/semanticcache"
	"cognida/internal/service/agent/skills"
	skilltools "cognida/internal/service/agent/skills/tools"
	agenttelemetry "cognida/internal/service/agent/telemetry"
	"cognida/internal/service/agent/termgrounding"
	ragtool "cognida/internal/service/agent/tools"
	"cognida/internal/service/agent/uibinding"
	knowledgesvc "cognida/internal/service/knowledge"

	"github.com/cloudwego/eino/components/embedding"
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

	// 多租户 fail-closed 兜底守卫：启用多租户时，对含 tenant_id 的表拦截无租户约束的读/改/删。
	// 单租户部署（TENANT_ENABLED 未开）此处为空操作，行为不变。
	if cfg.Tenant != nil && cfg.Tenant.EnableMultiTenant {
		if err := mysql.RegisterTenantGuard(db, true); err != nil {
			log.Fatalf("❌ 注册多租户守卫失败: %v", err)
		}
		log.Println("🔒 多租户 fail-closed 守卫已启用")
	}

	// 初始化 Milvus（如果配置了）
	// 〔GO-1〕已配置的依赖初始化失败必须 fail-fast 中止启动：此前 log 后继续会留下
	// nil 客户端，直到查询期才 NPE（隐蔽故障）。未配置（Host 为空）则整体跳过，不受影响。
	if cfg.Milvus != nil && cfg.Milvus.Host != "" {
		log.Println("🔧 初始化 Milvus...")
		if err := milvus.InitMilvus(cfg.Milvus); err != nil {
			log.Fatalf("❌ Milvus 已配置但初始化失败，中止启动: %v", err)
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
			// 〔GO-1〕已配置（URI 非空）的 Neo4j 连接失败必须中止启动，与 Milvus 同口径。
			log.Fatalf("❌ Neo4j 已配置但连接失败，中止启动: %v", err)
		}
		defer func() {
			if d, ok := neo4jDriver.(interface{ Close(context.Context) error }); ok {
				d.Close(context.Background())
				log.Println("🔌 关闭 Neo4j driver...")
			}
		}()
	}

	// 使用 Wire 初始化应用
	log.Println("🔧 初始化应用程序...")
	app, err := wire.InitializeApp(db, cfg)
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
				// 技能沉淀（把 skill_worthy 经验落成 SKILL.md）与图谱同属一次蒸馏 pass 的下游产物，
				// 但独立门控（EXPERIENCE_SKILL_SINK_ENABLED，默认关）：落盘目录取加载扫描范围内首个已存在者，
				// 并注入 registrar 把新技能即时注册进运行中的全局管理器（运行期可见，不必等下次加载）。
				var skillSink *experiencesvc.SkillSink
				if experiencesvc.SkillSinkEnabledFromEnv() {
					mgr := skills.GetGlobalManager()
					registrar := func(name, path string) error {
						if _, ok := mgr.GetSkill(name); ok {
							_ = mgr.UnloadSkill(name) // 已存在则先卸载再加载以刷新内容
						}
						_, err := mgr.LoadSkill(path)
						return err
					}
					skillSink = experiencesvc.NewSkillSink(skills.SkillDirFromEnv(), mysql.NewExperienceRepository(db), registrar)
				}
				experienceWorker = experiencesvc.NewWorker(
					expCfg,
					mysql.NewExperienceRepository(db),
					msgRepo,
					experiencesvc.NewSummarizer(toolModel),
					graphSink,
					skillSink,
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

			// 会话取数注册表（进程内）：记录每会话最近取数结果与 SQL 去重索引。
			// 与 Result Store 共享 TTL；支撑 sql_execute 复用同句结果、data_analysis
			// 缺 result_id 时回退到会话最近取数（收敛「取数→分析」委派链的空转与重查）。
			sessionResults := resultstore.NewSessionRegistry(resultstore.DefaultTTL)

			deps := ragtool.ToolDeps{
				SQLDB:          db,
				ResultStore:    rs,
				SessionResults: sessionResults,
				UIBinding:      uiBindingStore,
				// 列画像（数据事实）：get_schema 精确取表时惰性附上空值率/基数/枚举分布，
				// 写通落应用自身 MySQL；缺失时不带事实、零回归。
				ColumnProfileStore: mysql.NewColumnProfileRepository(db),
				// 指标语义层（NL2Semantics）：语义模型仓储 + 受信查询缓存 + 覆盖埋点
				SemanticRepo:  mysql.NewSemanticRepository(db),
				SemanticCache: sc,
				CoverageSink:  mysql.NewSemanticCoverageRepository(db),
				// 知识库列表工具（kb_list）仓储；nil 时工具报告未接线
				KnowledgeBaseRepo: app.KnowledgeBaseRepository,
				// Metaso 搜索客户端（web_search / search_multi）；API Key 缺失时调用报错
				MetasoClient: ragtool.NewMetasoClient(cfg.Search),
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
			skillCfg := cfg.Skill
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
			if app.RetrievalCapability != nil {
				deps.RAGService = agentadapters.NewRAGRetrieverAdapter(app.RetrievalCapability, app.KnowledgeBaseRepository)
				log.Println("✅ RAG 检索工具（rag_query）已接线统一检索能力（与 REST /knowledge/search 同源）")
			} else {
				log.Println("⚠️  检索能力未就绪，rag_query 工具未接线")
			}
			if app.GraphService != nil {
				deps.GraphService = agentadapters.NewGraphSearchAdapter(app.GraphService, app.KnowledgeBaseRepository)
				log.Println("✅ 图谱检索工具（graph_query）已接线真实图谱服务")
			} else {
				log.Println("⚠️  图谱服务未就绪，graph_query 工具未接线")
			}

			// 知识块精确拉取工具（kb_fetch_chunks）：按 kb/chunk/knowledge 等标量条件直取分块原文。
			// VectorRetriever 复用 Milvus 单例连接（构造廉价），租户与 KB 范围由 ctx 强制（同 rag_query 治理）。
			// 嵌入器/向量库未就绪时不接线，工具降级返回提示。
			if app.Embedder != nil {
				if vr, verr := milvusretriever.NewVectorRetriever(app.Embedder); verr != nil {
					log.Printf("⚠️  向量检索器初始化失败，kb_fetch_chunks 工具未接线: %v", verr)
				} else {
					deps.ChunkFetcher = agentadapters.NewChunkFetchAdapter(vr, app.KnowledgeBaseRepository)
					log.Println("✅ 知识块精确拉取工具（kb_fetch_chunks）已接线向量库")
				}
			} else {
				log.Println("⚠️  嵌入器未就绪，kb_fetch_chunks 工具未接线")
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
				// （Level 2）；未注入时降级为确定性模板（Level 1）。经 handler 实例注入，
				// 替代 genui 包级单例（消除隐藏全局态〔GO-2〕）。
				if app.AgentHandler != nil {
					app.AgentHandler.SetGenUIModel(toolModel)
					log.Println("✅ 生成式 UI（GenUI）已启用 LLM 定制布局")
				}

				// 历史经验召回器（自进化读侧）：EXPERIENCE_RECALL_ENABLED 开启且图谱可用时，
				// 用与沉淀同源的图谱仓储装配，供 Data Agent 首答注入过往会话经验。默认关 → nil（不召回）。
				var experienceRecall *experiencesvc.GraphRecaller
				if experiencesvc.RecallEnabledFromEnv() && app.GraphService != nil {
					experienceRecall = experiencesvc.NewGraphRecaller(app.GraphService.GetGraphRepo())
					log.Println("✅ 历史经验召回已启用（图谱召回 + 首答注入）")
				}

				// 语义缓存（一套向量底座，两种消费模式；默认全关 → nil）：
				//   - RAG 知识库助手 → 硬缓存短路返回（NewCachingAgent）
				//   - Data Agent → 软 few-shot 读侧（Before hook）+ 软写穿写侧（NewWriteThroughAgent）
				// 仅在真正装配（非 nil）时挂接 With*，避免把 typed-nil 具体指针混入接口字段。
				semanticCache := buildSemanticCache(ctx, app.Embedder)

				// 工具注册表就绪后构造 Initializer（工具注册表构造期显式注入，
				// 替代 tools 包级 GetTool/GetToolsByGroup 全局查询）。
				initializer := agentinit.NewInitializer(app.AgentRegistry, reg, msgRepo).
					WithEmbedder(app.Embedder).
					WithExperienceRecall(experienceRecall)
				if semanticCache != nil {
					initializer = initializer.
						WithSemanticFewShot(semanticCache).
						WithRAGHardCache(semanticCache).
						WithDataSoftCache(semanticCache)

					// 文档写入/删除后按库失效 RAG 硬缓存：把缓存服务经 setter 注入知识库服务
					// （setter 在具体类型上，接口未暴露 → 类型断言获取）。断言失败仅记日志，不阻断。
					if inv, ok := app.KnowledgeBaseService.(interface {
						SetCacheInvalidator(knowledgesvc.KBCacheInvalidator)
					}); ok {
						inv.SetCacheInvalidator(semanticCache)
						log.Println("✅ RAG 硬缓存按库失效已接线（文档变更 → InvalidateByKB）")
					} else {
						log.Println("⚠️  知识库服务未暴露 SetCacheInvalidator，RAG 硬缓存按库失效未接线")
					}
				}

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

	// 设置路由。限流器在组合根装配（Redis 令牌桶，Redis 未就绪则降级进程内），注入 router，
	// 使 handler 层不直接依赖 infrastructure/cache/redis（Clean Architecture 依赖方向）。
	app.Router.Setup(middleware.NewRateLimiter(rediscache.Client))

	// 启动 HTTP 服务器（后台 goroutine），主 goroutine 负责等待信号并驱动优雅关闭。
	// 关键：关闭走正常 return 而非 os.Exit(0)，以便 main 中已注册的 Milvus / Neo4j
	// defer 清理得到执行（旧实现在关闭 goroutine 里 os.Exit(0)，会跳过全部 defer）。
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("🚀 服务器启动中... 监听地址: %s", addr)
	serverErr := make(chan error, 1)
	go func() {
		if err := app.Router.Run(addr); err != nil {
			serverErr <- err
		}
	}()

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		// 监听失败（如端口占用）属启动阶段异常，直接终止。
		log.Fatalf("❌ 服务器启动失败: %v", err)
	case <-sigint:
		log.Println("🛑 收到关闭信号...")
	}

	// 优雅关闭序列
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 先停止接收新连接并 drain 在途 HTTP 请求。
	if err := app.Router.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP 服务器优雅关闭失败: %v", err)
	}

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
}

// ==================== 语义缓存装配（组合根，默认全关 → 零回归）====================

// 语义缓存的两个逻辑消费键（与领域 AgentType 无关，用于隔离命名空间/开关；见各 preset 说明）：
//   - ragCacheKey  "rag_assistant"：硬缓存（高阈值，命中短路返回，知识稳定安全）
//   - dataCacheKey "data_agent"   ：软缓存（低阈值，few-shot 注入 + 软写穿，数据会变需重算）
const (
	ragCacheKey  = "rag_assistant"
	dataCacheKey = dataagent.DataAgentCacheKey
)

// buildSemanticCache 按环境变量装配语义缓存服务（一套向量检索/回写底座，两种消费模式）。
// 总开关 SEMANTIC_CACHE_ENABLED 默认关；两种模式各自独立门控，均关闭时不装配。
// 任一前置（嵌入器/Milvus/Redis）不就绪时降级为「不装配」并记日志，绝不阻断启动。
// 返回 nil 表示未装配——调用方据此跳过全部 With* 挂接（避免 typed-nil 混入接口，见 preset 说明）。
func buildSemanticCache(ctx context.Context, embedder embedding.Embedder) *cachesvc.SemanticCacheService {
	if !getEnvBool("SEMANTIC_CACHE_ENABLED", false) {
		return nil
	}
	ragEnabled := getEnvBool("RAG_HARD_CACHE_ENABLED", false)
	dataEnabled := getEnvBool("DATA_SOFT_CACHE_ENABLED", false)
	if !ragEnabled && !dataEnabled {
		log.Println("ℹ️  语义缓存总开关开启，但 RAG_HARD_CACHE_ENABLED / DATA_SOFT_CACHE_ENABLED 均关，跳过装配")
		return nil
	}
	if embedder == nil {
		log.Println("⚠️  语义缓存已开启但嵌入器未就绪，跳过装配")
		return nil
	}
	if rediscache.Client == nil {
		log.Println("⚠️  语义缓存需要 Redis 存储内容但 Redis 未就绪，跳过装配")
		return nil
	}

	// 探测嵌入维度（DashScope v3=1024，模型不同维度不同）：用真实输出长度建表，避免维度不一致。
	vecs, err := embedder.EmbedStrings(ctx, []string{"semantic cache dimension probe"})
	if err != nil || len(vecs) == 0 || len(vecs[0]) == 0 {
		log.Printf("⚠️  语义缓存嵌入维度探测失败，跳过装配: %v", err)
		return nil
	}
	dim := len(vecs[0])

	if err := milvus.InitializeCacheCollection(ctx, dim); err != nil {
		log.Printf("⚠️  语义缓存 Milvus collection 初始化失败，跳过装配: %v", err)
		return nil
	}
	vectorRepo, err := milvus.NewCacheVectorRepository(dim)
	if err != nil {
		log.Printf("⚠️  语义缓存向量仓储构造失败，跳过装配: %v", err)
		return nil
	}
	contentRepo := redisrepo.NewCacheContentRepository(rediscache.Client)

	strategy := &domaincache.AgentCacheStrategy{
		Global: domaincache.SemanticCacheConfig{
			Enabled:   true,
			Threshold: getEnvFloat32("SEMANTIC_CACHE_THRESHOLD", 0.90),
			TTL:       getEnvDuration("SEMANTIC_CACHE_TTL", 24*time.Hour),
			TopK:      getEnvInt("SEMANTIC_CACHE_TOP_K", 5),
		},
		Defaults: domaincache.AgentCacheConfig{
			Enabled:   false, // 未显式列出的 agent 一律不启用（GetAgentConfig 兜底也返回关闭）
			Threshold: 0.90,
			TTL:       24 * time.Hour,
			TopK:      5,
		},
		Agents: map[string]domaincache.AgentCacheConfig{},
	}
	if ragEnabled {
		// RAG 硬缓存：高阈值（0.95）——只有极相似的问题才短路返回旧答案（知识稳定，误命中代价高）。
		strategy.Agents[ragCacheKey] = domaincache.AgentCacheConfig{
			Enabled:   true,
			Threshold: getEnvFloat32("RAG_HARD_CACHE_THRESHOLD", 0.95),
			TTL:       getEnvDuration("RAG_HARD_CACHE_TTL", 24*time.Hour),
			TopK:      getEnvInt("RAG_HARD_CACHE_TOP_K", 3),
		}
	}
	if dataEnabled {
		// Data 软缓存：低阈值（0.82）——较宽松地召回相似历史问答作 few-shot 参考（不短路，LLM 结合实时数据重算）。
		strategy.Agents[dataCacheKey] = domaincache.AgentCacheConfig{
			Enabled:   true,
			Threshold: getEnvFloat32("DATA_SOFT_CACHE_THRESHOLD", 0.82),
			TTL:       getEnvDuration("DATA_SOFT_CACHE_TTL", 6*time.Hour),
			TopK:      getEnvInt("DATA_SOFT_CACHE_TOP_K", 3),
		}
	}

	log.Printf("✅ 语义缓存已装配（dim=%d，RAG硬缓存=%v，Data软缓存=%v）", dim, ragEnabled, dataEnabled)
	return cachesvc.NewSemanticCacheService(vectorRepo, contentRepo, strategy, embedder)
}

// ---- 组合根本地 env 读取小工具（仅本包使用，避免暴露 config 内部 helper）----

func getEnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getEnvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvFloat32(key string, def float32) float32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return def
	}
	return float32(f)
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
