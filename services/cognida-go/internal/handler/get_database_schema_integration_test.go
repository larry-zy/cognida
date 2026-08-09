// Package handler 集成测试 - GetDatabaseSchema 端点（真实数据库）
//go:build integration
// +build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	ragtool "cognida/internal/service/agent/tools"
)

// schemaRespTable 对应 ragtool.TableSchema 的 JSON 结构
type schemaRespTable struct {
	TableName  string `json:"table_name"`
	PrimaryKey string `json:"primary_key"`
	Columns    []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Nullable    bool   `json:"nullable"`
		Description string `json:"description"`
	} `json:"columns"`
}

// schemaResp 对应统一响应包裹的 GetSchemaResult
type schemaResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Database string            `json:"database"`
		Tables   []schemaRespTable `json:"tables"`
	} `json:"data"`
}

// TestGetDatabaseSchemaIntegration 针对 GET /agent/text2sql/schema 端点的集成测试
func TestGetDatabaseSchemaIntegration(t *testing.T) {
	// 1. 加载 .env
	loadEnvForTest(t)

	// 2. 连接数据库
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
	defer sqlDB.Close()

	// 3. 创建已知结构的测试表
	setupSchemaTestTable(t, db)
	defer cleanupSchemaTestTable(t, db)

	// 4. 构造携真实 DB 的工具注册表（端点经注册表 FetchSchema 复用其 DB）
	reg, err := ragtool.NewToolRegistry(ragtool.ToolDeps{SQLDB: db})
	require.NoError(t, err, "构造工具注册表失败")

	// 5. 构建 Handler 与路由（GetDatabaseSchema 不依赖其他 use case，传 nil 即可）；
	//    经 SetToolGateway 注入工具网关，替代包级默认槽位。
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewAgentHandler(nil, nil, nil, nil, nil, nil)
	h.SetToolGateway(reg)
	router.GET("/api/v1/agent/text2sql/schema", h.GetDatabaseSchema)

	t.Run("指定表名返回精确结构", func(t *testing.T) {
		resp := requestSchema(t, router, "?table_name=t2s_schema_test")
		require.Equal(t, http.StatusOK, resp.code)
		require.Equal(t, 0, resp.body.Code)
		require.NotEmpty(t, resp.body.Data.Database, "应返回数据库名")

		require.Len(t, resp.body.Data.Tables, 1, "应只返回指定的一张表")
		tbl := resp.body.Data.Tables[0]
		assert.Equal(t, "t2s_schema_test", tbl.TableName)
		assert.Equal(t, "id", tbl.PrimaryKey, "主键应为 id")

		// 列以 map 形式校验，避免依赖列顺序
		cols := map[string]struct {
			Type        string
			Nullable    bool
			Description string
		}{}
		for _, c := range tbl.Columns {
			cols[c.Name] = struct {
				Type        string
				Nullable    bool
				Description string
			}{c.Type, c.Nullable, c.Description}
		}

		require.Contains(t, cols, "id")
		assert.False(t, cols["id"].Nullable, "id 应为 NOT NULL")

		require.Contains(t, cols, "name")
		assert.Equal(t, "varchar", cols["name"].Type, "name 类型应为 varchar")
		assert.False(t, cols["name"].Nullable, "name 应为 NOT NULL")
		assert.Equal(t, "名称", cols["name"].Description, "name 注释应被读取")

		require.Contains(t, cols, "remark")
		assert.True(t, cols["remark"].Nullable, "remark 应可空")
	})

	t.Run("不指定表名返回全部且包含测试表", func(t *testing.T) {
		resp := requestSchema(t, router, "")
		require.Equal(t, http.StatusOK, resp.code)
		require.Equal(t, 0, resp.body.Code)

		found := false
		for _, tbl := range resp.body.Data.Tables {
			if tbl.TableName == "t2s_schema_test" {
				found = true
				break
			}
		}
		assert.True(t, found, "全表列表应包含测试表 t2s_schema_test")
		assert.GreaterOrEqual(t, len(resp.body.Data.Tables), 1)
	})

	t.Run("不存在的表返回空列表且状态 200", func(t *testing.T) {
		resp := requestSchema(t, router, "?table_name=__no_such_table__")
		require.Equal(t, http.StatusOK, resp.code)
		require.Equal(t, 0, resp.body.Code)
		assert.Empty(t, resp.body.Data.Tables, "不存在的表应返回空列表")
	})
}

// schemaCall 封装一次请求的结果
type schemaCall struct {
	code int
	body schemaResp
}

// requestSchema 发起一次 GET 请求并解析响应
func requestSchema(t *testing.T, router *gin.Engine, query string) schemaCall {
	t.Helper()
	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "/api/v1/agent/text2sql/schema"+query, nil)
	router.ServeHTTP(w, r)

	t.Logf("GET %s -> %d, body=%s", query, w.Code, w.Body.String())

	var body schemaResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "响应应为合法 JSON")
	return schemaCall{code: w.Code, body: body}
}

// setupSchemaTestTable 创建结构已知的测试表
func setupSchemaTestTable(t *testing.T, db *gorm.DB) {
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS t2s_schema_test (
		id INT PRIMARY KEY AUTO_INCREMENT,
		name VARCHAR(100) NOT NULL COMMENT '名称',
		remark VARCHAR(255) COMMENT '备注'
	) COMMENT='Schema 端点集成测试表'`).Error, "创建测试表失败")
}

// cleanupSchemaTestTable 删除测试表
func cleanupSchemaTestTable(t *testing.T, db *gorm.DB) {
	db.Exec(`DROP TABLE IF EXISTS t2s_schema_test`)
}
