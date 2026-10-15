# 真实页面采集与发布验收

此套件通过真实页面登录、创建输入、启动 7 阶段流水线、批准发布并打开分析报告。只读 API 校验用于交叉核对页面结果；没有路由响应替换、假数据服务或人工修改完成状态。运行时使用本机 Google Chrome，不启动应用服务。

## 环境

必须先准备专用数据库、对象存储前缀和租户，并运行前后端、Python 计算服务及 lifecycle worker。测试账号应具备创建输入、运行流水线和批准发布权限。文本发布需要该账号实际可访问的知识库以及正常的文档处理依赖。

所有目标都必须显式传入，禁止用业务实例验收：

```sh
cd apps/cognida-web
export COGNIDA_E2E_ISOLATED=1
export COGNIDA_E2E_BASE_URL=http://127.0.0.1:18583
export COGNIDA_E2E_API_URL=http://127.0.0.1:18581
export COGNIDA_E2E_ACCOUNT_FILE=/private/tmp/your-isolated-test/account.json
export COGNIDA_E2E_KNOWLEDGE_BASE_ID=YOUR_AUTHORIZED_TEST_KB_ID
npm run test:e2e:check
npm run test:e2e:list
npm run test:e2e
```

凭据文件位于仓库外，格式支持 `{"email":"…","password":"…"}` 或 `{"credentials":{"email":"…","password":"…"}}`。套件只使用真实登录响应中的 token，不使用凭据文件内可能保存的旧 token。

登录遇到 HTTP 429 时遵循服务端 `Retry-After`，最多重新提交两次，初次提交、等待及重试合计限定在 60 秒内；缺少有效等待指示或其他 HTTP 错误立即失败。不修改服务端限流，也不重试整个用例。

默认运行 text/table 两个用例。表格明确选择 revenue 为数值列，使用 100、200、300，要求实际 API 报告和页面都显示 numeric_count=3、mean=200。文本检查真实入库回执、文档解析完成和非空分块，再通过知识库检索页面执行向量搜索，要求命中本次知识文档、返回真实 chunk_id，且响应 sources 包含 milvus_vector、不含 database_lexical 词法回退。手工文本加入每次运行独有的 marker，API 和页面必须同时找到；最多等待 60 秒的向量可见性延迟，每次重试均通过实际页面搜索。所有用例在审批前检查该流水线没有已发布资产，审批后要求 7 个阶段均为 succeeded。

默认分析目标留空，校验可复算的描述统计。如需验收真实模型分析，显式设置 `COGNIDA_E2E_ANALYSIS_PROMPT`，或在 fixture 中填写 `analysis_prompt`；此时额外要求实际 narrative、findings、有效 evidence 引用与未验证解释标记，缺少模型或失败时测试失败。

## 其他来源

将 `fixtures.example.json` 复制到仓库外，填写真实来源 ID 和目标知识库。通过 `COGNIDA_E2E_FIXTURES_FILE=/absolute/path/fixtures.json` 加载。指定文件后，其 scenarios 数组替换默认的 text/table 用例，可仅保留所需来源。

- web：明确的 HTTPS 地址；测试会真实发起受控采集。
- knowledge：当前租户已成功解析的知识文档 ID。
- collection：同一真实采集运行的 run_id/item_id，产物对象必须仍存在。
- datasource：当前租户授权的数据源、schema、业务表，最多导入 1,000 行快照。
- result：测试账号所属的真实 session_id/result_id，Result Store 句柄必须尚未过期。
- evaluation：当前账号可访问且已完成的真实评测任务 ID。

文本来源可提供 `search_query` 指定实际检索问题；未指定时使用本次发布文本前 300 字。仅回显请求模式或缺少召回 provenance 的 API 不满足向量检索断言。

数值列的 `expected.numeric_count` 始终精确检查；`mean`、`numeric_min`、`numeric_max` 仅在显式提供时同时检查 API 与报告页面。需要验证高精度数值未被舍入时，可用 `expected.published_exact_strings`，例如 `[{"row":0,"column":"precise_amount","value":"9007199254740993.0001"}]`，断言发布快照对应单元格仍为逐字相同的字符串；`row` 从 0 开始。

用 `npm run test:e2e -- --grep 'table /'` 只运行表格；`--headed` 可展示浏览器；`COGNIDA_E2E_TIMEOUT_MS` 可调整单个用例超时，默认 300000ms。不会重试整个用例以免创建重复输入。

`collection.spec.ts` 单独验收采集页。设置 `COGNIDA_E2E_COLLECTION_TASK_ID` 可复用隔离环境中已经真实完成的任务，点击详情并核对产物，再点击“转入流水线”，验证 run/item 预填且没有自动创建资产或运行。也可设置 `COGNIDA_E2E_COLLECTION_URL`，让用例通过页面真正创建并启动一条采集任务；这种模式还需目标知识库配置。两者都未设置时该用例显式跳过，不生成额外任务。

```sh
COGNIDA_E2E_COLLECTION_TASK_ID=YOUR_REAL_COLLECTION_TASK_ID npm run test:e2e -- e2e/collection.spec.ts
```

## 证据与清理

`test-results/` 保存审批前和报告页截图、创建资产引用、审批前运行状态、完成状态、发布回执及分析报告。`playwright-report/` 保存 HTML 报告。登录与 API 请求使用真实凭据，因此关闭 trace/video，以上产物目录和本地 fixture 文件均已加入忽略规则。

套件关闭自己创建的浏览器/API 上下文，不停止外部服务、不删除其他用户数据。已创建的不可变资产、流水线与知识文档保留供审计；验收结束由隔离环境所有者回收专用数据库及对象存储前缀。失败时 `last-run-state` 保留实际失败点，不将失败、阻塞或超时标记为成功。
