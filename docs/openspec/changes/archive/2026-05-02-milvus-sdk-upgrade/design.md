## Context

### 当前状态
- 项目使用 Milvus 作为向量数据库，存储知识库分块的向量表示
- BM25 检索使用假实现 (`containsKeyword()` 返回固定分数 0.7)
- SDK 版本: `github.com/milvus-io/milvus-sdk-go/v2` v2.4.2
- BM25 功能仅在 SDK v2.6+ 的 `entity.Text()` API 中可用

### 约束条件
- Milvus Server 版本需 ≥ 2.4.x
- 公共接口 `MilvusRetriever` 必须保持向后兼容
- 不能中断现有的向量检索功能
- 需要保持与 MySQL 数据的兼容性

### 相关方
- RAG 检索服务 - 使用 BM25 进行混合检索
- 知识库服务 - 需要创建支持 BM25 的 Collection

## Goals / Non-Goals

**Goals:**
- 升级 Milvus SDK 到 v2.6.3 以支持 BM25 全文搜索
- 实现 `entity.Text()` API 进行真正的 BM25 检索
- 重构现有代码以适配新 SDK 的 Option 模式 API
- 保持公共接口向后兼容

**Non-Goals:**
- 不修改 BM25 算法参数 (k1, b) 的配置管理（留待后续）
- 不实现数据迁移逻辑（假设 Milvus Collection 可重新创建）
- 不修改向量检索的业务逻辑

## Decisions

### 1. SDK 升级策略

**决策**: 完全替换到新 SDK 而非双 SDK 并存

**理由**:
- 新 SDK 是官方推荐路径，旧 SDK 已标记为 Deprecated
- 双 SDK 并存会增加代码复杂度和依赖大小
- 一次性迁移可以避免长期的维护负担

**替代方案**: 双 SDK 并存（旧 SDK 用于向量检索，新 SDK 仅用于 BM25）
- 被拒绝的原因：代码复杂度高，两套客户端管理困难

### 2. API 重构范围

**决策**: 重构所有 Milvus 客户端调用以使用新 SDK Option 模式

**理由**:
- 新 SDK 的所有方法都使用 Option 模式，无法渐进式迁移
- 一次性重构确保代码一致性
- 降低测试复杂度

**影响的方法**:
- `CreateCollection` → `NewCreateCollectionOption(name, schema)`
- `Search` → `NewSearchOption(name, topK, vectors).WithXXX()`
- `ListCollections` → `NewListCollectionOption()`
- `DropCollection` → `NewDropCollectionOption(name)`
- `HasCollection` → `NewHasCollectionOption(name)`
- `DescribeCollection` → `NewDescribeCollectionOption(name)`

### 3. 结果解析适配

**决策**: 重写结果解析逻辑以适配新 SDK 的返回类型

**理由**:
- 新 SDK 返回 `[]*ResultSet` 而非 `[]*SearchResult`
- 字段访问从 `field.(*entity.ColumnInt64)` 改为 `resultSet.GetColumn().GetScalars().GetLongData()`
- 旧 API 的 `entity.Column` 系列类型不再存在

### 4. BM25 Schema 设计

**决策**: 添加 `text` 和 `sparse` 字段，配置 BM25 Function

**Schema 结构**:
```go
// text 字段: VARCHAR(65535), enable_analyzer=true
entity.NewField().
    WithName("text").
    WithDataType(entity.FieldTypeVarChar).
    WithMaxLength(65535).
    WithEnableAnalyzer(true)  // 关键: 启用分词器

// sparse 字段: SPARSE_FLOAT_VECTOR
entity.NewField().
    WithName("sparse").
    WithDataType(entity.FieldTypeSparseVector)

// BM25 Function
entity.NewFunction().
    WithName("text_bm25_emb").
    WithInputFields("text").
    WithOutputFields("sparse").
    WithType(entity.FunctionTypeBM25)
```

**索引配置**:
```go
index.NewAutoIndex(entity.MetricType(entity.BM25)).
    WithExtraParam("inverted_index_algo", "DAAT_MAXSCORE").
    WithExtraParam("bm25_k1", 1.2).
    WithExtraParam("bm25_b", 0.75)
```

## Risks / Trade-offs

### 风险 1: 新 SDK API 稳定性
**风险**: 新 SDK v2.6 相对较新，可能有未发现的 bug
**缓解**: 在测试环境充分验证，保持 Milvus Server 版本兼容

### 风险 2: 大规模重构引入 Bug
**风险**: 所有 Milvus 调用都需要修改，可能引入回归
**缓解**:
- 逐步迁移，先完成 BM25 功能
- 保留单元测试和集成测试
- 添加 BM25 功能的专项测试

### 风险 3: 依赖冲突
**风险**: grpc 等依赖升级可能影响其他模块
**缓解**: 运行 `go mod tidy` 并检查所有依赖版本

### 风险 4: Collection 兼容性
**风险**: 新 Schema 可能与现有 Collection 不兼容
**缓解**: 支持创建新的 BM25-enabled Collection，旧 Collection 继续使用向量检索

## Migration Plan

### 阶段 1: SDK 升级和基础重构
1. 更新 go.mod 中的 SDK 依赖
2. 更新 `milvus_client.go` 中的 Client 初始化
3. 修复所有编译错误

### 阶段 2: API 重构
1. 重构 `CreateKnowledgeBase` 方法
2. 重构 `SearchVectors` 方法
3. 重构其他 CRUD 方法

### 阶段 3: BM25 功能实现
1. 添加 BM25 Schema 构建逻辑
2. 实现 `FullTextSearch` 方法
3. 添加 BM25 索引创建逻辑
4. 更新 `BM25Retrieve` 调用新 API

### 阶段 4: 测试和验证
1. 单元测试更新
2. 集成测试验证
3. 测试环境部署验证

### 回滚策略
- 保留旧代码在 git 历史中
- 如遇严重问题，可快速回退到旧 SDK
- 使用功能开关控制 BM25 是否启用

## Open Questions

1. **BM25 参数配置**: k1 和 b 参数是否需要可配置？
   - 当前设计: 使用默认值 (k1=1.2, b=0.75)
   - 后续: 可考虑添加到配置文件

2. **数据迁移**: 是否需要迁移现有 Collection 数据？
   - 当前假设: 重新创建 Collection
   - 需要确认: 生产环境数据迁移策略

3. **中文分词**: Milvus 的 standard 分析器是否支持中文？
   - 需要验证: 中文分词效果
   - 可能需要配置自定义分析器
