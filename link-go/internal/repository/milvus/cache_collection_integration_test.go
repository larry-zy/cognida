//go:build integration

// Package milvus 提供语义缓存向量仓储的集成测试。
//
// 运行方式（需先准备真实 Milvus 并配置 .env 的 MILVUS_HOST / MILVUS_TOKEN）：
//
//	INTEGRATION_TEST=1 go test -tags=integration ./internal/repository/milvus/ -run Integration -v
//
// 本地 standalone Milvus 示例（OrbStack/Docker）：
//
//	MILVUS_HOST=localhost:19530 MILVUS_TOKEN=root:Milvus INTEGRATION_TEST=1 \
//	  go test -tags=integration ./internal/repository/milvus/ -run Integration -v
package milvus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/infrastructure/config"
	"link/internal/model/cache"
)

// loadRepoEnv 从测试工作目录逐级向上查找并加载仓库根的 .env，
// 使 INTEGRATION_TEST / MILVUS_HOST / MILVUS_TOKEN 等以 .env 为准。
// godotenv.Load 不覆盖已存在的环境变量，因此命令行内联设置仍优先。
func loadRepoEnv() {
	wd, _ := os.Getwd()
	dir := wd
	for i := 0; i < 8; i++ {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			_ = godotenv.Load(envPath)
			fmt.Printf("[integration] loaded .env from: %s\n", envPath)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	fmt.Println("[integration] .env not found, relying on process env")
}

// TestMain 初始化 Milvus 客户端单例，供集成测试使用。
// 先加载 .env，再判断 INTEGRATION_TEST；未开启时不连接，用例自行跳过。
func TestMain(m *testing.M) {
	loadRepoEnv()

	if os.Getenv("INTEGRATION_TEST") == "1" {
		cfg := config.LoadMilvusConfig()
		if err := InitMilvus(cfg); err != nil {
			fmt.Printf("[integration] 初始化 Milvus 失败: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = CloseMilvus() }()
	}
	os.Exit(m.Run())
}

// TestCacheVectorRepositoryIntegration 验证语义缓存 Collection 的
// 建表/建索引/加载/插入/检索/删除全链路。
func TestCacheVectorRepositoryIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("设置 INTEGRATION_TEST=1 并配置真实 Milvus 后运行")
	}

	ctx := context.Background()

	// 创建仓储（探测维度传 0 → 回退 SemanticCacheVectorDim）
	repo, err := NewCacheVectorRepository(0)
	require.NoError(t, err)
	require.NotNil(t, repo)

	// 检查 Collection 是否存在
	has, err := repo.HasCacheCollection(ctx)
	require.NoError(t, err)

	// 如果不存在，创建
	if !has {
		err = repo.CreateCacheCollection(ctx)
		require.NoError(t, err)
	}

	// 创建索引
	err = repo.CreateCacheIndex(ctx)
	require.NoError(t, err)

	// 加载 Collection
	err = repo.LoadCacheCollection(ctx)
	require.NoError(t, err)

	// 测试插入
	entry := &cache.CacheEntry{
		CacheID:   "test_cache_001",
		Query:     "测试查询",
		Response:  "测试响应",
		Model:     "test-model",
		AgentType: "test_agent",
		TenantID:  1,
		Vector:    make([]float32, SemanticCacheVectorDim),
		CreatedAt: 1234567890,
	}

	err = repo.Insert(ctx, entry)
	assert.NoError(t, err)

	// 测试搜索
	results, err := repo.Search(ctx, entry.Vector, 1, "test_agent", 5)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// 测试删除
	err = repo.Delete(ctx, "test_cache_001")
	assert.NoError(t, err)

	// 测试按 Agent 删除
	err = repo.DeleteByAgent(ctx, 1, "test_agent")
	assert.NoError(t, err)

	// 测试按租户删除
	err = repo.DeleteByTenant(ctx, 1)
	assert.NoError(t, err)
}
