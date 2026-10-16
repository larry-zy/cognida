# 固定 HF 标注与数据集 benchmark

这是可重复的服务级测量，不是通过测试集调优的训练脚本。默认不会调用网络模型；
真实调用须显式选择 `model` 模式和 `COGNIDA_LABEL_BENCHMARK_LIVE=1`。

## 数据合同与预注册

每份 `label-config.json` 包含 `dataset`、40 位 `revision`、`language`（`en` 或
`zh-CN`）、`kind`（`classification` 或 `named_entity`）、`train_file`、`test_file`、
两个文件的 `*_sha256`，以及 `labels: [{id,name}]`。不接受可变的 `main` revision。
JSONL 每行包含唯一 `id`、`split`、原始 `text`。分类另含 `label`；NER 另含
`entities: [{label,start,end,text}]`，offset 是 Unicode codepoint、左闭右开。
下载器必须独立保存原文件摘要以及 token 拼接、IO 粗标签连续片段合并等转换说明。

先运行 `prepare`，使用 O_EXCL 新建 `protocol.json`。该文件包含数据、训练规则、
提示词和执行代码 SHA256、模型名、并发/调用限额与全部模型样本 ID/文本摘要。
后续模式必须复核这些摘要，不可覆盖旧协议再声称原先已预注册。
HF revision 是固定的数据版本；供应商模型名称可能是可变 alias，本测量不宣称固定模型权重。

- 英文规则是训练集词/双词，中文规则是固定 Unicode 2/3-gram；文档频率至少 2，
  class-vs-rest 平滑 log ratio 大于 0，每类最多 5 条。NER 使用训练集每类型最多
  50 个高频精确 surface。规则实际调用生产 `candidateFor`，不重新实现预测器。
- 分类模型预选按固定 seed 的 ID hash 排序，每类取 `min(2, 官方 test 类样本数)`。
  NER 模型预选为未在训练集出现过的文本中 hash 排名前 64 条。无长度/难度筛选。
- 本轮 Banking77 为 154 条，MASSIVE zh-CN 为 117 条，Few-NERD 为 64 条，共 335。
  MASSIVE test 的 `cooking_query=0`、`general_greet=1`，导致“每类 2 条”不可实现。
  这一协议可行性修订发生在 0 次模型调用时，另存 v2，未从 train 补样本、未重分配
  剩余 3 次额度；旧 Banking v1 预注册保留。
- 规则评分仍使用官方完整 test。训练/测试交集既可能按不同文本数计数，也可能按
  test 行数计数；报告的 `train_test_overlap_rows` 使用后者，重复出现的 test 文本
  不去重。另报 `unseen_text` 和 `train_overlap`，两者不替换官方分母。

## 真实执行与评分

`model` 模式使用真实 MySQL、真实本地对象存储、生产 `SuggestLabel` 的持久调用预留
和草稿校验。每条原文独立 text 资产，避免 table 格式的列名前缀改变 NER offset。
模型仅收到原文、公开 taxonomy 和固定通用指南，不收到 gold、训练例子或训练规则。
固定 20 秒/4096 tokens，每样本一次，总并发 2，无修复或重试。结构化请求仅复用
生产 `BoundedJSONOutput` 的既有 provider/model 白名单，其余适配器行为不变。

启动时 O_EXCL 写 `model.started`，每条安全类型结果与 usage 写 JSONL 并 `Sync`。
已启动目录不能再次运行 model；故障恢复只能读已有结果，不能自动重放付费调用。
不保存原始模型正文、推理文本、供应商错误正文、prompt 或密钥。有效结构化预测
仍作为结果保存，以便核对错误和指标。usage 缺失不猜算；费用未提供时为 null。
本轮未宣称测得真实供应商账单金额。

## 已发现的合同问题与分开的工程实验

首轮 v2 的 335 个测试样本全部实际调用并保留。合同错误分别是英文 146/154、
中文 92/117、NER 56/64；它们属于真实模型返回后的输出校验失败，不能报告成
“模型分类正确但仅格式问题”。它们在端到端指标中仍算失败/弃权。

其后单独授权的 12 次训练集诊断（两个分类语料原始前 6 条，gold 不入请求）发现：
6 个英文输出声明了错误 Unicode offset；7 个输出同时混入非空 tagging 字段。
这些计数允许重叠。必要原始模型响应只存本机 0600 文件，不复制到公开验收材料；
公开证据只保留固定错误类别、计数、摘要和 usage。

因此生产请求改为 `literal-anchors-v2`：每种任务只描述其允许分支；省略 start/end
时，服务端只对**精确、唯一**原文 literal 派生 Unicode 位置，并继续执行原完整
validator。重叠重复也算歧义；重复片段必须有正确的显式双 offset。显式 null、
单边 offset、错误显式位置、虚构/改写/大小写不一致片段、未知字段及非法非空分支
仍拒绝。没有自动重试、自动纠正错误声明或默默删非法标签。

旧 12 个响应离线重放仍是原来 8 拒绝、4 合法，旧分数不追认提升。随后新增的
12 次相同训练样本请求验证是独立工程回归（11 合法、1 精确匹配失败仍拒绝）。
最后另存 v3 协议，对**同一 335 个 ID、文本摘要和训练规则**执行一次完整复测，
不择优保留，不改变 taxonomy、gold、采样或阈值。前后结果分别报告；所有真实模型
预算为首轮 335 + 两轮训练诊断 24 + 复测 335，不包含本地 HTTP 测试替身。
v3 修改了结构化输出提示词与解码合同，不能描述成“提示词未变”。这些测试样本此前
已被观察，所以 v3 是固定样本上的工程回归，不是一个新的未观察泛化集。
`train-diagnostic-v2/offline-analysis.json` 解释新协议的省略 offset；不要把旧诊断器
对原始省略位置的检查误读为生产拒绝，生产 `error_code` 才是合同验收结果。

2026-10-16 的同样本结果如下。规则列使用原先全 test 运行中已保存的预测，按模型
预选 ID 离线重新评分，没有重新拟合规则。macro-F1 均包含完整 schema 的所有标签。

| 语料 / 样本数 | 同样本规则 macro-F1 | 模型 v2 macro-F1 | 模型 v3 macro-F1 | v3 coverage | v2 / v3 合同失败 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Banking77 / 154 | 33.94% | 4.33% | 71.49% | 98.05% | 146 / 0 |
| MASSIVE zh-CN / 117 | 27.78% | 15.39% | 78.79% | 95.73% | 92 / 0 |
| Few-NERD / 64 | 7.35% | 1.19% | 48.04% | 67.19% | 56 / 9 |

NER 的 9 个合同失败保留在分母；held-out runner 未捕获原始模型正文，因此不能逐条
追认它们属于哪种 wire/span 错误。NER 的 exact-span 质量和覆盖率仍有限，不能由
分类合同改善推导为所有标注能力已可用于无人审核发布。
六份 held-out journal 分别恰有 154/117/64 条，v2、v3 都与唯一预选 ID 及实际调用
计数一致。加上两个独立 12 条训练诊断，总计 694 次实际逻辑调用；观测 usage 为
965049 prompt tokens、69201 completion tokens、1034250 total tokens，费用为 null。

所有选中样本，包括超限、超时、非法 JSON/引用/span 与供应商失败，均留在分母。
分类多标签/uncertain 为弃权，不挑一个标签充作预测。NER 对所有合法返回 span 做
严格 `(type,start,end)` 集合匹配，重复预测额外计 FP；uncertain 的合法部分 span
仍计分，`coverage` 单独表示 matched。完整 schema 的 macro 指标和仅有 gold support
的 macro-F1 分开提供；准确率、micro P/R/F1、弃权、歧义、合同错误、执行错误及
每样本 service latency/分位数均保留。小样本结果不能代表总体模型能力保证。

`oracle` 模式只取预注册样本前三条，经过真实 `Preannotate` 与生产规则结果比对，
然后以 HF gold 做明确标记的人工修改/审核 fixture。HTTP 验证未审核不可发布、
已审核固定版本、幂等重放、后续标注修改不改变版本、另一用户与来源撤权拒读。
它证明审核发布/固定快照机制，**不计入自动标注准确率**。身份是明确测试 principal，
不宣称经过真实登录；同步路径拒绝任何意外队列任务，不冒充 worker 或浏览器验收。

## 重跑

在 Go 服务目录，先安全加载现有模型配置与独立数据库环境。数据库名必须以
`cognida_review_` 开头，先通过正式 migrations 建库；测试不 AutoMigrate。

```bash
export COGNIDA_LABEL_BENCHMARK_CONFIG=/absolute/data/label-config.json
export COGNIDA_LABEL_BENCHMARK_OUTPUT=/absolute/evidence/new-run
COGNIDA_LABEL_BENCHMARK_MODE=prepare go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1
COGNIDA_LABEL_BENCHMARK_MODE=rules go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -timeout 20m
COGNIDA_LABEL_BENCHMARK_MODE=oracle go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1
# 仅在审查协议及确认真实调用预算后执行一次：
COGNIDA_LABEL_BENCHMARK_MODE=model COGNIDA_LABEL_BENCHMARK_LIVE=1 go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -timeout 90m
```

每个模式单独保留日志。不得删除失败结果或改提示词/阈值后覆盖同一批。
真实 DB fixture 仅删除自己的随机 tenant；临时对象目录及 httptest server 自动清理。

同样本规则评分也有保留在仓库中的离线入口：
`label_benchmark_comparison_integration_test.go` 的 `TestHFLabelPairedRuleScores`。
输入目录包含三组 `<dataset>/v2/rules-report.json`、`<dataset>/v3/protocol.json`，
以及 protocol 指向的原始 JSONL。它复核数据文件与每条选中原文摘要，只读取既有预测；
不访问数据库、不拟合、不调用模型。每组输出为 `<dataset>/v3/paired-rules-report.json`。
输出使用独占创建；重复执行需复制输入证据到一个尚无该输出的新目录，保留旧结果。

```bash
COGNIDA_LABEL_COMPARISON_ROOT=/absolute/evidence/new-copy \
  go test -tags=integration ./internal/service/governance \
  -run '^TestHFLabelPairedRuleScores$' -count=1
```
