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

	"link/internal/infrastructure/config"
	llmchat "link/internal/infrastructure/llm/chat"
	"link/internal/infrastructure/mcp"
	"link/internal/repository/milvus"
	"link/internal/repository/mysql"
	"link/internal/service/agent/genui"
	agentinit "link/internal/service/agent/initializer"
	ragtool "link/internal/service/agent/tools"

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
		initializer := agentinit.NewInitializer(app.AgentRegistry)

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
