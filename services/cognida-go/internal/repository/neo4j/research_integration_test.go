//go:build integration

package neo4j

import (
	llmchat "cognida/internal/infrastructure/llm/chat"
	"cognida/internal/model/common"
	agentuc "cognida/internal/service/agent"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/knowledge"
	rag "cognida/internal/model/rag"
	mysqlrepo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/adapters"
	"cognida/internal/service/agent/tools"
	knowledgeuc "cognida/internal/service/knowledge"
	pipeline "cognida/internal/service/knowledge/pipeline"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	driver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type researchIntegrationLLM struct{ model.BaseChatModel }

type researchIntegrationRegistry map[string]tool.BaseTool

func (r researchIntegrationRegistry) Get(name string) (tool.BaseTool, bool) {
	value, ok := r[name]
	return value, ok
}

func (researchIntegrationLLM) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if strings.Contains(messages[0].Content, "should_continue") {
		return schema.AssistantMessage(`{"intermediate_answer":"Proof evidence","should_continue":false,"next_query":""}`, nil), nil
	}
	return schema.AssistantMessage("Proof", nil), nil
}

func TestResearchRealGraphAndMySQLScope(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	kb1, kb2 := uuid.NewString(), uuid.NewString()
	doc1, doc2 := uuid.NewString(), uuid.NewString()
	chunk1, chunk2, disabled := uuid.NewString(), uuid.NewString(), uuid.NewString()
	kbs := mysqlrepo.NewKnowledgeBaseRepository(db, true)
	docs := mysqlrepo.NewKnowledgeRepository(db)
	chunks := mysqlrepo.NewChunkRepository(db)
	t.Cleanup(func() {
		for _, table := range []string{"chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Delete(map[string]any{}).Error)
		}
	})
	for _, x := range []struct {
		kb, doc, chunk string
		owner          int64
	}{{kb1, doc1, chunk1, tenant}, {kb2, doc2, chunk2, tenant + 1}} {
		require.NoError(t, kbs.Create(ctx, &knowledge.KnowledgeBase{ID: x.kb, TenantID: x.owner, UserID: 9, Name: "Research review", Status: 1}))
		require.NoError(t, docs.Create(ctx, &knowledge.Knowledge{ID: x.doc, TenantID: x.owner, KnowledgeBaseID: x.kb, UserID: 9, Type: "text", Title: "Proof", Source: "review", EnableStatus: "enabled", ParseStatus: "completed"}))
		require.NoError(t, chunks.Create(ctx, &knowledge.Chunk{ID: x.chunk, TenantID: x.owner, KnowledgeBaseID: x.kb, KnowledgeID: x.doc, Content: "Proof evidence from a real scoped document.", IsEnabled: true, ChunkType: "text"}))
	}
	require.NoError(t, chunks.Create(ctx, &knowledge.Chunk{ID: disabled, TenantID: tenant, KnowledgeBaseID: kb1, KnowledgeID: doc1, Content: "Disabled evidence", IsEnabled: true, ChunkType: "text"}))
	require.NoError(t, db.Table("chunks").Where("id = ?", disabled).Update("is_enabled", false).Error)
	uri, username, password, database := getTestConfig()
	raw, err := driver.NewDriverWithContext(uri, driver.BasicAuth(username, password, ""))
	require.NoError(t, err)
	graph := &Neo4jRepository{driver: raw, dbName: database} // Avoid global index/normalization writes in isolated tests.
	t.Cleanup(func() { require.NoError(t, raw.Close(context.Background())) })
	ns := knowledge.NameSpace{TenantID: strconv.FormatInt(tenant, 10), KnowledgeBaseID: kb1}
	nsOther := knowledge.NameSpace{TenantID: strconv.FormatInt(tenant+1, 10), KnowledgeBaseID: kb2}
	t.Cleanup(func() {
		require.NoError(t, graph.DeleteGraph(context.Background(), []knowledge.NameSpace{ns, nsOther}))
	})
	require.NoError(t, graph.AddNode(ctx, ns, &knowledge.GraphNode{ID: uuid.NewString(), Name: "Proof", EntityType: "Review", Chunks: []string{chunk1, chunk2, disabled}}))
	require.NoError(t, graph.AddNode(ctx, nsOther, &knowledge.GraphNode{ID: uuid.NewString(), Name: "Proof", EntityType: "Review", Chunks: []string{chunk2}}))
	meta := mysqlrepo.NewGraphQueryRepository(db, true)
	meta.SetChunkRepository(chunks)
	ret := pipeline.NewRetriever(nil, chunks, nil, nil, pipeline.NewGraphRepositoryAdapter(graph), pipeline.NewGraphQueryAdapter(meta), nil)
	out, err := ret.GraphRetrieve(ctx, ns.TenantID, kb1, "Proof", &rag.RetrieveOptions{TopK: 10})
	require.NoError(t, err)
	require.Len(t, out.Results, 1)
	require.Equal(t, chunk1, out.Results[0].ChunkID)
	out, err = ret.GraphRetrieve(ctx, nsOther.TenantID, kb1, "Proof", &rag.RetrieveOptions{TopK: 10})
	require.NoError(t, err)
	require.Empty(t, out.Results)
	capability := knowledgeuc.NewRetrievalCapability(ret, nil)
	adapter := adapters.NewRAGRetrieverAdapter(capability, kbs)
	trusted := agentctx.WithGraphEnabled(agentctx.WithAllowedKBIDs(agentctx.NewAgentContext(ctx, "", tenant, 9, "Proof"), []string{kb1, kb2}), true)
	result, err := adapter.Query(trusted, &tools.RAGQueryRequest{Query: "Proof", TopK: 5, RetrievalMode: "graph"})
	require.NoError(t, err)
	require.Len(t, result.Chunks, 1)
	require.Equal(t, kb1, result.Chunks[0].KnowledgeBaseID)
	for _, base := range tools.NewRAGOptimizationTools(adapter, researchIntegrationLLM{}) {
		raw, err := base.(tool.InvokableTool).InvokableRun(trusted, `{"query":"Proof"}`)
		require.NoError(t, err)
		var result tools.RAGOptimizationResult
		require.NoError(t, json.Unmarshal([]byte(raw), &result))
		require.NotEmpty(t, result.Chunks)
		for _, c := range result.Chunks {
			require.Equal(t, kb1, c.KnowledgeBaseID)
			require.NotEqual(t, chunk2, c.ChunkID)
		}
	}
	if os.Getenv("RESEARCH_REAL_LLM") == "1" {
		// Only synthetic review evidence is sent to the configured provider.
		llm, err := llmchat.NewChatModel(ctx, &llmchat.ChatConfig{Source: common.ModelSource(os.Getenv("CHAT_SOURCE")), BaseURL: os.Getenv("CHAT_BASE_URL"), ModelName: os.Getenv("CHAT_MODEL_NAME"), APIKey: os.Getenv("CHAT_API_KEY"), Provider: os.Getenv("CHAT_PROVIDER")})
		require.NoError(t, err)
		registry := researchIntegrationRegistry{"rag_query": tools.NewRAGQueryTool(adapter)}
		for _, optimized := range tools.NewRAGOptimizationTools(adapter, llm) {
			info, err := optimized.Info(ctx)
			require.NoError(t, err)
			registry[info.Name] = optimized
		}
		report, err := agentuc.NewResearchService(nil, nil).WithDependencies(llm, registry).Execute(trusted, &agentuc.DeepResearchRequest{Query: "Research the Proof evidence. Explain what this source establishes and what remains unknown, with verbatim cited excerpts.", GraphEnabled: true, Options: &agentuc.DeepResearchOptions{MaxSubQuestions: 1}})
		require.NoError(t, err)
		require.True(t, report.Success)
		require.True(t, report.Metadata.CitationChecked)
		require.NotEmpty(t, report.CitationChecks)
		for _, source := range report.Sources {
			require.Equal(t, kb1, source.KnowledgeBaseID)
		}
		t.Log("configured model produced a scoped report with verified citation excerpts")
	}
	forbidden := agentctx.WithAllowedKBIDs(trusted, []string{kb2})
	result, err = adapter.Query(forbidden, &tools.RAGQueryRequest{Query: "Proof", TopK: 5, RetrievalMode: "graph"})
	require.NoError(t, err)
	require.Empty(t, result.Chunks)
	require.NoError(t, db.Table("knowledge").Where("id = ?", doc1).Update("enable_status", "disabled").Error)
	out, err = ret.GraphRetrieve(ctx, ns.TenantID, kb1, "Proof", &rag.RetrieveOptions{TopK: 10})
	require.NoError(t, err)
	require.Empty(t, out.Results)
}
