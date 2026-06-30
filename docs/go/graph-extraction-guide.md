# 图谱提取文档

## 概述

图谱提取是从文档中自动识别实体和关系，构建知识图谱的过程。本项目支持三种提取模式：

1. **顺序提取**：先提取实体，再提取关系（兼容模式）
2. **联合提取**：单次 LLM 调用同时提取实体和关系（推荐）
3. **增量提取**：基于已有图谱上下文的增量更新

## 提取模式对比

### 顺序提取 (Sequential)

**特点**：
- 两次独立的 LLM 调用
- 实体和关系分开提取
- 适合小规模文档

**优点**：
- 实现简单
- 易于调试
- 结果可控

**缺点**：
- LLM 调用次数多
- 实体关系可能不一致
- 成本较高

**使用方法**：
```go
// 应用层编排
service := graph.NewGraphService(graphRepo, llmChat)

inputs := []*knowledge.ChunkExtractionInput{
    {
        ChunkID:  "chunk-1",
        Document: "文档内容...",
        KBID:     "kb-1",
    },
}

graph, err := service.ExtractGraphFromChunks(ctx, inputs)
```

### 联合提取 (Joint)

**特点**：
- 单次 LLM 调用
- 同时提取实体和关系
- 提高一致性

**优点**：
- LLM 调用次数少
- 实体关系一致性好
- 成本更低

**缺点**：
- 响应可能较长
- 需要更大上下文窗口

**使用方法**：
```go
service := graph.NewGraphService(graphRepo, llmChat)

inputs := []*knowledge.ChunkExtractionInput{
    {
        ChunkID:  "chunk-1",
        Document: "文档内容...",
        KBID:     "kb-1",
    },
}

// 使用联合提取
graph, err := service.ExtractGraphJoint(ctx, inputs)
```

### 增量提取 (Incremental)

**特点**：
- 注入已有图谱上下文
- 只提取新实体和关系
- 更新已有实体属性

**优点**：
- 提取质量更高
- 避免重复实体
- 保持图谱一致性

**缺点**：
- 需要先获取上下文
- 复杂度较高

**使用方法**：
```go
service := graph.NewGraphService(graphRepo, llmChat)

inputs := []*knowledge.ChunkExtractionInput{
    {
        ChunkID:  "chunk-1",
        Document: "新文档内容...",
        KBID:     "kb-1",
    },
}

// 使用增量提取（自动获取已有上下文）
graph, err := service.ExtractGraphIncremental(ctx, inputs)
```

## 实体类型定义

系统支持 11 种标准实体类型：

| 类型 | 说明 | 示例 |
|------|------|------|
| Person | 人员、角色、职位 | 张三、CEO、开发者 |
| Organization | 公司、机构、组织 | Google、清华大学 |
| Product | 产品、服务、工具 | iPhone、MySQL |
| Technology | 编程语言、框架、算法 | Python、React、BERT |
| Concept | 概念、方法论、理论 | 机器学习、敏捷开发 |
| Document | 文档、规范、标准 | RFC 3986、API 文档 |
| Project | 项目、代码库 | Kubernetes、TensorFlow |
| Location | 地址、位置、路径 | /usr/bin、上海 |
| Event | 事件、会议、发布 | K8s 1.20 发布 |
| Time | 日期、时间、版本 | 2024年、v1.0 |
| Other | 其他重要名词 | 无法归类的实体 |

## 关系类型定义

系统支持 11 种标准关系类型：

| 类型 | 说明 | 示例 |
|------|------|------|
| CONTAINS | 包含 | 书籍包含章节 |
| RELATED_TO | 相关 | 两个技术相关 |
| DEPENDS_ON | 依赖 | 应用依赖数据库 |
| PART_OF | 组成 | 章节是书的一部分 |
| SIMILAR_TO | 相似 | CNN 与 RNN 相似 |
| CAUSES | 导致 | 错误导致崩溃 |
| LOCATED_IN | 位于 | 文件位于目录 |
| BELONGS_TO | 属于 | 员工属于公司 |
| CONNECTED_TO | 连接 | 前端连接后端 |
| PRECEDES | 先于 | 设计先于开发 |
| FOLLOWS | 后于 | 测试后于开发 |

## 提取配置

### LLM 配置

```go
// 基础 LLM 配置
llmChat := rag_infra.NewLLMChatAdapter(nil)

// 或使用现有的 LLM 服务
llmChat := someExistingLLMChat
```

### 提取器配置

```go
extractor := graph.NewLLMExtractor(llmChat)

// 联合提取
nodes, relations, err := extractor.ExtractGraphJoint(
    ctx,
    chunkID,
    document,
    existingContext, // 可选：已有图谱上下文
)

// 增量提取
nodes, relations, err := extractor.ExtractGraphIncremental(
    ctx,
    chunkID,
    document,
    existingContext, // 必需：已有图谱上下文
)
```

## 提取质量控制

### 实体验证

```go
// 自动验证实体类型
validTypes := map[string]bool{
    "Person": true,
    "Organization": true,
    // ... 其他类型
}

if !validTypes[node.EntityType] {
    node.EntityType = "Other" // 默认为 Other
}
```

### 关系验证

```go
// 自动验证关系类型
validRelations := map[string]bool{
    "CONTAINS": true,
    "RELATED_TO": true,
    // ... 其他类型
}

if !validRelations[relation.Type] {
    relation.Type = "RELATED_TO" // 默认为 RELATED_TO
}

// 验证关系的 source 和 target 存在
if !entitySet[relation.Source] || !entitySet[relation.Target] {
    // 跳过无效关系
    continue
}
```

## 提示词模板

### 模板位置

```
internal/infrastructure/config/prompt_templates/
├── entity_extraction.yaml          # 实体提取（独立）
├── entity_extraction_query.yaml    # 实体提取（查询模式）
├── relation_extraction.yaml        # 关系提取（独立）
├── relation_extraction_query.yaml  # 关系提取（查询模式）
├── graph_extraction.yaml           # 联合提取（推荐）
└── graph_incremental.yaml          # 增量提取
```

### 自定义模板

如需自定义提取模板，可以修改上述 YAML 文件：

```yaml
templates:
  - id: "graph_extraction"
    name: "图谱联合提取"
    description: "自定义提取模板"
    content: |
      # 在这里自定义你的提示词
      你的自定义提示词...
```

## 提取流程

### 完整提取流程

```
文档分块
    ↓
批量并发提取（可选择模式）
    ↓
提取结果合并（领域服务）
    ↓
图谱数据存储（Neo4j）
```

### 并发控制

默认并发数为 4，可在应用层调整：

```go
// 在 GraphService.concurrentExtract 中
semaphore := make(chan struct{}, 4) // 并发数
```

## 性能优化

### 1. 使用联合提取

减少 LLM 调用次数，提高一致性。

### 2. 启用增量提取

基于已有上下文，提高提取质量。

### 3. 批量处理

合并多个文档块进行批量提取。

### 4. 缓存已有实体

避免重复提取已知实体。

## 错误处理

### LLM 调用失败

```go
nodes, relations, err := extractor.ExtractGraphJoint(ctx, chunkID, document, nil)
if err != nil {
    log.Printf("Extraction failed for chunk %s: %v", chunkID, err)
    // 返回空结果或重试
    return &ExtractedGraphData{
        ChunkID: chunkID,
        Error:   err,
    }
}
```

### JSON 解析失败

```go
// 自动清理 LLM 响应
cleaned := cleanJSONResponse(response.Content)

// 解析 JSON
var result struct {
    Nodes     []*GraphNode    `json:"nodes"`
    Relations []*GraphRelation `json:"relations"`
}
if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
    return nil, fmt.Errorf("failed to parse: %w", err)
}
```

## 监控指标

建议监控以下指标：

1. **提取成功率**：成功提取的文档块比例
2. **实体数量**：平均每个文档块提取的实体数
3. **关系数量**：平均每个文档块提取的关系数
4. **LLM 调用次数**：各模式的调用次数
5. **平均响应时间**：LLM 响应时间
6. **错误率**：提取失败的比例

## 最佳实践

1. **首次提取**：使用联合提取快速构建初始图谱
2. **后续更新**：使用增量提取保持图谱更新
3. **查询模式**：使用查询模式提示词聚焦特定领域
4. **质量验证**：启用实体验证和关系验证
5. **错误重试**：对失败的提取进行重试
6. **并发控制**：根据 LLM 限流调整并发数

## 参考资料

- [Neo4j 索引文档](./neo4j-index-guide.md)
- [仓储迁移指南](./graph-repository-migration-guide.md)
- [Domain 层设计](../internal/domain/CLAUDE.md)
