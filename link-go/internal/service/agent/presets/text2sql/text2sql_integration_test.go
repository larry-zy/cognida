//go:build integration
// +build integration

package text2sql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentinit "link/internal/application/agent"
	ragtool "link/internal/service/agent/tools"
	infraagent "link/internal/service/agent/framework"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// loadEnvForTest 加载 .env 文件
func loadEnvForTest(t *testing.T) {
	paths := []string{
		".env",
		"../../.env",
		"../../../.env",
		"../../../../.env",
		"../../../../../.env",
	}

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if err := godotenv.Load(absPath); err == nil {
			t.Logf("加载 .env: %s", absPath)
			return
		}
	}
	// 尝试默认位置
	_ = godotenv.Load()
}

// TestText2SQL_AgentCreation 测试 Text2SQL Agent 创建和注册
func TestText2SQL_AgentCreation(t *testing.T) {
	loadEnvForTest(t)
	ctx := context.Background()

	// 创建模型
	toolModel := createText2SQLToolModel(t)

	// 创建注册中心
	registry := infraagent.NewMemoryRegistry()

	// 创建初始化器
	initializer := agentinit.NewInitializer(registry)

	// 初始化所有 Agent（包括 Text2SQL）
	err := initializer.Initialize(ctx, toolModel)
	require.NoError(t, err, "Agent 初始化应该成功")

	// 验证 Text2SQL Agent 已注册
	def, err := registry.Get(ctx, "agent-text2sql-per")
	require.NoError(t, err, "获取 Text2SQL Agent 应该成功")

	assert.Equal(t, "agent-text2sql-per", def.ID)
	assert.Equal(t, "text2sql_per", def.Name)
	assert.Equal(t, "2.0.0", def.Metadata["version"])
	assert.Equal(t, "sequential+retry", def.Metadata["pattern"])

	t.Logf("Text2SQL Agent 注册成功: %s (版本: %s, 模式: %s)",
		def.Name, def.Metadata["version"], def.Metadata["pattern"])
}

// TestText2SQL_AgentExecution 测试 Text2SQL Agent 执行（需要数据库）
func TestText2SQL_AgentExecution(t *testing.T) {
	// 加载 .env
	loadEnvForTest(t)

	// 跳过快捷测试
	if testing.Short() {
		t.Skip("跳过需要数据库的集成测试")
	}

	ctx := context.Background()

	// 1. 设置测试数据库
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// 2. 初始化 SQL 工具
	ragtool.InitSQLExecuteTool(db)
	ragtool.InitGetSchemaTool(db)

	// 3. 创建测试表和数据
	setupText2SQLSchema(t, db)

	// 4. 创建模型和注册中心
	toolModel := createText2SQLToolModel(t)
	registry := infraagent.NewMemoryRegistry()

	// 5. 初始化 Agent
	initializer := agentinit.NewInitializer(registry)
	err := initializer.Initialize(ctx, toolModel)
	require.NoError(t, err)

	// 6. 获取 Text2SQL Agent
	agent := agentinit.GetText2SQLAgent()
	require.NotNil(t, agent, "Text2SQL Agent 应该已初始化")

	// 7. 测试简单查询
	t.Run("简单计数查询", func(t *testing.T) {
		resp, err := agent.Chat(ctx, "查询 text2sql_users 表中有多少用户？")
		require.NoError(t, err, "Agent 调用应该成功")
		assert.NotEmpty(t, resp.Content, "响应内容不应为空")
		t.Logf("Agent 响应: %s", resp.Content)
	})

	// 8. 测试条件查询
	t.Run("条件查询", func(t *testing.T) {
		resp, err := agent.Chat(ctx, "查询 text2sql_users 表中状态为 1 的活跃用户有多少？")
		require.NoError(t, err, "Agent 调用应该成功")
		assert.NotEmpty(t, resp.Content, "响应内容不应为空")
		t.Logf("Agent 响应: %s", resp.Content)
	})
}

// ========================================
// 测试辅助函数
// ========================================

// setupTestDB 创建测试数据库连接
func setupTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		getEnvOrDefault("MYSQL_USER", "root"),
		getEnvOrDefault("MYSQL_PASSWORD", ""),
		getEnvOrDefault("MYSQL_HOST", "localhost"),
		getEnvOrDefault("MYSQL_PORT", "3306"),
		getEnvOrDefault("MYSQL_DATABASE", "link"))

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "连接 MySQL 失败")

	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Ping(), "Ping MySQL 失败")

	t.Logf("数据库连接成功: %s", getEnvOrDefault("MYSQL_HOST", "localhost"))
	return db
}

// cleanupTestDB 清理测试数据库
func cleanupTestDB(t *testing.T, db *gorm.DB) {
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}

// setupText2SQLSchema 创建 Text2SQL 测试表结构
func setupText2SQLSchema(t *testing.T, db *gorm.DB) {
	// 创建用户表
	db.Exec(`CREATE TABLE IF NOT EXISTS text2sql_users (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL COMMENT '用户名',
		email VARCHAR(200) COMMENT '邮箱',
		age INT COMMENT '年龄',
		status TINYINT DEFAULT 1 COMMENT '状态：1-活跃，0-禁用',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) COMMENT='Text2SQL测试用户表'`)

	// 清空旧数据
	db.Exec(`DELETE FROM text2sql_users WHERE name LIKE 'text2sql_%'`)

	// 插入测试数据
	db.Exec(`INSERT INTO text2sql_users (name, email, age, status) VALUES
		('text2sql_张三', 'zhangsan@test.com', 25, 1),
		('text2sql_李四', 'lisi@test.com', 30, 1),
		('text2sql_王五', 'wangwu@test.com', 28, 0),
		('text2sql_赵六', 'zhaoliu@test.com', 35, 1),
		('text2sql_孙七', 'sunqi@test.com', 22, 1)`)

	t.Logf("Text2SQL 测试表创建成功，插入 5 条测试数据")
}

// getEnvOrDefault 获取环境变量或默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// createText2SQLToolModel 创建支持工具调用的模型（优先使用 DeepSeek）
func createText2SQLToolModel(t *testing.T) interface{} {
	// 加载 .env
	loadEnvForTest(t)

	// 优先使用 DeepSeek 配置
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	modelName := os.Getenv("DEEPSEEK_MODEL_NAME")
	provider := "deepseek"

	// 降级到通用 Chat 配置
	if apiKey == "" {
		apiKey = os.Getenv("CHAT_API_KEY")
		baseURL = os.Getenv("CHAT_BASE_URL")
		modelName = os.Getenv("CHAT_MODEL_NAME")
		provider = os.Getenv("CHAT_PROVIDER")
	}

	// 再降级到 OpenAI
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		baseURL = os.Getenv("OPENAI_BASE_URL")
		if modelName == "" {
			modelName = "gpt-3.5-turbo"
		}
		if provider == "" {
			provider = "openai"
		}
	}

	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY 或 CHAT_API_KEY 或 OPENAI_API_KEY 环境变量未设置，跳过集成测试")
		return nil
	}

	config := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		ModelName: modelName,
		Provider:  provider,
	}

	t.Logf("使用模型: %s (%s)", modelName, provider)

	ctx := context.Background()
	toolModel, err := chat.NewToolCallingChatModel(ctx, config)
	require.NoError(t, err, "创建模型失败")

	return toolModel
}
