## 1. 准备阶段

- [ ] 1.1 确认生产环境 Milvus Server 版本（需 ≥ 2.4.x）
- [x] 1.2 检查 Milvus SDK 版本（当前 v2.4.2 已支持 BM25）
- [x] 1.3 运行 `go build ./internal/...` 验证当前代码编译通过

## 2. SDK 依赖升级

- [x] 2.1 更新 go.mod: 添加 `github.com/milvus-io/milvus/client/v2` v2.6.3
- [x] 2.2 更新 go.mod: 添加 `github.com/milvus-io/milvus/pkg/v2` v2.6.15
- [x] 2.3 运行 `go mod tidy` 解决依赖冲突
- [x] 2.4 验证 grpc 版本升级 (v1.48.0 → v1.79.3)
- [x] 2.5 移除旧 SDK `github.com/milvus-io/milvus-sdk-go/v2` 依赖

## 3. Milvus 客户端初始化更新

- [x] 3.1 更新 `milvus_client.go` 的 import: 使用 `milvusclient` 包
- [x] 3.2 更新 `MilvusClient` 类型: 从 `client.Client` 改为 `*milvusclient.Client`
- [x] 3.3 更新 `InitMilvus` 方法: 使用 `milvusclient.New(ctx, config)`
- [x] 3.4 更新 `ListCollections` 调用: 添加 `NewListCollectionOption()` 参数
- [x] 3.5 更新 `Close` 方法: 添加 Context 参数

## 4. Collection 管理 API 重构

- [x] 4.1 重构 `CreateKnowledgeBase`: 使用 `NewCreateCollectionOption(name, schema)`
- [x] 4.2 重构 `DeleteKnowledgeBase`: 使用 `NewDropCollectionOption(name)`
- [x] 4.3 重构 `HasKnowledgeBase`: 使用 `NewHasCollectionOption(name)`
- [x] 4.4 重构 `GetKnowledgeBaseInfo`: 使用 `NewDescribeCollectionOption(name)`
- [x] 4.5 重构 `ListKnowledgeBase`: 处理返回类型变化 (`[]string` vs `[]*entity.Collection`)

## 5. 向量搜索 API 重构

- [x] 5.1 重构 `SearchVectors` 方法: 使用 `NewSearchOption(name, topK, vectors)`
- [x] 5.2 更新搜索参数设置: 使用 `.WithOutputFields()`, `.WithANNSField()` 等
- [x] 5.3 重写结果解析逻辑: 适配 `[]*ResultSet` 返回类型
- [x] 5.4 更新字段访问: 使用 `resultSet.GetColumn()` 和 `col.Value()`
- [x] 5.5 更新 `Insert` 方法: 适配新的 Column API

## 6. BM25 Schema 扩展

- [x] 6.1 添加 `text` 字段定义 (VARCHAR, max_length=65535)
- [x] 6.2 添加 `sparse` 字段定义 (SPARSE_FLOAT_VECTOR)
- [x] 6.3 添加 BM25 Function 定义 (text → sparse 映射)
- [x] 6.4 添加 BM25 索引创建逻辑 (metric_type=BM25, inverted_index_algo=DAAT_MAXSCORE)
- [x] 6.5 添加 BM25 参数配置 (bm25_k1=1.2, bm25_b=0.75, drop_ratio_search=0.2)

## 7. FullTextSearch 实现

- [x] 7.1 实现 `FullTextSearch` 方法: 使用 `HybridSearchOption()` + `entity.Text()`
- [x] 7.2 配置搜索参数: `.WithANNSField("sparse")`
- [x] 7.3 实现结果解析: 提取所有输出字段并映射到 SearchResult
- [x] 7.4 添加错误处理: 失败时返回空结果以支持 fallback
- [x] 7.5 添加日志记录: 记录搜索结果数量和失败原因

## 8. 适配器更新

- [x] 8.1 更新 `adapter.go` 的 import: 使用新 SDK 包
- [x] 8.2 更新 `MilvusAdapter.Search` 方法: 适配新 API
- [x] 8.3 更新 `MilvusAdapter.FullTextSearch` 方法: 适配新 API
- [x] 8.4 更新 `buildFilterExpr` 函数: 保持过滤逻辑兼容

## 9. 上层调用更新

- [x] 9.1 更新 `retriever.go` 中的 `BM25Retrieve` 方法
- [x] 9.2 移除 `containsKeyword` 假实现函数 (保留作为 fallback)
- [x] 9.3 使用 Milvus 返回的真实 BM25 分数
- [x] 9.4 更新 SearchTrace 中的 BM25 相关字段

## 10. Wire 依赖注入更新

- [x] 10.1 更新 `wire.go` 的 import: 添加 retriever 包
- [x] 10.2 更新 `ProvideMilvusRetriever`: 使用新类型
- [x] 10.3 运行 `wire generate` 生成依赖注入代码
- [x] 10.4 验证 wire 编译通过

## 11. 旧代码清理

- [x] 11.1 移除或更新 `milvus_repo.go` (未使用的旧代码)
- [x] 11.2 更新 `integration_test.go`: 使用新 SDK API
- [x] 11.3 移除不再使用的 import
- [x] 11.4 清理 go.mod 中的无用依赖

## 12. 编译验证

- [x] 12.1 运行 `go build ./internal/...` 验证编译通过
- [x] 12.2 运行 `go build ./cmd/...` 验证主程序编译通过
- [x] 12.3 修复所有编译错误和警告

## 13. 测试

- [x] 13.1 编写 BM25 Schema 创建的单元测试
- [x] 13.2 编写 `FullTextSearch` 方法的单元测试
- [x] 13.3 编写 `BM25Retrieve` 方法的集成测试
- [x] 13.4 测试中文分词效果
- [ ] 13.5 测试 BM25 参数调优效果 (需要在有 BM25 索引的 collection 上测试)
- [x] 13.6 运行 `go test ./internal/...` 验证测试通过
- [x] 13.7 运行集成测试验证 Milvus SDK v2.6.3 兼容性
- [x] 13.8 代码清理: 净减少 701 行，删除 milvus_repo.go 旧代码

## 14. 文档和清理

- [ ] 14.1 更新 Milvus Collection 管理文档
- [ ] 14.2 更新 RAG 检索相关文档
- [ ] 14.3 添加 BM25 参数调优指南
- [ ] 14.4 代码注释更新

## 15. 上线验证

- [ ] 15.1 部署到测试环境，验证 BM25 功能正常
- [ ] 15.2 监控检索质量和性能指标
- [ ] 15.3 灰度上线到生产环境
- [ ] 15.4 全量上线，下线旧实现
- [ ] 15.5 清理旧代码和 Collection
