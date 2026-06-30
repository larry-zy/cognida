# Link 知识库数据处理流程文档

## 文档概述

本文档详细说明 Link 系统中知识库的数据处理流程，包括数据从上传到可检索的完整转换过程，重点介绍归一化处理、向量化等核心环节。

---

## 一、整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         知识库数据处理管道 (Data Pipeline)                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐                                                            │
│  │ 原始文档     │  PDF / DOCX / TXT / MD / HTML / URL                        │
│  └──────┬──────┘                                                            │
│         ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  1. 文档解析阶段 (Python gRPC - docreader)                          │   │
│  │     • 文本提取 (支持 PDF/DOCX/HTML)                                   │   │
│  │     • 编码转换 (UTF-8)                                               │   │
│  │     • 元数据提取 (页数、字符数、格式等)                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  2. 文档分块阶段 (Python gRPC - docreader)                          │   │
│  │     ┌───────────────────────────────────────────────────────────┐   │   │
│  │     │ 分块策略                                                  │   │   │
│  │     │ • Recursive: 递归分割 (默认)                               │   │   │
│  │     │ • Sentence: 句子级分割                                     │   │   │
│  │     │ • FixedSize: 固定大小分割                                  │   │   │
│  │     │ • Semantic: 语义分割 (基于向量相似度)                       │   │   │
│  │     │ • Paragraph: 段落级分割                                     │   │   │
│  │     └───────────────────────────────────────────────────────────┘   │   │
│  │     参数: chunk_size (默认1000), overlap (默认200)                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  3. 结构化存储 (MySQL)                                               │   │
│  │     • knowledge 表: 文档元数据                                       │   │
│  │     • chunks 表: 分块内容与索引                                      │   │
│  │     • 建立 pre_chunk_id 关联                                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  4. 并行数据准备阶段 (errgroup 并发控制)                             │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                   │   │
│  │  │ 稠密向量    │ │ 稀疏向量    │ │ 图谱提取    │                   │   │
│  │  │ Embedding   │ │ BM25        │ │ LLM 提取    │                   │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  5. 数据写入阶段                                                      │   │
│  │     • Milvus: 向量索引 (dense_vector + sparse_vector)                 │   │
│  │     • Neo4j: 图谱存储 (实体 + 关系)                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                    │
│  数据准备完成，可供检索                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、文档解析阶段

### 2.1 调用方式

通过 **Python gRPC 服务** (`docreader`) 进行文档解析：

```go
// 文件位置: link-go/internal/application/usecases/knowledge/document_processor.go

parseReq := &docreader.ParseDocumentRequest{
    Options: &docreader.ParseOptions{
        IncludeMetadata: true,
        ExtractTables:   false,
        ExtractImages:   false,
    },
}
```

### 2.2 数据源支持

| 数据源 | 参数 | 说明 |
|--------|------|------|
| 本地文件 | `FilePath` | 上传的文件路径 |
| URL | `URL` | 网络链接内容抓取 |
| 纯文本 | `Content` | 直接提供的文本内容 |

### 2.3 返回结果

```go
type ParseDocumentResponse struct {
    Success  bool              // 是否成功
    Text     string            // 提取的文本内容
    Error    string            // 错误信息
    Metadata *DocumentMetadata // 文档元数据
}

type DocumentMetadata struct {
    Format     string            // 文档格式 (pdf/docx/txt等)
    PageCount  int32             // 页数
    CharCount  int32             // 字符数
    Properties map[string]string // 其他属性
}
```

---

## 三、文档分块阶段

### 3.1 分块策略

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          文档分块策略对比                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐  │
│  │   Recursive     │     │   Sentence      │     │   Semantic      │  │
│  │   递归分割       │     │   句子级分割     │     │   语义分割       │  │
│  ├─────────────────┤     ├─────────────────┤     ├─────────────────┤  │
│  │ • 默认策略      │     │ • 按句子分割    │     │ • 基于相似度    │  │
│  │ • 多级分隔符    │     │ • 保持句子完整  │     │ • 语义边界识别  │  │
│  │ • 灵活适配      │     │ • 适合对话      │     │ • 需要向量化    │  │
│  └─────────────────┘     └─────────────────┘     └─────────────────┘  │
│                                                                         │
│  ┌─────────────────┐     ┌─────────────────┐                            │
│  │   FixedSize     │     │   Paragraph     │                            │
│  │   固定大小       │     │   段落级分割     │                            │
│  ├─────────────────┤     ├─────────────────┤                            │
│  │ • 固定字符数    │     │ • 按段落分割    │                            │
│  │ • 重叠处理      │     │ • 保持逻辑完整  │                            │
│  │ • 简单可控      │     │ • 适合长文档    │                            │
│  └─────────────────┘     └─────────────────┘                            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.2 分块参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `ChunkSize` | 1000 | 每块字符数 |
| `ChunkOverlap` | 200 | 块间重叠字符数 |
| `Separator` | `\n\n` | 分隔符优先级 |
| `MinChunkSize` | 100 | 最小块大小 |

### 3.3 分块结构

```go
type Chunk struct {
    ID          string    // 唯一标识
    TenantID    int64     // 租户 ID (多租户隔离)
    KBID        string    // 知识库 ID
    KnowledgeID string    // 所属文档 ID
    ChunkIndex  int       // 分块索引
    Content     string    // 分块内容
    IsEnabled   bool      // 是否启用
    StartAt     int       // 起始位置
    EndAt       int       // 结束位置
    PreChunkID  *string   // 前一个分块 ID (建立关联)
    ChunkType   string    // 分块类型 (text/image/table)
    TokenCount  *int      // Token 数量
    CreatedAt   time.Time // 创建时间
}
```

---

## 四、数据归一化处理

### 4.1 文本归一化

在相似度计算服务中，对文本进行 **Unicode 归一化**：

```go
// 文件位置: link-go/internal/domain/services/similarity.go

// tokenize 对文本进行分词和归一化
func tokenize(text string) []string {
    // 1. Unicode 归一化: NFD (Canonical Decomposition)
    t := transform.Chain(norm.NFD, runes.Remove(runes.Predicate(func(r rune) bool {
        return unicode.Is(unicode.Mn, r) // 去除变音符号
    })))
    normalized, _, _ := transform.String(t, text)

    // 2. 小写转换
    // 3. 分词处理
    return strings.FieldsFunc(normalized, func(r rune) bool {
        return !unicode.IsLetter(r) && !unicode.IsDigit(r)
    })
}
```

**归一化步骤**:
1. **Unicode NFD**: 分解组合字符 (如 é → e + ´)
2. **去除变音符号**: 删除 `Mn` 类别字符
3. **小写转换**: 统一大小写

### 4.2 向量归一化

#### 4.2.1 余弦相似度计算

```go
// 余弦相似度公式
cosine_similarity = dot_product / (norm_a × norm_b)

其中:
- dot_product = Σ(a[i] × b[i])
- norm_a = √(Σ(a[i]²))
- norm_b = √(Σ(b[i]²))
```

**向量范围**: [-1, 1]，值越大表示越相似

#### 4.2.2 Milvus 向量归一化

Milvus 在插入向量时会自动进行归一化处理：

```go
// 稠密向量存储格式
type DenseVector struct {
    Dimension: int32     // 维度 (如 1024)
    Values:    []float32 // 向量值
}

// 稀疏向量 (BM25) 存储格式
type SparseEmbedding struct {
    Indices []uint32  // 非零维度位置
    Values  []float32 // 对应权重 (TF 或 BM25 分数)
}
```

---

## 五、向量化处理

### 5.1 稠密向量生成 (Embedding)

**流程**:
```
1. 提取所有分块内容
2. 批量调用 Embedding API (DashScope text-embedding-v2)
3. 获取 1024 维向量
4. 转换为 float32 格式
5. 存储到 Milvus dense_vector 字段
```

**配置**:
- **模型**: DashScope text-embedding-v2
- **维度**: 1024
- **批量大小**: 25
- **度量类型**: IP (Inner Product) / COSINE

### 5.2 稀疏向量生成 (BM25)

**流程**:
```
1. 文本分词 (jieba)
2. 统计词频
3. 构建 {indices, values} 格式
4. 存储到 Milvus sparse_vector 字段
```

**代码实现**:
```go
func generateSparseVector(text string) (entity.SparseEmbedding, error) {
    words := tokenize(text)

    // 统计词频
    wordCount := make(map[string]uint32)
    position := uint32(0)
    for _, word := range words {
        if _, exists := wordPositions[word]; !exists {
            wordPositions[word] = position
            position++
        }
        wordCount[word]++
    }

    // 构建 sparse embedding
    indices := make([]uint32, 0, len(wordCount))
    values := make([]float32, 0, len(wordCount))
    for word, count := range wordCount {
        indices = append(indices, wordPositions[word])
        values = append(values, float32(count))
    }

    return entity.NewSliceSparseEmbedding(indices, values), nil
}
```

### 5.3 度量类型选择

| 向量类型 | 度量类型 | 原因 |
|---------|---------|------|
| **稠密向量** | COSINE | 方向相似度，不受模长影响 |
| **稀疏向量** | IP (内积) | 与 BM25 分数方向一致，计算高效 |

---

## 六、知识图谱提取

### 6.1 提取流程

```
┌─────────────────────────────────────────────────────────────┐
│                    图谱提取流程                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  输入: 所有分块内容                                          │
│         ↓                                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第一阶段: 实体提取                                  │   │
│  │  Prompt: 提取命名实体 (人物/组织/概念等)              │   │
│  │  输出: nodes [{name, entity_type, properties}]      │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第二阶段: 关系提取                                  │   │
│  │  Prompt: 提取实体关系                                │   │
│  │  输出: relations [{source, target, type, strength}] │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第三阶段: 权重计算                                  │   │
│  │  • PMI (点互信息) 计算                              │   │
│  │  • 结合 LLM 输出的 strength                         │   │
│  │  • 归一化到 [1, 10] 范围                            │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  输出: GraphData → Neo4j 存储                               │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 PMI 权重计算

```go
// PMI (Pointwise Mutual Information) 公式
PMI(x, y) = log2(P(x, y) / (P(x) × P(y)))

// 归一化到 [0, 1]
normalizedPMI = (PMI + 5) / 15
normalizedPMI = max(0, min(1, normalizedPMI))

// LLM strength 归一化 (1-10 → 0-1)
normalizedStrength = (strength - 1) / 9

// 综合权重 (PMI 60% + Strength 40%)
weight = 1.0 + 9.0 × (normalizedPMI × 0.6 + normalizedStrength × 0.4)
```

---

## 七、数据存储

### 7.1 MySQL (元数据存储)

| 表 | 用途 | 关键字段 |
|----|------|---------|
| `knowledge_base` | 知识库配置 | id, name, embedding_model, chunk_strategy |
| `knowledge` | 文档元数据 | id, kb_id, title, parse_status, chunk_count |
| `chunks` | 分块内容 | id, kb_id, knowledge_id, content, pre_chunk_id |

### 7.2 Milvus (向量存储)

**Collection Schema**:
```
┌────────────────┬─────────────────┬─────────────────────────┐
│ 字段           │ 类型            │ 说明                    │
├────────────────┼─────────────────┼─────────────────────────┤
│ chunk_id       │ VARCHAR(36)     │ 主键 (对应 chunks.id)   │
│ knowledge_id   │ VARCHAR(36)     │ 文档 ID                 │
│ kb_id          │ VARCHAR(36)     │ 知识库 ID               │
│ tenant_id      │ INT64           │ 租户 ID                 │
│ chunk_index    │ INT64           │ 分块索引                │
│ content        │ VARCHAR(65535)  │ 分块内容                │
│ is_enabled     │ BOOL            │ 是否启用                │
│ dense_vector   │ FLOAT_VECTOR(1024) │ 稠密向量 (语义检索)  │
│ sparse_vector  │ SPARSE_VECTOR   │ 稀疏向量 (BM25 检索)    │
└────────────────┴─────────────────┴─────────────────────────┘
```

**索引配置**:
```go
// 稠密向量索引
denseIdx, _ := entity.NewIndexIvfFlat(
    entity.COSINE,  // 或 IP
    128,           // nlist
)

// 稀疏向量索引
sparseIdx, _ := entity.NewIndexSparseInverted(
    entity.IP,    // 内积度量
    0.2,         // drop_ratio (丢弃低权重 20%)
)
```

### 7.3 Neo4j (图谱存储)

**节点结构**:
```cypher
(:ENTITY:KB_{kb_id} {
    id: "uuid",
    name: "实体名称",
    entity_type: "Person|Organization|Concept|...",
    kb_id: "知识库ID",
    chunks: ["chunk_id1", "chunk_id2"]
})
```

**关系结构**:
```cypher
-[:RELATES_TO {
    id: "uuid",
    type: "CONTAINS|RELATED_TO|CAUSES|...",
    strength: 5.0,
    weight: 5.0,
    chunk_ids: ["chunk_id1", "chunk_id2"]
}]->
```

---

## 八、检索阶段

### 8.1 检索模式

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            检索模式                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐               │
│  │   Vector    │     │    BM25     │     │    Graph    │               │
│  │  向量检索    │     │  关键词检索  │     │   图谱检索   │               │
│  ├─────────────┤     ├─────────────┤     ├─────────────┤               │
│  │ • 语义匹配  │     │ • 精确匹配  │     │ • 实体关联  │               │
│  │ • 余弦相似度│     │ • BM25 算法 │     │ • 关系遍历  │               │
│  │ • TopK=15  │     │ • TopK=15   │     │ • TopK=10   │               │
│  └─────────────┘     └─────────────┘     └─────────────┘               │
│         │                   │                   │                       │
│         └───────────────────┼───────────────────┘                       │
│                             ↓                                           │
│                  ┌─────────────────────┐                                │
│                  │   RRF 融合重排      │                                │
│                  │   score = Σ(w/(k+rank)) │                           │
│                  └─────────────────────┘                                │
│                             ↓                                           │
│                    最终检索结果 (TopK=5)                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 8.2 RRF 融合算法

```go
// Reciprocal Rank Fusion
RRF_score(d) = α / (k + rank_vector(d)) + (1-α) / (k + rank_keyword(d))

默认参数:
- k = 60 (平滑常数)
- α = 0.5 (向量检索权重，可调整)
```

**优势**:
- 分数归一化: 不同检索器分数可以公平比较
- 排名敏感: 只考虑排名，不关心具体分数
- 权重灵活: α 参数可调整权重

---

## 九、并行处理优化

### 9.1 并发控制

使用 `errgroup` 实现并发控制，限制并发数为 4：

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(4)  // 最多 4 个并发任务

// 任务 1: 稠密向量生成 (必须)
g.Go(func() error {
    return uc.vectorizeChunks(ctx, ...)
})

// 任务 2: 稀疏向量生成 (可选)
if bm25Enabled {
    g.Go(func() error {
        return uc.generateBM25Vectors(ctx, ...)
    })
}

// 任务 3: 图谱提取 (可选)
if graphEnabled {
    g.Go(func() error {
        return uc.extractGraph(ctx, ...)
    })
}

if err := g.Wait(); err != nil {
    // 处理错误
}
```

### 9.2 性能指标

| 文档类型 | 大小 | 分块数 | 处理时间 |
|---------|------|--------|---------|
| TXT | 100 KB | ~200 | ~10 秒 |
| PDF | 500 KB | ~1000 | ~45 秒 |
| MD | 50 KB | ~100 | ~5 秒 |

---

## 十、API 接口

### 10.1 创建文档

```http
POST /api/v1/knowledge/documents
Content-Type: multipart/form-data

{
    "kb_id": "kb-xxx",
    "title": "文档标题",
    "file": <binary>,           // 文件
    "url": "https://...",       // 或 URL
    "content": "纯文本内容",     // 或直接文本
    "chunk_strategy": "semantic",
    "chunk_size": 1000,
    "chunk_overlap": 200,
    "graph_enabled": true
}
```

### 10.2 响应

```json
{
    "document_id": "doc-xxx",
    "chunk_count": 150,
    "chunk_ids": ["chunk-1", "chunk-2", ...],
    "parse_status": "completed",
    "process_time": 15000,
    "storage_size": 524288,
    "vectorized": true,
    "graph_extracted": true
}
```

---

## 十一、关键代码位置

| 功能 | 文件路径 |
|------|---------|
| 文档处理用例 | `link-go/internal/application/usecases/knowledge/document_processor.go` |
| 向量检索器 | `link-go/internal/infrastructure/persistence/milvus/retriever/repository.go` |
| RAG 检索 | `link-go/internal/application/rag/retriever.go` |
| 相似度计算 | `link-go/internal/domain/services/similarity.go` |
| RRF 重排 | `link-go/internal/application/rag/rerank.go` |
| 图谱仓储 | `link-go/internal/infrastructure/persistence/neo4j/graph_repo.go` |
| Python 文档解析 | `link-python/grpc/servicer/docreader_servicer.py` |

---

## 十二、总结

Link 知识库数据处理流程的核心特点：

1. **多阶段管道**: 解析 → 分块 → 存储 → 向量化 → 图谱提取
2. **混合检索**: 稠密向量 + 稀疏向量 + 图谱检索
3. **智能分块**: 支持多种分块策略，包括语义分块
4. **并行优化**: 使用 errgroup 并发处理提升效率
5. **PMI 加权**: 结合统计和语义信息计算关系权重
6. **RRF 融合**: 多路检索结果智能融合
7. **多租户隔离**: 通过 tenant_id 实现数据隔离

---

*文档版本: 1.0*
*创建日期: 2026-05-12*
*维护者: Link Team*
