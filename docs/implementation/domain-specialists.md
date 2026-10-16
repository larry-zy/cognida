# 特征工程与数据标注领域能力

> 历史首批实现记录。下文的「本批」限制和测试数量不代表当前全部功能；当前独立入口、prepared/标准前置与 SFT 范围见[独立数据能力支持矩阵](standalone-data-capabilities.md)，特征增量/回填和 embedding 的最新验收边界见[主 README](../../README.md)。

本批扩展的是实际业务处理能力，并把它们接入已有 Supervisor／持久任务控制器。Sandbox、RL 和模型训练不在本批范围内。

## 能力与产物

| 能力 | 输入及处理 | 实际产物 |
| --- | --- | --- |
| 特征工程 | 固定表格资产；字段、类型化表达式、实体时间窗口聚合 | 固定定义版本、特征值、可用时间、物化回执和带来源的结果资产 |
| 知识打标 | 显式标签及关键词；提取规则命中位置 | 待审核标签、命中依据、无命中状态；不自动修改知识文档 |
| 分类预标注 | 标签名称与独立关键词规则 | 带原文证据的分类候选 |
| 实体预标注 | 每类实体的明确词条字典 | Unicode 字符区间；重叠或歧义提示人工处理 |
| 问答标注 | 表格中的问题列和参考答案列 | 真实问答样本、人工修正、审核与 approved-only JSONL 导出 |

规则不输出伪造的模型置信度。无法匹配或有歧义时保留 `uncertain`。人工修改、已审核候选不会被再次预标注覆盖；修改内容后清除旧审核结论。关系标注保留人工编辑，不宣称已有关系抽取模型。

## 特征定义与时间语义

表达式只允许 `column/literal/add/sub/mul/div/abs/round/clamp/lower/upper/trim/concat/category_map/to_number`。不执行任意脚本；AST 最大深度 8、节点数 64，每次最多 1,000 行。`embedding` 明确拒绝，不生成占位向量。

窗口按实体分组，对每条记录的事件时间 `t` 计算 `(t-window, t]`，支持 sum、mean、min、max、count。预览先检查完整输入，再截取展示行；预览不写特征值。除法不能有限表示时，保留 18 位小数并使用远离零的半入舍入。

数值使用十进制计算。定义保留精确文本；数据库 `value_exact` 与响应 `value_exact` 保留原始 JSON 数值，页面优先展示精确文本。客户端处理大整数或高精度小数时应消费该字段，避免 JavaScript `Number` 舍入。普通 `value` 保持原有兼容类型。

每条值同时记录事件时间 `timestamp` 和数据库赋予的 `available_at`。历史查询要求两者均不晚于 `as_of`，避免后补写值泄漏到先前快照。相同定义版本、实体和事件时间不能覆盖为不同值；幂等键重复返回同一回执，内容变化返回冲突。旧值迁移时以迁移时间作为保守可用时间，不伪造历史可用性。

## 页面操作

1. 在「处理与发布」创建表格或文本资产。
2. 「特征工程」配置字段、表达式或窗口，预览后注册定义；修改生成新版本，选择版本后物化并查看回执。
3. 「标注与智能打标」填写标签与关键词／实体字典，或问答列映射，创建任务并审核；通过后导出标注。
4. 「任务中心」展开领域专家表单，固定参数并预览，显式保存计划，再启动执行。执行详情可跳回实际领域任务。

五个能力 ID 为 `governance.feature-engineering`、`governance.tagging`、`governance.classification`、`governance.named_entity`、`governance.qa`。计划中的 `parameters` 是封闭的类型化分支，参与固定快照及提交摘要。特征任务必须提供真实 feature_id 和版本；不能由规划器编造。标注规则也必须来自明确配置。

受控提交以任务、计划版本、节点和尝试生成稳定身份，领域包装记录与实际任务事务落库。取消先写持久状态，特征首次提交在同一操作行锁内检查取消；已经提交的真实回执优先于后到的取消或失败。读取产物及其派生资产时重新检查受控来源、祖先可用性和当前权限。

## 接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/features/preview` | `{feature, limit}`，无写入预览 |
| GET | `/features/:id/versions/:version` | 固定定义版本 |
| POST | `/features/:id/materialize` | `{version}` + `Idempotency-Key`，返回回执 |
| POST | `/feature-values/query` | 实体、定义版本和 `as_of`，返回 `items` |
| GET | `/governance-executions/:id` | 只读解析受控包装和实际领域任务 |

既有创建／修正／审核接口保留。部署前运行 `cmd/migrate-db up`，本批新增迁移 `000015_feature_availability`。向下迁移会退回旧 JSON 数值精度且移除回执与可用时间，生产降级前需导出精确数据，并处理只在大小写上不同的实体ID，否则旧排序规则下可能出现唯一键冲突。

## 仍需后续实现

- 知识标签审核后正式应用到知识元数据，以及撤回与版本跟踪。
- 已审核样本生成不可变数据集版本，并供训练／评测消费。
- 更大规模的特征分区／水位、增量回填、撤回，以及 embedding 特征。
- 真实漂移基线、监控调度及自动治理返工。
- 语义标注模型与人工质量集；本批规则流程成功不代表标注准确率已经达标。

验证脚本：`apps/cognida-web/e2e/domain-specialists.spec.ts`；执行时必须显式配置隔离环境和 `COGNIDA_E2E_DOMAIN_SPECIALISTS=1`。真实数据库测试位于 `internal/service/governance/feature_engine_integration_test.go` 与 `internal/service/taskcontrol/governanceadapter/integration_test.go`。
