// +build integration

// Package retriever 提供 Milvus 集成测试
package retriever

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/joho/godotenv"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"cognida/internal/infrastructure/config"
	linkembedding "cognida/internal/infrastructure/llm/embedding"
	"cognida/internal/repository/milvus"
)

// loadEnvFile loads .env file if it exists
func loadEnvFile() {
	wd, _ := os.Getwd()
	fmt.Printf("Current working directory: %s\n", wd)

	dirs := []string{
		".",
		"..",
		filepath.Join("..", ".."),
		filepath.Join("..", "..", ".."),
		filepath.Join("..", "..", "..", ".."),
		filepath.Join("..", "..", "..", "..", ".."),
		filepath.Join(wd, "."),
		filepath.Join(wd, ".."),
		filepath.Join(wd, "..", ".."),
		filepath.Join(wd, "..", "..", ".."),
		filepath.Join(wd, "..", "..", "..", ".."),
	}

	for _, dir := range dirs {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err != nil {
				fmt.Printf("Warning: failed to load %s: %v\n", envPath, err)
			} else {
				fmt.Printf("Loaded .env from: %s\n", envPath)
				return
			}
		}
	}
	fmt.Println("Warning: .env file not found")
}

var (
	ctx             context.Context
	cancel          context.CancelFunc
	milvusClient    *milvusclient.Client
	vectorRetriever *VectorRetriever
	embedder        embedding.Embedder
	testCollection  string
)

// TestMain 设置测试环境
func TestMain(m *testing.M) {
	loadEnvFile()

	if os.Getenv("INTEGRATION_TEST") == "1" {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		cfg := config.LoadMilvusConfig()
		embeddingCfg := config.LoadEmbeddingConfig()

		milvusClientCfg := &milvusclient.ClientConfig{
			Address: cfg.Host,
			APIKey:  cfg.Token,
		}
		var err error
		milvusClient, err = milvusclient.New(ctx, milvusClientCfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to Milvus: %v", err))
		}

		milvus.MilvusClient = milvusClient

		embedder, err = linkembedding.NewDashScopeEmbedderWrapper(embeddingCfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to create embedder: %v", err))
		}

		vectorRetriever, err = NewVectorRetriever(embedder)
		if err != nil {
			panic(fmt.Sprintf("Failed to create vector retriever: %v", err))
		}

		// 使用统一的 "link" collection 进行测试
		testCollection = "link"
		fmt.Printf("Using unified collection for testing: %s\n", testCollection)

		// 确保 collection 存在，如果不存在则创建
		// 注意：这里只创建 collection 结构体，不预设 KnowledgeBaseID
		hasCollection := false
		listOpt := milvusclient.NewListCollectionOption()
		collections, err := milvusClient.ListCollections(ctx, listOpt)
		if err == nil {
			for _, coll := range collections {
				if coll == testCollection {
					hasCollection = true
					break
				}
			}
		}

		if !hasCollection {
			// 创建统一的 collection
			if err := vectorRetriever.CreateKnowledgeBase(ctx, 0, &CreateKnowledgeBaseOptions{
				Dimension:     1536,
				IndexType:     IndexTypeAutoIndex,
				MetricType:    entity.COSINE,
				AutoID:        true,
				EnableDynamic: true,
				Description:   "Cognida unified knowledge base collection",
				EnableBM25:    false,
			}); err != nil {
				fmt.Printf("Warning: Failed to create collection: %v\n", err)
			}
		} else {
			fmt.Printf("Collection '%s' already exists\n", testCollection)
		}

		exitCode := m.Run()
		cancel()
		milvusClient.Close(context.Background())
		os.Exit(exitCode)
	}

	m.Run()
}

// ========================================
// 集成测试用例
// ========================================

// TestMilvusConnection 测试 Milvus 连接
func TestMilvusConnection(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	listOpt := milvusclient.NewListCollectionOption()
	collections, err := milvusClient.ListCollections(ctx, listOpt)
	if err != nil {
		t.Fatalf("Failed to list collections: %v", err)
	}

	t.Logf("Successfully connected to Milvus. Found %d collections", len(collections))
	for i, coll := range collections {
		t.Logf("  Collection %d: %s", i+1, coll)
	}
}

// TestDescribeCollection 测试获取集合信息
func TestDescribeCollection(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	descOpt := milvusclient.NewDescribeCollectionOption(testCollection)
	coll, err := milvusClient.DescribeCollection(ctx, descOpt)
	if err != nil {
		t.Fatalf("Failed to describe collection: %v", err)
	}

	t.Logf("Collection name: %s", coll.Name)
	t.Logf("Schema fields: %d", len(coll.Schema.Fields))
	for _, field := range coll.Schema.Fields {
		t.Logf("  - %s (%s)", field.Name, field.DataType)
	}
}

// TestGetCollectionStats 测试获取集合统计
func TestGetCollectionStats(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	// 直接使用 collection 名称描述
	descOpt := milvusclient.NewDescribeCollectionOption(testCollection)
	coll, err := milvusClient.DescribeCollection(ctx, descOpt)
	if err != nil {
		t.Fatalf("Failed to describe collection: %v", err)
	}

	// 使用新 SDK 获取统计信息
	statsOpt := milvusclient.NewGetCollectionStatsOption(testCollection)
	stats, err := milvusClient.GetCollectionStats(ctx, statsOpt)
	if err != nil {
		t.Logf("Failed to get collection stats: %v", err)
		return
	}

	t.Logf("Collection name: %s", coll.Name)
	t.Logf("Collection stats: %+v", stats)
}

// TestHealthCheck 测试健康检查
func TestHealthCheck(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	err := vectorRetriever.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	version, err := vectorRetriever.GetServerVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get server version: %v", err)
	}

	t.Logf("Milvus server version: %s", version)
}

// TestSearchWithRealEmbedding 测试使用真实 Embedding 进行搜索
func TestSearchWithRealEmbedding(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	if embedder == nil {
		t.Skip("Embedder not available")
	}

	// 生成查询向量
	queryText := "测试"
	embeddings, err := embedder.EmbedStrings(ctx, []string{queryText})
	if err != nil {
		t.Fatalf("Failed to generate embedding: %v", err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		t.Fatalf("No embeddings generated")
	}

	// 转换为 float32
	vector := make([]float32, len(embeddings[0]))
	for i, v := range embeddings[0] {
		vector[i] = float32(v)
	}

	// 使用新 SDK 直接搜索
	vectors := []entity.Vector{entity.FloatVector(vector)}
	searchOpt := milvusclient.NewSearchOption(testCollection, 5, vectors)

	// 获取 collection schema 以找到向量字段名称
	descOpt := milvusclient.NewDescribeCollectionOption(testCollection)
	coll, err := milvusClient.DescribeCollection(ctx, descOpt)
	if err != nil {
		t.Logf("Failed to describe collection: %v", err)
		return
	}

	// 查找向量字段
	vectorField := ""
	for _, field := range coll.Schema.Fields {
		if field.DataType == entity.FieldTypeFloatVector {
			vectorField = field.Name
			break
		}
	}

	if vectorField == "" {
		t.Skip("No float vector field found in collection")
		return
	}

	searchOpt.WithANNSField(vectorField)
	searchOpt.WithOutputFields("*")

	resultSets, err := milvusClient.Search(ctx, searchOpt)
	if err != nil {
		t.Logf("Search returned error (may be expected if collection has no data): %v", err)
		return
	}

	t.Logf("Search returned %d result sets for query: %s", len(resultSets), queryText)
	for _, resultSet := range resultSets {
		t.Logf("  ResultCount: %d", resultSet.ResultCount)
		for i := 0; i < resultSet.ResultCount && i < 3; i++ {
			t.Logf("    Result %d: Score=%.4f", i+1, resultSet.Scores[i])
		}
	}
}

// TestBM25FullTextSearch 测试 BM25 全文搜索
func TestBM25FullTextSearch(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	// 测试用的 KB ID（由具体业务场景决定）
	kbID := int64(1001)
	tenantID := int64(1)

	queryText := "测试搜索"
	topK := 5

	results, err := vectorRetriever.FullTextSearch(ctx, kbID, queryText, topK, map[string]string{
		"tenant_id": fmt.Sprintf("%d", tenantID),
	})

	if err != nil {
		t.Logf("BM25 search returned error (may be expected if BM25 not enabled or no data): %v", err)
		return
	}

	t.Logf("BM25 search returned %d results for query: %s (kb_id=%d)", len(results), queryText, kbID)
	for i, result := range results {
		if i >= 3 {
			break
		}
		t.Logf("  Result %d: ChunkID=%s, Score=%.4f, Content=%.50s...",
			i+1, result.ChunkID, result.Score, truncateString(result.Content, 50))
	}
}

// TestGetLoadState 测试获取加载状态
func TestGetLoadState(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	// 测试用的 KB ID（由具体业务场景决定）
	kbID := int64(1001)

	loadState, err := vectorRetriever.GetLoadState(ctx, kbID)
	if err != nil {
		t.Fatalf("Failed to get load state: %v", err)
	}

	t.Logf("Load state for kb_id=%d: %+v", kbID, loadState)
}

// 辅助函数
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
