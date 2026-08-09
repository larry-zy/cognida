# 数据写入阶段并发控制与锁机制详解

## 文档说明

本文档详细描述 Cognida 系统数据写入阶段的并发控制和锁机制设计，包括任务队列、并行数据准备、共享资源保护、缓存锁等核心技术实现。

---

## 一、并发控制架构总览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      并发控制层次结构                                   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │              第一层：任务队列并发控制                             │   │
│  │  • TaskProcessor（异步任务处理器）                                │   │
│  │  • Channel 队列（容量 100）                                       │   │
│  │  • Worker Pool（可配置 worker 数量）                             │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │              第二层：文档级并发控制                               │   │
│  │  • 多个文档可同时处理（不同 worker）                              │   │
│  │  • 同一文档串行处理（状态检查 + 事务）                            │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │              第三层：数据准备并行控制                             │   │
│  │  • errgroup（并发数限制为 4）                                     │   │
│  │  • sync.Mutex（共享结果数组保护）                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                  ↓                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │              第四层：资源级锁机制                                 │   │
│  │  • 数据库事务锁（MySQL）                                          │   │
│  │  • 图谱缓存锁（RWMutex）                                         │   │
│  │  • Channel 并发安全                                              │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 二、任务队列并发控制（TaskProcessor）

### 2.1 架构设计

```go
// internal/handler/task_processor.go:11-16

type TaskProcessor struct {
    queue            chan *Task      // 任务队列（Channel，并发安全）
    workerCount      int             // Worker 数量
    wg               sync.WaitGroup  // WaitGroup（等待所有 worker 完成）
    knowledgeHandler *KnowledgeHandlerFull
}
```

### 2.2 并发模型

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    TaskProcessor 并发模型                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│     HTTP 请求                                                             │
│        ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                   提交任务到 Channel                            │   │
│  │                   Submit(Task)                                   │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          │                                             │
│                          ↓ (Channel: buffered, capacity=100)           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                        任务队列                                   │   │
│  │  ┌────┬────┬────┬────┬────┬────┬─────────┬────┬────┐           │   │
│  │  │ T1 │ T2 │ T3 │ T4 │ T5 │ T6 │  ...    │T99 │T100│           │   │
│  │  └────┴────┴────┴────┴────┴────┴─────────┴────┴────┘           │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          │                                             │
│            ┌─────────────┼─────────────┐                               │
│            ↓             ↓             ↓                               │
│     ┌──────────┐ ┌──────────┐ ┌──────────┐                           │
│     │ Worker 1 │ │ Worker 2 │ │ Worker N │  ← 并发消费               │
│     │  (gorout)│ │  (gorout)│ │  (gorout)│                           │
│     └──────────┘ └──────────┘ └──────────┘                           │
│                                                                         │
│  Channel 特性：                                                          │
│  • 天然并发安全（Go runtime 保证）                                      │
│  • 阻塞式提交（队列满时阻塞 5 秒后超时）                                 │
│  • 范围关闭（close 后 worker 优雅退出）                                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.3 任务提交机制

```go
// internal/handler/task_processor.go:65-74

func (p *TaskProcessor) Submit(task *Task) error {
    select {
    case p.queue <- task:
        // 成功提交到队列
        log.Printf("[TaskProcessor] Task submitted: type=%s, id=%s", task.Type, task.ID)
        return nil
    case <-time.After(5 * time.Second):
        // 队列满，5秒超时
        return ErrQueueFull
    }
}
```

### 2.4 Worker 并发控制

```go
// internal/handler/task_processor.go:46-54

func (p *TaskProcessor) Start() {
    log.Printf("[TaskProcessor] Starting with %d workers", p.workerCount)

    // 启动多个 worker goroutine
    for i := 0; i < p.workerCount; i++ {
        p.wg.Add(1)
        go p.worker(i)  // 每个 worker 独立运行
    }
}

// internal/handler/task_processor.go:76-87

func (p *TaskProcessor) worker(id int) {
    defer p.wg.Done()

    log.Printf("[TaskProcessor-%d] Worker started", id)

    // 从 channel 中取任务（自动阻塞，channel 关闭时退出）
    for task := range p.queue {
        p.processTask(id, task)  // 串行处理单个任务
    }

    log.Printf("[TaskProcessor-%d] Worker stopped", id)
}
```

### 2.5 为什么选择 Channel 而非 Mutex

| 方面 | Channel | Mutex + Queue |
|------|---------|---------------|
| **并发安全** | 天然保证 | 需要手动加锁 |
| **阻塞控制** | 自动（满/空） | 需要条件变量 |
| **代码复杂度** | 简洁 | 较复杂 |
| **性能** | 稍低（锁竞争） | 稍高 |
| **推荐场景** | 任务队列 | 高频数据交换 |

---

## 三、数据准备阶段并行控制（errgroup + Mutex）

### 3.1 并行数据准备架构

```go
// internal/handler/knowledge.go:271-280

g, ctx := errgroup.WithContext(ctx)
g.SetLimit(4)  // 最多 4 个并发任务

// 结果容器（需要并发保护）
denseVectors := make([][]float32, len(chunks))
sparseVectors := make([]entity.SparseEmbedding, len(chunks))
graphData := make([]*types.GraphData, 1)
vectorMutex := sync.Mutex{}  // 共享数组保护锁
```

### 3.2 并发控制图示

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  errgroup 并发控制模型（limit=4）                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  errgroup Context                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              Semaphore (limit=4)                         │    │   │
│  │  │  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐              │    │   │
│  │  │  │ Slot1 │ │ Slot2 │ │ Slot3 │ │ Slot4 │              │    │   │
│  │  │  └───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘              │    │   │
│  │  └──────┼──────────┼──────────┼──────────┼─────────────────┘    │   │
│  │         │          │          │          │                       │   │
│  │         ↓          ↓          ↓          ↓                       │   │
│  │    ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐                 │   │
│  │    │ gorout │ │ gorout │ │ gorout │ │ gorout │                 │   │
│  │    │   1    │ │   2    │ │   3    │ │   4    │                 │   │
│  │    │ Dense  │ │ Sparse │ │ Graph  │ │ (空闲) │                 │   │
│  │    │ Vector │ │ Vector │ │ Extract│ │        │                 │   │
│  │    └────┬───┘ └────┬───┘ └────┬───┘ └────────┘                 │   │
│  │         │          │          │                               │   │
│  │         └──────────┼──────────┼───────────────┐                │   │
│  │                    │          │               │                │   │
│  │                    ↓          ↓               ↓                │   │
│  │              ┌─────────────────────────────────┐                │   │
│  │              │     sync.Mutex (vectorMutex)   │                │   │
│  │              │  保护共享数组:                  │                │   │
│  │              │  • denseVectors[i]             │                │   │
│  │              │  • sparseVectors[i]            │                │   │
│  │              │  • graphData[0]                │                │   │
│  │              └─────────────────────────────────┘                │   │
│  └───────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  g.Wait() → 等待所有 goroutine 完成（任一失败则全部取消）                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3.3 共享数组的并发保护

#### 问题：为什么需要 Mutex？

```go
// ❌ 错误示范（无锁保护）
denseVectors := make([][]float32, len(chunks))

// goroutine 1
denseVectors[0] = vec1  // 写入位置 0

// goroutine 2
denseVectors[1] = vec2  // 同时写入位置 1

// 问题：Go 的 slice 写入不是原子操作！
// - slice 底层数组写入可能发生竞态条件
// - 不同 goroutine 写入不同位置仍可能并发问题
```

#### 正确做法：Mutex 保护

```go
// internal/handler/knowledge.go:283-307

g.Go(func() error {
    for i, chunkContent := range chunks {
        embeddings, err := h.embedder.EmbedStrings(ctx, []string{chunkContent})
        if err != nil {
            return fmt.Errorf("dense embedding failed for chunk %d: %w", i, err)
        }

        denseVec := make([]float32, len(embeddings[0]))
        for j, v := range embeddings[0] {
            denseVec[j] = float32(v)
        }

        // 🔒 获取锁
        vectorMutex.Lock()
        denseVectors[i] = denseVec  // 安全写入
        vectorMutex.Unlock()       // 🔒 释放锁
    }
    return nil
})
```

### 3.4 锁的粒度设计

| 锁类型 | 保护范围 | 粒度 | 性能影响 |
|--------|---------|------|---------|
| **vectorMutex** | 共享结果数组 | 细粒度（每次写入） | 低 |
| **graphCache.mutex** | 整个图谱缓存 | 粗粒度（整个合并过程） | 中 |
| **数据库事务** | 单个文档的所有 chunk | 粗粒度 | 低（事务短） |

### 3.5 errgroup 的错误传播

```go
// errgroup 关键特性

if err := g.Wait(); err != nil {
    // 1. 任一 goroutine 返回 error：
    //    • ctx 被取消
    //    • 其他正在运行的 goroutine 收到取消信号
    //    • g.Wait() 返回第一个非 nil 错误

    return fmt.Errorf("parallel build failed: %w", err)
}
```

---

## 四、图谱缓存并发控制（RWMutex）

### 4.1 图谱服务缓存结构

```go
// internal/application/graph.go:94-99

type graphCache struct {
    mutex     sync.RWMutex                        // 读写锁
    nodes     map[string]*types.GraphNode         // key: entity title
    relations map[string]*types.GraphRelation     // key: "source#target"
}
```

### 4.2 读写锁的使用场景

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    RWMutex 使用场景                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  写操作（Lock/Unlock）:                                                 │
│  • 清空缓存                                                             │
│  • 合并新节点                                                           │
│  • 删除节点/关系                                                        │
│  • 更新权重                                                             │
│                                                                         │
│  读操作（RLock/RUnlock）:                                               │
│  • 查询节点                                                             │
│  • 查询关系                                                             │
│  • 统计信息                                                             │
│  • 遍历缓存                                                             │
│                                                                         │
│  特性：                                                                 │
│  • 多个读操作可以并发执行                                              │
│  • 写操作独占访问                                                       │
│  • 适合读多写少场景                                                     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 4.3 写锁（合并图谱）

```go
// internal/application/graph.go:425-427

func (s *GraphService) mergeExtractedGraphs(...) (*types.GraphData, error) {
    // 🔒 获取写锁（独占访问）
    s.graphCache.mutex.Lock()
    defer s.graphCache.mutex.Unlock()

    // 修改缓存（其他 goroutine 无法访问）
    for _, data := range dataList {
        for _, node := range data.Nodes {
            if existingNode, exists := s.graphCache.nodes[node.Name]; exists {
                // 合并节点...
            } else {
                s.graphCache.nodes[node.Name] = node
            }
        }
    }

    // 计算权重，修改 relations...
}
```

### 4.4 读锁（查询缓存）

```go
// internal/application/graph.go:869-882

func (s *GraphService) FindRelationsByChunk(...) ([]*types.GraphRelation, error) {
    // 🔓 获取读锁（共享访问）
    s.graphCache.mutex.RLock()
    defer s.graphCache.mutex.RUnlock()

    // 读取缓存（多个 goroutine 可以同时读）
    var result []*types.GraphRelation
    for _, rel := range s.graphCache.relations {
        for _, cid := range rel.ChunkIDs {
            if cid == chunkID {
                result = append(result, rel)
                break
            }
        }
    }
    return result, nil
}
```

### 4.5 RWMutex vs Mutex 选择

| 场景 | 推荐锁类型 | 理由 |
|------|-----------|------|
| 共享数组写入 | `sync.Mutex` | 写操作频繁，无并发读 |
| 图谱缓存 | `sync.RWMutex` | 读多写少 |
| 图谱合并 | `sync.Mutex` | 写操作期间无读需求 |
| 查询缓存 | `sync.RWMutex` | 允许并发读 |

---

## 五、数据库事务锁

### 5.1 MySQL 事务隔离

```go
// internal/handler/knowledge.go:516-555

func (h *KnowledgeHandlerFull) saveChunksToDB(...) ([]string, error) {
    chunkIDs := make([]string, len(chunks))
    db := h.kbService.GetDB()

    // 🔒 开启事务（自动加锁）
    tx := db.Begin()
    if tx.Error != nil {
        return nil, tx.Error
    }

    for i, chunkContent := range chunks {
        chunkID := uuid.New().String()

        // INSERT 操作（行锁）
        chunkRecord := &types.Chunk{
            ID:          chunkID,
            TenantID:    knowledge.TenantID,
            KBID:        knowledge.KBID,
            KnowledgeID: knowledge.ID,
            // ...
        }

        if err := tx.Create(chunkRecord).Error; err != nil {
            tx.Rollback()  // ❌ 失败回滚
            return nil, fmt.Errorf("failed to create chunk %d: %w", i, err)
        }
        chunkIDs[i] = chunkID
    }

    // ✅ 提交事务（释放锁）
    if err := tx.Commit().Error; err != nil {
        return nil, fmt.Errorf("failed to commit transaction: %w", err)
    }

    return chunkIDs, nil
}
```

### 5.2 事务锁特性

| 特性 | 说明 |
|------|------|
| **原子性** | 所有 chunk 要么全部成功，要么全部失败 |
| **隔离性** | 其他事务看不到未提交的数据 |
| **行锁** | INSERT 时自动加行锁 |
| **锁释放** | Commit 或 Rollback 时释放 |
| **死锁风险** | 低（单事务顺序写入） |

---

## 六、Channel 并发安全

### 6.1 TaskProcessor 的 Channel

```go
// internal/handler/task_processor.go:39

queue: make(chan *Task, 100),  // 缓冲 channel，容量 100
```

### 6.2 Channel 并发安全保证

```
Go Channel 的并发安全保证：
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│  发送 (send):                                                           │
│  channel <- value                                                       │
│  • 多个 goroutine 可同时发送                                            │
│  • Go runtime 保证每次只有一个发送成功                                   │
│  • 队列满时阻塞                                                         │
│                                                                         │
│  接收 (receive):                                                        │
│  value := <-channel                                                     │
│  • 多个 goroutine 可同时接收                                            │
│  • Go runtime 保证每个值只被接收一次                                    │
│  • 队列空时阻塞                                                         │
│                                                                         │
│  关闭 (close):                                                          │
│  close(channel)                                                         │
│  • 只能发送方关闭                                                       │
│  • 关闭后发送 panic，接收返回零值                                        │
│  • 多次关闭 panic                                                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 七、并发控制流程图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    完整数据写入流程（含并发控制）                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  用户上传文件                                                             │
│       ↓                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Handler.Submit() - HTTP 请求处理                                 │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  TaskProcessor.Submit() - 提交到 Channel 队列                    │   │
│  │  • Channel 并发安全                                              │   │
│  │  • 队列满时阻塞 5 秒                                              │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Worker.processTask() - 从队列取任务                              │   │
│  │  • 多个 worker 并发处理不同文档                                    │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  1. 状态检查 + 更新（MySQL）                                      │   │
│  │     • pending → processing                                        │   │
│  │     • 避免重复处理                                                │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  2. 文档分块 (Chunker)                                            │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  3. MySQL 事务写入（串行）                                        │   │
│  │     • BEGIN TRANSACTION                                          │   │
│  │     • INSERT chunks（行锁）                                      │   │
│  │     • COMMIT / ROLLBACK                                          │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  4. 并行数据准备（errgroup + Mutex）                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │  errgroup (limit=4)                                      │    │   │
│  │  │  • goroutine 1: denseVectors (mutex 保护)                 │    │   │
│  │  │  • goroutine 2: sparseVectors (mutex 保护)                │    │   │
│  │  │  • goroutine 3: graphData (mutex 保护)                    │    │   │
│  │  │  • g.Wait(): 任一失败则全部取消                            │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  5. Milvus 写入（独立 context）                                   │   │
│  │     • 独立 context 避免被取消                                     │   │
│  │     • 批量 Insert                                                │   │
│  │     • Flush 确保可搜索                                            │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  6. Neo4j 写入（可选，独立 context）                              │   │
│  │     • 失败仅记录警告                                              │   │
│  │     • 不影响主流程                                                │   │
│  └───────────────────────┬─────────────────────────────────────────┘   │
│                          ↓                                             │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  7. 状态更新（MySQL）                                             │   │
│  │     • processing → completed                                     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                          ↓                                             │
│                     处理完成                                            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 八、设计原则总结

### 8.1 锁的选择原则

| 场景 | 推荐方案 | 理由 |
|------|---------|------|
| 任务队列 | Channel | 天然并发安全，阻塞控制 |
| 共享数组写入 | sync.Mutex | 简单高效，写入频繁 |
| 读多写少缓存 | sync.RWMutex | 允许并发读 |
| 数据库写入 | 事务 | 原子性，自动行锁 |

### 8.2 并发控制原则

| 原则 | 说明 | 实现 |
|------|------|------|
| **最小锁粒度** | 只锁必要部分 | vectorMutex 只锁单次写入 |
| **锁持有时间短** | 减少锁内操作 | 先准备数据，再获取锁写入 |
| **避免嵌套锁** | 防止死锁 | 不在锁内调用可能加锁的函数 |
| **错误传播** | errgroup 自动取消 | 任一失败则全部取消 |
| **资源隔离** | 独立 context | Milvus/Neo4j 写入使用独立 context |

### 8.3 常见陷阱与避免

| 陷阱 | 说明 | 避免 |
|------|------|------|
| **Slice 竞态** | 多个 goroutine 写入 slice 不同位置 | 使用 Mutex 保护 |
| **Map 竞态** | 并发读写 map | 使用 sync.Map 或 Mutex |
| **忘记 Unlock** | Lock 后忘记 Unlock | 使用 defer Unlock() |
| **死锁** | 嵌套锁或循环等待 | 避免嵌套锁，统一加锁顺序 |
| **goroutine 泄漏** | goroutine 无法退出 | 使用 context 控制 |

---

*文档版本: 1.0*
*更新日期: 2025-02-16*
*作者: Cognida 开发团队*
