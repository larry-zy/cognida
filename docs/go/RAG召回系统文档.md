# RAG 召回系统文档

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              RAG Pipeline                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐               │
│  │ Query        │────▶│ Retrieval    │────▶│ Result       │               │
│  │ Strengthening│     │ Stage        │     │ Merging      │               │
│  │ (Optional)   │     │              │     │              │               │
│  └──────────────┘     └──────┬───────┘     └──────────────┘               │
│                              │                                              │
│               ┌────────────────┼────────────────┐                          │
│               ▼                ▼                ▼                          │
│      ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │
│      │   Vector    │  │    BM25     │  │   Graph     │                    │
│      │  Retrieve   │  │  Retrieve   │  │  Retrieve   │                    │
│      └─────────────┘  └─────────────┘  └─────────────┘                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 2. 核心组件

### 2.1 Pipeline（管道）

**文件位置**: `internal/application/rag/pipeline.go`

**职责**: RAG 流程的总控，协调检索、重排等阶段

**核心结构**:

```go
type Pipeline struct {
    retriever       *Retriever                    // 检索器
    embedder        embedding.Embedder            // 向量化模型
    milvusRetriever interface{}                   // Milvus 向量检索器
}

type PipelineConfig struct {
    // 检索模式（可多选，向量检索必选）
    RetrievalModes      []string  // vector(必选), bm25, graph

    // 检索参数
    VectorTopK          int       // 向量检索返回数量
    KeywordTopK         int       // 关键词检索返回数量
    GraphTopK           int       // 图谱检索返回数量
    SimilarityThreshold float64   // 相似度阈值
    Alpha               float32   // 向量检索权重（混合检索用）
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Execute()` | 执行完整 RAG 流程 |
| `SimpleExecute()` | 使用默认配置执行 |
| `ExecuteWithMode()` | 指定检索模式执行 |

### 2.2 Retriever（检索器）

**文件位置**: `internal/application/rag/retriever.go`

**职责**: 执行多种检索模式，获取相关文档

**支持的检索模式**:

| 模式 | 说明 | 实现方法 |
|------|------|----------|
| `vector` | 向量语义检索 | `vectorRetrieveWithEmbedding()` |
| `bm25` / `keyword` | 关键词BM25检索 | `bm25Retrieve()` |
| `hybrid` | 向量+关键词混合 | `hybridRetrieve()` |
| `graph` | 知识图谱检索 | `graphRetrieve()` |

**检索流程**:

```
                ┌───────────────────┐
                │   Retrieve()      │
                │  统一检索入口     │
                └─────────┬─────────┘
                          │
                          ▼
           ┌──────────────────────────────┐
           │ 获取知识库设置                │
           │ - retrieval_mode             │
           │ - top_k                      │
           │ - similarity_threshold       │
           └──────────────┬───────────────┘
                          │
           ┌──────────────┴───────────────┐
           ▼                              ▼
    ┌──────────────┐              ┌──────────────┐
    │ Milvus可用?  │──No──▶       │ 应用层检索    │
    └──────┬───────┘              │ (内存向量)   │
           │Yes                     └──────────────┘
           ▼
    ┌──────────────┐
    │ Milvus检索   │
    └──────────────┘
```

**向量检索实现**:

1. **Milvus 方式**（优先）:
   - 检查 collection 是否存在
   - 调用 `milvusRetriever.Search()`
   - 转换结果格式

2. **应用层方式**（回退）:
   - 获取启用的分块（候选数 = TopK * 5）
   - 将查询文本向量化
   - 批量将分块内容向量化
   - 计算余弦相似度
   - 过滤低于阈值的结果
   - 排序并返回 TopK

**BM25 检索实现**:

```
1. 获取分块（候选数 = TopK * 10）
2. 对查询进行分词（支持中文）
3. 计算文档统计：
   - totalDocs: 总文档数
   - avgDocLen: 平均文档长度
   - docFreq: 包含每个词的文档数
4. 计算每个分块的 BM25 分数：
   score = Σ IDF(qi) × TF(qi, doc)

   其中：
   IDF = log((N - df(qi) + 1) / (df(qi) + 1) + 1)
   TF = (freq × (k1 + 1)) / (freq + k1 × (1 - b + b × docLen / avgDocLen))

   默认参数：k1 = 1.5, b = 0.75
5. 排序并返回 TopK
```

**混合检索（RRF 融合）**:

```
RRF (Reciprocal Rank Fusion) 算法：

score(chunk) = α / (K + rank_vector) + (1-α) / (K + rank_keyword)

默认参数：K = 60, α = 0.5
```

**知识图谱检索流程**:

```
1. 从查询中提取实体（分词 + 去停用词）
2. 调用 Neo4j SearchNodeV2 查询关联实体
3. 根据关系 strength 排序，收集 chunk_ids
4. 按 strength 取 TopK 个 chunk
5. 从数据库获取具体内容
6. 返回结果（分数 = strength）
```

**核心数据结构**:

```go
// RetrieveOptions 检索选项
type RetrieveOptions struct {
    TopK                int     // 返回结果数量
    SimilarityThreshold float64 // 相似度阈值
    RerankEnabled       bool    // 是否重排序（暂未实现）
    GraphEnabled        bool    // 是否使用知识图谱（暂未实现）
    Alpha               float32 // 混合检索中向量检索的权重（默认0.5）
}

// RetrieveResult 检索结果
type RetrieveResult struct {
    ChunkID       string
    KnowledgeID   string
    KBID          string
    Content       string
    ChunkIndex    int
    Score         float32
    MatchType     string // "vector", "keyword", "hybrid"
    StartPosition int
    EndPosition   int
}
```

### 2.3 Reranker（重排器）

**文件位置**: `internal/application/rag/rerank.go`

**职责**: 对多路检索结果进行融合重排

**支持的重排策略**:

| 策略 | 说明 | 公式 |
|------|------|------|
| `rrf` | 倒数排名融合 | `weight / (K + rank)` |
| `weighted` | 加权分数融合 | `Σ(score × weight)` |
| `weighted_rrf` | 加权 RRF | `Σ(weight_i / (K + rank_i))` |
| `model` | 模型重排 | 使用专门的重排模型 |
| `hybrid` | 混合重排 | 先 RRF 融合，再模型重排 |

**重排器结构**:

```go
type Reranker struct {
    strategies map[string]RerankStrategy  // 注册的策略
    embedder   RerankEmbedder             // 可选的重排模型
}

type RerankOptions struct {
    TopK             int     // 返回结果数量
    Strategy         string  // 重排策略：rrf, weighted, model, weighted_rrf
    Alpha            float32 // 向量检索权重（用于加权融合）
    Beta             float32 // 关键词检索权重（用于加权融合）
    Gamma            float32 // 图谱检索权重（用于加权融合）
    RerankModel      string  // 重排模型名称（如果使用模型重排）
    RerankTopK       int     // 重排前保留的候选数量
}
```

**RRF 策略实现**:

```go
type RRFStrategy struct {
    K float64 // RRF 常数，默认 60
}

// RRF 公式: weight / (k + rank)
// 对每个结果列表计算 RRF 分数，然后合并
```

### 2.4 QueryStrengthener（查询增强器）

**文件位置**: `internal/application/rag/query_strength.go`

**职责**: 在检索前对查询进行优化

**支持功能**:

1. **查询重写**:
   - 解决指代消解（"他"、"它"等）
   - 补充省略信息
   - 使查询更清晰完整

2. **查询拆分**:
   - 识别复杂查询中的多个问题
   - 拆分为独立的子查询
   - 保留依赖关系

**重写判断条件**:

- 包含代词（他/她/它/这个/那个等）
- 包含省略号
- 问号开头
- 简短疑问句（<15字 且包含"怎么"/"如何"）

**拆分判断条件**:

- 包含多个疑问词
- 包含"和"、"与"、"以及"
- 包含"区别"、"对比"、"比较"
- 包含"列举"、"所有"

**数据结构**:

```go
type StrengthenedQuery struct {
    OriginalQuery  string   // 原始查询
    RewrittenQuery string   // 重写后的查询
    SubQueries     []string // 拆分的子查询
    RewriteApplied bool     // 是否应用了重写
    SplitApplied   bool     // 是否应用了拆分
    ProcessingTime int64    // 处理耗时（毫秒）
}

type StrengthOptions struct {
    EnableRewrite bool    // 是否启用查询重写
    EnableSplit   bool    // 是否启用查询拆分
    Temperature   float64 // LLM 温度参数
    MaxTokens     int     // 最大 token 数
}
```

### 2.5 RAGChatService（聊天服务集成）

**文件位置**: `internal/application/rag/rag_chat.go`

**职责**: 将 RAG 能力集成到聊天服务中

---

## 3. RAG 检索优化

### 3.1 HyDE (Hypothetical Document Embeddings)

**文件位置**: `internal/infrastructure/rag/hyde_generator.go`

**职责**: 生成假设性文档，用于提高向量检索效果

**原理**: 将用户查询转换为可能的理想答案，然后对答案进行向量化检索，而非直接检索查询本身。

**核心接口**:
```go
type HyDEGenerator interface {
    GenerateHypotheticalDoc(ctx context.Context, query string, opts *HyDEOptions) (string, error)
    GenerateMultiple(ctx context.Context, query string, count int, opts *HyDEOptions) ([]string, error)
}
```

**配置选项**:
```go
type HyDEOptions struct {
    Temperature            float64   // 生成温度 (0.1-1.0)
    MaxTokens              int       // 最大生成 token 数
    Domain                 string    // 领域上下文
    UseConversationHistory bool      // 是否使用对话历史
    ConversationHistory    []string  // 对话历史
}
```

**HTTP 接口**:
- `POST /api/v1/rag/hyde/generate` - 生成假设文档
- `POST /api/v1/rag/hyde/generate-multiple` - 生成多个假设文档

### 3.2 查询重写 (Query Rewriting)

**文件位置**: `internal/infrastructure/rag/hyde_generator.go` (QueryRewriterImpl)

**职责**: 优化用户查询，使其更适合信息检索

**功能**:
1. **查询重写**: 补充省略信息、解决指代消解
2. **查询扩展**: 生成多个查询变体，提高召回率
3. **查询分解**: 将复杂查询拆分为可独立检索的子查询

**核心接口**:
```go
type QueryRewriter interface {
    RewriteQuery(ctx context.Context, query string, opts *RewriteOptions) (*RewrittenQuery, error)
    ExpandQuery(ctx context.Context, query string, count int, opts *RewriteOptions) ([]string, error)
    DecomposeQuery(ctx context.Context, query string, opts *RewriteOptions) ([]*SubQuery, error)
}
```

**HTTP 接口**:
- `POST /api/v1/rag/query/rewrite` - 重写查询
- `POST /api/v1/rag/query/expand` - 扩展查询
- `POST /api/v1/rag/query/decompose` - 分解查询

### 3.3 多跳检索 (Multi-Hop Retrieval)

**文件位置**: `internal/infrastructure/rag/multi_hop.go`

**职责**: 处理需要多次检索才能回答的复杂问题

**原理**:
1. 第一跳：基于原始查询检索
2. 生成中间结论/下一步查询
3. 第二跳：基于中间结论继续检索
4. 重复直到达到最大跳数或获得足够信息

**核心接口**:
```go
type MultiHopRetriever interface {
    MultiHopRetrieve(ctx context.Context, query string, opts *MultiHopOptions) (*MultiHopResult, error)
}
```

**配置选项**:
```go
type MultiHopOptions struct {
    MaxHops        int     // 最大跳数 (默认 3)
    DocsPerHop     int     // 每跳检索文档数 (默认 5)
    Temperature    float64 // 生成温度
    MaxTokens      int     // 最大 token 数
    MergeStrategy  string  // 结果合并策略 (all/latest/rerank)
}
```

**HTTP 接口**:
- `POST /api/v1/rag/multi-hop/retrieve` - 执行多跳检索

### 3.4 综合优化服务

**HTTP 接口**:
- `POST /api/v1/rag/optimize/query` - 综合查询优化（可选启用 HyDE、重写、扩展）
- `GET /api/v1/rag/optimize/config` - 获取当前优化配置
- `PUT /api/v1/rag/optimize/config` - 更新优化配置

---

## 4. Agent 集成

### 4.1 工具方式集成

RAG 功能通过 **工具调用** 方式集成到 Agent 中。

**工具列表**:

| 工具 | 功能 | 文件 |
|------|------|------|
| `rag_query` | 文档检索 + 优化 | `tools/rag_query.go` |
| `graph_query` | 知识图谱检索 | `tools/graph_query.go` |
| `web_search` | 网络搜索 | `tools/web_search.go` |
| `kb_select` | 知识库选择 | `tools/knowledge_base_tool.go` |

### 4.2 RAG 工具配置

**请求结构** (`RAGQueryRequest`):
```go
type RAGQueryRequest struct {
    // 基础参数
    Query         string  // 查询内容（必需）
    KBID          string  // 知识库ID
    TopK          int     // 返回数量（默认5）
    RetrievalMode string  // vector/bm25/hybrid
    MinScore      float64 // 相似度阈值（默认0.7）
    EnableRerank  bool    // 是否重排序

    // 优化参数
    EnableHyDE          bool     // 启用 HyDE
    HyDECount           int      // HyDE 文档数量
    EnableQueryRewrite  bool     // 启用查询重写
    EnableQueryExpansion bool     // 启用查询扩展
    ExpansionCount      int      // 扩展查询数量
    EnableMultiHop      bool     // 启用多跳检索
    MaxHops             int      // 最大跳数
    Domain              string   // 知识域
    ConversationHistory []string // 对话历史
}
```

### 4.3 创建带 RAG 的 Agent

```go
import "link/internal/application/usecases/agent"

// 初始化工具注册中心
registry := agent.GetDefaultRegistry()
registry.RegisterDefaultTools() // 注册 rag_query, graph_query 等

// 方式一：指定工具
myAgent := agent.New(toolModel).
    Name("知识库助手").
    Prompt(`你是一个专业的文档助手。
              使用 rag_query 工具查询文档内容。
              对于复杂查询，可以启用 enable_hyde 优化。
              对于有上下文的问题，启用 enable_query_rewrite。`).
    ToolsFromRegistry("rag_query", "graph_query").
    Build()

// 方式二：自动选择（LLM 决定）
advancedAgent := agent.New(toolModel).
    Name("智能助手").
    WithRegistry(registry).
    ToolsAutoSelect(). // LLM 自动选择合适工具
    Build()
```

### 4.4 LLM 调用示例

当 LLM 需要查询知识库时，会自动调用工具：

```json
{
  "name": "rag_query",
  "arguments": {
    "query": "如何配置反向代理？",
    "enable_hyde": true,
    "enable_query_rewrite": true,
    "retrieval_mode": "hybrid",
    "top_k": 5
  }
}
```

### 4.5 优化策略选择

| 查询类型 | 推荐配置 | 理由 |
|----------|----------|------|
| 简单事实 | 默认配置 | 快速返回结果 |
| 概念解释 | `enable_hyde=true` | 提高语义匹配 |
| 上下文问题 | `enable_query_rewrite=true` + `conversation_history` | 解决指代消解 |
| 全面检索 | `enable_query_expansion=true` | 提高召回率 |
| 推理问题 | `enable_multi_hop=true` | 多步推理 |

---

## 5. 安全护栏 (Guardrail)

### 4.1 功能概述

**文件位置**:
- Domain: `internal/domain/guardrail/`
- Application: `internal/application/guardrail/`
- Infrastructure: `internal/infrastructure/guardrail/`
- Handler: `internal/interface/http/handler/guardrail_handler.go`

**职责**: 检测和过滤输入/输出中的不安全内容

### 4.2 输入检查

**功能**:
- 敏感词检测
- PII（个人隐私信息）检测
- SQL 注入检测
- XSS 攻击检测

**HTTP 接口**:
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/guardrail/input/check` | POST | 检查输入是否安全 |
| `/api/v1/guardrail/input/sanitize` | POST | 清理输入内容 |

### 4.3 输出检查

**功能**:
- PII 检测和脱敏
- 脏话检测
- 仇恨言论检测
- 暴力内容检测

**HTTP 接口**:
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/guardrail/output/check` | POST | 检查输出是否安全 |
| `/api/v1/guardrail/output/sanitize` | POST | 清理输出内容 |

### 4.4 越狱检测

**功能**:
- 识别常见的越狱攻击模式
- LLM 辅助的智能检测

**HTTP 接口**:
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/guardrail/jailbreak/check` | POST | 检测越狱攻击 |
| `/api/v1/guardrail/is-jailbreak` | POST | 快速越狱检查 |

### 4.5 综合检查

**HTTP 接口**:
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/guardrail/full-check` | POST | 完整输入检查（含越狱） |
| `/api/v1/guardrail/check-both` | POST | 同时检查输入和输出 |
| `/api/v1/guardrail/quick-check` | POST | 快速检查 |
| `/api/v1/guardrail/quick-sanitize` | POST | 快速清理 |

### 4.6 配置和建议

**HTTP 接口**:
| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/v1/guardrail/config/default` | GET | 获取默认配置 |
| `/api/v1/guardrail/recommendation` | POST | 获取处理建议 |

---

## 6. 数据流

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Chat()` | 支持同步聊天 |
| `ChatStream()` | 支持流式聊天 |
| `SetPipeline()` | 设置 Pipeline |
| `EnableRAG()` | 启用/禁用 RAG |

**RAG 执行流程**:

```
1. 检查 RAG 是否启用
2. 获取 RAGConfig（从请求或会话）
3. 执行 Pipeline.Execute() 检索
4. 构建增强后的消息内容（检索结果 + 原问题）
5. 调用基础聊天服务
6. 返回结果（附加 RAG 上下文）
```

## 7. 数据流

### 7.1 请求响应数据结构

```go
// 请求
type ChatRequest struct {
    Content    string
    History    []Message
    RAGConfig  *RAGConfig
}

type RAGConfig struct {
    Enabled             bool
    KBID                string
    RetrievalModes      []string
    VectorTopK          int
    KeywordTopK         int
    GraphTopK           int
    SimilarityThreshold float64
    Alpha               float32
}

// 响应
type PipelineResult struct {
    Query          string
    FinalQuery     string
    RetrievedDocs  []*RetrieveResult
    GraphRelations []*GraphRelationRes
    SourceTypes    []string
    Success        bool
    Stages         map[string]*StageResult
}

type StageResult struct {
    Name        string      // 阶段名称
    Input       interface{} // 输入
    Output      interface{} // 输出
    Success     bool        // 是否成功
    Error       string      // 错误信息
    InputCount  int         // 输入数量
    OutputCount int         // 输出数量
}
```

### 7.2 结果合并逻辑

```go
func mergeAllResults(resultLists [][]*RetrieveResult) []*RetrieveResult {
    // 1. 按 chunk_id 去重，保留分数最高的
    // 2. 按分数降序排序
    // 3. 返回合并后的结果
}
```

## 8. 配置说明

### 8.1 默认配置

```go
func DefaultPipelineConfig() *PipelineConfig {
    return &PipelineConfig{
        RetrievalModes:      []string{"vector"},  // 默认仅向量检索
        VectorTopK:          15,
        KeywordTopK:         15,
        GraphTopK:           10,
        SimilarityThreshold: 0.0,
        Alpha:               0.6,
    }
}

func DefaultRerankOptions() *RerankOptions {
    return &RerankOptions{
        TopK:       5,
        Strategy:   "rrf",
        Alpha:      0.5,
        Beta:       0.3,
        Gamma:      0.2,
        RerankTopK: 20,
    }
}

func DefaultStrengthOptions() *StrengthOptions {
    return &StrengthOptions{
        EnableRewrite: true,
        EnableSplit:   true,
        Temperature:   0.1,  // 低温度保证稳定性
        MaxTokens:     2000,
    }
}
```

### 8.2 检索模式对应关系

| retrieval_mode | 使用的检索方法 |
|----------------|----------------|
| `vector` | 仅向量检索 |
| `bm25` / `keyword` | 仅BM25检索 |
| `hybrid` | 向量 + BM25 混合 |
| `graph` | 图谱检索 |

## 9. 文件结构

```
internal/application/rag/
├── pipeline.go       # RAG 管道，总控
├── retriever.go      # 检索器，执行各种检索
├── rerank.go         # 重排器，多路结果融合
├── query_strength.go # 查询增强器
└── rag_chat.go       # 聊天服务集成
```

## 10. 使用示例

### 6.1 创建 Pipeline

```go
import "link/internal/application/rag"

pipeline, err := rag.NewPipeline(
    chatConfig,         // 聊天配置
    kbSettingRepo,      // 知识库设置仓储
    chunkRepo,          // 分块仓储
    embedder,           // 向量化模型
    milvusRetriever,    // Milvus 检索器（可选）
    neo4jRepo,          // Neo4j 仓储（可选）
    graphQueryRepo,     // 图谱查询仓储（可选）
)
```

### 6.2 执行 RAG

```go
// 使用默认配置
result, err := pipeline.SimpleExecute(
    ctx,
    tenantID,
    kbID,
    query,
)

// 使用自定义配置
result, err := pipeline.Execute(
    ctx,
    tenantID,
    kbID,
    query,
    conversationHistory,
    &rag.PipelineConfig{
        RetrievalModes: []string{"vector", "bm25", "graph"},
        VectorTopK:     15,
        KeywordTopK:    15,
        GraphTopK:      10,
        SimilarityThreshold: 0.7,
        Alpha:          0.6,
    },
)

// 使用指定模式
result, err := pipeline.ExecuteWithMode(
    ctx,
    tenantID,
    kbID,
    query,
    "hybrid",  // retrievalMode: vector, bm25, graph, hybrid
    10,        // topK
)
```

### 6.3 获取检索结果

```go
// 获取纯文本内容列表
contexts := result.GetContexts()

// 获取带分数的内容列表
contextsWithScore := result.GetContextsWithScore()
// 返回: []map[string]interface{}{
//     {"content": "...", "score": 0.95, "chunk_id": "...", "source": "vector"},
//     ...
// }

// 检查是否成功
if result.Success {
    log.Printf("检索到 %d 个文档", len(result.RetrievedDocs))
}

// 获取各阶段详情
for stageName, stageResult := range result.Stages {
    log.Printf("%s: 输入=%d, 输出=%d, 成功=%v",
        stageName,
        stageResult.InputCount,
        stageResult.OutputCount,
        stageResult.Success,
    )
}
```

### 6.4 创建 RAG 聊天服务

```go
// 创建带 RAG 的聊天服务
ragChatService, err := rag.NewRAGChatService(
    chatConfig,
    kbSettingRepo,
    chunkRepo,
    embedder,
    milvusRetriever,
    neo4jRepo,
    graphQueryRepo,
    retrievalSettingRepo,
    chatService,
)

// 发起聊天请求
req := &types.ChatRequest{
    Content: "用户问题",
    History: []types.Message{},
    RAGConfig: &types.RAGConfig{
        Enabled:      true,
        KBID:         "kb-xxx",
        RetrievalModes: []string{"vector", "graph"},
        VectorTopK:   10,
    },
}

resp, err := ragChatService.Chat(ctx, req)

// 访问 RAG 上下文
if resp.RAGContext != nil {
    log.Printf("检索到 %d 个文档", resp.RAGContext.RetrievedCount)
    log.Printf("来源类型: %v", resp.RAGContext.SourceTypes)
}
```

### 6.5 查询增强使用

```go
import "link/internal/application/rag"

// 创建查询增强器
strengthener, err := rag.NewQueryStrengthener(chatConfig)

// 增强查询
strengthened, err := strengthener.StrengthenQuery(
    ctx,
    "他怎么使用这个功能？",
    "用户之前问过关于登录的问题",  // 对话历史
    &rag.StrengthOptions{
        EnableRewrite: true,
        EnableSplit:   true,
        Temperature:   0.1,
    },
)

// 获取增强后的查询列表
queries := strengthened.GetQueriesForRetrieve()
// 可能返回:
// [
//   "用户如何使用登录功能？",  // 重写后的查询
//   "他怎么使用这个功能？",    // 原查询
// ]

// 查看增强摘要
summary := strengthened.GetSummary()
// {
//     "original_query": "他怎么使用这个功能？",
//     "rewritten_query": "用户如何使用登录功能？",
//     "rewrite_applied": true,
//     "split_applied": false,
//     "processing_time_ms": 234
// }
```

## 11. 算法详解

### 11.1 余弦相似度计算

```go
func calculateCosineSimilarity(vec1, vec2 []float32) float32 {
    // dotProduct = Σ(vec1[i] × vec2[i])
    // norm1 = √Σ(vec1[i]²)
    // norm2 = √Σ(vec2[i]²)
    // cosine = dotProduct / (norm1 × norm2)

    return dotProduct / (sqrt(norm1) × sqrt(norm2))
}
```

### 11.2 BM25 算法

BM25 是一种用于信息检索的排序函数，用于评估文档与搜索查询的相关性。

**公式**:

```
score(D, Q) = Σ IDF(qi) × (f(qi, D) × (k1 + 1)) / (f(qi, D) + k1 × (1 - b + b × |D| / avgdl))

其中:
- qi: 查询中的词项
- D: 文档
- f(qi, D): 词项 qi 在文档 D 中的词频
- |D|: 文档长度（词数）
- avgdl: 平均文档长度
- k1: 调节词频饱和度的参数 (默认 1.5)
- b: 调节文档长度归一化程度的参数 (默认 0.75)
- IDF(qi): 逆文档频率
```

**IDF 计算**:

```
IDF(qi) = log((N - df(qi) + 1) / (df(qi) + 1) + 1)

其中:
- N: 文档总数
- df(qi): 包含词项 qi 的文档数
```

### 11.3 RRF (Reciprocal Rank Fusion)

RRF 是一种用于合并多个排序列表的算法。

**公式**:

```
score(d) = Σ (weight_i / (k + rank_i(d)))

其中:
- d: 文档
- weight_i: 第 i 个列表的权重
- rank_i(d): 文档 d 在第 i 个列表中的排名
- k: 常数，用于平滑排名差异 (默认 60)
```

**优势**:
- 不需要归一化分数
- 对不同检索器的分数差异不敏感
- 计算简单高效

## 12. 注意事项

1. **向量检索是必选的**: `RetrievalModes` 必须包含 `vector`，系统会自动添加

2. **候选数量设置**: 为提高召回质量，检索时获取的候选数量通常是 TopK 的 3-10 倍

3. **Milvus 回退**: 当 Milvus 不可用时，系统会自动回退到应用层向量检索

4. **图谱检索依赖**: 知识图谱检索需要 `neo4jRepo` 和 `graphQueryRepo` 都已配置

5. **分词支持**: 系统内置了中文分词支持，单独处理中文字符

6. **停用词过滤**: BM25 检索和图谱实体提取时会过滤常见停用词

## 13. 性能优化建议

1. **批量向量化**: 使用 `embedTexts()` 批量处理而非逐个处理

2. **缓存向量**: 考虑缓存文档向量避免重复计算

3. **限制候选数**: 根据实际效果调整候选倍数

4. **并行检索**: 多路检索可以并行执行（当前实现为串行）

5. **阈值过滤**: 合理设置 `SimilarityThreshold` 减少低质量结果

---

**文档版本**: 1.1
**最后更新**: 2026-05-05
**维护者**: Link Team
