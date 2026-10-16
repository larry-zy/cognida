# Hugging Face 公开数据效果基准

协议日期：2026-10-16。目标是度量实际生产代码路径的效果和边界，不将公开数据成绩当作业务投产批准。样本选择、规则生成、漂移阈值先于结果固定；不通过查看测试标签调整规则、prompt 或阈值。

## 数据与溯源

`datasets.lock.json` 固定 Hugging Face 仓库提交、文件 URL、字节数和 SHA256。下载器只读取 JSONL、GZip、CSV、Parquet 和数据卡，不执行远端 dataset script。`prepare.py` 校验原文件摘要，再保存归一数据、转换规则与派生摘要。

| 数据 | 来源与许可记录 | 用途 |
| --- | --- | --- |
| BANKING77 | [MTEB 镜像](https://huggingface.co/datasets/mteb/banking77)，原始 [PolyAI](https://huggingface.co/datasets/PolyAI/banking77) 数据卡声明 CC BY 4.0；镜像卡另标 MIT，保留二者来源，不据此抹掉原数据要求 | 英文 77 类分类、文本清洗保真 |
| MASSIVE zh-CN | [MTEB 中文子集](https://huggingface.co/datasets/mteb/amazon_massive_intent)，原始 [Amazon MASSIVE](https://huggingface.co/datasets/AmazonScience/massive) 声明 CC BY 4.0；镜像卡另标 Apache 2.0 | 中文 60 类分类、文本清洗保真 |
| Few-NERD supervised | [DFKI-SLT](https://huggingface.co/datasets/DFKI-SLT/few-nerd)，CC BY-SA 4.0 | 英文 8 粗类实体识别 |
| Wine Quality red/white | [CodeSignal](https://huggingface.co/datasets/codesignal/wine-quality)，CC BY 4.0，保留原始数据来源说明 | 表格保真、特征计算与漂移对照 |

不在源码库提交整份原始或转换语料。报告保留归因链接、revision 和摘要；原始数据下载到调用方指定目录。Few-NERD 用单个 ASCII 空格连接 tokens，偏移为 Unicode codepoint；连续相同非 O 的粗粒度 IO 标签构成实体，不能恢复原句空白或相邻同类实体的隐藏边界。原始 tokens/tags 仍保留。

官方 train/test 保持原样。相同正文跨划分重合单独计数，并报告 unseen-text 子集结果；不为了提高分数删除难题、重复项、无实体样本或请求失败。公开基准可能已进入基础模型预训练，本实验不能证明没有模型训练污染，也不等同私有业务泛化能力。

## 清洗与采集保真

对中英文全部 train/test 原文分批走实际 lifecycle HTTP 计算合同，每批不超过输入上限。原始行的字段、类型、顺序和内容单独校验。再派生明确标为人工扰动的 working_text 首尾空白、精确复制和空行；保留原文，使用独立 oracle 检查应保留与应删除行、来源映射、误删/漏删、清洗恢复及重复执行结果。

额外类型控制包括布尔、整数、浮点、空值、空字符串、带前导零字符串、大整数和精确 DECIMAL 字符串。确定性保真门槛为零意外删改；任何一次失败保留前后证据。人工扰动结果不表示自然业务脏数据的误删或脱敏召回率。通过 Go HTTP→真实 MySQL/Redis→Python HTTP 的跨服务路径另查输入、任务、清洗与结果持久化。

## 分类、实体与数据集

- 英文规则仅用 train 的 unigram/bigram，中文仅用 train 的连续 Unicode 2/3gram；固定文档频率和类间比值排序，每类最多 5 个关键词。NER 词典仅取 train，每类最多 50 个高频 surface，遵守生产 schema 上限。
- 官方 test 全量跑实际规则实现。分类多命中记 ambiguous/abstention，不替规则选一个正确标签；NER 严格比较 `(label,start,end)`，空 gold 与空预测另行说明。
- 真实模型分类按固定哈希每类最多 2 条：BANKING77 154 条、中文 MASSIVE 117 条；NER 从未在 train 出现的正文中固定选 64 条。合计 335 次逻辑建议调用、并发最多 2。预先保存样本 ID、正文摘要、规则和 prompt 摘要；失败、超时、非法输出均计入原分母，不择优重试。
- 采样前置核验修正：最初计划中文每类 2 条共 120 条，但官方 test 中 `cooking_query` 为 0 条、`general_greet` 为 1 条。零模型调用时将采样改为 `min(2, test 类样本量)`，仍报告全部 60 类 macro 和实际有支持的类别指标；不从 train 补样、不将空余额度分配给其他类别。总预算由 338 收紧为 335，原始预注册历史保留。
- 原始分类、NER 的质量分数与 schema/evidence 约束错误分开。报告 macro/micro P/R/F1、accuracy、coverage、abstention、契约/执行错误、延时分位数；实际不可取得的 token 或货币费用填 null，不能估算后冒充实收。
- 数据集发布用少量明确标注为 oracle fixture 的人工正确标签走真实审核与不可变版本流程；该成绩不进入自动标注准确率。正式业务质量门槛需用业务标注集确定，本次不设一个看结果后才决定的“生产合格线”。

## 特征与漂移

Wine 6,497 行保留原始十进制文本；用独立精确有理数 oracle 核验表达式、窗口与物化值。原数据没有事件时间和实体，测试添加的 ID/时间明确为合成元数据。另以明确重复扩展的 10,000 行压力输入验证既有上限，不能冒充 10,000 个独立原始观测；不得放宽 10 万单元格限制。

真实 MySQL/HTTP 核验全量、增量、回填、撤回、时点查询、源撤权和回执。测量本机耗时与结果，不能据此宣称生产 SLA 或特征带来的下游预测收益。

漂移协议固定 seed=20261016、effect threshold=0.2、minimum=20、采样最多 1,000 条：

- 参考 1,000 行包含 250 red＋750 white；20 个独立稳定窗口各 200 行，保持 50 red＋150 white，记录 false-alarm 窗口比例。
- 原样与置换应零距离。red/white 各 1,000 行的比较只解释为真实群组差异，不充当天然漂移事件 gold。
- 在同一批真实行副本施加明示扰动：alcohol +5；渐进 +0.1/+0.25/+0.5/+0.75/+1/+1.5/+2（前置 3 个不变窗口）；alcohol 50% 缺失；删除 density；显式类型变化。
- 用独立 CDF、频数和缺失率 oracle 验证统计量，报告各窗口样本量、检出比例、首次告警窗口和耗时。效应阈值不是 p 值；合成检出不代表业务误报/漏报水平。固定快照监控不宣称自动获取最新上游数据。

## 模型输出合同的独立工程回归

第一轮 `v2` 的 335 次模型结果永久保留，不能覆盖。因大量输出被生产校验拒绝，另注册训练集固定前 6 条英文＋6 条中文的诊断（12 次），不含 heldout、gold 不入模型。诊断发现原文字面片段存在但字符位置错误，以及分类任务输出互斥的标签分支；原始响应仅保存在本机 0600 文件，不进入仓库报告。

工程修复为 `literal-anchors-v2` 输出合同：按任务类型约束分支，仅当 start/end 均省略且原文 literal 精确唯一时，由服务端派生 Unicode 位置，再经过完整既有校验。错误显式位置、单边/null 位置、重复歧义、伪造片段、未知字段和非法分支继续拒绝。分类语义、标签 schema、评分和测试样本选择保持不变。

后续验证单独注册：同 12 条训练样本最多 12 次回归；修复后 `v3` 对**同一批固定 335 条 heldout**各调用一次，最多并发 2，不额外抽题、不择优重试。各实验分别保留代码/prompt/样本摘要、完整分母、费用可用性与失败。第一次 heldout 已经被观测，因此 `v3` 是工程修复的同集回归，不冒充从未观测的全新泛化测试。新实验的上限与第一次 335 分开记录，总计最多 694 次真实请求。

## 运行与结果判定

仅使用专用 `cognida_review_` 数据库和任务所有的服务；记录并精确清理测试对象与进程。单元测试验证 benchmark 本身的指标与 oracle，真实存储/API 验证业务实现。生产代码修复保留修复前证据并独立 Review，涉及 heldout 的结果不得被静默覆盖。

最终报告区分：执行成功、确定性正确性、统计质量、当前容量边界和仍未验证的生产条件。公开 benchmark 不替代真实中文业务实体/关系、复杂文件解析、自然脏数据、真实上游持续采集、生产负载和长期运行验收。
