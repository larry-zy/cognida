# 知识图谱构建完整流程

## 从文档切片到权重计算的完整链路

```
用户上传文档 → 切片 → Embedding → Milvus → LLM提取 → Neo4j → 分片合并 → PMI权重计算
```

---

## 阶段1：文档切片（Chunking）

### 代码位置
`internal/handler/knowledge.go:189-229`

### 流程
```go
// 1. 读取文档文件
content, err := os.ReadFile(knowledge.FilePath)

// 2. 解析分片配置
chunkSize := 512          // 每个分片大小（字符数）
chunkOverlap := 100        // 分片重叠大小（字符数）

// 3. 创建 Chunker
newChunker := chunker.NewSimpleChunker(&chunker.SimpleConfig{
    ChunkSize:     int(chunkSize),
    Overlap:        int(chunkOverlap),
    Separators:     []string{"\n\n", "\n", "。", "！", "？", ".", "!"},
    KeepSeparator:  true,
})

// 4. 执行分片
chunks, err := newChunker.Split(ctx, string(content))
```

### 分片策略
- **简单分片**：按分隔符（\n\n、。、！等）切分
- **重叠**：相邻分片重叠 100 个字符
- **大小限制**：100-2000 字符

---

## 阶段2：保存分片到数据库

### 代码位置
`internal/handler/knowledge.go:256-301`

### 流程
```go
// 创建事务
tx := db.Begin()

// 遍历每个分片
for i, chunkContent := range chunks {
    chunkID := uuid.New().String()

    // 创建 Chunk 记录
    chunkRecord := &types.Chunk{
        ID:          chunkID,
        TenantID:    knowledge.TenantID,
        KBID:        knowledge.KBID,
        KnowledgeID:  knowledge.ID,
        ChunkIndex:   i,
        Content:      chunkContent,
        IsEnabled:    true,
        TokenCount:   len(chunkContent) / 2,
    }

    tx.Create(chunkRecord)
    chunkIDs[i] = chunkID
}

// 提交事务
tx.Commit()
```

### 数据库字段
- `chunk_id`: UUID（主键）
- `tenant_id`: 租户ID
- `kb_id`: 知识库ID
- `knowledge_id`: 关联的知识条目ID
- `chunk_index`: 分片索引（从0开始）
- `content`: 分片内容
- `is_enabled`: 是否启用（true）
- `token_count`: 字符数/2（粗略估算）

---

## 阶段3：向量索引（Embedding + Milvus）

### 代码位置
`internal/handler/knowledge.go:303-446`

### 流程

#### 3.1 并发生成 Embeddings（4个并发）

```go
// 创建 errgroup 限制并发数为 4
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(4)

// 准备数据
denseVectors := make([][]float32, len(chunks))
sparseVectors := make([]entity.SparseEmbedding, len(chunks))

// 并发处理
for i, chunkContent := range chunks {
    g.Go(func() error {
        // 生成稠密向量
        embeddings, err := h.embedder.EmbedStrings(ctx, []string{chunkContent})

        // 转换为 float32
        denseVec := make([]float32, len(embeddings[0]))
        for j, v := range embeddings[0] {
            denseVec[j] = float32(v)
        }

        // 生成稀疏向量（简单BM25）
        sparseVec, err := h.generateSparseVector(chunkContent)
        // 基于词频统计的稀疏向量

        // 线程安全保存
        vectorMutex.Lock()
        denseVectors[i] = denseVec
        sparseVectors[i] = sparseVec
        vectorMutex.Unlock()
    })
}
```

#### 3.2 写入 Milvus

```go
collectionName := "link"

// 构建插入列
chunkIDColumn := entity.NewColumnVarChar("chunk_id", chunkIDs)
knowledgeIDColumn := entity.NewColumnVarChar("knowledge_id", ...)
kbIDColumn := entity.NewColumnVarChar("kb_id", ...)
tenantIDColumn := entity.NewColumnInt64("tenant_id", ...)
contentColumn := entity.NewColumnVarChar("content", chunks)

// 向量列
denseVectorColumn := entity.NewColumnFloatVector("dense_vector", ...)
sparseVectorColumn := entity.NewColumnSparseVectors("sparse_vector", ...)

// 批量插入
_, err := h.milvusClient.Insert(ctx, collectionName, "",
    chunkIDColumn,
    knowledgeIDColumn,
    kbIDColumn,
    tenantIDColumn,
    chunkIndexColumn,
    contentColumn,
    isEnabledColumn,
    startAtColumn,
    endAtColumn,
    tokenCountColumn,
    denseVectorColumn,
    sparseVectorColumn,
)

// 刷新以确保可搜索
h.milvusClient.Flush(ctx, collectionName, false)
```

### 向量类型
- **稠密向量**：通过 Embedding 模型生成（1536维）
- **稀疏向量**：基于 BM25 词频统计生成

---

## 阶段4：LLM 图谱提取

### 代码位置
`internal/handler/knowledge.go:481-525`

### 流程

#### 4.1 构建提取输入

```go
inputs := make([]*service.ChunkExtractionInput, len(chunks))

for i, chunk := range chunks {
    inputs[i] = &service.ChunkExtractionInput{
        ChunkID:  chunkIDs[i],
        Document: chunk,
        Query:    "请提取实体和关系",
    }
}

// 调用 GraphService.ExtractGraphFromChunks
graphData, err := h.graphService.ExtractGraphFromChunks(ctx, inputs)
```

#### 4.2 实体提取（Entity Extraction）

**代码位置**：`internal/application/service/graph.go:159-227`

```go
// 1. 加载实体提取 Prompt 模板
promptTemplate, err := config.LoadPromptTemplate("entity_extraction")

// 2. 替换占位符
prompt := strings.Replace(promptTemplate, "{{document}}", document, 1)

// 3. 创建 Chat
chatConfig := config.LoadChatConfig()
chatModel, err := chat.NewChat(&chat.ChatConfig{
    Source:    chatConfig.Source,
    BaseURL:   chatConfig.BaseURL,
    ModelName: chatConfig.ModelName,
    APIKey:    chatConfig.APIKey,
    Provider:  chatConfig.Provider,
})

// 4. 调用 LLM
messages := []chat.Message{
    {Role: "user", Content: prompt},
}

response, err := chatModel.Chat(ctx, messages, nil)

// 5. 清理响应内容
cleanedContent := cleanJSONResponse(response.Content)

// 6. 解析 JSON
var result struct {
    Nodes []*types.GraphNode `json:"nodes"`
}
json.Unmarshal([]byte(cleanedContent), &result)

// 7. 为每个节点添加 chunk_id 追踪
for _, node := range result.Nodes {
    if node.ID == "" {
        node.ID = uuid.New().String()
    }
    if node.Title == "" {
        node.Title = node.Name
    }
    if !slices.Contains(node.Chunks, chunkID) {
        node.Chunks = append(node.Chunks, chunkID)
    }
}
```

**Prompt 模板**：`config/prompt_templates/entity_extraction.yaml`

```yaml
指令: |
  你是一个专业的知识图谱实体提取专家。

  任务：从给定的文档中提取所有有意义的实体。

  提取规则：
  1. 实体应该是文档中的核心概念、人名、公司名、产品名等
  2. 实体应该是独立的、有明确边界的概念单元
  3. 优先提取能构成图谱节点的实体
  4. 避免提取过于宽泛或模糊的概念

  输出格式（JSON）：
  {
    "nodes": [
      {"name": "实体名称", "type": "实体类型"}
    ]
  }

  示例：
  文档：张三是公司的技术总监，负责AI产品研发。
  输出：
  {
        "nodes": [
          {"name": "张三", "type": "Person"},
          {"name": "技术总监", "type": "Role"},
          {"name": "AI产品", "type": "Product"},
          {"name": "研发", "type": "Activity"}
        ]
  }
```

#### 4.3 关系提取（Relation Extraction）

**代码位置**：`internal/application/service/graph.go:228-323`

```go
// 1. 加载关系提取 Prompt 模板
promptTemplate, err := config.LoadPromptTemplate("relation_extraction")

// 2. 构建实体列表 JSON
entityNames := make([]string, 0, len(entities))
for _, e := range entities {
    entityNames = append(entityNames, fmt.Sprintf(`"%s"`, e.Title))
}

// 3. 替换占位符
prompt := strings.Replace(promptTemplate, "{{entities}}", strings.Join(entityNames, ", "), 1)
prompt = strings.Replace(promptTemplate, "{{document}}", document, 1)

// 4. 创建 Chat 并调用
messages := []chat.Message{
    {Role: "user", Content: prompt},
}

response, err := chatModel.Chat(ctx, messages, nil)

// 5. 解析 JSON 响应
var result struct {
    Relations []*types.GraphRelation `json:"relations"`
}
json.Unmarshal([]byte(cleanedContent), &result)

// 6. 为关系添加 chunk_id
for _, rel := range result.Relations {
    if !slices.Contains(rel.ChunkIDs, chunkID) {
        rel.ChunkIDs = append(rel.ChunkIDs, chunkID)
    }
}
```

**Prompt 模板**：`config/prompt_templates/relation_extraction.yaml`

```yaml
指令: |
  你是一个专业的知识图谱关系提取专家。

  任务：从给定的实体列表中，提取实体之间所有有意义的关系。

  关系类型：
  - belongs_to: 从属关系（A属于B）
  - part_of: 组成关系（A是B的一部分）
  - leads: 领导关系（A领导B）
  - partnered_with: 合作关系（A与B合作）
  - integrates_with: 集成关系（A集成B）
  - colleague_of: 同事关系（A是B的同事）

  输出格式（JSON）：
  {
    "relations": [
      {
        "source": "源实体名称",
        "target": "目标实体名称",
        "type": "关系类型",
        "description": "关系描述"
      }
    ]
  }

  示例：
  实体：[张三, 李四, 技术部]
  文档：张三负责技术部，李四是技术部的架构师。
  输出：
  {
        "relations": [
          {"source": "张三", "target": "技术部", "type": "leads", "description": "负责"},
          {"source": "李四", "target": "技术部", "type": "part_of", "description": "架构师"}
        ]
  }
```

---

## 阶段5：写入 Neo4j（初次写入）

### 代码位置
`internal/application/repository/retriever/neo4j/repository.go:71-183`

### 流程

```cypher
// ========== 创建节点 ==========
UNWIND $data AS row
MATCH (source:nodeLabel {id: row.source_id})
MERGE (source)
SET source.id = row.id
SET source.name = row.name
SET source.title = COALESCE($title, $name)
SET source.entity_type = $entity_type
SET source.knowledge_id = $knowledge_id

// ========== 创建关系（含权重计算） ==========
UNWIND $data AS row
MATCH (source:nodeLabel {id: row.source_id})
MATCH (target:nodeLabel {id: row.target_id})

// 计算源节点和目标节点的度数
OPTIONAL MATCH (source)-[outRel]-()
WITH source, target, row, count(outRel) AS sourceDegree
OPTIONAL MATCH (target)-[inRel]-()
WITH source, target, row, sourceDegree, count(inRel) AS targetDegree

// 计算 CombinedDegree = 出度 + 入度
WITH source, target, row, sourceDegree + targetDegree AS combinedDegree

// 创建关系
MERGE (source)-[r:RELATES_TO {id: row.id}]->(target)
SET r.source = row.source          // 源节点名称
SET r.target = row.target          // 目标节点名称
SET r.type = COALESCE(row.type, '关联')           // 关系类型（默认关联）
SET r.description = row.description               // 关系描述
SET r.strength = COALESCE(row.strength, 5.0)   // ★ 默认 5.0（LLM未传递时）
SET r.weight = COALESCE(row.weight, 5.0)          // ★ 默认 5.0（前端未传递时）
SET r.combined_degree = combinedDegree                // 节点度数
```

### 数据结构
- **节点**：ID、name、title、entity_type、knowledge_id
- **关系**：ID、source、target、type、description、strength、weight、combined_degree

### 默认值说明
- `strength`: **5.0**（LLM 未返回时）
- `weight`: **5.0**（前端未传递时）
- `type`: **"关联"**（LLM 未返回时）

---

## 阶段6：分片合并（Graph Merging）

### 代码位置
`internal/application/service/graph.go:326-443`

### 触发时机
- 当所有分片的 LLM 提取完成后
- 在 `mergeExtractedGraphs` 函数中执行

### 流程

#### 6.1 合并节点（去重）

```go
// 使用图谱缓存（graphCache）合并节点
for _, data := range dataList {
    for _, node := range data.Nodes {
        key := node.Title  // 使用实体标题作为唯一键

        if existingNode, exists := s.graphCache.nodes[key]; exists {
            // 合并 chunks（去重）
            for _, chunk := range node.Chunks {
                if !slices.Contains(existingNode.Chunks, chunk) {
                    existingNode.Chunks = append(existingNode.Chunks, chunk)
                }
            }

            // 合并 attributes（去重）
            for _, attr := range node.Attributes {
                if !slices.Contains(existingNode.Attributes, attr) {
                    existingNode.Attributes = append(existingNode.Attributes, attr)
                }
            }
        } else {
            // 新节点，直接添加
            s.graphCache.nodes[key] = node
        }
    }
}
```

#### 6.2 合并关系（去重 ChunkIDs）

```go
for _, data := range dataList {
    for _, rel := range data.Relations {
        key := fmt.Sprintf("%s#%s", rel.Source, rel.Target)

        if existingRel, exists := s.graphCache.relations[key]; exists {
            // 合并 chunk_ids（去重）
            for _, chunkID := range rel.ChunkIDs {
                if !slices.Contains(existingRel.ChunkIDs, chunkID) {
                    existingRel.ChunkIDs = append(existingRel.ChunkIDs, chunkID)
                }
            }
        } else {
            // 新关系，直接添加
            s.graphCache.relations[key] = rel
        }
    }
}
```

#### 6.3 PMI 权重重新计算

```go
// ========== 统计实体出现信息 ==========
totalChunks := len(dataList)                               // 总文档块数
entityChunkCount := make(map[string]int)                 // 每个实体出现的文档块数
coOccurrenceCount := make(map[string]int)                  // 每对实体共同出现的文档块数

// 第一阶段：收集统计数据
for _, data := range dataList {
    for _, node := range data.Nodes {
        entityChunkCount[node.Title] = len(node.Chunks)
    }

    for _, rel := range data.Relations {
        sourceChunks := s.graphCache.nodes[rel.Source].Chunks
        targetChunks := s.graphCache.nodes[rel.Target].Chunks
        commonChunks := intersection(sourceChunks, targetChunks)
        coOccurrenceCount[fmt.Sprintf("%s#%s", rel.Source, rel.Target)] = len(commonChunks)
    }
}

// ========== 第二阶段：计算 PMI 和权重 ==========
for _, rel := range s.graphCache.relations {
    // 1. 计算概率
    p_x_y := float64(coOccurrenceCount[key]) / float64(totalChunks)
    p_x := float64(entityChunkCount[rel.Source]) / float64(totalChunks)
    p_y := float64(entityChunkCount[rel.Target]) / float64(totalChunks)

    // 2. 计算 PMI（点互信息）
    var pmi float64
    if p_x > 0 && p_y > 0 && p_x_y > 0 {
        pmi = math.Log2(p_x_y / (p_x * p_y))
    }

    // 3. 归一化 PMI 到 [0, 1]（假设范围 [-5, 10]）
    normalizedPMI := (pmi + 5) / 15
    normalizedPMI = math.Max(0, math.Min(1, normalizedPMI))

    // 4. 归一化 Strength 到 [0, 1]（假设范围 [1, 10]）
    normalizedStrength := (rel.Strength - 1) / 9
    normalizedStrength = math.Max(0, math.Min(1, normalizedStrength))

    // 5. 重新计算 Weight ★重点★
    // Weight = 1.0 + 9.0 * (normalizedPMI * 0.6 + normalizedStrength * 0.4)
    rel.Weight = 1.0 + 9.0 * (normalizedPMI * 0.6 + normalizedStrength * 0.4)

    // 6. 重新计算 CombinedDegree
    sourceDegree := 0
    targetDegree := 0
    for _, r := range s.graphCache.relations {
        if r.Source == rel.Source || r.Target == rel.Source {
            sourceDegree++
        }
        if r.Source == rel.Target || r.Target == rel.Target {
            targetDegree++
        }
    }
    rel.CombinedDegree = sourceDegree + targetDegree

    log.Printf("[GraphService] Relation %s -> %s: PMI=%.2f, Weight=%.2f, CombinedDegree=%d",
        rel.Source, rel.Target, pmi, rel.Weight, rel.CombinedDegree)
}
```

### PMI 权重公式详解

```
PMI（点互信息）= log2(P(x,y) / (P(x) * P(y)))

其中：
- P(x,y): 实体对x和y在同一个文档中共同出现的概率
- P(x): 实体x在所有文档中出现的概率
- P(y): 实体y在所有文档中出现的概率

归一化：
- normalizedPMI = (PMI + 5) / 15    // 范围 [-5, 10] → [0, 1]
- normalizedStrength = (strength - 1) / 9  // 范围 [1, 10] → [0, 1]

最终权重：
Weight = 1.0 + 9.0 × (normalizedPMI × 0.6 + normalizedStrength × 0.4)
      = 基础分1.0 + PMI贡献(5.4×PMI) + 强度贡献(0.36×strength)

解释：
- PMI 越高，实体对越相关 → 权重越高
- Strength 越高，关系越强 → 权重越高
- 归一化确保各项在合理范围内
```

---

## 数据流图

```
┌─────────────────────────────────────────────────────────┐
│ 用户上传文档                                       │
└──────────────┬────────────────────────────────────────┘
               │
               ▼
      ┌──────────────────────────────┐
      │ 1. 文档切分             │
      │ (SimpleChunker)           │
      │ - 按 \n\n、。分隔       │
      │ - 重叠 100 字符         │
      │ - 每片 100-2000 字符     │
      └──────────────┬────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────┐
      │ 2. 保存分片到数据库               │
      │ chunks 表                           │
      │ - chunk_id (UUID)                 │
      │ - content (分片内容)                │
      │ - chunk_index (序号)                 │
      │ - is_enabled (true)                  │
      └──────────────┬────────────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────────────┐
      │ 3. 并发 Embedding 生成 (4并发)      │
      │                                      │
      │ 稠密向量 (1536维)                  │
      │ 稀疏向量 (BM25)                    │
      │                                      │
      └──────────────┬────────────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────────────┐
      │ 4. 写入 Milvus                       │
      │ collection: "link"                   │
      │ - dense_vector                         │
      │ - sparse_vector                       │
      │ - chunk_id, content, ...               │
      └──────────────┬────────────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────────────┐
      │ 5. LLM 图谱提取 (并发限4)            │
      │                                      │
      │ ┌────────────────────────────────────────┐ │
      │ │ 5.1 Entity Extraction            │ │
      │ │ - LLM: GPT-3.5/4 等            │ │
      │ │ - Prompt: entity_extraction.yaml  │ │
      │ │ - Input: 分片内容                │ │
      │ │ - Output: 节点列表              │ │
      │ └────────────────┬───────────────────────┘ │
      │                  │                   │
      │ ┌────────────────┴─────────────────────┐ │
      │ │ 5.2 Relation Extraction         │ │
      │ │ - LLM: GPT-3.5/4 等          │ │
      │ │ - Prompt: relation_extraction.yaml │ │
      │ │ - Input: 实体列表 + 分片内容   │ │
      │ │ - Output: 关系列表              │ │
      │ └────────────────┬───────────────────────┘ │
      │                  │                   │
      └──────────────────┴───────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────────────┐
      │ 6. 写入 Neo4j                       │
      │                                      │
      │ - 节点 + 关系                        │
      │ - strength: 5.0 (默认值)             │
      │ - weight: 5.0 (默认值)               │
      │ - combined_degree: 节点度数           │
      └──────────────┬────────────────────────────┘
                     │
                     ▼
      ┌──────────────────────────────────────────────────┐
      │ 7. 分片合并 (可选，后台任务)          │
      │                                      │
      │ - 合并节点 (去重 chunks)              │
      │ - 合并关系 (去重 chunk_ids)           │
      │ - 重新计算 PMI 权重 ★★★★            │
      │ - Weight = 1.0 + 9.0 × (PMI×0.6 + Strength×0.4) │
      │ - 更新 Neo4j                       │
      └──────────────────────────────────────────────────┘
```

---

## 关键配置

### 切片配置
```go
ChunkSize: 512 字符（默认）
ChunkOverlap: 100 字符
Separators: ["\n\n", "\n", "。", "！", "？", ".", "!"]
```

### Embedding 配置
- **模型**：通过 `embedder.Embedder` 接口配置
- **向量维度**：1536
- **并发数**：4

### LLM 配置
```go
// config/prompt_templates/entity_extraction.yaml
// config/prompt_templates/relation_extraction.yaml
```

### 并发控制
- **切片保存**：同步事务
- **Embedding**：4个并发
- **LLM 提取**：4个并发（semaphore限流）
- **分片合并**：后台异步任务

---

## 重要说明

### 1. 权重不是在切片时计算的
- 切片只是按字符和分隔符切分文档
- 权重计算在**分片合并阶段**，使用 PMI 算法

### 2. 初次写入的默认值
```cypher
SET r.strength = COALESCE(row.strength, 5.0)
SET r.weight = COALESCE(row.weight, 5.0)
```
如果 LLM 没有返回 strength/weight，会使用默认值 **5.0**

### 3. 合并后的权重会更高
- PMI 算法会根据实体共现频率重新计算
- 权重范围：1.0 + 9.0 × ([0,1] + [0,1]) = [1.0, 19.0]
- 典型值：7-15 左右

### 4. combined_degree 不等于权重
- `combined_degree` = 节点出度 + 入度
- 用于图谱可视化，不影响关系权重
- 前端返回为0说明查询有问题（标签不匹配）

---

## 优化建议

### 1. 添加前端传递权重
- 让用户在编辑关系时能设置 strength/weight
- 传递到 LLM 提取时作为示例

### 2. 调整默认值
- 考虑是否需要将默认值从 5.0 调整为其他值

### 3. PMI 参数调优
- 根据实际效果调整归一化范围
- 修改权重公式中 PMI 和 Strength 的系数（0.6 和 0.4）
