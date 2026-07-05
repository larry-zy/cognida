// Package text2sql 多轮对话测试
//go:build integration
// +build integration

package text2sql_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentinit "link/internal/service/agent/initializer"
	"link/internal/service/agent/presets/text2sql"
	ragtool "link/internal/service/agent/tools"
	infraagent "link/internal/service/agent/framework"
	"link/internal/model/common"
	"link/internal/infrastructure/llm/chat"
)

// TestText2SQL_MultiTurnConversation 测试多轮对话
func TestText2SQL_MultiTurnConversation(t *testing.T) {
	loadEnvForTest(t)

	ctx := context.Background()

	// 1. 设置测试数据库
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// 2. 构造携真实 DB 的工具注册表（SQL 工具随构造注册）；经 Initializer 显式注入
	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	// 3. 创建测试表和数据
	setupText2SQLSchema(t, db)

	// 4. 创建模型和注册中心
	toolModel := createText2SQLToolModel(t)
	registry := infraagent.NewSpecRegistry()

	// 5. 初始化 Agent
	initializer := agentinit.NewInitializer(registry, reg)
	err = initializer.Initialize(ctx, toolModel)
	require.NoError(t, err)

	// 6. 获取 Text2SQL Agent（从 SpecRegistry 解析运行实例）
	agent, ok := registry.GetInstance(text2sql.Text2SQLAgentID)
	require.True(t, ok, "Text2SQL Agent 应已注册")
	require.NotNil(t, agent)

	t.Run("多轮对话：追问和修改", func(t *testing.T) {
		// 第一轮：查询所有用户
		t.Logf("=== 第一轮：查询所有用户 ===")
		resp1, err := agent.Chat(ctx, "查询 text2sql_users 表中所有用户")
		require.NoError(t, err)
		t.Logf("第一轮响应: %s", resp1.Content)

		// 第二轮：追问 - 按年龄排序
		t.Logf("=== 第二轮：按年龄排序 ===")
		resp2, err := agent.Chat(ctx, "按年龄从大到小排序")
		require.NoError(t, err)
		t.Logf("第二轮响应: %s", resp2.Content)

		// 第三轮：进一步筛选 - 只要前3个
		t.Logf("=== 第三轮：只要前3个 ===")
		resp3, err := agent.Chat(ctx, "只显示前3个")
		require.NoError(t, err)
		t.Logf("第三轮响应: %s", resp3.Content)
	})

	t.Run("多轮对话：聚合后的追问", func(t *testing.T) {
		// 第一轮：统计用户数
		t.Logf("=== 第一轮：统计用户数 ===")
		resp1, err := agent.Chat(ctx, "text2sql_users 表中有多少个用户？")
		require.NoError(t, err)
		t.Logf("第一轮响应: %s", resp1.Content)

		// 第二轮：查看活跃用户数
		t.Logf("=== 第二轮：查看活跃用户数 ===")
		resp2, err := agent.Chat(ctx, "其中状态为1的活跃用户有多少个？")
		require.NoError(t, err)
		t.Logf("第二轮响应: %s", resp2.Content)
	})

	t.Run("多轮对话：复杂场景", func(t *testing.T) {
		// 第一轮：查询年龄大于25的用户
		t.Logf("=== 第一轮：年龄大于25的用户 ===")
		resp1, err := agent.Chat(ctx, "查询 text2sql_users 表中年龄大于25的用户")
		require.NoError(t, err)
		t.Logf("第一轮响应: %s", resp1.Content)

		// 第二轮：按年龄升序排列
		t.Logf("=== 第二轮：按年龄升序 ===")
		resp2, err := agent.Chat(ctx, "按年龄从小到大排序")
		require.NoError(t, err)
		t.Logf("第二轮响应: %s", resp2.Content)

		// 第三轮：只要姓名和年龄两个字段
		t.Logf("=== 第三轮：只要姓名和年龄 ===")
		resp3, err := agent.Chat(ctx, "只显示姓名和年龄两列")
		require.NoError(t, err)
		t.Logf("第三轮响应: %s", resp3.Content)
	})
}

// ========================================
// 辅助函数
// ========================================

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
	_ = godotenv.Load()
}

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		getEnvOrDefault("DB_USER", getEnvOrDefault("MYSQL_USER", "root")),
		getEnvOrDefault("DB_PASSWORD", getEnvOrDefault("MYSQL_PASSWORD", "")),
		getEnvOrDefault("DB_HOST", getEnvOrDefault("MYSQL_HOST", "localhost")),
		getEnvOrDefault("DB_PORT", getEnvOrDefault("MYSQL_PORT", "3306")),
		getEnvOrDefault("DB_NAME", getEnvOrDefault("MYSQL_DATABASE", "link")))

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "连接 MySQL 失败")

	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Ping(), "Ping MySQL 失败")

	t.Logf("数据库连接成功")
	return db
}

func cleanupTestDB(t *testing.T, db *gorm.DB) {
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.Close()
	}
}

func setupText2SQLSchema(t *testing.T, db *gorm.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS text2sql_users (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL,
		email VARCHAR(200),
		age INT,
		status TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`DELETE FROM text2sql_users WHERE name LIKE 'text2sql_%'`)

	db.Exec(`INSERT INTO text2sql_users (name, email, age, status) VALUES
		('text2sql_张三', 'zhangsan@test.com', 25, 1),
		('text2sql_李四', 'lisi@test.com', 30, 1),
		('text2sql_王五', 'wangwu@test.com', 28, 0),
		('text2sql_赵六', 'zhaoliu@test.com', 35, 1),
		('text2sql_孙七', 'sunqi@test.com', 22, 1)`)

	t.Logf("测试数据准备完成，5条记录")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createText2SQLToolModel(t *testing.T) interface{} {
	loadEnvForTest(t)

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	modelName := os.Getenv("DEEPSEEK_MODEL_NAME")
	provider := "deepseek"

	if apiKey == "" {
		apiKey = os.Getenv("CHAT_API_KEY")
		baseURL = os.Getenv("CHAT_BASE_URL")
		modelName = os.Getenv("CHAT_MODEL_NAME")
		provider = os.Getenv("CHAT_PROVIDER")
	}

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
		t.Skip("API Key 未设置")
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
	require.NoError(t, err)

	return toolModel
}
