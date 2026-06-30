# Link AI 服务增强规划

## 文档说明

本文档聚焦Link AI服务核心能力的增强，暂不涉及数据治理等工程能力。

**更新时间**: 2026-05-03

---

## 目录

- [一、AI服务现状分析](#一ai服务现状分析)
- [二、核心增强方向](#二核心增强方向)
- [三、优先级P0：智能检索增强](#三优先级p0智能检索增强)
- [四、优先级P1：Agent能力增强](#四优先级p1agent能力增强)
- [五、优先级P2：评测能力增强](#五优先级p2评测能力增强)
- [六、实施路线图](#六实施路线图)

---

## 一、AI服务现状分析

### 1.1 已有AI能力

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           当前AI能力盘点                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        检索能力 ✅                                  │   │
│  │  ├─ 向量检索 (Milvus)       稠密向量语义检索                          │   │
│  │  ├─ BM25检索               稀疏向量关键词检索                        │   │
│  │  ├─ 图谱检索 (Neo4j)       实体关系图谱检索                          │   │
│  │  └─ 混合检索                RRF融合多种检索结果                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        Agent能力 ✅                                  │   │
│  │  ├─ MultiAgentOrchestrator 多Agent协调                              │   │
│  │  ├─ AgenticRAGAgent        简化版RAG Agent                          │   │
│  │  ├─ Deep Research Agent    深度研究Agent                            │   │
│  │  ├─ ReAct模式              思考-行动循环                            │   │
│  │  └─ 工具调用                rag_query, web_search, graph_query        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        评测能力 ✅                                  │   │
│  │  ├─ 检索评测                NDCG, MRR, MAP, Precision, Recall       │   │
│  │  ├─ 生成评测                BLEU, ROUGE                             │   │
│  │  └─ 数据集管理             QA对管理、评估任务                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        知识能力 ✅                                  │   │
│  │  ├─ 文档处理                上传、分块、向量化                        │   │
│  │  ├─ 知识图谱                实体关系提取、存储                       │   │
│  │  └─ 知识库管理              多租户知识库                             │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 与业界对比

| 能力维度 | Link | Dify | LlamaIndex | LangChain |
|---------|------|------|------------|----------|
| 向量检索 | ✅ | ✅ | ✅ | ✅ |
| 混合检索 | ✅ | ⚠️ | ⚠️ | ⚠️ |
| 知识图谱 | ✅ 深度 | ❌ | ⚠️ Property Graph | ⚠️ |
| Agent编排 | ✅ StateGraph | ✅ 工作流 | ✅ Agent Workflows | ✅ LangGraph |
| 深度研究 | ✅ Deep Research | ❌ | ⚠️ | ⚠️ |
| 评测系统 | ✅ 完整 | ⚠️ 基础 | ❌ | ❌ |
| Rerank | ✅ | ⚠️ | ✅ | ✅ |
| 多模态 | 🚧 | 🚧 | ✅ LlamaParse | ⚠️ |

### 1.3 核心差异优势

```
Link的独特优势：
├─ 混合检索 + Rerank (召回更准)
├─ Neo4j深度知识图谱 (理解更深)
├─ StateGraph编排 (控制更精)
├─ Deep Research Agent (研究更深)
└─ 完整评测系统 (优化有据)
```

---

## 二、核心增强方向

### 2.1 增强优先级矩阵

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         AI服务增强优先级矩阵                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  重要性              │ 低 │ 中 │ 高 │                                      │
│  ─────────────────────┼───┼───┼───┼───────────────────────────────────  │
│  实现难度            │   │   │   │                                      │
│  ─────────────────────┼───┼───┼───┼───────────────────────────────────  │
│                      │   │   │   │                                      │
│  低                  │ P2 │ P2 │ P1 │  Query理解、多轮对话优化              │
│  中                  │ P3 │ P1 │ P0 │  重排序、混合检索优化              │
│  高                  │ P3 │ P2 │ P1 │  Agent记忆、自主学习              │
│                      │   │   │   │                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  P0: 智能检索增强 - 提升核心RAG能力                                │   │
│  │  P1: Agent能力增强 - 让Agent更聪明                                  │   │
│  │  P2: 评测能力增强 - 精准评估和优化                                  │   │
│  │  P3: 交互体验增强 - 更自然的对话体验                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、优先级P0：智能检索增强

### 3.1 重排序优化

#### 当前状态
- 已有基础Rerank支持
- 需要增强：多模型支持、策略优化

#### 增强方案

```go
// internal/infrastructure/reranker/enhanced_reranker.go
package reranker

// 增强型重排序器
type EnhancedReranker struct {
    // 支持多个重排序模型
    models map[string]RerankModel
    
    // 重排序策略
    strategies map[string]RerankStrategy
}

type RerankModel interface {
    Rerank(ctx context.Context, query string, documents []Document, topK int) ([]Document, error)
    Name() string
}

// Cohere Rerank
type CohereRerankModel struct {
    client *http.Client
    apiKey string
    model  string // rerank-v3, rerank-multilingual-v3
}

func (m *CohereRerankModel) Rerank(ctx context.Context, query string, documents []Document, topK int) ([]Document, error) {
    // 调用Cohere Rerank API
    payload := map[string]interface{}{
        "query": query,
        "documents": func() (docs []string) {
            for _, d := range documents {
                docs = append(docs, d.Content)
            }
            return docs
        }(),
        "top_n": topK,
        "model": m.model,
    }
    
    resp, err := m.client.Post("https://api.cohere.ai/v1/rerank", payload)
    if err != nil {
        return nil, err
    }
    
    // 解析结果
    var result CohereRerankResponse
    json.Unmarshal(resp, &result)
    
    // 重新排序文档
    reranked := make([]Document, len(result.Results))
    for i, r := range result.Results {
        reranked[i] = documents[r.Index]
    }
    
    return reranked, nil
}

// BGE Rerank (本地模型)
type BGERerankModel struct {
    model *embedding.Model
}

func (m *BGERerankModel) Rerank(ctx context.Context, query string, documents []Document, topK int) ([]Document, error) {
    // 计算query和每个document的相似度
    queryEmbedding := m.model.Embed(query)
    
    type docScore struct {
        doc   Document
        score float64
    }
    
    scores := make([]docScore, len(documents))
    for i, doc := range documents {
        docEmbedding := m.model.Embed(doc.Content)
        score := cosineSimilarity(queryEmbedding, docEmbedding)
        scores[i] = docScore{doc: doc, score: score}
    }
    
    // 按分数排序
    sort.Slice(scores, func(i, j int) bool {
        return scores[i].score > scores[j].score
    })
    
    // 返回topK
    result := make([]Document, min(topK, len(scores)))
    for i := 0; i < len(result); i++ {
        result[i] = scores[i].doc
    }
    
    return result, nil
}

// 重排序策略
type RerankStrategy struct {
    Name      string
    Models    []string              // 使用的模型列表
    Weights   map[string]float64    // 模型权重
    FusionMethod string             // 融合方法: weighted, rank, reciprocal_rank
}

func (s *RerankStrategy) Execute(ctx context.Context, query string, documents []Document, topK int) ([]Document, error) {
    // 并行执行多个重排序模型
    var wg sync.WaitGroup
    var mu sync.Mutex
    results := make(map[string][]Document)
    errs := make(map[string]error)
    
    for _, modelName := range s.Models {
        wg.Add(1)
        go func(name string) {
            defer wg.Done()
            
            model := s.models[name]
            reranked, err := model.Rerank(ctx, query, documents, topK)
            
            mu.Lock()
            defer mu.Unlock()
            
            if err != nil {
                errs[name] = err
            } else {
                results[name] = reranked
            }
        }(modelName)
    }
    
    wg.Wait()
    
    // 融合多个模型的结果
    switch s.FusionMethod {
    case "weighted":
        return s.weightedFusion(results, s.Weights)
    case "reciprocal_rank":
        return s.reciprocalRankFusion(results)
    case "rank":
        return s.rankFusion(results, s.Weights)
    default:
        return s.reciprocalRankFusion(results)
    }
}

func (s *RerankStrategy) reciprocalRankFusion(results map[string][]Document) ([]Document, error) {
    // RRF融合算法
    docScores := make(map[string]float64)
    
    k := 60.0 // RRF常数
    
    for _, docs := range results {
        for rank, doc := range docs {
            score := 1.0 / (float64(k) + float64(rank))
            docScores[doc.ID] += score
        }
    }
    
    // 按分数排序
    type scoreDoc struct {
        id    string
        score float64
    }
    
    var sorted []scoreDoc
    for id, score := range docScores {
        sorted = append(sorted, scoreDoc{id: id, score: score})
    }
    
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].score > sorted[j].score
    })
    
    // 返回排序后的文档
    // ... 从原documents中提取
    
    return sortedDocs, nil
}
```

### 3.2 混合检索优化

#### 增强方案

```go
// internal/application/rag/hybrid_retriever.go
package rag

// 增强型混合检索器
type HybridRetriever struct {
    denseRetriever   VectorRetriever   // 向量检索
    sparseRetriever  SparseRetriever  // BM25检索
    graphRetriever   GraphRetriever   // 图谱检索
    reranker         EnhancedReranker // 重排序
    
    // 融合策略
    fusionStrategy FusionStrategy
}

type FusionStrategy string

const (
    FusionRRR        FusionStrategy = "rrr"        // Reciprocal Rank Fusion
    FusionWeighted   FusionStrategy = "weighted"   // 加权融合
    FusionSwitch     FusionStrategy = "switch"     // 动态切换
    FusionLearned    FusionStrategy = "learned"    // 学习型融合
)

// 智能融合决策
type AdaptiveFusion struct {
    // 查询分析器
    queryAnalyzer *QueryAnalyzer
    
    // 融合策略选择器
    strategySelector *StrategySelector
}

type QueryAnalyzer struct {
    // 分析查询类型
    llmClient LLMClient
}

type QueryAnalysis struct {
    Type      string  `json:"type"` // factual, navigational, transactional, comparison
    Keywords  []string `json:"keywords"`
    Entities  []string `json:"entities"`
    Intent    string  `json:"intent"` // search, compare, summarize, complex
}

func (a *QueryAnalyzer) Analyze(ctx context.Context, query string) (*QueryAnalysis, error) {
    // 使用LLM分析查询
    prompt := fmt.Sprintf(`
分析以下查询的类型和意图：
查询：%s

请返回JSON格式：
{
    "type": "查询类型",
    "keywords": ["关键词列表"],
    "entities": ["实体列表"],
    "intent": "用户意图"
}

查询类型包括：
- factual: 事实性查询（"什么是..."）
- navigational: 导航性查询（"如何找到..."）
- transactional: 事务性查询（"创建..."）
- comparison: 比较性查询（"A和B的区别"）
- complex: 复杂查询（多步骤、多方面）

用户意图包括：
- search: 搜索信息
- compare: 对比比较
- summarize: 总结概括
- execute: 执行操作
`, query)
    
    response, err := a.llmClient.Generate(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    var analysis QueryAnalysis
    json.Unmarshal([]byte(response), &analysis)
    
    return &analysis, nil
}

// 根据查询分析选择最优检索策略
func (s *AdaptiveFusion) SelectStrategy(analysis *QueryAnalysis) FusionStrategy {
    switch analysis.Intent {
    case "search":
        // 搜索意图：混合检索
        if analysis.Type == "factual" {
            // 事实性查询：偏重图谱和关键词
            return FusionWeighted // 权重：图谱>BM25>向量
        } else {
            // 其他查询：均衡混合
            return FusionRRR
        }
    case "compare":
        // 比较意图：需要图谱关系
        return FusionWeighted // 权重：图谱>向量>BM25
    case "summarize":
        // 总结意图：需要向量检索
        return FusionRRR
    case "execute":
        // 执行意图：可能不需要检索
        return FusionSwitch
    default:
        return FusionRRR
    }
}

// 增强型检索
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, opts *RetrieverOptions) (*RetrievalResult, error) {
    // 1. 分析查询
    analysis, _ := r.queryAnalyzer.Analyze(ctx, query)
    
    // 2. 选择融合策略
    strategy := r.strategySelector.SelectStrategy(analysis)
    
    // 3. 并行执行多种检索
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(3)
    
    var (
        denseDocs  []Document
        sparseDocs []Document
        graphDocs  []Document
    )
    
    // 向量检索
    g.Go(func() error {
        docs, err := r.denseRetriever.Retrieve(ctx, query, opts.TopK*3) // 召回更多
        if err == nil {
            denseDocs = docs
        }
        return nil
    })
    
    // BM25检索
    g.Go(func() error {
        docs, err := r.sparseRetriever.Retrieve(ctx, query, opts.TopK*3)
        if err == nil {
            sparseDocs = docs
        }
        return nil
    })
    
    // 图谱检索
    g.Go(func() error {
        docs, err := r.graphRetriever.Retrieve(ctx, query, opts.TopK*3)
        if err == nil {
            graphDocs = docs
        }
        return nil
    })
    
    g.Wait()
    
    // 4. 初步融合
    var fusedDocs []Document
    switch strategy {
    case FusionRRR:
        fusedDocs = r.rrfFusion(denseDocs, sparseDocs, graphDocs, k:60)
    case FusionWeighted:
        // 根据查询类型调整权重
        weights := r.calculateWeights(analysis)
        fusedDocs = r.weightedFusion(denseDocs, sparseDocs, graphDocs, weights)
    case FusionSwitch:
        // 根据查询类型选择单一最佳检索方式
        fusedDocs = r.switchFusion(analysis, denseDocs, sparseDocs, graphDocs)
    }
    
    // 5. 重排序
    if r.reranker != nil {
        finalDocs, err := r.reranker.Rerank(ctx, query, fusedDocs, opts.TopK)
        if err == nil {
            return &RetrievalResult{
                Documents: finalDocs,
                Strategy:  string(strategy),
                Reranked:  true,
            }, nil
        }
    }
    
    return &RetrievalResult{
        Documents: fusedDocs[:opts.TopK],
        Strategy:  string(strategy),
        Reranked:  false,
    }, nil
}

// RRF融合
func (r *HybridRetriever) rrfFusion(dense, sparse, graph []Document, k int) []Document {
    scores := make(map[string]float64)
    
    for _, docs := range [][]Document{dense, sparse, graph} {
        for rank, doc := range docs {
            score := 1.0 / (float64(k) + float64(rank))
            scores[doc.ID] += score
        }
    }
    
    // 按分数排序去重
    return r.sortByScores(scores)
}
```

### 3.3 查询理解增强

```go
// internal/application/rag/query_understanding.go
package rag

// 查询理解器
type QueryUnderstanding struct {
    llmClient LLMClient
    kbService KnowledgeBaseService
}

type QueryRewrite struct {
    Original      string   `json:"original"`
    Rewritten     []string `json:"rewritten"` // 多个改写查询
    Intent        string   `json:"intent"`
    Entities      []string `json:"entities"`
    Scope         []string `json:"scope"`     // 相关知识库
}

func (q *QueryUnderstanding) Understand(ctx context.Context, query string) (*QueryRewrite, error) {
    // 1. 分析查询意图
    intent, err := q.analyzeIntent(ctx, query)
    
    // 2. 识别实体
    entities, err := q.extractEntities(ctx, query)
    
    // 3. 确定范围
    scope, err := q.determineScope(ctx, query, entities)
    
    // 4. 查询重写
    rewritten, err := q.rewriteQuery(ctx, query, intent, entities)
    
    return &QueryRewrite{
        Original:  query,
        Rewritten: rewritten,
        Intent:    intent,
        Entities:   entities,
        Scope:      scope,
    }, nil
}

func (q *QueryUnderstanding) rewriteQuery(ctx context.Context, query string, intent string, entities []string) ([]string, error) {
    prompt := fmt.Sprintf(`
作为搜索专家，请重写以下查询以获得更好的搜索结果。

原始查询：%s
查询意图：%s
识别实体：%v

请生成3-5个重写版本的查询，要求：
1. 保留原始意图
2. 使用同义词和不同的表达方式
3. 考虑用户可能的真实需求
4. 针对搜索引擎优化

返回JSON格式：
{
    "rewritten": ["查询1", "查询2", "查询3"]
}
`, query, intent, entities)
    
    response, err := q.llmClient.Generate(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    var result struct {
        Rewritten []string `json:"rewritten"`
    }
    json.Unmarshal([]byte(response), &result)
    
    return result.Rewritten, nil
}

// 多路检索 + 结果聚合
func (r *HybridRetriever) MultiRetrieve(ctx context.Context, queryRewrite *QueryRewrite, opts *RetrieverOptions) (*RetrievalResult, error) {
    // 合并原始查询和重写查询
    queries := append([]string{queryRewrite.Original}, queryRewrite.Rewritten...)
    
    // 对每个查询执行检索
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(len(queries))
    
    results := make(map[string][]Document)
    var mu sync.Mutex
    
    for _, q := range queries {
        q := q // capture loop variable
        g.Go(func() error {
            docs, err := r.Retrieve(ctx, q, opts)
            if err == nil {
                mu.Lock()
                results[q] = docs.Documents
                mu.Unlock()
            }
            return nil
        })
    }
    
    g.Wait()
    
    // 聚合所有结果，去重，重排序
    allDocs := r.aggregateResults(results)
    
    // 最终重排序
    if r.reranker != nil {
        finalDocs, err := r.reranker.Rerank(ctx, queryRewrite.Original, allDocs, opts.TopK)
        if err == nil {
            return &RetrievalResult{
                Documents: finalDocs,
                Strategy:  "multi_query",
                Reranked:  true,
            }, nil
        }
    }
    
    // 去重返回
    return &RetrievalResult{
        Documents: r.deduplicate(allDocs)[:opts.TopK],
        Strategy:  "multi_query",
        Reranked:  false,
    }, nil
}
```

---

## 四、优先级P1：Agent能力增强

### 4.1 Agent记忆系统

```go
// internal/application/agent/memory/memory.go
package memory

// Agent记忆系统
type AgentMemory struct {
    // 短期记忆：当前会话
    shortTerm *ShortTermMemory
    
    // 长期记忆：跨会话持久化
    longTerm *LongTermMemory
    
    // 语义记忆：向量化存储
    semantic *SemanticMemory
    
    // 程序记忆：工具使用经验
    procedural *ProceduralMemory
}

// 短期记忆：会话上下文
type ShortTermMemory struct {
    SessionID string
    Messages  []Message
    Context   map[string]interface{}
    
    // 当前任务的状态
    TaskState map[string]interface{}
}

// 长期记忆：用户偏好、历史
type LongTermMemory struct {
    UserID string
    TenantID int64
    
    // 用户偏好
    Preferences *UserPreferences
    
    // 历史交互
    Interactions []*Interaction
    
    // 学习到的知识
    LearnedKnowledge []*LearnedItem
}

type UserPreferences struct {
    // 检索偏好
    RetrievalStyle string `json:"retrieval_style"` // concise, detailed
    
    // 响应风格
    ResponseStyle string `json:"response_style"` // formal, casual
    
    // 常用知识库
    FavoriteKBs []string `json:"favorite_kbs"`
    
    // 领域专长
    Domains []string `json:"domains"`
}

// 语义记忆：向量化存储的长期记忆
type SemanticMemory struct {
    vectorStore VectorStore
    
    // 记忆类型
    Types []MemoryType
}

type MemoryType string

const (
    MemoryTypeFact      MemoryType = "fact"       // 事实
    MemoryTypeEvent     MemoryType = "event"      // 事件
    MemoryTypeConcept   MemoryType = "concept"    // 概念
    MemoryTypeProcedure MemoryType = "procedure"  // 程序
    MemoryTypePreference MemoryType = "preference" // 偏好
)

// 存储记忆
func (m *AgentMemory) Store(ctx context.Context, memory *Memory) error {
    // 1. 存储到短期记忆
    m.shortTerm.Add(memory)
    
    // 2. 重要记忆存储到长期记忆
    if memory.Importance > 0.7 {
        if err := m.longTerm.Store(ctx, memory); err != nil {
            return err
        }
    }
    
    // 3. 可搜索的记忆存储到语义记忆
    if memory.Searchable {
        embedding := m.embed(memory.Content)
        m.semantic.Store(ctx, memory.ID, embedding, memory)
    }
    
    return nil
}

// 检索相关记忆
func (m *AgentMemory) Recall(ctx context.Context, query string, memoryTypes []MemoryType) ([]*Memory, error) {
    // 从语义记忆中检索
    embeddings := m.embed(query)
    
    results, err := m.semantic.Search(ctx, embeddings, topK: 10)
    if err != nil {
        return nil, err
    }
    
    // 按类型过滤
    var filtered []*Memory
    for _, r := range results {
        if contains(memoryTypes, r.Type) {
            filtered = append(filtered, r)
        }
    }
    
    return filtered, nil
}

// 记忆结构
type Memory struct {
    ID          string                 `json:"id"`
    Type        MemoryType             `json:"type"`
    Content     string                 `json:"content"`
    Importance  float64                `json:"importance"` // 0-1
    Searchable  bool                   `json:"searchable"`
    CreatedAt   time.Time              `json:"created_at"`
    AccessedAt  time.Time              `json:"accessed_at"`
    AccessCount int                    `json:"access_count"`
    Metadata    map[string]interface{} `json:"metadata"`
    
    // 关联信息
    RelatedIDs  []string               `json:"related_ids"`
    Source      string                 `json:"source"` // user, agent, system
}
```

### 4.2 Agent自我反思

```go
// internal/application/agent/reflection/reflection.go
package reflection

// 自我反思模块
type ReflectionModule struct {
    llmClient LLMClient
    memory   AgentMemory
}

// 执行反思
func (r *ReflectionModule) Reflect(ctx context.Context, execution *AgentExecution) (*Reflection, error) {
    prompt := r.buildReflectionPrompt(execution)
    
    response, err := r.llmClient.Generate(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    reflection := r.parseReflection(response)
    
    // 存储反思结果
    r.memory.Store(ctx, &Memory{
        Type:       MemoryTypeEvent,
        Content:    reflection.Summary,
        Importance: execution.QualityScore, // 低质量执行更重要
        Searchable: true,
        Metadata: map[string]interface{}{
            "type":       "reflection",
            "execution":   execution.ID,
            "suggestions": reflection.Suggestions,
        },
    })
    
    return reflection, nil
}

func (r *ReflectionModule) buildReflectionPrompt(execution *AgentExecution) string {
    return fmt.Sprintf(`
请对以下Agent执行过程进行反思和评估：

【用户问题】
%s

【执行过程】
%s

【最终答案】
%s

【执行质量】
用户满意度：%.2f

请从以下维度进行反思：

1. **问题理解**
   - 是否正确理解了用户意图？
   - 是否遗漏了关键信息？

2. **检索质量**
   - 检索到的信息是否相关？
   - 是否有更相关的信息被遗漏？

3. **推理过程**
   - 思考步骤是否合理？
   - 是否存在逻辑跳跃？

4. **工具使用**
   - 工具选择是否恰当？
   - 工具参数是否优化？

5. **答案质量**
   - 答案是否准确？
   - 答案是否完整？
   - 表达是否清晰？

请以JSON格式返回：
{
    "understanding": {"score": 0.8, "issues": ["问题1"]},
    "retrieval": {"score": 0.7, "issues": ["问题2"]},
    "reasoning": {"score": 0.9, "issues": ["问题3"]},
    "tool_use": {"score": 0.8, "issues": ["问题4"]},
    "answer_quality": {"score": 0.7, "issues": ["问题5"]},
    "overall_score": 0.78,
    "summary": "总结",
    "suggestions": ["改进建议1", "改进建议2"]
}
`, execution.Query, execution.Process, execution.Answer, execution.UserSatisfaction)
}

type Reflection struct {
    Understanding   *DimensionScore `json:"understanding"`
    Retrieval      *DimensionScore `json:"retrieval"`
    Reasoning      *DimensionScore `json:"reasoning"`
    ToolUse        *DimensionScore `json:"tool_use"`
    AnswerQuality  *DimensionScore `json:"answer_quality"`
    OverallScore   float64        `json:"overall_score"`
    Summary        string          `json:"summary"`
    Suggestions    []string        `json:"suggestions"`
}

type DimensionScore struct {
    Score  float64  `json:"score"`
    Issues []string `json:"issues"`
}

// 根据反思调整策略
func (r *ReflectionModule) AdjustStrategy(ctx context.Context, reflection *Reflection) (*StrategyAdjustment, error) {
    adjustments := &StrategyAdjustment{}
    
    // 根据反思结果生成调整建议
    if reflection.Understanding.Score < 0.7 {
        adjustments.QueryUnderstanding = "增加查询重写步骤"
    }
    
    if reflection.Retrieval.Score < 0.7 {
        adjustments.RetrievalStrategy = "增加召回数量，优化融合权重"
    }
    
    if reflection.ToolUse.Score < 0.7 {
        adjustments.ToolSelection = "调整工具选择策略，尝试不同工具组合"
    }
    
    if reflection.AnswerQuality.Score < 0.7 {
        adjustments.AnswerGeneration = "增加答案验证步骤，要求引用来源"
    }
    
    return adjustments, nil
}

type StrategyAdjustment struct {
    QueryUnderstanding string `json:"query_understanding,omitempty"`
    RetrievalStrategy  string `json:"retrieval_strategy,omitempty"`
    ToolSelection     string `json:"tool_selection,omitempty"`
    AnswerGeneration  string `json:"answer_generation,omitempty"`
}
```

### 4.3 多Agent协作增强

```go
// internal/application/agent/collaboration/team.go
package collaboration

// Agent团队
type AgentTeam struct {
    ID       string
    Name     string
    Members  []*TeamMember
    Strategy TeamStrategy
}

type TeamMember struct {
    ID          string
    AgentType   string // planner, researcher, analyst, reviewer
    Capabilities []string
    Availability string // available, busy, offline
}

type TeamStrategy struct {
    // 协作模式
    Mode       CollaborationMode // sequential, parallel, hierarchical
    Leader     string              // 领队Agent（hierarchical模式）
    
    // 决策规则
    Consensus  bool                // 是否需要共识
    VotingRule string              // majority, super_majority, unanimous
    
    // 冲突解决
    ConflictResolution ConflictResolutionMode
}

type CollaborationMode string

const (
    ModeSequential    CollaborationMode = "sequential"    // 顺序执行
    ModeParallel      CollaborationMode = "parallel"       // 并行执行
    ModeHierarchical CollaborationMode = "hierarchical"   // 层级协调
    ModeConsensus     CollaborationMode = "consensus"      // 共识决策
)

// 团队执行
func (t *AgentTeam) Execute(ctx context.Context, task *TeamTask) (*TeamResult, error) {
    switch t.Strategy.Mode {
    case ModeSequential:
        return t.executeSequential(ctx, task)
    case ModeParallel:
        return t.executeParallel(ctx, task)
    case ModeHierarchical:
        return t.executeHierarchical(ctx, task)
    case ModeConsensus:
        return t.executeConsensus(ctx, task)
    default:
        return t.executeParallel(ctx, task)
    }
}

// 共识决策模式
func (t *AgentTeam) executeConsensus(ctx context.Context, task *TeamTask) (*TeamResult, error) {
    // 1. 每个Agent独立执行
    individualResults := make(map[string]*AgentExecution)
    
    g, ctx := errgroup.WithContext(ctx)
    for _, member := range t.Members {
        if member.Availability == "available" {
            member := member
            g.Go(func() error {
                result, err := t.executeMember(ctx, member, task)
                if err == nil {
                    individualResults[member.ID] = result
                }
                return nil
            })
        }
    }
    g.Wait()
    
    // 2. 收集所有结果
    var results []*AgentExecution
    for _, result := range individualResults {
        results = append(results, result)
    }
    
    // 3. 如果有分歧，进行讨论
    if t.hasDisagreement(results) {
        // 使用Reviewer Agent进行评估
        consensus, err := t.buildConsensus(ctx, results)
        if err != nil {
            return nil, err
        }
        return consensus, nil
    }
    
    // 4. 返回共识结果
    return &TeamResult{
        TeamID:    t.ID,
        TaskID:    task.ID,
        Result:    results[0], // 假设结果一致
        Agreement: true,
    }, nil
}

func (t *AgentTeam) hasDisagreement(results []*AgentExecution) bool {
    if len(results) <= 1 {
        return false
    }
    
    // 比较结果
    firstAnswer := results[0].Answer
    for _, result := range results[1:] {
        // 计算相似度
        similarity := cosineSimilarity(embed(firstAnswer), embed(result.Answer))
        if similarity < 0.8 {
            return true // 有分歧
        }
    }
    
    return false
}

func (t *AgentTeam) buildConsensus(ctx context.Context, results []*AgentExecution) (*TeamResult, error) {
    // 使用LLM作为仲裁者
    prompt := t.buildConsensusPrompt(results)
    
    response, err := t.llmClient.Generate(ctx, prompt)
    if err != nil {
        return nil, err
    }
    
    // 解析共识结果
    consensus := &AgentExecution{
        Answer: response,
        Reasoning: "基于多Agent讨论的共识结果",
    }
    
    return &TeamResult{
        TeamID:    t.ID,
        Result:    consensus,
        Agreement: false,
        Discussion: true,
    }, nil
}
```

---

## 五、优先级P2：评测能力增强

### 5.1 LLM-as-a-Judge 评测

```python
# link_python/services/evaluation/llm_judge.py

class LLMJudgeEvaluator:
    """使用LLM作为裁判进行评测"""
    
    async def evaluate(
        self,
        question: str,
        generated: str,
        reference: str = None,
        context: List[str] = None
    ) -> JudgeResult:
        """
        执行LLM评测
        
        维度：
        - 准确性：事实是否正确
        - 完整性：是否完整回答问题
        - 相关性：是否针对问题
        - 清晰度：表达是否清晰
        - 有用性：是否有实际帮助
        """
        prompt = self._build_judge_prompt(question, generated, reference, context)
        response = await self.llm.generate(prompt)
        
        return self._parse_judge_result(response)
    
    def _build_judge_prompt(self, question, generated, reference, context) -> str:
        context_str = "\n".join([f"{i+1}. {ctx}" for i, ctx in enumerate(context)])
        
        prompt = f"""
请评估以下AI回答的质量：

【用户问题】
{question}

【参考信息】
{context_str}

【AI回答】
{generated}
"""

        if reference:
            prompt += f"""
【参考答案】
{reference}
"""

        prompt += """
请从以下维度评分（0-10分）：

1. 准确性
   - 回答中的事实是否正确
   - 是否存在幻觉或编造内容
   - 是否与参考信息一致

2. 完整性
   - 是否完整回答了问题的所有方面
   - 是否遗漏了关键信息
   - 是否需要追问才能获得完整答案

3. 相关性
   - 回答是否直接针对问题
   - 是否偏离主题
   - 是否包含无关信息

4. 清晰度
   - 表达是否清晰易懂
   - 逻辑是否连贯
   - 是否存在模糊不清的表述

5. 有用性
   - 回答是否解决了用户的问题
   - 是否提供了实际可用的信息
   - 用户是否能够基于回答采取行动

请以JSON格式返回：
{
    "accuracy": {"score": 8.5, "reasoning": "原因"},
    "completeness": {"score": 7.0, "reasoning": "原因"},
    "relevance": {"score": 9.0, "reasoning": "原因"},
    "clarity": {"score": 8.0, "reasoning": "原因"},
    "usefulness": {"score": 7.5, "reasoning": "原因"},
    "overall_score": 8.0,
    "summary": "总体评价",
    "strengths": ["优点1", "优点2"],
    "weaknesses": ["不足1", "不足2"],
    "suggestions": ["改进建议1", "改进建议2"]
}
"""
        return prompt
```

### 5.2 自动评测任务

```go
// internal/application/evaluation/auto_evaluator.go
package evaluation

// 自动评测器
type AutoEvaluator struct {
    llmJudge      *LLMJudgeEvaluator
    ruleBasedEval *RuleBasedEvaluator
    similarityEval *SimilarityEvaluator
}

type EvaluationTask struct {
    ID          string
    Name        string
    Type        EvalType
    
    // 数据集
    DatasetID   string
    
    // 配置
    Config      *EvalConfig
    
    // 状态
    Status      EvalStatus
    Progress    *EvalProgress
    Result      *EvalResult
}

type EvalType string

const (
    EvalTypeRetrieval  EvalType = "retrieval"  // 检索评测
    EvalTypeGeneration EvalType = "generation" // 生成评测
    EvalTypeEndToEnd  EvalType = "end_to_end" // 端到端评测
    EvalTypeCustom     EvalType = "custom"     // 自定义评测
)

type EvalConfig struct {
    // 检索配置
    RetrievalConfig *RetrievalEvalConfig `json:"retrieval_config,omitempty"`
    
    // 生成配置
    GenerationConfig *GenerationEvalConfig `json:"generation_config,omitempty"`
    
    // LLM Judge配置
    LLMJudgeConfig *LLMJudgeConfig `json:"llm_judge_config,omitempty"`
    
    // 评测指标
    Metrics []MetricConfig `json:"metrics"`
}

// 执行自动评测
func (e *AutoEvaluator) Execute(ctx context.Context, task *EvaluationTask) (*EvalResult, error) {
    // 1. 加载数据集
    dataset, err := e.loadDataset(ctx, task.DatasetID)
    if err != nil {
        return nil, err
    }
    
    // 2. 根据评测类型选择评估器
    switch task.Type {
    case EvalTypeRetrieval:
        return e.evaluateRetrieval(ctx, dataset, task.Config)
    case EvalTypeGeneration:
        return e.evaluateGeneration(ctx, dataset, task.Config)
    case EvalTypeEndToEnd:
        return e.evaluateEndToEnd(ctx, dataset, task.Config)
    case EvalTypeCustom:
        return e.evaluateCustom(ctx, dataset, task.Config)
    default:
        return nil, fmt.Errorf("unknown eval type: %s", task.Type)
    }
}

func (e *AutoEvaluator) evaluateRetrieval(ctx context.Context, dataset *Dataset, config *EvalConfig) (*EvalResult, error) {
    results := &EvalResult{}
    
    for _, qa := range dataset.QAPairs {
        // 执行检索
        retrieved, err := e.retriever.Retrieve(ctx, qa.Question, 10)
        if err != nil {
            return nil, err
        }
        
        // 计算检索指标
        metrics := e.calculateRetrievalMetrics(qa.PIDs, retrieved)
        
        results.Metrics = append(results.Metrics, metrics)
        
        // 更新进度
        task.Progress.Completed++
        task.Progress.Percentage = float64(task.Progress.Completed) / float64(len(dataset.QAPairs)) * 100
    }
    
    // 聚合结果
    results.Aggregated = e.aggregateMetrics(results.Metrics)
    
    return results, nil
}

func (e *AutoEvaluator) evaluateEndToEnd(ctx context.Context, dataset *Dataset, config *EvalConfig) (*EvalResult, error) {
    results := &EvalResult{}
    
    for _, qa := range dataset.QAPairs {
        // 1. 检索
        retrieved, err := e.retriever.Retrieve(ctx, qa.Question, 10)
        if err != nil {
            return nil, err
        }
        
        // 2. 生成
        generated, err := e.generator.Generate(ctx, qa.Question, retrieved)
        if err != nil {
            return nil, err
        }
        
        // 3. LLM Judge评测
        judgeResult, err := e.llmJudge.Evaluate(ctx, &JudgeRequest{
            Question:  qa.Question,
            Generated: generated,
            Reference: qa.Answer,
            Context:   retrieved.Contents(),
        })
        if err != nil {
            return nil, err
        }
        
        results.JudgeResults = append(results.JudgeResults, judgeResult)
        
        // 更新进度
        task.Progress.Completed++
        task.Progress.Percentage = float64(task.Progress.Completed) / float64(len(dataset.QAPairs)) * 100
    }
    
    // 聚合LLM Judge结果
    results.Aggregated = e.aggregateJudgeResults(results.JudgeResults)
    
    return results, nil
}
```

---

## 六、实施路线图

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          AI服务增强实施路线图                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  Q2 2026 (4-6月) ──────────────────────────────────────────────────────────   │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │ P0: 智能检索增强                                                            │   │
│  │  ├─ 多模型Rerank (2周)       ├─ 自适应融合策略 (2周)                         │   │
│  │  ├─ 查询理解增强 (2周)        └─ 多路检索+聚合 (1周)                           │   │
│  │  └─ ─────────────────────────────────────────────────────────────        │   │
│  │                                                                              │   │
│  │  交付: 召回准确率提升15-20%                                                   │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  Q3 2026 (7-9月) ──────────────────────────────────────────────────────────   │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │ P1: Agent能力增强                                                          │   │
│  │  ├─ Agent记忆系统 (3周)       ├─ 自我反思模块 (2周)                           │   │
│  │  ├─ 多Agent协作优化 (2周)     └─ 工具使用学习 (2周)                           │   │
│  │  └─ ─────────────────────────────────────────────────────────────        │   │
│  │                                                                              │   │
│  │  交付: Agent任务成功率提升20%, 复杂任务处理能力显著提升                    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  Q4 2026 (10-12月) ────────────────────────────────────────────────────────   │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │ P2: 评测能力增强                                                            │   │
│  │  ├─ LLM-as-a-Judge (3周)     ├─ 自动评测任务 (2周)                            │   │
│  │  ├─ 评测报告生成 (1周)        └─ A/B测试框架 (3周)                             │   │
│  │  └─ ─────────────────────────────────────────────────────────────        │   │
│  │                                                                              │   │
│  │  交付: 具备完整的评测和优化能力，支持系统持续改进                          │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 附录

### A. 快速验证方案

每个增强点都可以快速验证：

| 增强功能 | 验证方式 | 成功标准 |
|---------|---------|---------|
| 多模型Rerank | A/B测试 | 准确率提升5% |
| 查询理解 | 离线评估 | 召回率提升10% |
| Agent记忆 | 用户反馈 | 满意度提升 |
| LLM评测 | 人工对比 | 相关性>0.8 |

### B. Python工具分工

| 工具 | Python | Go |
|-----|--------|-----|
| Rerank模型调用 | ✅ | - |
| 查询分析 | ✅ | - |
| LLM评测 | ✅ | - |
| 记忆向量化 | ✅ | - |
| 检索调度 | - | ✅ |
| 结果聚合 | - | ✅ |

### C. Python服务gRPC接口设计

```protobuf
// link_python/proto/ai_service.proto
syntax = "proto3";

package ai.v1;

service AIService {
    // === 检索增强相关 ===
    
    // 重排序
    rpc Rerank(RerankRequest) returns (RerankResponse);
    
    // 查询理解
    rpc AnalyzeQuery(QueryAnalysisRequest) returns (QueryAnalysisResponse);
    rpc RewriteQuery(QueryRewriteRequest) returns (QueryRewriteResponse);
    
    // 查询分类（用于自适应融合策略选择）
    rpc ClassifyQuery(QueryClassificationRequest) returns (QueryClassificationResponse);
    
    // === Agent能力增强相关 ===
    
    // 记忆存储
    rpc StoreMemory(StoreMemoryRequest) returns (StoreMemoryResponse);
    rpc RecallMemory(RecallMemoryRequest) returns (RecallMemoryResponse);
    
    // 记忆向量化
    rpc EmbedMemory(EmbedMemoryRequest) returns (EmbedMemoryResponse);
    
    // 自我反思
    rpc ReflectExecution(ReflectionRequest) returns (ReflectionResponse);
    
    // === 评测能力相关 ===
    
    // LLM-as-a-Judge评测
    rpc LLMJudge(LLMJudgeRequest) returns (LLMJudgeResponse);
    
    // 批量评测
    rpc BatchEvaluate(BatchEvaluateRequest) returns (stream BatchEvaluateResponse);
    
    // 相似度计算
    rpc CalculateSimilarity(SimilarityRequest) returns (SimilarityResponse);
}

// === 检索增强消息定义 ===

message RerankRequest {
    string query = 1;
    repeated Document documents = 2;
    int32 top_k = 3;
    string model = 4; // cohere, bge, jina
    map<string, string> options = 5;
}

message RerankResponse {
    repeated Document results = 1;
    repeated float scores = 2;
    string model_used = 3;
    double latency_ms = 4;
}

message Document {
    string id = 1;
    string content = 2;
    map<string, string> metadata = 3;
    float score = 4;
}

message QueryAnalysisRequest {
    string query = 1;
    string session_id = 2;
    map<string, string> context = 3;
}

message QueryAnalysisResponse {
    string query_type = 1; // factual, navigational, transactional, comparison, complex
    string intent = 2; // search, compare, summarize, execute
    repeated string keywords = 3;
    repeated string entities = 4;
    string suggested_strategy = 5; // rrr, weighted, switch
    map<string, float> retrieval_weights = 6;
}

message QueryRewriteRequest {
    string query = 1;
    string query_type = 2;
    string intent = 3;
    repeated string entities = 4;
    int32 num_variations = 5;
}

message QueryRewriteResponse {
    repeated string rewrites = 1;
    string original = 2;
    map<string, float> confidence_scores = 3;
}

// === Agent记忆消息定义 ===

message StoreMemoryRequest {
    string agent_id = 1;
    string session_id = 2;
    Memory memory = 3;
}

message Memory {
    string id = 1;
    string type = 2; // fact, event, concept, procedure, preference
    string content = 3;
    double importance = 4; // 0-1
    bool searchable = 5;
    map<string, string> metadata = 6;
    repeated string related_ids = 7;
    string source = 8; // user, agent, system
    int64 created_at = 9;
}

message StoreMemoryResponse {
    string memory_id = 1;
    bool stored_long_term = 2;
    bool indexed = 3;
}

message RecallMemoryRequest {
    string agent_id = 1;
    string query = 2;
    repeated string memory_types = 3;
    int32 top_k = 4;
    string session_id = 5; // 可选，优先从当前会话回忆
    double min_importance = 6;
}

message RecallMemoryResponse {
    repeated Memory memories = 1;
    repeated float scores = 2;
    string recall_strategy = 3;
}

// === LLM评测消息定义 ===

message LLMJudgeRequest {
    string question = 1;
    string generated = 2;
    string reference = 3; // 可选
    repeated string context = 4;
    repeated string dimensions = 5; // accuracy, completeness, relevance, clarity, usefulness
    string judge_model = 6;
}

message LLMJudgeResponse {
    map<string, DimensionScore> dimension_scores = 1;
    double overall_score = 2;
    string summary = 3;
    repeated string strengths = 4;
    repeated string weaknesses = 5;
    repeated string suggestions = 6;
}

message DimensionScore {
    double score = 1; // 0-10
    string reasoning = 2;
}

// === 批量评测消息定义 ===

message BatchEvaluateRequest {
    repeated EvaluationItem items = 1;
    EvaluationConfig config = 2;
}

message EvaluationItem {
    string question = 1;
    string generated = 2;
    string reference = 3;
    repeated string context = 4;
    string item_id = 5;
}

message EvaluationConfig {
    repeated string dimensions = 1;
    string judge_model = 2;
    int32 concurrency = 3;
}

message BatchEvaluateResponse {
    string item_id = 1;
    LLMJudgeResponse result = 2;
    int32 completed = 3;
    int32 total = 4;
}

// === 相似度计算消息定义 ===

message SimilarityRequest {
    repeated Embedding embeddings = 1;
    string method = 2; // cosine, euclidean, manhattan, dot
}

message Embedding {
    repeated float values = 1;
    int32 dimension = 2;
    string text = 3; // 原文（可选）
}

message SimilarityResponse {
    repeated SimilarityPair similarities = 1;
}

message SimilarityPair {
    int32 index_1 = 1;
    int32 index_2 = 2;
    double score = 3;
}
```

### D. Go服务调用示例

```go
// internal/infrastructure/ai/ai_client.go
package ai

import (
    "context"
    "time"
)

// AIClient Python AI服务客户端
type AIClient struct {
    client ai.v1.AIServiceClient
}

// NewAIClient 创建AI客户端
func NewAIClient(conn *grpc.ClientConn) *AIClient {
    return &AIClient{
        client: ai.v1.NewAIServiceClient(conn),
    }
}

// Rerank 重排序文档
func (c *AIClient) Rerank(ctx context.Context, query string, docs []Document, topK int) ([]Document, error) {
    req := &ai.v1.RerankRequest{
        Query: query,
        Documents: func() []*ai.v1.Document {
            pbDocs := make([]*ai.v1.Document, len(docs))
            for i, doc := range docs {
                pbDocs[i] = &ai.v1.Document{
                    Id:      doc.ID,
                    Content: doc.Content,
                    Metadata: doc.Metadata,
                    Score:   doc.Score,
                }
            }
            return pbDocs
        }(),
        TopK:  int32(topK),
        Model: "cohere", // 可配置
    }
    
    resp, err := c.client.Rerank(ctx, req)
    if err != nil {
        return nil, err
    }
    
    // 转换结果
    result := make([]Document, len(resp.Results))
    for i, doc := range resp.Results {
        result[i] = Document{
            ID:      doc.Id,
            Content: doc.Content,
            Metadata: doc.Metadata,
            Score:   resp.Scores[i],
        }
    }
    
    return result, nil
}

// AnalyzeQuery 分析查询
func (c *AIClient) AnalyzeQuery(ctx context.Context, query string, sessionID string) (*QueryAnalysis, error) {
    req := &ai.v1.QueryAnalysisRequest{
        Query:     query,
        SessionId: sessionID,
    }
    
    resp, err := c.client.AnalyzeQuery(ctx, req)
    if err != nil {
        return nil, err
    }
    
    return &QueryAnalysis{
        Type:          resp.QueryType,
        Intent:        resp.Intent,
        Keywords:      resp.Keywords,
        Entities:      resp.Entities,
        SuggestedStrategy: resp.SuggestedStrategy,
        RetrievalWeights: resp.RetrievalWeights,
    }, nil
}

// RecallMemory 回忆记忆
func (c *AIClient) RecallMemory(ctx context.Context, agentID, query string, memoryTypes []string) ([]Memory, error) {
    req := &ai.v1.RecallMemoryRequest{
        AgentId:     agentID,
        Query:       query,
        MemoryTypes: memoryTypes,
        TopK:        10,
    }
    
    resp, err := c.client.RecallMemory(ctx, req)
    if err != nil {
        return nil, err
    }
    
    memories := make([]Memory, len(resp.Memories))
    for i, m := range resp.Memories {
        memories[i] = Memory{
            ID:         m.Id,
            Type:       m.Type,
            Content:    m.Content,
            Importance: m.Importance,
            Searchable: m.Searchable,
            Metadata:   m.Metadata,
            RelatedIDs: m.RelatedIds,
            Source:     m.Source,
            CreatedAt:  time.Unix(m.CreatedAt, 0),
        }
    }
    
    return memories, nil
}

// LLMJudge LLM评测
func (c *AIClient) LLMJudge(ctx context.Context, question, generated, reference string, contexts []string) (*LLMJudgeResult, error) {
    req := &ai.v1.LLMJudgeRequest{
        Question:  question,
        Generated: generated,
        Reference: reference,
        Context:   contexts,
        Dimensions: []string{"accuracy", "completeness", "relevance", "clarity", "usefulness"},
        JudgeModel: "gpt-4",
    }
    
    resp, err := c.client.LLMJudge(ctx, req)
    if err != nil {
        return nil, err
    }
    
    return &LLMJudgeResult{
        DimensionScores: func() map[string]DimensionScore {
            scores := make(map[string]DimensionScore)
            for dim, score := range resp.DimensionScores {
                scores[dim] = DimensionScore{
                    Score:     score.Score,
                    Reasoning: score.Reasoning,
                }
            }
            return scores
        }(),
        OverallScore: resp.OverallScore,
        Summary:      resp.Summary,
        Strengths:    resp.Strengths,
        Weaknesses:   resp.Weaknesses,
        Suggestions:  resp.Suggestions,
    }, nil
}
```

### E. 增强型检索配置

```yaml
# config/retrieval_enhanced.yaml
retrieval:
  # 重排序配置
  rerank:
    enabled: true
    models:
      cohere:
        api_key: "${COHERE_API_KEY}"
        model: "rerank-v3"
        max_chunks: 500
      bge:
        model_path: "/models/bge-reranker-v2-m3"
        device: "cuda"
        batch_size: 32
    
    # 融合策略
    fusion:
      method: "weighted"  # weighted, rrf, rank
      models: ["cohere", "bge"]
      weights:
        cohere: 0.6
        bge: 0.4
    
    # 自适应选择
    adaptive:
      enabled: true
      rules:
        - condition: "query_length > 50"
          model: "cohere"
        - condition: "contains_code"
          model: "bge"
        - condition: "default"
          model: "cohere"

  # 查询理解配置
  query_understanding:
    enabled: true
    
    # 查询分析
    analysis:
      llm_model: "gpt-4o-mini"
      max_tokens: 300
      cache_ttl: 3600  # 缓存1小时
    
    # 查询重写
    rewrite:
      enabled: true
      num_variations: 3
      min_similarity: 0.7  # 最小相似度，避免重复
    
    # 多路检索
    multi_retrieve:
      enabled: true
      fusion_method: "rrf"
      rrf_k: 60

  # 混合检索配置
  hybrid:
    # 向量检索
    dense:
      enabled: true
      top_k: 30
      score_threshold: 0.6
    
    # 稀疏检索
    sparse:
      enabled: true
      top_k: 30
      bm25_k1: 1.2
      bm25_b: 0.75
    
    # 图谱检索
    graph:
      enabled: true
      top_k: 20
      depth: 2
    
    # 融合策略
    fusion:
      # 默认权重
      default_weights:
        dense: 0.4
        sparse: 0.3
        graph: 0.3
      
      # 按查询类型调整
      type_weights:
        factual:
          dense: 0.2
          sparse: 0.4
          graph: 0.4
        navigational:
          dense: 0.3
          sparse: 0.3
          graph: 0.4
        comparison:
          dense: 0.3
          sparse: 0.2
          graph: 0.5
        complex:
          dense: 0.5
          sparse: 0.2
          graph: 0.3

  # 缓存配置
  cache:
    query_cache:
      enabled: true
      ttl: 3600
      max_size: 10000
    
    document_cache:
      enabled: true
      ttl: 7200
      max_size: 50000
```

### F. 部署架构

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           增强型AI服务部署架构                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                           Go服务层                                       │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │   │
│  │  │ API Gateway │  │ Agent编排   │  │ 检索调度    │  │ 结果聚合     │    │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                      │ gRPC                                      │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                          Python服务层                                    │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │                      AI能力服务                                   │    │   │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │    │   │
│  │  │  │ Rerank服务  │  │ 查询理解    │  │ LLM评测     │             │    │   │
│  │  │  └─────────────┘  └─────────────┘  └─────────────┘             │    │   │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │    │   │
│  │  │  │ 记忆管理    │  │ 向量化服务  │  │ 相似度计算  │             │    │   │
│  │  │  └─────────────┘  └─────────────┘  └─────────────┘             │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  │                                                                             │   │
│  │  ┌─────────────────────────────────────────────────────────────────┐    │   │
│  │  │                      模型加载层                                   │    │   │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │    │   │
│  │  │  │ BGE Rerank  │  │ Embeddings  │  │ 轻量LLM     │             │    │   │
│  │  │  │ (本地模型)  │  │ (本地模型)  │  │ (本地/远程) │             │    │   │
│  │  │  └─────────────┘  └─────────────┘  └─────────────┘             │    │   │
│  │  └─────────────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │                          外部服务                                        │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │   │
│  │  │ Cohere API  │  │ OpenAI API  │  │ Milvus      │  │ Neo4j       │    │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### G. 监控指标

```yaml
# 关键监控指标
monitoring:
  retrieval:
    - name: "rerank_latency"
      description: "重排序延迟"
      threshold: 500  # ms
    - name: "fusion_strategy_distribution"
      description: "融合策略使用分布"
      labels: ["rrf", "weighted", "switch"]
    - name: "query_analysis_accuracy"
      description: "查询分析准确率"
      threshold: 0.85
    - name: "recall_rate"
      description: "召回率"
      threshold: 0.8
  
  agent:
    - name: "memory_recall_hit_rate"
      description: "记忆召回命中率"
      threshold: 0.7
    - name: "reflection_execution_ratio"
      description: "反思执行比例"
    - name: "multi_agent_success_rate"
      description: "多Agent成功率"
      threshold: 0.85
  
  evaluation:
    - name: "llm_judge_latency"
      description: "LLM评测延迟"
      threshold: 3000  # ms
    - name: "evaluation_task_duration"
      description: "评测任务持续时间"
    - name: "judge_score_distribution"
      description: "评测分数分布"
      buckets: [0, 4, 6, 8, 10]
```

---

**文档版本**: v1.1
**更新时间**: 2026-05-03
