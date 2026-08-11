---
name: attribution-analysis
description: 归因/根因分析技能，把指标的期间变化拆解到候选维度，定位驱动因子并给出置信与下钻建议。
when_to_use: 当用户追问某指标「为什么变化」「根因是什么」「谁在拉动/拖累」「按维度拆解贡献」（如“这个月 GMV 为什么跌了”“哪些品类拉低了复购”）时使用。
category: data
tags:
  - attribution
  - root-cause
  - driver
  - analytics
version: "1.0.0"
author: Cognida Team
allowed_tools:
  - get_schema
  - semantic_query
  - sql_execute
  - data_analysis
---

# 归因 / 根因分析

面向「指标变了 → 是什么在驱动」的场景：取「期间 × 候选维度 × 指标」的行集，用 `data_analysis(analysis_type=attribution)` 把期间变化拆解到各维度，得到驱动因子（drivers）、洞察与置信，再按下钻建议校验后定论。区别于趋势分析（量化怎么变）与即席取数（要一个值）。

## 何时用 / 何时不用

- ✅ 用：追问「为什么变化」「根因」「谁在拉动/拖累」「各维度的贡献拆解」。
- ❌ 不用：
  - 只问「涨了还是跌了、幅度多少」 → `trend-analysis`
  - 只要一个数/明细 → `text2sql-adhoc`

## 标准流程

```
1. 明确被归因的指标、对比的两个期间、候选拆解维度（如品类/地区/渠道）
   ↓
2. 取「期间 × 候选维度 × 指标」行集
   ↓  受治理指标优先 semantic_query，其余 get_schema 后 sql_execute
3. data_analysis(analysis_type=attribution, result_id=<行集>,
     options={value_col:<指标列>, period_col:<期间列>})
   → 得 drivers（各维度贡献）、insight、confidence、drill_down
   ↓
4. 以 drivers/insight 为准给结论，标注 confidence；
   按 drill_down 建议对可疑驱动因子下钻校验后再定论
```

必要时用 `analysis_type=comparison`（两期对比）或 `correlation`（相关性）做补充验证。

## 工具

- `semantic_query` / `sql_execute`：取带期间列与候选维度列的指标行集。
- `data_analysis`：
  - `analysis_type=attribution`，`options.value_col`（指标列）+ `options.period_col`（期间列），得 drivers/confidence/drill_down。
  - `analysis_type=comparison` / `correlation`：补充验证驱动因子。

## 编排（按上下文重量分流）

- 归因通常上下文重（大行集 + 多轮下钻），默认委派：`SQLAuthor` 取「期间 × 候选维度 × 指标」行集回传
  `result_id`，`Analysis` 做 `analysis_type=attribution`；你串 `result_id`、综合各轮下钻结论并标注 confidence。
  子代理内部的试错与大结果不进你的主上下文。
- 仅当行集小、单轮即可定论时才直接做，省委派往返。判据是上下文重量，不是步数。
- 每次委派携最小 scope（read）；drivers 落的新 `result_id` 再串进下钻委派的 `inputs.result_id`。

## 数据传递

- 取数返回大结果以 `result_id` 承载；归因分析传 `result_id`，drivers 结果也会落新的 `result_id` 供下钻取回，**不要**复制大结果进参数。

## 渲染

- 驱动因子排行首选一张 `BarChart` 或 `Table`（数字用 `{"path":"/..."}` 绑定），配 `Callout` 写根因结论与置信。
- 两数值列相关性可用 `ScatterChart`。不要叠重复同列数字的多余组件。

## 约束

- 归因结论以 `data_analysis` 的 drivers/insight 为准，并显式标注 confidence，不臆断成因。
- confidence 低或存在混杂时，先按 drill_down 建议下钻校验，再给定论。
- 拆解口径（指标列、期间列、维度粒度）在结论里写明。
