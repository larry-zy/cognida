---
name: text2sql-adhoc
description: 即席取数技能，把一句话的具体数据问题翻译成 SQL 并执行返回结果。
when_to_use: 当用户要查一个具体的数字、明细或清单（如“上个月有多少订单”“列出金额 top10 的客户”），且不涉及治理指标口径、也不需要把多段查询整合成报告时使用。
category: data
tags:
  - text2sql
  - sql
  - ad-hoc
  - query
version: "1.0.0"
author: Link Team
allowed_tools:
  - get_schema
  - sql_execute
model: inherit
context_mode: inline
effort: 6
---

# Text2SQL 即席取数

面向「单个具体问题 → 一条查询 → 一个结果」的词法 NL2SQL 场景。区别于综合报告（多段整合）与语义指标取数（走治理口径）。

## 何时用 / 何时不用

- ✅ 用：一句话能说清的取数，答案是一个数、一张明细表或一个排行。
- ❌ 不用（改走别的 skill）：
  - 问的是**有口径的指标**（GMV、营收、客单价、复购率、客户数）→ `semantic-metric`
  - 要的是**一份综合报告 / 多维度汇总** → `report-composition`

## 标准流程

```
1. get_schema 探明相关表结构与字段口径
   ↓
2. 构造最小可用 SQL（只查需要的列，带 LIMIT）
   ↓
3. sql_execute 执行
   ↓
4. 校验结果（空值/量级是否合理），说明数据来源与口径
```

## 工具

### get_schema — 探表结构
```json
{ "tool": "get_schema", "parameters": { "table": "orders" } }
```
不确定表名时先不传 `table`，拿到表清单再定位。

### sql_execute — 执行查询
```json
{ "tool": "sql_execute", "parameters": { "query": "SELECT COUNT(*) FROM orders WHERE created_at >= '2026-07-01'" } }
```

## 约束

- 先 `get_schema` 再写 SQL，不要凭表名/字段名猜测。
- 只读取所需列，避免 `SELECT *`；大表务必带 `LIMIT`。
- 结果回答里标注：命中的表、时间范围、口径（如“订单数=去重 order_id”）。
- 只做查询（SELECT）；写操作不属于本技能范围。
