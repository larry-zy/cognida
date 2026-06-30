# Link 知识图谱系统架构文档

## 文档说明

本文档描述 Link 知识图谱系统的整体架构，重点关注**数据写入阶段**的流程设计、技术栈选型和实现细节。

---

## 一、系统概述

### 1.1 项目简介

Link 是一个基于 **RAG（检索增强生成）** 和 **知识图谱** 的智能知识管理系统，支持多模态文档处理、语义检索和图谱可视化。

### 1.2 核心能力

| 能力 | 描述 |
|------|------|
| **文档处理** | 支持上传、解析、分块、向量化处理 |
| **向量检索** | 基于 Milvus 的稠密向量语义检索 |
| **关键词检索** | 基于 BM25 的稀疏向量关键词检索 |
| **知识图谱** | 基于 Neo4j 的实体关系图谱 |
| **智能问答** | 结合检索和图谱的增强问答 |
| **多租户** | 完全的租户隔离机制 |

### 1.3 架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           系统架构概览                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                        前端层 (Vue 3)                             │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │   │
│  │  │ 知识库管理 │ │ 图谱可视化 │ │ 智能问答  │ │ 后台管理  │           │   │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓ ↑                                    │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                       API 层 (Gin)                                │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │   │
│  │  │ 认证中间件 │ │ 租户中间件 │ │ 路由处理  │ │ 响应处理  │           │   │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                       业务层 (Service)                            │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │   │
│  │  │ KnowledgeSvc │ │  GraphSvc   │ │  ChatSvc    │               │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘               │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │   │
│  │  │  Chunker    │ │ Embedder    │ │ Coordinator │               │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                       仓储层 (Repository)                         │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │   │
│  │  │   MySQL     │ │   Milvus    │ │   Neo4j     │               │   │
│  │  │  Repository │ │  Retriever  │ │  Repository │               │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                       存储层 (Storage)                             │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │   │
│  │  │   MySQL     │ │   Milvus    │ │   Neo4j     │               │   │
│  │  │ (结构化数据) │ │ (向量数据)   │ │ (图谱数据)   │               │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘               │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 二、技术栈

### 2.1 后端技术栈

| 类别 | 技术 | 版本 | 用途 |
|------|------|------|------|
| **语言** | Go | 1.21+ | 开发语言 |
| **框架** | Gin | - | HTTP 服务框架 |
| **ORM** | GORM | v1.31.1 | 数据库 ORM |
| **数据库** | MySQL | 8.0+ | 主数据库 |
| **向量库** | Milvus | v2.3+ | 向量检索 |
| **图数据库** | Neo4j | v5.0+ | 知识图谱 |
| **AI 框架** | CloudWeGo Eino | latest | 语义分块、嵌入 |
| **认证** | golang-jwt/jwt | v5.3.1 | JWT 认证 |
| **日志** | logrus | - | 日志记录 |
| **配置** | godotenv | - | 环境变量加载 |

### 2.2 前端技术栈

| 类别 | 技术 | 版本 | 用途 |
|------|------|------|------|
| **框架** | Vue | 3.4+ | 前端框架 |
| **语言** | TypeScript | 5.0+ | 类型系统 |
| **UI 组件** | Element Plus | latest | UI 库 |
| **状态管理** | Pinia | latest | 状态管理 |
| **路由** | Vue Router | v4.0+ | 路由管理 |
| **构建工具** | Vite | v5.0+ | 构建工具 |
| **图谱可视化** | vis-network | latest | 图谱展示 |
| **HTTP 客户端** | Axios | latest | 请求库 |

### 2.3 外部服务依赖

| 服务 | 用途 |
|------|------|
| **Embedding API** | 文本向量化（支持 DashScope、OpenAI 等） |
| **LLM API** | 实体提取、关系提取、智能问答 |

### 2.4 目录结构

```
link/
├── cmd/                        # 可执行程序入口
│   └── server/                 # 主服务器入口
│       └── main.go
├── internal/                   # 核心业务代码
│   ├── agent/                  # Agent 相关
│   ├── application/            # 应用层
│   │   ├── chunker/            # 文档分块器
│   │   │   ├── semantic.go     # 语义分块
│   │   │   └── simple.go       # 简单分块
│   │   └── repository/         # 仓储层
│   │       ├── milvus/         # Milvus 仓储
│   │       ├── neo4j/          # Neo4j 仓储
│   │       └── retriever/      # 检索器
│   ├── config/                 # 配置管理
│   │   └── prompt_templates/   # AI 提示词模板
│   ├── container/              # 依赖注入容器
│   ├── handler/                # HTTP 处理器
│   │   ├── knowledge.go        # 知识库处理
│   │   ├── knowledge_base.go   # 知识库管理
│   │   ├── chat.go             # 聊天处理
│   │   └── session.go          # 会话管理
│   ├── middleware/             # 中间件
│   ├── models/                 # 数据模型
│   ├── router/                 # 路由配置
│   └── types/                  # 类型定义
│       └── interfaces/         # 接口定义
├── config/                     # 配置文件
│   └── prompt_templates/       # 提示词模板
├── web/                        # 前端项目
│   ├── src/
│   │   ├── components/         # 组件
│   │   ├── views/              # 页面
│   │   ├── router/             # 路由
│   │   ├── stores/             # 状态管理
│   │   └── types/              # 类型定义
│   └── package.json
├── migrations/                 # 数据库迁移
├── uploads/                    # 文件上传目录
├── docs/                       # 文档
└── .env                        # 环境变量配置
```

---

## 三、数据写入流程

### 3.1 整体流程图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        数据写入整体流程                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  用户上传文件                                                             │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  1. 文件上传阶段 (Handler)                                        │   │
│  │     • 接收 multipart/form-data 请求                                │   │
│  │     • 保存文件到 uploads 目录                                       │   │
│  │     • 创建 Knowledge 记录 (status=pending)                          │   │
│  │     • 提交异步处理任务                                              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  2. 异步处理阶段 (TaskProcessor)                                  │   │
│  │     • Worker 从任务队列获取任务                                    │   │
│  │     • 检查状态，避免重复处理                                        │   │
│  │     • 更新状态为 processing                                        │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  3. 文档分块阶段 (Chunker)                                        │   │
│  │     • 读取文件内容                                                 │   │
│  │     • 使用语义分块器进行分块                                        │   │
│  │     • 生成 chunk_id，建立前后关系                                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  4. MySQL 写入阶段 (事务)                                         │   │
│  │     • 创建 Chunk 记录                                             │   │
│  │     • 关联 Knowledge、KB                                          │   │
│  │     • 提交事务                                                    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  5. 数据准备阶段 (并行)                                           │   │
│  │     ┌─────────────┐ ┌─────────────┐ ┌─────────────┐             │   │
│  │     │ 稠密向量生成 │ │ 稀疏向量生成 │ │ 图谱数据提取 │             │   │
│  │     │ (Embedding) │ │  (BM25)     │ │  (LLM)      │             │   │
│  │     └─────────────┘ └─────────────┘ └─────────────┘             │   │
│  │            ↓                ↓                ↓                    │   │
│  │     dense_vectors    sparse_vectors    graph_data                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  6. Milvus 写入阶段 (串行)                                        │   │
│  │     • 构建向量列                                                   │   │
│  │     • Insert 批量写入                                              │   │
│  │     • Flush 确保数据可搜索                                          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  7. Neo4j 写入阶段 (可选，串行)                                    │   │
│  │     • MERGE 节点                                                  │   │
│  │     • MERGE 关系                                                  │   │
│  │     • 设置属性和权重                                                │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  8. 完成阶段                                                       │   │
│  │     • 更新 Knowledge.status = completed                            │   │
│  │     • 更新 Knowledge.chunk_count                                   │   │
│  │     • 设置 processed_at 时间                                         │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       ↓                                                                 │
│  处理完成                                                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.2 详细流程说明

#### 阶段 1: 文件上传 (`internal/handler/knowledge.go`)

```go
// UploadKnowledgeFile 处理文件上传
func (h *KnowledgeHandlerFull) UploadKnowledgeFile(c *gin.Context) {
    // 1. 获取租户和用户信息（从中间件）
    tenantID := middleware.GetTenantID(c)
    userID, _ := middleware.GetUserID(c)

    // 2. 解析表单数据
    fileHeader, _ := c.FormFile("file")
    title := c.PostForm("title")
    fileType := c.PostForm("file_type")
    chunkSize := h.postFormIntDefault(c, "chunk_size", 512)
    chunkOverlap := h.postFormIntDefault(c, "chunk_overlap", 100)

    // 3. 保存文件到本地
    filename := uuid.New().String() + ext
    filePath := filepath.Join(h.uploadDir, filename)
    c.SaveUploadedFile(fileHeader, filePath)

    // 4. 创建 Knowledge 记录
    knowledge := &types.Knowledge{
        ID:          uuid.New().String(),
        TenantID:    tenantID,
        UserID:      userID,
        KBID:        kbID,
        Type:        fileType,
        Title:       title,
        Source:      "upload",
        FilePath:    filePath,
        StorageSize: fileHeader.Size,
        ParseStatus: "pending",  // 初始状态
        EnableStatus: "enabled",
    }

    // 5. 保存到数据库
    h.kbService.Create(c.Request.Context(), knowledge)

    // 6. 提交异步任务
    task := &Task{
        ID:   uuid.New().String(),
        Type: TaskTypeKnowledgeProcess,
        Data: &KnowledgeTaskData{
            KnowledgeID:  knowledge.ID,
            TenantID:     tenantID,
            ChunkSize:    chunkSize,
            ChunkOverlap: chunkOverlap,
        },
    }
    h.taskProcessor.Submit(task)

    return c.JSON(200, gin.H{
        "message":      "file uploaded successfully",
        "knowledge_id": knowledge.ID,
        "status":       "pending",
    })
}
```

#### 阶段 2: 异步任务处理 (`internal/handler/knowledge.go`)

```go
// TaskProcessor 任务处理器
type TaskProcessor struct {
    taskChan chan *Task
    workers  int
    handler  *KnowledgeHandlerFull
}

// Start 启动 Worker
func (tp *TaskProcessor) Start() {
    for i := 0; i < tp.workers; i++ {
        go tp.worker()
    }
}

// worker 处理任务
func (tp *TaskProcessor) worker() {
    for task := range tp.taskChan {
        switch task.Type {
        case TaskTypeKnowledgeProcess:
            data := task.Data.(*KnowledgeTaskData)
            tp.handler.ProcessKnowledgeTask(context.Background(),
                data.KnowledgeID, data.ChunkSize, data.ChunkOverlap)
        }
    }
}
```

#### 阶段 3: 文档分块 (`internal/handler/knowledge.go`)

```go
// 使用 SimpleChunker 进行分块
newChunker := chunker.NewSimpleChunker(&chunker.SimpleConfig{
    ChunkSize:     chunkSize,      // 默认 512
    Overlap:       chunkOverlap,    // 默认 100
    Separators:    []string{"\n\n", "\n", "。", "！", "？", ".", "!"},
    KeepSeparator: true,
})

chunks, err := newChunker.Split(ctx, string(content))
```

#### 阶段 4: MySQL 写入（事务）

```go
func (h *KnowledgeHandlerFull) saveChunksToDB(
    ctx context.Context,
    knowledge *types.Knowledge,
    chunks []string,
) ([]string, error) {

    chunkIDs := make([]string, len(chunks))
    db := h.kbService.GetDB()

    // 开启事务
    tx := db.Begin()
    if tx.Error != nil {
        return nil, tx.Error
    }

    for i, chunkContent := range chunks {
        chunkID := uuid.New().String()

        // 计算前置 chunk ID
        var preChunkID *string
        if i > 0 {
            preID := chunkIDs[i-1]
            preChunkID = &preID
        }

        tokenCount := len(chunkContent) / 2

        chunkRecord := &types.Chunk{
            ID:          chunkID,
            TenantID:    knowledge.TenantID,
            KBID:        knowledge.KBID,
            KnowledgeID: knowledge.ID,
            ChunkIndex:  i,
            Content:     chunkContent,
            IsEnabled:   true,
            StartAt:     0,
            EndAt:       len(chunkContent),
            PreChunkID:  preChunkID,
            ChunkType:   "text",
            TokenCount:  &tokenCount,
        }

        if err := tx.Create(chunkRecord).Error; err != nil {
            tx.Rollback()
            return nil, fmt.Errorf("failed to create chunk %d: %w", i, err)
        }
        chunkIDs[i] = chunkID
    }

    // 提交事务
    if err := tx.Commit().Error; err != nil {
        return nil, fmt.Errorf("failed to commit transaction: %w", err)
    }

    return chunkIDs, nil
}
```

#### 阶段 5: 并行数据准备 (`internal/handler/knowledge.go`)

```go
// 使用 errgroup 并行执行，限制 4 个并发
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(4)

var denseVectors [][]float32
var sparseVectors []entity.SparseEmbedding
var graphData []*types.GraphData
var vectorMutex sync.Mutex{}

// 稠密向量生成（必须）
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

// 稀疏向量生成（可选）
if bm25Enabled {
    g.Go(func() error {
        for i, chunkContent := range chunks {
            sparseVec, err := h.generateSparseVector(chunkContent)
            if err != nil {
                return fmt.Errorf("sparse vector generation failed: %w", err)
            }
            vectorMutex.Lock()
            sparseVectors[i] = sparseVec
            vectorMutex.Unlock()
        }
        return nil
    })
}

// 图谱数据提取（可选）
if graphEnabled {
    g.Go(func() error {
        inputs := make([]*service.ChunkExtractionInput, len(chunks))
        for i, chunk := range chunks {
            inputs[i] = &service.ChunkExtractionInput{
                KBID:     knowledge.KBID,
                ChunkID:  chunkIDs[i],
                Document: chunk,
                Query:    "请提取实体和关系",
            }
        }
        graph, err := h.graphService.ExtractGraphFromChunks(ctx, inputs)
        if err != nil {
            return fmt.Errorf("graph extraction failed: %w", err)
        }
        vectorMutex.Lock()
        graphData = graph
        vectorMutex.Unlock()
        return nil
    })
}

// 等待所有任务完成
if err := g.Wait(); err != nil {
    return fmt.Errorf("parallel build failed: %w", err)
}
```

#### 阶段 6: Milvus 写入 (`internal/handler/knowledge.go`)

```go
func (h *KnowledgeHandlerFull) insertToMilvus(
    ctx context.Context,
    knowledge *types.Knowledge,
    chunkIDs []string,
    chunks []string,
    denseVectors [][]float32,
    sparseVectors []entity.SparseEmbedding,
    bm25Enabled bool,
) error {

    collectionName := "link"

    // 使用独立的 context 避免被外部取消
    milvusCtx, milvusCancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer milvusCancel()

    // 构建列
    chunkIDColumn := entity.NewColumnVarChar("chunk_id", chunkIDs)
    knowledgeIDColumn := entity.NewColumnVarChar("knowledge_id",
        fillString(knowledge.ID, len(chunkIDs)))
    kbIDColumn := entity.NewColumnVarChar("kb_id",
        fillString(knowledge.KBID, len(chunkIDs)))
    tenantIDColumn := entity.NewColumnInt64("tenant_id",
        fillInt64(knowledge.TenantID, len(chunkIDs)))
    contentColumn := entity.NewColumnVarChar("content", chunks)
    denseVectorColumn := entity.NewColumnFloatVector("dense_vector",
        len(denseVectors[0]), denseVectors)

    columns := []entity.Column{
        chunkIDColumn,
        knowledgeIDColumn,
        kbIDColumn,
        tenantIDColumn,
        contentColumn,
        denseVectorColumn,
    }

    if bm25Enabled {
        sparseVectorColumn := entity.NewColumnSparseVectors("sparse_vector", sparseVectors)
        columns = append(columns, sparseVectorColumn)
    }

    // 插入数据
    _, err := h.milvusClient.Insert(milvusCtx, collectionName, "", columns...)
    if err != nil {
        return fmt.Errorf("failed to insert into Milvus: %w", err)
    }

    // 刷新以确保可搜索
    if err := h.milvusClient.Flush(ctx, collectionName, false); err != nil {
        log.Printf("[Milvus] Warning: flush failed: %v", err)
    }

    return nil
}
```

#### 阶段 7: Neo4j 写入 (`internal/handler/knowledge.go`)

```go
if graphEnabled && graphData[0] != nil {
    namespace := types.NameSpace{
        TenantID:  fmt.Sprintf("%d", knowledge.TenantID),
        KBID:      knowledge.KBID,
        Knowledge: knowledge.ID,
        Type:      knowledge.Type,
    }

    // 使用独立的 context 避免 HTTP 请求超时导致写入失败
    neo4jCtx, neo4jCancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer neo4jCancel()

    if err := h.graphService.AddGraph(neo4jCtx, namespace,
        []*types.GraphData{graphData[0]}); err != nil {
        log.Printf("[ParallelBuild] Warning: failed to save graph to Neo4j: %v", err)
        // 图谱写入失败不影响主流程
    }
}
```

---

## 四、数据模型设计

### 4.1 核心实体关系

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           数据模型 ER 图                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌──────────────┐                                                       │
│  │   Tenant     │ 1                                                    │
│  │  (租户)       │                                                      │
│  └──────┬───────┘                                                      │
│         │                                                               │
│         │ 1                                                           │
│         │                                                               │
│  ┌──────▼───────┐ 1       ┌──────────────┐ 1       ┌──────────────┐   │
│  │ KnowledgeBase│ ◄─────── │ Knowledge    │ 1       ┌──────────────┐   │
│  │  (知识库)     │ N        │  (知识条目)   │ ◄─────── │   Chunk     │   │
│  │              │          │              │ N        │  (文档分块)   │   │
│  │ - id         │          │ - id         │          │ - id         │   │
│  │ - tenant_id  │          │ - kb_id      │          │ - content    │   │
│  │ - name       │          │ - tenant_id  │          │ - chunk_index│   │
│  │ - desc       │          │ - title      │          │ - pre_chunk_id│   │
│  │              │          │ - file_path  │          │              │   │
│  └──────┬───────┘          └──────┬───────┘          └──────┬───────┘   │
│         │                          │                           │             │
│         │                          │                           │             │
│         │ 1                 N      │                           │             │
│  ┌──────▼───────┐       ┌─────────▼──────────┐        ┌──────────▼─────┐│
│  │  KBSetting   │       │    GraphNode       │        │  GraphRelation ││
│  │ (知识库设置)  │       │    (图谱节点)       │        │   (图谱关系)    ││
│  │              │       │                    │        │                ││
│  │ - graph_enabled│      │ - id               │        │ - id           ││
│  │ - bm25_enabled │      │ - name             │        │ - source       ││
│  │ - retrieval_setting│ │ - entity_type      │        │ - target       ││
│  │              │       │ - chunks []        │        │ - type         ││
│  └──────────────┘       │ - attributes []    │        │ - weight       ││
│                         └────────────────────┘        └────────────────││
│                                                             │         │
│                                                         N   │         │ N
│                                                             └────┬────┘
│                                                                  │     │
│                                                        ┌─────────▼──────▼─────┐
│                                                        │     Chunk []         │
│                                                        │  (关联的分块 ID)       │
│                                                        └──────────────────────┘
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 4.2 核心数据表

#### knowledge_bases（知识库表）

```sql
CREATE TABLE `knowledge_bases` (
  `id` VARCHAR(36) PRIMARY KEY,
  `tenant_id` BIGINT NOT NULL,
  `user_id` BIGINT,
  `name` VARCHAR(255) NOT NULL,
  `description` TEXT,
  `avatar` VARCHAR(512),
  `permission` VARCHAR(50) DEFAULT 'private',
  `chunk_count` INT DEFAULT 0,
  `knowledge_count` INT DEFAULT 0,
  `is_enabled` BOOLEAN DEFAULT TRUE,
  `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  INDEX `idx_tenant_id` (`tenant_id`),
  INDEX `idx_user_id` (`user_id`)
);
```

#### knowledges（知识条目表）

```sql
CREATE TABLE `knowledges` (
  `id` VARCHAR(36) PRIMARY KEY,
  `tenant_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `kb_id` VARCHAR(36) NOT NULL,
  `type` VARCHAR(50) DEFAULT 'document',
  `title` VARCHAR(512),
  `source` VARCHAR(50) DEFAULT 'upload',
  `file_path` VARCHAR(512),
  `storage_size` BIGINT,
  `parse_status` VARCHAR(50) DEFAULT 'pending',
  `enable_status` VARCHAR(50) DEFAULT 'enabled',
  `chunk_count` INT DEFAULT 0,
  `processed_at` DATETIME,
  `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  INDEX `idx_tenant_id` (`tenant_id`),
  INDEX `idx_kb_id` (`kb_id`),
  INDEX `idx_parse_status` (`parse_status`)
);
```

#### chunks（文档分块表）

```sql
CREATE TABLE `chunks` (
  `id` VARCHAR(36) PRIMARY KEY,
  `tenant_id` BIGINT NOT NULL,
  `tag_id` BIGINT,
  `kb_id` VARCHAR(36) NOT NULL,
  `knowledge_id` VARCHAR(36) NOT NULL,
  `chunk_index` INT NOT NULL,
  `content` TEXT NOT NULL,
  `is_enabled` BOOLEAN DEFAULT TRUE,
  `start_at` INT DEFAULT 0,
  `end_at` INT DEFAULT 0,
  `pre_chunk_id` VARCHAR(36),
  `chunk_type` VARCHAR(50) DEFAULT 'text',
  `token_count` INT,
  `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  INDEX `idx_tenant_id` (`tenant_id`),
  INDEX `idx_kb_id` (`kb_id`),
  INDEX `idx_knowledge_id` (`knowledge_id`),
  INDEX `idx_pre_chunk_id` (`pre_chunk_id`)
);
```

#### kb_settings（知识库设置表）

```sql
CREATE TABLE `kb_settings` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `kb_id` VARCHAR(36) NOT NULL UNIQUE,
  `graph_enabled` BOOLEAN DEFAULT FALSE,
  `bm25_enabled` BOOLEAN,
  `retrieval_setting` JSON,
  `chunker_setting` JSON,
  `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  INDEX `idx_kb_id` (`kb_id`)
);
```

---

## 五、存储层设计

### 5.1 MySQL - 结构化数据存储

**职责：**
- 存储元数据（知识库、知识条目、分块）
- 维护数据关联关系
- 支持事务和复杂查询
- 作为"真理来源"（Source of Truth）

**特点：**
- 使用 GORM ORM
- 支持软删除
- 多租户隔离（tenant_id）
- 外键关联

### 5.2 Milvus - 向量数据存储

**职责：**
- 存储稠密向量（语义检索）
- 存储稀疏向量（BM25 检索）
- 提供高效的向量相似度搜索

**Schema 设计：**

| 字段 | 类型 | 说明 |
|------|------|------|
| chunk_id | VARCHAR(36) | 对应 MySQL chunks.id |
| knowledge_id | VARCHAR(36) | 对应 MySQL knowledges.id |
| kb_id | VARCHAR(36) | 对应 MySQL knowledge_bases.id |
| tenant_id | INT64 | 租户隔离 |
| chunk_index | INT64 | 分块索引 |
| content | VARCHAR(65535) | 分块内容 |
| is_enabled | BOOL | 是否启用 |
| dense_vector | FLOAT_VECTOR | 稠密向量（语义检索） |
| sparse_vector | SPARSE_VECTOR | 稀疏向量（BM25） |

**检索配置：**
```go
type IndexType string

const (
    IndexTypeHnsw    IndexType = "HNSW"     // 高性能检索
    IndexTypeIvfFlat IndexType = "IVF_FLAT" // 平衡性能和精度
    IndexTypeFlat    IndexType = "FLAT"     // 精确检索
)
```

### 5.3 Neo4j - 图谱数据存储

**职责：**
- 存储实体（节点）
- 存储关系（边）
- 支持图谱查询和路径搜索

**节点标签设计：**
```
ENTITY:KB_{kb_id前8位}
```

**关系类型：**
```
RELATES_TO {id, kb_id}
```

**节点属性：**
```cypher
(n:ENTITY {
    id: "uuid",           // 节点唯一ID
    name: "实体名称",      // 实体名称
    title: "实体名称",     // 冗余字段
    entity_type: "类型",   // 实体类型
    kb_id: "知识库ID",     // 知识库关联
    knowledge_id: "知识ID", // 知识条目关联
    chunks: ["chunk_id"],  // 关联的分块
    attributes: []         // 其他属性
})
```

**关系属性：**
```cyprus
(r:RELATES_TO {
    id: "uuid",           // 关系唯一ID
    source: "源实体名",     // 源节点
    target: "目标实体名",   // 目标节点
    type: "关系类型",       // 关系类型
    description: "描述",    // 关系描述
    strength: 5.0,         // 强度 1-10
    weight: 5.0,           // 权重（计算得出）
    combined_degree: 4,    // 节点度数
    kb_id: "知识库ID",     // 知识库关联
    chunk_ids: []         // 关联的分块
})
```

---

## 六、设计理念

### 6.1 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                      表现层 (Presentation)                   │
│                      internal/handler/                       │
│                      处理 HTTP 请求/响应                        │
└─────────────────────────────────────────────────────────────┘
                            ↓ ↑
┌─────────────────────────────────────────────────────────────┐
│                      业务层 (Application)                    │
│                      internal/application/                    │
│                      核心业务逻辑、流程编排                    │
└─────────────────────────────────────────────────────────────┘
                            ↓ ↑
┌─────────────────────────────────────────────────────────────┐
│                      仓储层 (Repository)                     │
│                      internal/application/repository/           │
│                      数据访问抽象、持久化操作                   │
└─────────────────────────────────────────────────────────────┘
                            ↓ ↑
┌─────────────────────────────────────────────────────────────┐
│                      基础设施层 (Infrastructure)               │
│                      MySQL / Milvus / Neo4j                   │
│                      外部服务、数据存储                        │
└─────────────────────────────────────────────────────────────┘
```

### 6.2 设计原则

| 原则 | 说明 | 实现方式 |
|------|------|---------|
| **单一职责** | 每个组件只负责一个功能 | Service/Repository 分离 |
| **接口隔离** | 面向接口编程 | interfaces/ 目录定义接口 |
| **依赖注入** | 解耦组件依赖 | container/ 容器管理 |
| **多租户优先** | 租户数据完全隔离 | tenant_id 字段 + 中间件 |
| **异步优先** | 耗时操作异步处理 | TaskProcessor 任务队列 |
| **可观测性** | 日志记录关键操作 | log 统一记录 |
| **幂等性** | 操作可重复执行 | 状态检查 + 事务 |

### 6.3 并发模型

**并发数据准备：**
```
使用 errgroup + 信号量实现并发控制

┌─────────────────────────────────────────────┐
│              errgroup (limit=4)              │
├─────────────────────────────────────────────┤
│                                             │
│  goroutine 1:  denseVectors               │
│       ↓                                    │
│  goroutine 2:  sparseVectors (optional)    │
│       ↓                                    │
│  goroutine 3:  graphData (optional)        │
│       ↓                                    │
│  goroutine 4:  (预留)                      │
│                                             │
└─────────────────────────────────────────────┘
              ↓
         g.Wait() 等待全部完成
```

**串行数据写入：**
```
Milvus Insert (独立 context 5min)
    ↓
Neo4j AddGraph (独立 context 5min)
    ↓
更新状态
```

**为什么这样设计？**
1. **数据准备可以并行** - 各数据之间无依赖
2. **写入必须串行** - 需要所有数据准备完成后才能写入
3. **独立 context** - 避免 HTTP 超时导致写入中断

### 6.4 错误处理策略

| 阶段 | 错误处理 | 说明 |
|------|---------|------|
| 文件上传 | 返回 400/500 | 文件保存失败立即返回 |
| 分块处理 | 标记 failed | 状态标记 + 错误信息 |
| MySQL 写入 | 事务回滚 | 全部成功或全部失败 |
| 数据准备 | errgroup 取消 | 任一失败则全部取消 |
| Milvus 写入 | 返回错误 | MySQL 已提交，需手动清理 |
| Neo4j 写入 | 仅记录警告 | 不影响主流程 |

---

## 七、数据一致性

### 7.1 当前一致性状态

| 存储组合 | 一致性保证 | 说明 |
|---------|-----------|------|
| MySQL + Milvus | ⚠️ 无分布式事务 | Milvus 失败时 MySQL 已提交 |
| MySQL + Neo4j | ⚠️ 最终一致 | Neo4j 失败仅记录警告 |
| Milvus + Neo4j | ❌ 无关联 | 两者独立写入 |

### 7.2 数据一致性解决方案

详见 [`docs/saga_implementation.md`](./saga_implementation.md)

**核心思路：**
1. **Saga 模式** - 补偿事务机制
2. **状态记录** - 记录每步执行状态
3. **自动重试** - 失败自动重试（最多3次）
4. **人工介入** - 超过重试次数需人工处理

---

## 八、配置系统

### 8.1 配置文件结构

```go
type Config struct {
    Database   *DatabaseConfig
    Milvus     *MilvusConfig
    Neo4j      *Neo4jConfig
    JWT        *JWTConfig
    Tenant     *TenantConfig
    Chat       *ChatConfig
    Search     *SearchConfig
    Embedding  *EmbeddingConfig
    Server     *ServerConfig
}
```

### 8.2 关键配置项

```yaml
# 数据库配置
database:
  host: localhost
  port: 3306
  user: root
  password: ""
  database: link_db

# Milvus 配置
milvus:
  address: localhost:19530
  collection_name: link
  dimension: 1024

# Neo4j 配置
neo4j:
  uri: bolt://localhost:7687
  username: neo4j
  password: ""
  database: neo4j

# Embedding 配置
embedding:
  provider: dashscope
  model: text-embedding-v2
  api_key: ""
  dimension: 1024

# 检索配置
search:
  top_k: 10
  score_threshold: 0.7
  rerank_enabled: true
```

---

## 九、API 接口设计

### 9.1 知识库管理 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/knowledge-bases | 创建知识库 |
| GET | /api/v1/knowledge-bases | 查询知识库列表 |
| GET | /api/v1/knowledge-bases/:id | 获取知识库详情 |
| PUT | /api/v1/knowledge-bases/:id | 更新知识库 |
| DELETE | /api/v1/knowledge-bases/:id | 删除知识库 |
| GET | /api/v1/knowledge-bases/:id/stats | 获取统计信息 |

### 9.2 知识条目 API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/v1/knowledge-bases/:id/knowledge/file | 上传文件 |
| GET | /api/v1/knowledge-bases/:id/knowledge | 查询知识条目列表 |
| GET | /api/v1/knowledge-bases/:id/knowledge/:kid | 获取知识条目详情 |
| DELETE | /api/v1/knowledge-bases/:id/knowledge/:kid | 删除知识条目 |
| GET | /api/v1/knowledge-bases/:id/knowledge/:kid/status | 获取处理状态 |

### 9.3 图谱 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v1/knowledge-bases/:id/graph | 获取完整图谱 |
| POST | /api/v1/knowledge-bases/:id/graph/node | 添加节点 |
| PUT | /api/v1/knowledge-bases/:id/graph/node/:nid | 更新节点 |
| DELETE | /api/v1/knowledge-bases/:id/graph/node/:nid | 删除节点 |
| POST | /api/v1/knowledge-bases/:id/graph/relation | 添加关系 |
| PUT | /api/v1/knowledge-bases/:id/graph/relation/:rid | 更新关系 |
| DELETE | /api/v1/knowledge-bases/:id/graph/relation/:rid | 删除关系 |

---

## 十、监控和运维

### 10.1 状态监控

**知识条目处理状态：**
| 状态 | 说明 |
|------|------|
| `pending` | 待处理 |
| `processing` | 处理中 |
| `completed` | 已完成 |
| `failed` | 失败 |

**监控 SQL：**
```sql
-- 查看处理中的任务
SELECT kb_id, COUNT(*) as count
FROM knowledges
WHERE parse_status = 'processing'
GROUP BY kb_id;

-- 查看失败的任务
SELECT id, kb_id, title, parse_status
FROM knowledges
WHERE parse_status = 'failed'
ORDER BY created_at DESC;
```

### 10.2 数据清理

**清理过期数据：**
```sql
-- 删除已完成且超过 30 天的 Saga 记录
DELETE FROM knowledge_sagas
WHERE status = 'completed'
AND completed_at < DATE_SUB(NOW(), INTERVAL 30 DAY);
```

---

## 十一、总结

### 11.1 架构特点

| 特点 | 说明 |
|------|------|
| **前后端分离** | Vue 3 + Gin，RESTful API |
| **多模态存储** | MySQL（元数据）+ Milvus（向量）+ Neo4j（图谱） |
| **异步处理** | 耗时操作异步化，避免阻塞 |
| **多租户** | 完全的租户数据隔离 |
| **可扩展** | 分层架构，易于扩展新功能 |

### 11.2 技术亮点

1. **混合检索** - 向量检索 + BM25 + 图谱检索
2. **语义分块** - 基于语义的智能文档分块
3. **图谱构建** - LLM 自动提取实体关系
4. **可视化** - 前端图谱可视化展示
5. **灵活配置** - 支持多种嵌入模型和检索策略

### 11.3 后续优化方向

1. **数据一致性** - 实现 Saga 模式（见 saga_implementation.md）
2. **性能优化** - 批量写入优化、并发控制优化
3. **可观测性** - Prometheus 指标、链路追踪
4. **高可用** - 服务降级、熔断机制
5. **安全加固** - 数据加密、访问控制增强

---

*文档版本: 1.0*
*更新日期: 2025-02-15*
