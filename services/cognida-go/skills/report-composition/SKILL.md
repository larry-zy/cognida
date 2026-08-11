---
name: report-composition
description: 综合报告生成技能，把多段取数（总览+趋势+分群等）整合成一份带洞察的结构化报告。
when_to_use: 当用户要“一份报告/综合分析/经营概览”，需要多个维度、多段查询并整合成趋势与洞察时使用；单个数字或单条查询不属于此技能。
category: data
tags:
  - report
  - analytics
  - composition
  - insight
version: "1.0.0"
author: Cognida Team
allowed_tools:
  - semantic_query
  - sql_execute
  - data_analysis
---

# 综合报告生成

面向「多段取数 → 整合 → 洞察」的报告场景：先拆解报告需要的指标块，分别取数，再汇总成带趋势与结论的结构化报告。

## 何时用 / 何时不用

- ✅ 用：要一份报告/经营概览，涉及**多个指标 + 趋势 + 分群/对比**，需要整合叙述。
- ❌ 不用：
  - 只要一个数或一张明细 → `text2sql-adhoc`
  - 只取单个治理指标 → `semantic-metric`

## 标准流程

```
1. 拆解报告结构：核心指标总览 / 趋势 / 分群 / 对比
   ↓
2. 逐块取数：能走语义层的用 semantic_query，其余用 sql_execute
   ↓
3. 计算派生指标（客单价、复购率等）并按时间聚合出趋势
   ↓
4. data_analysis 整合各段数据 + 洞察，输出结构化报告
```

## 工具

- `semantic_query`：按治理口径取核心指标（优先）。
- `sql_execute`：取语义层未覆盖的明细/派生计算（如按客户分组统计复购）。
- `data_analysis`：把多段结果与洞察整合为报告。

### 复购率示例（派生计算）
```sql
SELECT
  SUM(CASE WHEN cnt >= 2 THEN 1 ELSE 0 END) * 1.0 / COUNT(*) AS repurchase_rate
FROM (SELECT customer_id, COUNT(*) cnt FROM orders GROUP BY customer_id) t;
```

## 编排（多主题务必扇出隔离）

- 报告多主题、每主题各自展开取数+分析，上下文最重：用 `delegate_parallel` 并行委派 `Insight`（每主题一个），
  每个 Insight 内部自行取数+分析、只回传结论摘要 + `result_id`，你汇总各主题结论。**不要**在主循环里逐一
  展开各主题，否则中间数据会撑爆上下文。
- 仅单一主题的轻量报告可直接按标准流程 `data_analysis(analysis_type=report)` 一次成形。
- 每次委派携最小 scope（read）；汇总时以各 `Insight` 回传的 `result_id` 承载明细，正文只放结论与口径。

## 输出结构

```markdown
## 经营指标综合报告
### 核心指标总览   —— GMV / 订单数 / 客单价
### 趋势分析       —— 按月聚合的走势
### 分群 / 复购     —— 复购率、关键客群
### 关键洞察与建议
### 数据来源与口径
```

## 约束

- 每段取数标注口径与来源；派生指标写明公式。
- 优先复用语义层口径，保证报告内各指标一致、可对齐。
- 报告结论需有数据支撑，不臆断趋势成因。
