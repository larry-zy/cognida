---
name: graph-retrieval
description: 图谱与血缘检索技能，查询实体之间的关系、上下游依赖与数据血缘。
when_to_use: 当问题是关于实体间关系、依赖、影响面或数据血缘（如“这个字段的上游来自哪”“A 和 B 怎么关联”“改动这张表影响谁”）时使用，走知识图谱而非向量文档检索。
category: retrieval
tags:
  - graph
  - lineage
  - relationship
  - knowledge-graph
version: "1.0.0"
author: Link Team
allowed_tools:
  - graph_query
model: inherit
context_mode: inline
effort: 6
---

# 图谱 / 血缘检索

面向「查实体关系与数据血缘」的场景：沿图上的边遍历上下游、依赖与影响面。区别于文档问答（查文本片段）。

## 何时用 / 何时不用

- ✅ 用：问的是**关系/连接/依赖/血缘/影响面**——上游来源、下游消费、A 与 B 如何关联、改动波及谁。
- ❌ 不用：
  - 问题答案在文档正文里 → `doc-qa`
  - 要的是数据库里的统计数字 → `text2sql-adhoc` / `semantic-metric`

## 标准流程

```
1. 明确起点实体与关系方向（上游/下游/双向）
   ↓
2. graph_query 沿关系边检索（可限定跳数/关系类型）
   ↓
3. 归纳路径与影响面，必要时二次下钻
   ↓
4. 结构化呈现关系链 / 血缘路径
```

## 工具

### graph_query — 图谱关系检索
```json
{ "tool": "graph_query", "parameters": { "entity": "orders.amount", "direction": "upstream" } }
```
按实际参数 schema 传入起点、方向与跳数；不确定时先小跳数探路再扩展。

## 约束

- 先确定方向与范围，避免全图无界遍历。
- 呈现关系链时标明每一跳的关系类型与来源。
- 图谱未启用或实体不在图上时如实说明，不臆造关系。
