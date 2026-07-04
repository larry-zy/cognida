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
	"link/internal/repository/milvus"
	"link/internal/repository/mysql"
	neo4jrepo "link/internal/repository/neo4j"
	agentadapters "link/internal/service/agent/adapters"
	"link/internal/service/agent/genui"
	agentinit "link/internal/service/agent/initializer"
	"link/internal/service/agent/pendingaction"
	"link/internal/service/agent/resultstore"
	"link/internal/service/agent/uibinding"
	"link/internal/service/agent/semanticcache"
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

	// 初始化 Agent 注册中心
	if app.AgentRegistry != nil && app.ChatConfig != nil && app.ChatConfig.APIKey != "" {
		log.Println("🔧 初始化 Agent 注册中心...")
		// 注入消息仓储：Data Agent 据此从 messages 表回放会话历史，具备跨轮对话记忆
		msgRepo := mysql.NewMessageRepository(db)
		initializer := agentinit.NewInitializer(app.AgentRegistry, msgRepo)

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
			// 初始化 SQL 工具
			ragtool.InitSQLExecuteTool(db)
			ragtool.InitGetSchemaTool(db)
			log.Println("✅ SQL 工具初始化完成")

			// 初始化指标语义层工具（NL2Semantics）：注入语义模型仓储
			ragtool.InitSemanticTools(mysql.NewSemanticRepository(db))
			log.Println("✅ 指标语义层工具初始化完成")

			// 初始化 Result Store（data-by-reference）：完整结果集落库、回灌 LLM 只给信封。
			// Redis 可用则用 Redis 后端；否则降级为进程内内存后端（单实例可用）。
			var rs resultstore.Store
			if rediscache.Client != nil {
				rs = resultstore.NewRedisStore(rediscache.Client)
				log.Println("✅ Result Store 使用 Redis 后端")
			} else {
				rs = resultstore.NewMemoryStore()
				log.Println("⚠️  Redis 未配置，Result Store 降级为进程内内存后端")
			}
			ragtool.InitResultStore(rs)

			// 初始化 UI 交互绑定存储（surface ↔ result_id + token，会话 TTL）：
			// 支撑 render_ui 的 Filter/Pagination 等组件回调路由。
			if rediscache.Client != nil {
				uibinding.SetStore(uibinding.NewRedisStore(rediscache.Client))
				log.Println("✅ UI 交互绑定存储使用 Redis 后端")
			} else {
				uibinding.SetStore(uibinding.NewMemoryStore())
				log.Println("⚠️  Redis 未配置，UI 交互绑定存储降级为进程内内存后端")
			}

			// 初始化受信查询缓存（Verified/Golden Query）：键含语义模型版本，版本变更即失效。
			// 复用 Result Store 的 Redis 客户端；Redis 不可用则降级为进程内内存缓存。
			var sc semanticcache.Cache
			if rediscache.Client != nil {
				sc = semanticcache.NewRedisCache(rediscache.Client)
			} else {
				sc = semanticcache.NewMemoryCache()
			}
			ragtool.InitSemanticCache(sc)
			log.Println("✅ 受信查询缓存初始化完成")

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
					ragtool.InitTermGrounding(termgrounding.NewGraphAdapter(graphRepo, ""))
					log.Println("✅ 术语接地已叠加知识图谱增强")
				}
			} else {
				log.Println("ℹ️  未连接 Neo4j，术语接地仅用模型内同义词")
			}

			// 初始化操作工具（sql_mutate / etl_run / data_export）：
			// 审计仓储走 MySQL；待确认存储 Redis 可用则 Redis，否则进程内内存。
			var pendingStore pendingaction.Store
			if rediscache.Client != nil {
				pendingStore = pendingaction.NewRedisStore(rediscache.Client)
				log.Println("✅ 待确认操作存储使用 Redis 后端")
			} else {
				pendingStore = pendingaction.NewMemoryStore()
				log.Println("⚠️  Redis 未配置，待确认操作存储降级为进程内内存后端")
			}
			ragtool.InitOperationTools(ragtool.OperationConfig{
				DB:      db,
				Audit:   mysql.NewOperationAuditRepository(db),
				Pending: pendingStore,
				// 红线原始业务表：Agent 禁止直接修改的系统核心表
				RedlineTables: []string{
					"tenants", "tenant_users", "users", "refresh_tokens", "sessions",
					"audit_logs", "agent_operation_audit",
				},
			})
			log.Println("✅ 操作工具（写/ETL/导出）初始化完成")

			// 初始化数据分析工具：注入 MCP 调用器（与 skill 同源端点）
			skillCfg := config.LoadSkillConfig()
			mcpClient, mcpErr := mcp.NewMCPClient(&mcp.Config{
				Endpoint: skillCfg.Endpoint,
				Timeout:  time.Duration(skillCfg.Timeout) * time.Second,
				CacheTTL: time.Duration(skillCfg.CacheTTL) * time.Second,
			})
			if mcpErr != nil {
				log.Printf("⚠️  data_analysis MCP 调用器初始化失败: %v", mcpErr)
			} else {
				ragtool.InitDataAnalysisTool(mcpClient)
				log.Println("✅ 数据分析工具初始化完成")
			}

			// 注入生成式 UI 的 LLM：Text2SQL 取数+分析后，由 Go 端用它定制布局
			// （Level 2）；未注入时降级为确定性模板（Level 1）。
			genui.SetModel(toolModel)
			log.Println("✅ 生成式 UI（GenUI）已启用 LLM 定制布局")

			// 初始化知识库检索/图谱工具：将已接线的领域服务经适配器注入工具层。
			// 检索范围与图谱开关由会话 ctx 强制（用户在入口选定），工具/Agent 不再自行选库。
			if app.Retriever != nil {
				ragtool.InitRAGQueryTool(agentadapters.NewRAGRetrieverAdapter(app.Retriever, app.KnowledgeBaseRepository))
				log.Println("✅ RAG 检索工具（rag_query）已接线真实检索器")
			} else {
				log.Println("⚠️  检索器未就绪，rag_query 工具未接线")
			}
			if app.GraphService != nil {
				ragtool.InitGraphQueryTool(agentadapters.NewGraphSearchAdapter(app.GraphService, app.KnowledgeBaseRepository))
				log.Println("✅ 图谱检索工具（graph_query）已接线真实图谱服务")
			} else {
				log.Println("⚠️  图谱服务未就绪，graph_query 工具未接线")
			}
			if app.KnowledgeBaseRepository != nil {
				ragtool.InitKnowledgeBaseTool(app.KnowledgeBaseRepository)
				log.Println("✅ 知识库列表工具（kb_list）已接线知识库仓储")
			}

			// 初始化所有 Agents
			if err := initializer.Initialize(ctx, toolModel); err != nil {
				log.Printf("⚠️  Agent 初始化失败: %v", err)
			} else {
				log.Println("✅ Agent 注册中心初始化完成")
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
