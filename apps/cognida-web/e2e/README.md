# 真实页面采集与发布验收

此套件通过真实页面登录、创建输入、启动 7 阶段流水线、批准发布并打开分析报告。只读 API 校验用于交叉核对页面结果；没有路由响应替换、假数据服务或人工修改完成状态。运行时使用本机 Google Chrome，不启动应用服务。

## 环境

必须先准备专用数据库、对象存储前缀和租户，并运行前后端、Python 计算服务及 lifecycle worker。测试账号应具备创建输入、运行流水线和批准发布权限。文本发布需要该账号实际可访问的知识库以及正常的文档处理依赖。验证已修复的共享向量实例时，先备份，使用不与原业务重叠的测试租户及新知识库；回收时只能删除本次已记录的测试 scope，不能删除共享集合。

所有目标都必须显式传入，禁止向业务数据库或现有业务知识库写入测试数据：

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

默认来源为 text/table 两个完整流程。表格明确选择 revenue 为数值列，使用 100、200、300，要求实际 API 报告和页面都显示 numeric_count=3、mean=200。文本检查真实入库回执、文档解析完成和非空分块，再通过知识库检索页面执行向量搜索，要求命中本次知识文档、返回真实 chunk_id，且响应 sources 包含 milvus_vector、不含 database_lexical 词法回退。手工文本加入每次运行独有的 marker，API 和页面必须同时找到；最多等待 60 秒的向量可见性延迟，每次重试均通过实际页面搜索。所有用例在审批前检查该流水线没有已发布资产，审批后要求 7 个阶段均为 succeeded，页面阶段卡片按执行流程排序。

完整套件另运行 `workspace.spec.ts` 的三个交互场景：总览数量与真实 API 对照、运行深链打开报告、手机导航和弹窗键盘操作，以及独立质量专家执行后重新打开结果。该文件需要前面的 lifecycle 场景已创建成功运行和发布资产；单独运行它时也必须准备这些真实数据。所有失败都会保留截图，不会伪造状态来通过断言。

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

### Supervisor 规划验收

`supervisor.spec.ts` 单独覆盖真实输入创建、实际模型候选、显式保存和刷新恢复。它不执行流水线、审批、发布或 Sandbox；只需前端、Go、隔离 MySQL、独立缓存和本地对象存储，以及可用的模型配置。Go 的当前启动依赖仍需 Milvus 连通，但此场景不写向量。可关闭后台 worker、数据源健康检查、经验蒸馏和语义缓存；使用正常注册、登录和当前项目默认租户配置。

沿用上面的三个 URL／账号环境变量和隔离环境标记，额外显式允许实际模型调用：

```sh
COGNIDA_E2E_SUPERVISOR=1 npm run test:e2e -- e2e/supervisor.spec.ts
```

场景使用合成文本，请求质检与清洗两个候选节点，验证模型输出确实通过服务端预检；预览前后没有计划写入或子任务。保存后读取实际计划 v1，刷新只发起查询，不重新规划；同时覆盖未保存候选的同页面导航提示，以及桌面和手机布局。没有模型配置、模型超时或候选无效均算失败，不用响应替换通过断言。未显式启用时用例跳过，跳过不计为验收通过。

### 任务控制器真实执行验收

`task-runtime.spec.ts` 需要启用任务控制器与 lifecycle worker，并启动真实 Python 计算服务。它不调用模型，也不启动 Sandbox。页面创建可信手工输入，通过真实 HTTP 保存固定计划，再从 `/tasks` 页面启动、申请取消及刷新；计划 fixture 读取真实能力注册表中的版本、工具、权限与调用成本，不替换接口响应。

```sh
COGNIDA_E2E_TASK_EXECUTION=1 npm run test:e2e -- e2e/task-runtime.spec.ts
```

三个场景分别验证四节点依赖执行、两个独立质检节点在并发预算内投递、以及执行后的取消。四节点依次包含质检、清洗、规则增强和本地完整性评估；当前评估合同接受质量报告，因此评估节点等待增强完成后读取清洗质量报告，不声称评估模型准确率。验收核对真实子任务句柄、领域终态、产物内容与版本、领域详情深链、桌面和手机布局；刷新与自动查询不得重复提交命令。并行场景通过真实输入 API 准备 10,000 行 × 10 列、低于 4 MiB 的合成表格，使真实 Python 计算时间足以观察重叠，并严格核对领域记录的时间区间。小文本质检可能早于第二次提交完成，不能据此把并发上限误解为重叠保证。

取消场景使用 32 个质检节点和单并发，为页面发起取消保留可观测窗口。若 worker 恰好推进版本导致取消返回 409，只通过页面重新核对状态后再次显式提交，最多三次；最终按真实节点事实验收：已有成功节点时必须为 `partially_completed` 并保留其确认产物，没有成功节点时为 `cancelled`。场景退出时只对本用例已创建且仍运行的任务申请取消，保留资产与执行记录，由隔离环境所有者统一回收。

### 产物与回收

`test-results/` 保存审批前和报告页截图、创建资产引用、审批前运行状态、完成状态、发布回执及分析报告。`playwright-report/` 保存 HTML 报告。登录与 API 请求使用真实凭据，因此关闭 trace/video，以上产物目录和本地 fixture 文件均已加入忽略规则。

套件关闭自己创建的浏览器/API 上下文，不停止外部服务、不删除其他用户数据。已创建的不可变资产、流水线与知识文档保留供审计；验收结束由隔离环境所有者回收专用数据库及对象存储前缀。失败时 `last-run-state` 保留实际失败点，不将失败、阻塞或超时标记为成功。
