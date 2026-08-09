// Package handler 端到端测试 - Data Agent 全流程
//go:build integration
// +build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentinit "cognida/internal/service/agent/initializer"
	dataagent "cognida/internal/service/agent/presets/data_agent"
	ragtool "cognida/internal/service/agent/tools"
	infraagent "cognida/internal/service/agent/framework"
	"cognida/internal/infrastructure/llm/chat"
)

// TestStack 测试所需的所有组件
type TestStack struct {
	DB     *gorm.DB
	Router *gin.Engine
}

// setupE2ETest 创建端到端测试环境（使用本地 MySQL）
func setupE2ETest(t *testing.T) *TestStack {
	// 1. 加载 .env
	loadEnvForTest(t)

	// 2. 连接本地数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		getEnv("DB_USER", "root"),
		getEnv("DB_PASSWORD", ""),
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "3306"),
		getEnv("DB_NAME", "cognida"))

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "连接 MySQL 失败")

	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Ping(), "Ping MySQL 失败")

	// 3. 创建测试表
	setupE2ETestSchema(t, db)

	// 4. 构造携真实 DB 的工具注册表（SQL 工具随构造注册）；经 Initializer 显式注入
	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	// 5. 创建 ChatModel
	chatModel := setupTestChatModel(t)
	if chatModel == nil {
		t.Skip("跳过 E2E 测试: CHAT_API_KEY 未设置")
		return nil
	}

	// 6. 初始化 Agent（含 Data Agent）
	ctx := context.Background()
	registry, err := initE2EAgents(ctx, chatModel, reg)
	require.NoError(t, err, "初始化 Agent 失败")

	// 7. 创建 Router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", int64(1))
		c.Set("user_id", int64(1))
		c.Next()
	})

	// 注册聊天路由（简化版，直接调用 Agent）
	router.POST("/api/v1/chat", func(c *gin.Context) {
		var req struct {
			Content string `json:"content"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			BadRequest(c, err.Error())
			return
		}

		// 使用 Data Agent（从 SpecRegistry 解析运行实例）
		agent, ok := registry.GetInstance(dataagent.DataAgentID)
		if !ok || agent == nil {
			InternalError(c, "Data Agent 未初始化")
			return
		}

		ctx := c.Request.Context()

		// 调用 Agent
		agentResp, err := agent.Chat(ctx, req.Content)
		if err != nil {
			InternalError(c, err.Error())
			return
		}

		OK(c, map[string]interface{}{
			"content":    agentResp.Content,
			"agent_id":   dataagent.DataAgentID,
			"request_id": "test-request-001",
		})
	})

	return &TestStack{
		DB:     db,
		Router: router,
	}
}

// setupE2ETestSchema 创建端到端测试的表结构
func setupE2ETestSchema(t *testing.T, db *gorm.DB) {
	// 用户表
	db.Exec(`CREATE TABLE IF NOT EXISTS e2e_users (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL COMMENT '用户名',
		email VARCHAR(200) COMMENT '邮箱',
		age INT COMMENT '年龄',
		status TINYINT DEFAULT 1 COMMENT '状态：1-活跃，0-禁用',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) COMMENT='端到端测试用户表'`)

	// 订单表
	db.Exec(`CREATE TABLE IF NOT EXISTS e2e_orders (
		id INT PRIMARY KEY AUTO_INCREMENT,
		user_id INT NOT NULL COMMENT '用户ID',
		order_no VARCHAR(50) NOT NULL COMMENT '订单号',
		total_amount DECIMAL(10,2) NOT NULL COMMENT '订单金额',
		status VARCHAR(20) DEFAULT 'pending' COMMENT '订单状态',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		KEY idx_user_id (user_id)
	) COMMENT='端到端测试订单表'`)

	// 产品表
	db.Exec(`CREATE TABLE IF NOT EXISTS e2e_products (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(200) NOT NULL COMMENT '产品名称',
		price DECIMAL(10,2) NOT NULL COMMENT '价格',
		stock INT DEFAULT 0 COMMENT '库存',
		category VARCHAR(50) COMMENT '分类'
	) COMMENT='端到端测试产品表'`)

	// 清空旧测试数据
	db.Exec(`DELETE FROM e2e_orders WHERE user_id IN (SELECT id FROM e2e_users WHERE name LIKE 'e2e_%')`)
	db.Exec(`DELETE FROM e2e_users WHERE name LIKE 'e2e_%'`)
	db.Exec(`DELETE FROM e2e_products WHERE name LIKE 'e2e_%'`)

	// 插入测试数据
	db.Exec(`INSERT INTO e2e_users (name, email, age, status) VALUES
		('e2e_张三', 'zhangsan@test.com', 25, 1),
		('e2e_李四', 'lisi@test.com', 30, 1),
		('e2e_王五', 'wangwu@test.com', 28, 0),
		('e2e_赵六', 'zhaoliu@test.com', 35, 1),
		('e2e_孙七', 'sunqi@test.com', 22, 1)`)

	db.Exec(`INSERT INTO e2e_orders (user_id, order_no, total_amount, status) VALUES
		((SELECT id FROM e2e_users WHERE name='e2e_张三' LIMIT 1), 'E2E001', 99.99, 'completed'),
		((SELECT id FROM e2e_users WHERE name='e2e_张三' LIMIT 1), 'E2E002', 149.50, 'pending'),
		((SELECT id FROM e2e_users WHERE name='e2e_李四' LIMIT 1), 'E2E003', 299.00, 'completed'),
		((SELECT id FROM e2e_users WHERE name='e2e_王五' LIMIT 1), 'E2E004', 49.99, 'cancelled'),
		((SELECT id FROM e2e_users WHERE name='e2e_赵六' LIMIT 1), 'E2E005', 199.99, 'completed')`)

	db.Exec(`INSERT INTO e2e_products (name, price, stock, category) VALUES
		('e2e_笔记本电脑', 999.99, 10, '电子产品'),
		('e2e_无线鼠标', 29.99, 50, '电子产品'),
		('e2e_机械键盘', 79.99, 30, '电子产品'),
		('e2e_显示器', 299.99, 15, '电子产品'),
		('e2e_办公桌', 499.99, 5, '办公用品')`)

	time.Sleep(100 * time.Millisecond)
}

// setupTestChatModel 设置测试用的 ChatModel
func setupTestChatModel(t *testing.T) interface{} {
	apiKey := os.Getenv("CHAT_API_KEY")
	if apiKey == "" {
		return nil
	}

	config := &chat.ChatConfig{
		Source:    "remote",
		APIKey:    apiKey,
		BaseURL:   os.Getenv("CHAT_BASE_URL"),
		ModelName: os.Getenv("CHAT_MODEL_NAME"),
		Provider:  os.Getenv("CHAT_PROVIDER"),
	}

	ctx := context.Background()
	chatModel, err := chat.NewToolCallingChatModel(ctx, config)
	if err != nil {
		t.Fatalf("创建 ChatModel 失败: %v", err)
	}

	return chatModel
}

// initE2EAgents 初始化端到端测试的 Agent，返回可解析运行实例的 SpecRegistry。
// 工具注册表经 Initializer 显式注入（替代包级默认槽位）。
func initE2EAgents(ctx context.Context, chatModel interface{}, reg *ragtool.ToolRegistry) (*infraagent.SpecRegistry, error) {
	registry := infraagent.NewSpecRegistry()

	// 使用新的 Initializer 方式初始化
	initializer := agentinit.NewInitializer(registry, reg)
	if err := initializer.Initialize(ctx, chatModel); err != nil {
		return nil, err
	}
	return registry, nil
}

// ========================================
// 端到端测试
// ========================================

func TestE2E_DataAgent_AgentFlow(t *testing.T) {
	stack := setupE2ETest(t)
	if stack == nil {
		return
	}

	t.Run("完整流程：用户查询->Agent处理->返回结果", func(t *testing.T) {
		// 构造请求
		req := map[string]string{
			"content": "查询 e2e_users 表中有多少用户？",
		}

		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		r, _ := http.NewRequest("POST", "/api/v1/chat", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		// 执行请求
		stack.Router.ServeHTTP(w, r)

		// 验证 HTTP 状态
		assert.Equal(t, http.StatusOK, w.Code)

		// 解析响应 - OK() 返回 {"success": true, "data": {...}}
		var respWrapper map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &respWrapper)
		require.NoError(t, err)

		t.Logf("完整响应: %s", w.Body.String())

		// 验证响应结构
		assert.Equal(t, float64(0), respWrapper["code"], "请求code应该为0")

		data := respWrapper["data"].(map[string]interface{})
		content := data["content"].(string)

		// 验证响应内容
		t.Logf("Agent 响应: %s", content)
		assert.NotEmpty(t, content, "响应内容不应为空")
	})
}

func TestE2E_DataAgent_VariousQueries(t *testing.T) {
	stack := setupE2ETest(t)
	if stack == nil {
		return
	}

	testCases := []struct {
		name     string
		question string
		verify   func(t *testing.T, content string)
	}{
		{
			name:     "条件查询",
			question: "查询 e2e_users 表中 status=1 的活跃用户有多少？",
			verify: func(t *testing.T, content string) {
				assert.True(t, len(content) > 10, "响应内容应该有实质内容")
			},
		},
		{
			name:     "聚合查询",
			question: "查询 e2e_orders 表中所有订单的总金额是多少？",
			verify: func(t *testing.T, content string) {
				assert.True(t, len(content) > 10, "响应内容应该有实质内容")
			},
		},
		{
			name:     "排序查询",
			question: "查询 e2e_users 表中年龄最大的3个用户",
			verify: func(t *testing.T, content string) {
				assert.True(t, len(content) > 10, "响应内容应该有实质内容")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := map[string]string{
				"content": tc.question,
			}

			body, _ := json.Marshal(req)
			w := httptest.NewRecorder()
			r, _ := http.NewRequest("POST", "/api/v1/chat", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			stack.Router.ServeHTTP(w, r)

			assert.Equal(t, http.StatusOK, w.Code)

			var respWrapper map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &respWrapper)

			data := respWrapper["data"].(map[string]interface{})
			content := data["content"].(string)

			t.Logf("问题: %s", tc.question)
			t.Logf("回答: %s", content)

			if tc.verify != nil {
				tc.verify(t, content)
			}
		})
	}
}

// ========================================
// 辅助函数
// ========================================

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func loadEnvForTest(t *testing.T) {
	paths := []string{
		".env",
		"../../.env",
		"../../../.env",
		"../../../../.env",
		"../../../../../.env",
	}

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
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
