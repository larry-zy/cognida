# GraphRAG 社区检测实现方案

## 一、概述

本文档描述了 GraphRAG（Graph-based Retrieval Augmented Generation）中社区检测（Community Detection）功能的实现方案。社区检测是 GraphRAG 的核心能力之一，能够将知识图谱中的实体组织成有意义的社区结构，从而实现全局和局部两种视角的知识检索。

### 1.1 核心价值

- **全局视角**：通过社区摘要回答宏观问题（如"公司的产品线有哪些？"）
- **局部视角**：通过社区内实体关系回答具体问题（如"张三负责什么项目？"）
- **层级结构**：支持多级社区粒度，适应不同抽象层次的问题
- **语义理解**：LLM 生成的社区摘要提供更高层次的语义表达

### 1.2 技术选型

| 组件 | 技术方案 |
|-----|---------|
| 社区检测算法 | Leiden 算法（Neo4j GDS 库） |
| 社区摘要生成 | LLM（GPT-4/Claude 等） |
| 存储模型 | Neo4j Community 节点 + BELONGS_TO 关系 |
| 检索策略 | Global/Local/Hybrid 三种模式 |

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        GraphRAG Pipeline                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐          │
│  │ 1. 图构建    │    │ 2. 社区检测  │    │ 3. 社区摘要  │          │
│  │              │───▶│              │───▶│              │          │
│  │ Neo4j 导出   │    │ Leiden 算法  │    │ LLM 生成    │          │
│  └──────────────┘    └──────────────┘    └──────────────┘          │
│         │                    │                    │                 │
│         ▼                    ▼                    ▼                 │
│   GraphData         CommunityHierarchy      CommunitySummary        │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ 4. 分层检索                                                    │   │
│  │   • Global Level: 社区摘要作为上下文                            │   │
│  │   • Local Level: 社区内实体 + 关系                             │   │
│  │   • Hybrid Level: 融合全局和局部信息                            │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 三、数据模型设计

### 3.1 Neo4j Schema

#### Community 节点

```cypher
(:Community {
    id: string,              // 社区唯一标识
    level: integer,          // 层级级别 (0=最细粒度)
    title: string,           // 社区标题/主题
    description: string,     // 社区描述
    summary: string,         // LLM 生成的摘要
    node_count: integer,     // 社区内节点数量
    edge_count: integer,     // 社区内边数量
    density: float,          // 社区密度
    parent_id: string,       // 父社区ID
    hierarchy_path: string,  // 层级路径 "c100/c10/c1"
    created_at: datetime,    // 创建时间
    updated_at: datetime     // 更新时间
})
```

#### Entity 节点扩展

```cypher
(:Entity {
    // 原有属性
    id: string,
    name: string,
    entity_type: string,
    attributes: list,
    chunks: list,

    // 新增属性（用于快速查询）
    community_id: string,     // 所属社区ID (level 0)
    community_path: string    // 层级路径 "c100/c10/c1"
})
```

#### 关系定义

```cypher
// Entity -> Community (归属关系)
(:Entity)-[:BELONGS_TO {
    level: integer,
    membership_strength: float
}]->(:Community)

// Community -> Entity (反向查询)
(:Community)-[:HAS_MEMBER]->(:Entity)

// Community -> Community (层级关系)
(:Community {level: 1})-[:HAS_CHILD {
    child_count: integer
}]->(:Community {level: 0})
```

### 3.2 Go 数据结构

```go
// Community 社区结构
type Community struct {
    ID          string    `json:"id"`
    Level       int       `json:"level"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Summary     string    `json:"summary"`

    // 统计信息
    NodeCount   int     `json:"node_count"`
    EdgeCount   int     `json:"edge_count"`
    Density     float64 `json:"density"`

    // 层级信息
    ParentID      *string `json:"parent_id,omitempty"`
    HierarchyPath string  `json:"hierarchy_path"`

    // 时间戳
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// CommunityHierarchy 社区层级结构
type CommunityHierarchy struct {
    Communities   []*Community `json:"communities"`
    Levels        int          `json:"levels"`
    MaxLevel      int          `json:"max_level"`
    RawModularity float64      `json:"raw_modularity"`
}

// GlobalCommunityContext 全局社区上下文
type GlobalCommunityContext struct {
    CommunitySummaries []string `json:"community_summaries"`
    TotalCommunities   int      `json:"total_communities"`
    KeyEntities        []string `json:"key_entities"`
}

// LocalCommunityContext 局部社区上下文
type LocalCommunityContext struct {
    CommunityID  string           `json:"community_id"`
    Summary      string           `json:"summary"`
    Nodes        []*GraphNode     `json:"nodes"`
    Relations    []*GraphRelation `json:"relations"`
    RelatedNodes []*GraphNode     `json:"related_nodes,omitempty"`
}

// CommunityBasedQueryResult 基于社区的查询结果
type CommunityBasedQueryResult struct {
    GlobalContext  *GlobalCommunityContext `json:"global_context,omitempty"`
    LocalContext   *LocalCommunityContext  `json:"local_context,omitempty"`
    RelevantChunks []string                `json:"relevant_chunks"`
}

// CommunityDetectionConfig 社区检测配置
type CommunityDetectionConfig struct {
    Resolution        float64 `json:"resolution"`         // 分辨率参数 (1.0)
    Iterations        int     `json:"iterations"`         // 最大迭代次数 (10)
    MinCommunitySize  int     `json:"min_community_size"` // 最小社区大小 (3)
    MaxLevel          int     `json:"max_level"`          // 最大层级深度 (5)
    RandomSeed        int64   `json:"random_seed"`        // 随机种子 (42)
}
```

---

## 四、Leiden 社区检测算法

### 4.1 算法原理

Leiden 算法是 Louvain 算法的改进版，保证社区是 well-connected 的：

```
┌─────────────────────────────────────────────────────────┐
│                    Leiden Algorithm                     │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  1. 移动阶段 (Move Phase)                               │
│     对每个节点计算移动到邻居社区的模度增益，如果增益 > 0 │
│     则移动到该社区，重复直到无法移动                     │
│                                                          │
│  2. 细化阶段 (Refinement Phase)                         │
│     确保每个社区是 "well-connected" 的                   │
│     即：没有节点移出会使社区不连通                       │
│                                                          │
│  3. 聚合阶段 (Aggregation Phase)                        │
│     将每个社区聚合为单个节点，社区间权重变为节点间权重   │
│                                                          │
│  4. 检查收敛                                            │
│     如果未达到收敛且未超过最大层级，返回步骤1            │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### 4.2 模度计算公式

```
Q = 1/(2m) * Σ[Aij - (ki*kj)/(2m)] * δ(ci, cj)

其中:
- m = 边权重总和
- Aij = 节点i和j之间的边权重
- ki = 节点i的度数
- ci, cj = 节点i和j所属社区
- δ(ci, cj) = 1 如果 ci == cj，否则为 0
```

### 4.3 Neo4j GDS 实现

```cypher
// 1. 投影图谱
CALL gds.graph.project(
    'entityGraph',
    ['Entity', 'Concept'],
    {
        MENTIONED_IN: {
            orientation: 'UNDIRECTED',
            properties: ['weight']
        },
        RELATES_TO: {
            orientation: 'UNDIRECTED',
            properties: ['weight']
        }
    }
)

// 2. 运行 Leiden 算法
CALL gds.leiden.write('entityGraph', {
    writeProperty: 'community',
    includeIntermediateCommunities: true,
    maxLevels: 5,
    gamma: 1.0,
    theta: 0.01,
    tolerance: 0.0001,
    randomSeed: 42
})
YIELD communities, modularity, modularities, levels
RETURN communities, modularity, modularities, levels

// 3. 获取社区统计
CALL gds.leiden.stats('entityGraph')

// 4. 清理投影图谱
CALL gds.graph.drop('entityGraph')
```

### 4.4 Go 实现接口

```go
type CommunityDetector interface {
    // Detect 检测社区
    Detect(ctx context.Context, namespace NameSpace, config CommunityDetectionConfig) (*CommunityHierarchy, error)

    // DetectWithLevel 检测指定层级的社区
    DetectWithLevel(ctx context.Context, namespace NameSpace, level int, config CommunityDetectionConfig) ([]*Community, error)

    // GetNodeCommunity 获取节点所属社区
    GetNodeCommunity(ctx context.Context, namespace NameSpace, nodeID string) (*Community, error)

    // GetCommunityByLevel 获取指定层级的所有社区
    GetCommunityByLevel(ctx context.Context, namespace NameSpace, level int) ([]*Community, error)
}
```

---

## 五、社区摘要生成

### 5.1 Prompt 模板

```yaml
system_prompt: |
  你是一个知识图谱分析专家。请根据给定的实体和关系，生成社区摘要。

  社区摘要应该包含:
  1. 主题描述: 这个社区主要讨论什么主题?
  2. 关键实体: 哪些实体是核心?
  3. 关键关系: 实体之间有什么重要关联?
  4. 应用场景: 这些信息可能用于回答什么类型的问题?

user_prompt: |
  请分析以下知识图谱社区:

  ## 实体列表
  {{range .Nodes}}
  - {{.Name}} ({{.EntityType}})
  {{end}}

  ## 关系列表
  {{range .Relations}}
  - {{.Source}} --{{.Type}}(权重:{{.Weight}})--> {{.Target}}
  {{end}}

  请生成:
  1. 社区标题 (简短, 10字以内)
  2. 社区描述 (1-2句话)
  3. 详细摘要 (100-200字)
  4. 关键主题标签 (3-5个)
```

### 5.2 摘要生成服务

```go
type CommunitySummarizer struct {
    llm   LLMClient
    neo4j Neo4jRepository
    cache *redis.Client
}

// GenerateSummary 生成单个社区摘要
func (c *CommunitySummarizer) GenerateSummary(ctx context.Context, communityID string, level int) (*CommunitySummary, error)

// GenerateAllSummaries 批量生成所有社区摘要
func (c *CommunitySummarizer) GenerateAllSummaries(ctx context.Context, namespace NameSpace) error
```

---

## 六、基于社区的检索策略

### 6.1 检索流程

```
┌─────────────────────────────────────────────────────────────┐
│                     Query Processing                        │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                   Query Classification                      │
│   ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  │
│   │ Global        │  │ Local         │  │ Hybrid        │  │
│   │ "公司的产品线?"│  │ "张三负责什么?"│  │ "产品A和谁相关?"││
│   └───────┬───────┘  └───────┬───────┘  └───────┬───────┘  │
│           │                  │                  │           │
│           ▼                  ▼                  ▼           │
│   Community Summary    Entity + Relation     Both + Fusion │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                    Context Building                         │
│   • Global: 社区摘要列表                                     │
│   • Local: 相关实体+关系                                     │
│   • chunks: 相关文档块                                       │
└─────────────────────────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                       LLM Generation                         │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 检索模式

#### Global 检索

```go
// 全局检索: 使用社区摘要回答宏观问题
func (r *GraphRAGRetriever) GlobalRetrieve(ctx context.Context, query string, namespace NameSpace) (*GlobalContext, error) {
    // 1. 对查询进行 embedding
    queryEmb := r.llm.Embed(query)

    // 2. 检索相关社区摘要
    communities, _ := r.neo4j.GetAllCommunities(ctx, namespace)

    // 3. 计算相似度
    for _, comm := range communities {
        summaryEmb := r.llm.Embed(comm.Summary)
        score := cosineSimilarity(queryEmb, summaryEmb)
        // ...
    }

    // 4. 取 Top-K 社区构建全局上下文
    return &GlobalContext{Summaries: summaries}, nil
}
```

#### Local 检索

```go
// 局部检索: 在相关社区内检索实体和关系
func (r *GraphRAGRetriever) LocalRetrieve(ctx context.Context, query string, namespace NameSpace) (*LocalContext, error) {
    // 1. 先进行全局检索，确定相关社区
    global, _ := r.GlobalRetrieve(ctx, query, namespace)

    // 2. 提取查询中的实体提及
    entities := r.llm.ExtractEntities(query)

    // 3. 匹配包含这些实体的社区
    targetCommunity := r.matchCommunity(global, entities)

    // 4. 获取社区内的详细图结构
    nodes, relations := r.neo4j.GetCommunityGraph(ctx, targetCommunity.ID)

    return &LocalContext{
        CommunityID: targetCommunity.ID,
        Summary:     targetCommunity.Summary,
        Nodes:       nodes,
        Relations:   relations,
    }, nil
}
```

#### Hybrid 检索

```go
// 混合检索: 融合全局和局部信息
func (r *GraphRAGRetriever) HybridRetrieve(ctx context.Context, query string, namespace NameSpace) (*CommunityBasedResult, error) {
    // 并行执行全局和局部检索
    global, _ := r.GlobalRetrieve(ctx, query, namespace)
    local, _ := r.LocalRetrieve(ctx, query, namespace)

    return &CommunityBasedResult{
        GlobalContext: global,
        LocalContext:  local,
    }, nil
}
```

### 6.3 Neo4j 查询示例

```cypher
// 获取某个层级的所有社区
MATCH (c:Community {level: $level})
WHERE c.namespace = $namespace
RETURN c ORDER BY c.node_count DESC

// 获取社区的所有成员
MATCH (c:Community {id: $community_id})-[:HAS_MEMBER]->(e:Entity)
RETURN e

// 获取社区的子树
MATCH path = (c:Community {id: $community_id})-[:HAS_CHILD*0..3]->(descendant:Community)
RETURN path

// 获取节点的完整社区路径
MATCH (e:Entity {id: $entity_id})-[:BELONGS_TO*]->(c:Community)
RETURN c ORDER BY c.level
```

---

## 七、API 设计

### 7.1 社区检测 API

```http
# 触发社区检测
POST /api/kb/{kb_id}/community/detect
Content-Type: application/json

{
    "resolution": 1.0,
    "max_level": 5,
    "min_community_size": 3,
    "random_seed": 42
}

Response:
{
    "hierarchy_id": "ch_123",
    "status": "processing",
    "total_communities": 0,
    "levels": 0
}
```

### 7.2 社区查询 API

```http
# 获取社区列表
GET /api/kb/{kb_id}/communities?level=0&page=1&page_size=20

Response:
{
    "communities": [
        {
            "id": "c1",
            "level": 0,
            "title": "产品研发团队",
            "summary": "...",
            "node_count": 15
        }
    ],
    "total": 42
}

# 获取社区详情
GET /api/kb/{kb_id}/community/{community_id}

Response:
{
    "id": "c1",
    "level": 0,
    "title": "产品研发团队",
    "summary": "...",
    "nodes": [...],
    "relations": [...],
    "parent_id": "c10",
    "children_ids": ["c100", "c101"]
}
```

### 7.3 社区摘要 API

```http
# 重新生成单个社区摘要
POST /api/kb/{kb_id}/community/{community_id}/summary

Response:
{
    "id": "c1",
    "title": "产品研发团队",
    "description": "...",
    "summary": "...",
    "key_entities": ["张三", "产品A"],
    "key_themes": ["研发", "产品"]
}

# 批量生成摘要
POST /api/kb/{kb_id}/community/summary/generate

Response:
{
    "status": "processing",
    "total": 42,
    "completed": 0
}
```

### 7.4 基于社区的检索 API

```http
# 社区检索
POST /api/kb/{kb_id}/retrieve/community
Content-Type: application/json

{
    "query": "公司的核心产品有哪些?",
    "mode": "global"   // "local" | "hybrid"
}

Response:
{
    "global_context": {
        "community_summaries": [
            "社区1: 产品研发团队...",
            "社区2: 市场营销团队..."
        ],
        "key_entities": [...]
    },
    "local_context": {
        "community_id": "c1",
        "summary": "...",
        "nodes": [...],
        "relations": [...]
    }
}
```

---

## 八、数据库迁移脚本

### 8.1 约束和索引

```cypher
// 1. 创建 Community 节点约束
CREATE CONSTRAINT community_id_unique IF NOT EXISTS
FOR (c:Community) REQUIRE c.id IS UNIQUE;

// 2. 创建索引
CREATE INDEX community_level_idx IF NOT EXISTS
FOR (c:Community) ON (c.level);

CREATE INDEX community_namespace_idx IF NOT EXISTS
FOR (c:Community) ON (c.namespace);

CREATE INDEX entity_community_idx IF NOT EXISTS
FOR (e:Entity) ON (e.community_id);

// 3. 为 Entity 添加社区属性
MATCH (e:Entity)
WHERE e.community_id IS NULL
SET e.community_id = null, e.community_path = null;
```

### 8.2 初始化脚本

```cypher
// 为现有 Entity 添加命名空间（如果需要）
MATCH (e:Entity)
WHERE e.namespace IS NULL
SET e.namespace = "default";

// 验证图谱结构
MATCH (e:Entity)
RETURN count(e) AS entity_count,
       count(DISTINCT e.entity_type) AS type_count;
```

---

## 九、实现优先级

| 阶段 | 任务 | 预计工作量 |
|-----|------|----------|
| **Phase 1** | 数据库 Schema 迁移 | 0.5天 |
| **Phase 2** | 使用 Neo4j GDS 实现社区检测 | 2-3天 |
| **Phase 3** | 社区数据存储和查询 API | 1-2天 |
| **Phase 4** | LLM 社区摘要生成 | 2-3天 |
| **Phase 5** | 全局/局部检索实现 | 3-4天 |
| **Phase 6** | 前端可视化（社区层级展示） | 3-5天 |
| **Phase 7** | 优化和测试 | 2-3天 |

**总计**: 约 14-21 天

---

## 十、参考资源

- [Microsoft GraphRAG](https://github.com/microsoft/graphrag)
- [Neo4j GDS Library - Leiden Algorithm](https://neo4j.com/docs/graph-data-science/current/algorithms/leiden/)
- [From Louvain to Leiden: guaranteeing well-connected communities](https://www.nature.com/articles/s41598-019-41695-z)
- [GraphRAG: Unlocking LLM Discovery on Narrative Knowledge](https://arxiv.org/abs/2404.16130)
