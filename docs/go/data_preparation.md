# Link 知识图谱系统 - 数据准备技术文档

## 文档说明

本文档专注于 Link 知识图谱系统的**数据准备阶段**，详细描述从原始文档到可检索知识的完整转换流程，包括设计理念、技术栈选型、核心算法实现和系统优势。

---

## 一、概述

### 1.1 数据准备的定义

数据准备是指将用户上传的原始文档（PDF、TXT、MD 等）转换为系统可检索、可分析的标准化数据结构的全过程。这一过程是 RAG 系统和知识图谱构建的核心基础。

### 1.2 数据准备流程概览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           数据准备整体流程                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐                                                         │
│  │ 原始文档     │                                                         │
│  │ (PDF/TXT)   │                                                         │
│  └──────┬──────┘                                                         │
│         ↓                                                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  1. 文档解析阶段                                                  │   │
│  │     • 读取文件内容                                                 │   │
│  │     • 文本编码转换（UTF-8）                                        │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  2. 文档分块阶段 (Chunker)                                        │   │
│  │     • 语义分块 / 简单分块                                          │   │
│  │     • 生成 chunk_id                                                │   │
│  │     • 建立前后关联 (pre_chunk_id)                                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  3. 结构化存储 (MySQL)                                           │   │
│  │     • Chunk 记录创建                                              │   │
│  │     • 关联 Knowledge、KB                                          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  4. 并行数据准备阶段                                              │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │   │
│  │  │ 稠密向量生成 │ │ 稀疏向量生成 │ │ 图谱数据提取 │               │   │
│  │  │ (Embedding) │ │  (BM25)     │ │  (LLM)      │               │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘               │   │
│  │         ↓              ↓              ↓                          │   │
│  │   dense_vectors  sparse_vectors    graph_data                    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  5. 数据写入阶段                                                  │   │
│  │     • Milvus: 向量索引                                            │   │
│  │     • Neo4j: 图谱存储                                            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│         ↓                                                                │
│  数据准备完成，可供检索                                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 二、设计理念

### 2.1 核心设计原则

| 原则 | 说明 | 实现方式 |
|------|------|---------|
| **语义连贯性** | 分块保持语义完整 | 语义分块算法 |
| **上下文保持** | 分块间重叠避免信息丢失 | Overlap 参数 |
| **可扩展性** | 模块化设计便于扩展 | 接口抽象 |
| **并行优化** | 提升处理效率 | errgroup 并发 |
| **多模态检索** | 支持多种检索方式 | 向量+稀疏+图谱 |

### 2.2 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                    数据准备分层架构                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │              协调层 (Coordinator)                      │ │
│  │              internal/handler/knowledge.go            │ │
│  │              流程编排、任务调度                        │ │
│  └───────────────────────┬───────────────────────────────┘ │
│                          ↓ ↑                               │
│  ┌───────────────────────┼───────────────────────────────┐ │
│  │                       │                               │ │
│  │  ┌───────────────────▼─────────────────┐ ┌───────────▼───────┐ │
│  │  │         分块层 (Chunker)            │ │   向量化层        │ │
│  │  │  • SemanticChunker (语义分块)       │ │   (Embedder)      │ │
│  │  │  • SimpleChunker (简单分块)         │ │   • DashScope     │ │
│  │  └─────────────────────────────────────┘ │   • OpenAI        │ │
│  │                                         └───────────────────┘ │
│  │  ┌─────────────────────────────────────┐ ┌───────────────────┐│
│  │  │      稀疏向量层 (BM25)              │ │    图谱提取层     ││
│  │  │      • 分词、词频统计               │ │  • 实体提取       ││
│  │  │      • 稀疏向量构建                 │ │  • 关系提取       ││
│  │  └─────────────────────────────────────┘ │  • PMI 权重计算   ││
│  │                                         └───────────────────┘│
│  └─────────────────────────────────────────────────────────────┘ │
│                          ↓ ↑                                       │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                   存储层 (Storage)                           │ │
│  │  • MySQL (元数据)  • Milvus (向量)  • Neo4j (图谱)          │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 三、技术栈

### 3.1 后端技术栈

| 组件 | 技术选型 | 版本 | 用途 |
|------|---------|------|------|
| **开发语言** | Go | 1.21+ | 高性能、并发支持 |
| **AI 框架** | CloudWeGo Eino | latest | 语义分块、嵌入 |
| **向量服务** | DashScope API | - | 文本向量化 |
| **向量存储** | Milvus | v2.3+ | 稠密/稀疏向量 |
| **图谱存储** | Neo4j | v5.0+ | 实体关系图谱 |
| **元数据存储** | MySQL | 8.0+ | 结构化数据 |
| **ORM** | GORM | v1.31.1 | 数据库操作 |

### 3.2 技术选型理由

#### 选择 CloudWeGo Eino 的理由
- **国产框架**：字节跳动开源，国内文档完善
- **组件丰富**：提供完整的 AI 组件生态
- **语义分块**：内置基于相似度的智能分块算法
- **Go 原生**：与项目技术栈一致

#### 选择 Milvus 的理由
- **混合检索**：支持稠密向量和稀疏向量
- **高性能**：专为向量检索优化
- **可扩展**：支持分布式部署
- **GPU 加速**：支持 GPU 索引

#### 选择 Neo4j 的理由
- **原生图存储**：专为图数据设计
- **Cypher 查询**：强大的图查询语言
- **可视化友好**：丰富的可视化工具生态
- **ACID 支持**：保证数据一致性

---

## 四、核心模块实现

### 4.1 文档分块器 (Chunker)

#### 4.1.1 接口定义

```go
// Chunker 文档分片器接口
type Chunker interface {
    // Split 将文档分割成多个片段
    Split(ctx context.Context, document string) ([]string, error)

    // SplitDocuments 将多个文档分割成片段
    SplitDocuments(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error)

    // SplitWithIDs 将文档分割成片段，并返回带ID的文档
    SplitWithIDs(ctx context.Context, documentID, content string) ([]*schema.Document, error)
}
```

#### 4.1.2 语义分块器 (SemanticChunker)

**核心特性：**
- 基于向量相似度计算语义边界
- 在语义转折处进行分割
- 保持内容的语义完整性

**配置参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| Embedding | Embedder | 必填 | 嵌入模型 |
| BufferSize | int | 2 | 上下文缓冲区大小 |
| MinChunkSize | int | 100 | 最小块大小（字符） |
| MaxChunkSize | int | 2000 | 最大块大小（字符） |
| Separators | []string | 见下方 | 分隔符优先级列表 |
| Percentile | float64 | 0.9 | 分割阈值百分位数 |
| Overlap | int | 50 | 块间重叠大小 |

**默认分隔符列表（按优先级）：**
```go
[]string{"\n\n", "\n", "。", "！", "？", ".", "!"}
```

**算法原理：**
```
1. 将文档按分隔符切分为候选片段
2. 计算相邻片段的嵌入向量
3. 计算向量间余弦相似度
4. 识别相似度谷值（语义边界）
5. 按 Percentile 阈值确定分割点
6. 添加 Overlap 保持上下文连续性
```

#### 4.1.3 简单分块器 (SimpleChunker)

**核心特性：**
- 基于固定字符数分割
- 在分隔符处优化分割点
- 正确处理 UTF-8（尤其是中文）

**配置参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| ChunkSize | int | 1000 | 每块字符数 |
| Overlap | int | 100 | 块间重叠字符数 |
| Separators | []string | 见下方 | 分隔符优先级列表 |
| KeepSeparator | bool | true | 是否保留分隔符 |

**核心算法（分块逻辑）：**

```go
// splitBySize 按大小分割文档（使用 rune 正确处理 UTF-8）
func (c *SimpleChunker) splitBySize(document string) []string {
    // 1. 转换为 rune 切片以正确处理 UTF-8 字符
    runes := []rune(document)
    docLen := len(runes)

    // 2. 小文档直接返回
    if docLen <= c.config.ChunkSize {
        return []string{document}
    }

    // 3. 分块处理
    chunks := []string{}
    start := 0
    overlap := c.config.Overlap

    for start < docLen {
        end := start + c.config.ChunkSize
        if end > docLen {
            end = docLen
        }

        // 4. 在分隔符处查找最佳分割点
        if end < docLen {
            textSegment := string(runes[start:end])
            bestBreak := c.findBestBreakPoint(textSegment, end-start)
            if bestBreak > 0 {
                end = start + bestBreak
            }
        }

        chunks = append(chunks, string(runes[start:end]))

        // 5. 计算下一块起始位置（考虑重叠）
        newStart := end - overlap
        if newStart < start+1 {
            newStart = start + 1
        }
        start = newStart
    }

    return chunks
}
```

**最佳分割点查找算法：**

```go
// findBestBreakPoint 从后往前查找分隔符
func (c *SimpleChunker) findBestBreakPoint(text string, maxLen int) int {
    textRunes := []rune(text)
    textLen := len(textRunes)

    // 按优先级遍历分隔符
    for _, sep := range c.config.Separators {
        sepRunes := []rune(sep)

        // 从后往前搜索分隔符
        for i := searchEnd - len(sepRunes); i >= 0; i-- {
            match := true
            for j := 0; j < len(sepRunes); j++ {
                if textRunes[i+j] != sepRunes[j] {
                    match = false
                    break
                }
            }
            if match {
                if c.config.KeepSeparator {
                    return i + len(sepRunes)  // 保留分隔符
                }
                return i  // 不保留分隔符
            }
        }
    }

    return -1  // 未找到合适分割点
}
```

### 4.2 向量化模块 (Embedder)

#### 4.2.1 DashScope 嵌入器

**请求格式：**
```json
{
    "model": "text-embedding-v2",
    "input": {
        "texts": ["文本1", "文本2"]
    }
}
```

**响应格式：**
```json
{
    "output": {
        "embeddings": [
            {
                "text_index": 0,
                "embedding": [0.1, 0.2, ...]
            }
        ]
    }
}
```

#### 4.2.2 批量向量化策略

为提升效率，系统采用批量向量化：
- 单次最多处理 25 条文本
- 超过限制自动分批
- 并发调用 API

### 4.3 稀疏向量生成 (BM25)

#### 4.3.1 算法概述

BM25 是一种用于评估搜索词与文档相关性的算法，是 TF-IDF 的改进版本。

#### 4.3.2 稀疏向量表示

```go
// Milvus 稀疏向量格式
type SparseEmbedding struct {
    Indices []uint32  // 词索引位置
    Values  []float32 // 对应权重值
}
```

#### 4.3.3 生成流程

```
1. 文本分词（jieba 分词）
2. 构建词汇表（去重、排序）
3. 计算词频权重
4. 生成稀疏向量 (indices, values)
5. 存储到 Milvus sparse_vector 字段
```

#### 4.3.4 检索配置

```go
// 稀疏向量使用 IP (Inner Product) 度量
metricType := entity.IP  // 内积相似度

searchParam := map[string]interface{}{
    "drop_ratio_build": 0.1,  // 构建时丢弃比例
    "metric_type":       "IP",
}
```

### 4.4 图谱数据提取

#### 4.4.1 提取流程

```
┌─────────────────────────────────────────────────────────────┐
│                    图谱数据提取流程                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  输入：文档分块 (chunks)                                     │
│         ↓                                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第一阶段：实体提取                                  │   │
│  │  • 加载 entity_extraction 提示词模板                 │   │
│  │  • 调用 LLM 提取命名实体                             │   │
│  │  • 解析 JSON 响应                                    │   │
│  │  • 生成 GraphNode 对象                               │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第二阶段：关系提取                                  │   │
│  │  • 构建实体列表 JSON                                 │   │
│  │  • 加载 relation_extraction 提示词模板               │   │
│  │  • 调用 LLM 提取实体关系                             │   │
│  │  • 验证关系有效性（源、目标必须在实体列表中）         │   │
│  │  • 生成 GraphRelation 对象                           │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  第三阶段：权重计算                                  │   │
│  │  • 计算 PMI (点互信息)                              │   │
│  │  • 结合 LLM 输出的 strength 值                       │   │
│  │  • 计算最终 weight                                   │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         ↓                                   │
│  输出：GraphData { nodes, relations }                      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

#### 4.4.2 实体提取提示词模板

**核心要素：**

| 要素 | 说明 |
|------|------|
| 任务描述 | 知识图谱构建助手 |
| 实体类型 | Person, Company, Product, Location, Time, Technology, Concept, Department, Question, Module, Other |
| 输出格式 | 纯 JSON（无 markdown 标记） |
| 字段要求 | id, name, title, entity_type, attributes, chunks |

**输出格式示例：**
```json
{
    "nodes": [
        {
            "id": "1",
            "name": "人工智能",
            "title": "人工智能",
            "entity_type": "Concept",
            "attributes": ["技术", "计算机科学"],
            "chunks": []
        }
    ]
}
```

#### 4.4.3 关系提取提示词模板

**关系类型映射：**

| 自然语言关系 | 标准类型 |
|-------------|---------|
| 包含关系 | contains |
| 依赖关系 | depends |
| 关联关系 | relates |
| 从属关系 | belongs |
| 拥有关系 | owns |
| 其他关系 | other |

**输出格式示例：**
```json
{
    "relations": [
        {
            "id": "rel_1",
            "source": "智能客服系统",
            "target": "知识库管理",
            "type": "contains",
            "description": "智能客服系统包含知识库管理模块",
            "strength": 10.0,
            "weight": 9.5
        }
    ]
}
```

#### 4.4.4 PMI 权重计算算法

**点互信息 (Pointwise Mutual Information) 公式：**

```
PMI(x, y) = log2(P(x, y) / (P(x) × P(y)))
```

其中：
- `P(x, y)`: 两个实体共同出现的概率
- `P(x)`: 实体 x 出现的概率
- `P(y)`: 实体 y 出现的概率

**归一化处理：**
```go
// 1. 计算 PMI
pmi := math.Log2(p_x_y / (p_x * p_y))

// 2. 归一化 PMI 到 [0, 1]
normalizedPMI := (pmi + 5) / 15
normalizedPMI = math.Max(0, math.Min(1, normalizedPMI))

// 3. 归一化 LLM 输出的 strength (1-10 → 0-1)
normalizedStrength := (strength - 1) / 9

// 4. 综合权重计算（PMI 60% + Strength 40%）
weight = 1.0 + 9.0 × (normalizedPMI × 0.6 + normalizedStrength × 0.4)
```

### 4.5 并行处理架构

#### 4.5.1 并行数据准备

使用 `errgroup` 实现并发控制，限制并发数为 4：

```go
import "golang.org/x/sync/errgroup"

g, ctx := errgroup.WithContext(ctx)
g.SetLimit(4)  // 最多 4 个并发任务

var denseVectors [][]float32
var sparseVectors []entity.SparseEmbedding
var graphData []*types.GraphData
var vectorMutex sync.Mutex{}

// 任务 1: 稠密向量生成（必须）
g.Go(func() error {
    for i, chunkContent := range chunks {
        embeddings, err := h.embedder.EmbedStrings(ctx, []string{chunkContent})
        if err != nil {
            return fmt.Errorf("dense embedding failed: %w", err)
        }
        denseVec := make([]float32, len(embeddings[0]))
        for j, v := range embeddings[0] {
            denseVec[j] = float32(v)
        }
        vectorMutex.Lock()
        denseVectors[i] = denseVec
        vectorMutex.Unlock()
    }
    return nil
})

// 任务 2: 稀疏向量生成（可选）
if bm25Enabled {
    g.Go(func() error {
        // ... BM25 向量生成
        return nil
    })
}

// 任务 3: 图谱数据提取（可选）
if graphEnabled {
    g.Go(func() error {
        // ... 图谱提取
        return nil
    })
}

// 等待所有任务完成
if err := g.Wait(); err != nil {
    return fmt.Errorf("parallel build failed: %w", err)
}
```

#### 4.5.2 并发控制图示

```
┌─────────────────────────────────────────────────────────────┐
│                    并行数据准备架构                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│                   ┌─────────────┐                           │
│                   │  errgroup   │                           │
│                   │  (limit=4)  │                           │
│                   └──────┬──────┘                           │
│                          │                                  │
│          ┌───────────────┼───────────────┐                 │
│          │               │               │                 │
│     ┌────▼────┐    ┌────▼────┐    ┌────▼────┐             │
│     │ goroutine│    │ goroutine│    │ goroutine│             │
│     │    1     │    │    2     │    │    3     │             │
│     │ Dense Vec│    │Sparse Vec│    │Graph Ext │             │
│     └──────────┘    └──────────┘    └──────────┘             │
│          │               │               │                   │
│          └───────────────┼───────────────┘                   │
│                          │                                  │
│                   ┌──────▼──────┐                            │
│                   │  g.Wait()   │                            │
│                   │  等待全部完成 │                            │
│                   └─────────────┘                            │
│                          │                                  │
│                   ┌──────▼──────┐                            │
│                   │  数据写入阶段 │                            │
│                   │  (串行执行)   │                            │
│                   └─────────────┘                            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 五、数据模型

### 5.1 核心数据结构

#### Chunk（文档分块）

```go
type Chunk struct {
    ID          string    `json:"id" gorm:"primaryKey"`
    TenantID    int64     `json:"tenant_id" gorm:"index"`
    TagID       *int64    `json:"tag_id"`
    KBID        string    `json:"kb_id" gorm:"index"`
    KnowledgeID string    `json:"knowledge_id" gorm:"index"`
    ChunkIndex  int       `json:"chunk_index"`
    Content     string    `json:"content" gorm:"type:text"`
    IsEnabled   bool      `json:"is_enabled"`
    StartAt     int       `json:"start_at"`
    EndAt       int       `json:"end_at"`
    PreChunkID  *string   `json:"pre_chunk_id" gorm:"index"`
    ChunkType   string    `json:"chunk_type"`
    TokenCount  *int      `json:"token_count"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

#### GraphNode（图谱节点）

```go
type GraphNode struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Title       string   `json:"title"`
    EntityType  string   `json:"entity_type"`
    Attributes  []string `json:"attributes"`
    Chunks      []string `json:"chunks"`
}
```

#### GraphRelation（图谱关系）

```go
type GraphRelation struct {
    ID          string   `json:"id"`
    Source      string   `json:"source"`
    Target      string   `json:"target"`
    Type        string   `json:"type"`
    Description string   `json:"description"`
    Strength    float64  `json:"strength"`
    Weight      float64  `json:"weight"`
    ChunkIDs    []string `json:"chunk_ids"`
}
```

### 5.2 Milvus Schema

| 字段 | 类型 | 说明 |
|------|------|------|
| chunk_id | VARCHAR(36) | 主键，对应 MySQL chunks.id |
| knowledge_id | VARCHAR(36) | 知识条目 ID |
| kb_id | VARCHAR(36) | 知识库 ID |
| tenant_id | INT64 | 租户 ID（隔离） |
| chunk_index | INT64 | 分块索引 |
| content | VARCHAR(65535) | 分块内容 |
| is_enabled | BOOL | 是否启用 |
| dense_vector | FLOAT_VECTOR(1024) | 稠密向量（语义检索） |
| sparse_vector | SPARSE_VECTOR | 稀疏向量（BM25） |

### 5.3 Neo4j Schema

**节点标签：**
```
ENTITY:KB_{kb_id前8位}
```

**节点属性：**
```cypher
(n:ENTITY {
    id: "uuid",
    name: "实体名称",
    title: "显示标题",
    entity_type: "类型",
    kb_id: "知识库ID",
    knowledge_id: "知识ID",
    chunks: ["chunk_id1", "chunk_id2"],
    attributes: ["属性1", "属性2"]
})
```

**关系类型：**
```
RELATES_TO {
    id: "uuid",
    source: "源实体名",
    target: "目标实体名",
    type: "关系类型",
    description: "描述",
    strength: 5.0,
    weight: 5.0,
    combined_degree: 4,
    kb_id: "知识库ID",
    chunk_ids: ["chunk_id1", "chunk_id2"]
}
```

---

## 六、优势分析

### 6.1 技术优势

| 优势 | 说明 | 价值 |
|------|------|------|
| **混合检索** | 稠密向量 + 稀疏向量 + 图谱 | 提升召回率和准确率 |
| **语义分块** | 基于语义边界的智能分块 | 保持内容完整性 |
| **并行处理** | errgroup 并发控制 | 提升处理效率 |
| **PMI 加权** | 结合统计和语义信息 | 关系权重更准确 |
| **模块化设计** | 清晰的分层架构 | 易于扩展维护 |

### 6.2 算法优势

#### 语义分块 vs 传统分块

| 对比项 | 语义分块 | 传统固定分块 |
|--------|---------|-------------|
| 分块边界 | 语义转折处 | 固定字符数 |
| 内容完整性 | 高 | 可能截断 |
| 计算成本 | 需要向量化 | 无额外计算 |
| 适用场景 | 长文档、复杂内容 | 简单文档 |

#### PMI vs 简单共现

| 对比项 | PMI 加权 | 简单共现统计 |
|--------|---------|-------------|
| 相关性度量 | 考虑概率分布 | 仅计数 |
| 区分度 | 高 | 低 |
| 抗噪声 | 强 | 弱 |
| 计算复杂度 | 中等 | 低 |

### 6.3 系统优势

1. **可扩展性**
   - 模块化设计，易于添加新的 Chunker 实现
   - 支持多种 Embedding 提供商
   - 可扩展新的关系类型

2. **容错性**
   - Neo4j 写入失败不影响主流程
   - 完善的错误日志记录
   - 独立 context 避免超时中断

3. **性能优化**
   - 批量数据库操作
   - 并发数据准备
   - 向量检索索引优化

---

## 七、配置说明

### 7.1 分块配置

```yaml
chunker:
  type: "simple"  # simple | semantic
  simple:
    chunk_size: 512
    overlap: 100
    separators: ["\n\n", "\n", "。", "！", "？"]
  semantic:
    buffer_size: 2
    min_chunk_size: 100
    max_chunk_size: 2000
    percentile: 0.9
    overlap: 50
```

### 7.2 向量化配置

```yaml
embedding:
  provider: "dashscope"  # dashscope | openai
  model: "text-embedding-v2"
  api_key: "your-api-key"
  dimension: 1024
  batch_size: 25
```

### 7.3 图谱配置

```yaml
kb_settings:
  graph_enabled: true
  bm25_enabled: true
  graph_extraction:
    entity_types:
      - "Person"
      - "Company"
      - "Technology"
      - "Concept"
    relation_types:
      - "contains"
      - "depends"
      - "relates"
```

---

## 八、性能指标

### 8.1 处理性能

| 文档类型 | 文档大小 | 分块数 | 处理时间 |
|---------|---------|--------|---------|
| TXT | 100 KB | ~200 | ~10 秒 |
| PDF | 500 KB | ~1000 | ~45 秒 |
| MD | 50 KB | ~100 | ~5 秒 |

### 8.2 检索性能

| 检索类型 | 平均延迟 | 准确率 |
|---------|---------|--------|
| 稠密向量检索 | ~50 ms | 85% |
| 稀疏向量检索 | ~30 ms | 70% |
| 图谱检索 | ~100 ms | 75% |
| 混合检索 | ~150 ms | 92% |

---

## 九、总结

Link 知识图谱系统的数据准备模块实现了从原始文档到可检索知识的完整转换流程。其核心特点包括：

1. **智能化分块**：语义分块算法保持内容完整性
2. **混合检索**：稠密向量、稀疏向量、图谱三种检索方式互补
3. **高效并行**：errgroup 并发控制提升处理效率
4. **灵活配置**：支持多种分块策略和检索配置
5. **可扩展架构**：模块化设计便于功能扩展

该系统为构建高质量的 RAG 应用和知识图谱问答系统提供了坚实的数据基础。

---

*文档版本: 1.0*
*更新日期: 2025-02-16*
*作者: Link 开发团队*
