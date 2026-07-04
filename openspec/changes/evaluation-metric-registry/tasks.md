## 1. Python 注册表元数据与 compute 路径改造

- [x] 1.1 为每个 grader 增加元数据字段:`name`、`label`、`group`、`requires_reference`、`requires_contexts`、`eval_types`(llm/qa、rag、agent,可多值)
- [x] 1.2 在 `graders/registry.py` 暴露读取元数据的接口(如 `list_graders()` 返回含元数据的条目)
- [x] 1.3 将 `fastapi_app.py` `/compute-metrics` 改为遍历 `request.graders` 从注册表解析并调用,删除写死的 if 链
- [x] 1.4 每项与聚合结果输出动态 `scores{name:value}` map,同时保留现有固定字段
- [x] 1.5 未注册的指标名跳过并在响应中标记为 unsupported,而非静默忽略
- [x] 1.6 确认既有优化不回退:语义相似度批量单次加载、零值聚合保留、llm_reasoning 解析
- [x] 1.7 更新/新增 pytest:注册表驱动计算、动态 scores map、目录内每项均有可执行 grader、零值保留回归

## 2. Python 可用指标目录数据源

- [x] 2.1 提供按 `eval_type` 过滤 grader 元数据的函数/端点(供 Go 拉取)
- [x] 2.2 未知 `eval_type` 返回校验错误而非无过滤全量
- [x] 2.3 pytest 覆盖:类型过滤、共用指标跨类型出现、未知类型报错

## 3. Go 动态载体与类型

- [x] 3.1 `internal/service/evaluation/types.go` 增加 `Scores map[string]float64`(item 与 aggregate),保留固定字段
- [x] 3.2 `EvaluationType` 增加 `llm`,并将 `llm` 与 `qa` 做向后兼容映射
- [x] 3.3 `python_client.go` 透传 `scores` map(不回退已修复的重试重建请求逻辑)
- [x] 3.4 worker/service 将 `scores` map 传递到存储层
- [x] 3.5 go test/vet 通过

## 4. MySQL JSON 列(评测表无 AutoMigrate)

- [x] 4.1 为 `evaluation_qa_results` 等增加 JSON `scores` 列(手动 ALTER TABLE,保留固定列)
- [x] 4.2 repository 读写 `scores` JSON 列;读侧对 NULL/空列兼容为空 map
- [x] 4.3 集成测试:真实 DB 写入并读回动态 scores,固定字段仍一致

## 5. 后端可用指标目录接口

- [x] 5.1 Go 暴露"可用指标目录"端点,按 `eval_type` 过滤返回元数据(name/label/group/requires_reference/requires_contexts/eval_types)
- [x] 5.2 目录数据来源于注册表元数据(启动/定时缓存或代理),保证目录内每项都有可执行 grader
- [x] 5.3 API 测试:按类型过滤、契约字段齐全

## 6. 前端目录后端驱动与动态渲染

- [x] 6.1 `views/evaluation/evaluation-config.ts` 改为拉取后端目录,移除写死的 `GRADER_GROUPS`(保留一版回退)
- [x] 6.2 创建任务时按所选评测类型展示对应可用指标
- [x] 6.3 结果展示按动态 `scores` map 渲染,兼容固定字段
- [x] 6.4 消除 `llm_factual`/`llm_safety` 等漂移项(以后端目录为准)

## 7. Review 与交付

- [x] 7.1 触发 code-review skill 修复问题（自审 + 子代理评审：修复快速切换评测类型的目录竞态；其余为已知权衡）
- [ ] 7.2 各阶段独立提交,确认不破坏现有功能且已修复 bug 未回退
- [ ] 7.3 任务完成后终止所有开启的服务进程
