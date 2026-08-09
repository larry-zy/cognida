## Why

当前 Milvus SDK v2.4.2 不支持 BM25 全文搜索功能 (`entity.Text()`)。我们需要升级到 SDK v2.6+ 以实现真正的 BM25 关键词检索，替代当前的假实现 (`containsKeyword()` 返回固定分数 0.7)。

## What Changes

- **BREAKING**: 升级 Milvus SDK 从 `github.com/milvus-io/milvus-sdk-go/v2` v2.4.2 到 `github.com/milvus-io/milvus/client/v2` v2.6.3
- **BREAKING**: 重构所有 Milvus 客户端调用，从直接参数模式改为 Option 模式
- 添加 BM25 全文搜索支持，使用 `entity.Text()` API
- 添加 BM25 Schema 字段 (`text`, `sparse`) 和 BM25 Function 定义
- 重构 `FullTextSearch()` 方法实现
- 更新 `milvus_client.go` 中的 Client 类型为 `*milvusclient.Client`

## Capabilities

### New Capabilities
- `bm25-fulltext-search`: Milvus BM25 全文搜索功能，支持使用原始文本进行 BM25 相关性评分检索

### Modified Capabilities
- `vector-search`: 现有向量检索需要适配新 SDK API（实现细节变化，需求级别不变）

## Impact

### 受影响的文件
- `internal/infrastructure/persistence/milvus/milvus_client.go` - Client 初始化和类型变更
- `internal/infrastructure/persistence/milvus/retriever/repository.go` - 所有 client 方法调用重构
- `internal/infrastructure/persistence/milvus/retriever/adapter.go` - 适配器更新
- `internal/infrastructure/persistence/milvus/milvus_repo.go` - 更新或移除
- `internal/infrastructure/rag/retriever.go` - BM25Retrieve 调用新 API
- `cmd/wire/wire.go` - 依赖注入更新

### 依赖变化
- 新增: `github.com/milvus-io/milvus/client/v2` v2.6.3
- 新增: `github.com/milvus-io/milvus/pkg/v2` v2.6.15
- 升级: `google.golang.org/grpc` v1.48.0 → v1.79.3
- 移除: `github.com/milvus-io/milvus-sdk-go/v2` v2.4.2 (被新 SDK 替代)

### API 兼容性
- 公共接口 `MilvusRetriever` 保持不变
- 内部实现完全重构，对上层调用者透明
