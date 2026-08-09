// Package milvus provides unit tests for cache collection
package milvus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ========================================
// Unit Tests
// ========================================

// TestBuildSchema 测试 Schema 构建
func TestBuildSchema(t *testing.T) {
	repo := &CacheVectorRepository{}
	schema := repo.buildSchema()

	assert.NotNil(t, schema)
	// Schema 构建成功即可验证基础功能
}

// TestBuildFilterExpr 测试过滤表达式构建
func TestBuildFilterExpr(t *testing.T) {
	repo := &CacheVectorRepository{}

	tests := []struct {
		name      string
		tenantID  int64
		agentType string
		expected  string
	}{
		{
			name:      "仅租户过滤",
			tenantID:  123,
			agentType: "",
			expected:  "tenant_id == 123",
		},
		{
			name:      "租户+Agent过滤",
			tenantID:  456,
			agentType: "rag_agent",
			expected:  "tenant_id == 456 && agent_type == 'rag_agent'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repo.buildFilterExpr(tt.tenantID, tt.agentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// 注意：集成测试 TestCacheVectorRepositoryIntegration 已移至
// cache_collection_integration_test.go（受 //go:build integration 约束），
// 在该文件中通过 TestMain 初始化 Milvus 客户端单例后运行。

// TestGetCollectionName 测试集合名称生成
func TestGetCollectionName(t *testing.T) {
	// Semantic cache 使用固定名称
	const expectedName = "semantic_cache_vectors"
	assert.Equal(t, expectedName, SemanticCacheCollectionName)
}

// ========================================
// Benchmark Tests
// ========================================

// BenchmarkBuildFilterExpr 性能测试
func BenchmarkBuildFilterExpr(b *testing.B) {
	repo := &CacheVectorRepository{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.buildFilterExpr(12345, "rag_agent")
	}
}
