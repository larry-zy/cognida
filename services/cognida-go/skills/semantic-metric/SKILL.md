---
name: semantic-metric
description: 语义指标取数技能，先查治理好的指标语义模型，再按统一口径取指标，避免对物理表猜测。
when_to_use: 当用户问的是有口径的经营指标（如 GMV、营收、客单价、复购率、客户数、转化率）时使用；优先走语义层拿到一致口径，而非直接拼物理表 SQL。
category: data
tags:
  - semantic-layer
  - metric
  - governance
  - kpi
version: "1.0.0"
author: Cognida Team
allowed_tools:
  - semantic_models
  - semantic_query
---

# 语义指标取数

面向「按治理口径取经营指标」的场景：先看有哪些已治理的指标/维度，再用语义名提交结构化取数请求，口径由语义层保证一致。

## 何时用 / 何时不用

- ✅ 用：问的是指标（GMV、营收、客单价、复购率、客户数、转化率、留存……），需要**一致口径**、可复用、可对齐报表。
- ❌ 不用：
  - 只是查一条明细/清单、没有口径概念 → `text2sql-adhoc`
  - 要整合成多维度报告 → `report-composition`（其内部也可调用语义取数）

## 标准流程

```
1. semantic_models 列出受治理的指标/维度目录（含口径、同义词）
   ↓
2. 对齐术语：把用户口语（“营业额”）映射到语义名（“GMV”）
   ↓
3. semantic_query 用语义名提交 metrics/dimensions 结构化取数
   ↓
4. 结果注明口径来源（语义模型名 + 指标口径）
```

## 工具

### semantic_models — 列出指标语义目录
```json
{ "tool": "semantic_models", "parameters": { "model": "" } }
```
`model` 留空列出全部生效模型；拿到目录后按 `name`/`synonyms` 对齐术语。

### semantic_query — 结构化取数
```json
{
  "tool": "semantic_query",
  "parameters": {
    "model": "电商销售模型",
    "metrics": ["GMV", "订单数"],
    "dimensions": ["月份"]
  }
}
```

## 约束

- 一定先 `semantic_models` 确认指标存在且口径匹配，再 `semantic_query`。
- 目标指标不在语义目录里时，**回退**到 `text2sql-adhoc`（get_schema + sql_execute）走词法 NL2SQL，并在回答里说明“无治理口径，按物理表推断”。
- 计算型指标（如客单价=GMV/订单数）优先取语义层已定义口径，其次才手工推导并标注公式。
