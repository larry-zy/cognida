---
name: "统计用户总数"
description: "统计用户总数"
when_to_use: "遇到类似问题时：用户需要获取用户总数"
category: experience
experimental: true
author: experience-distill
tags:
  - "数据查询"
  - "电商分析"
  - "用户统计"
disable_model_invocation: true
---

# 统计用户总数

> 本技能由会话经验自动沉淀生成，供遇到同类问题时复用。

## 适用场景

用户需要获取用户总数

## 操作指引

### 统计用户总数

当用户需要统计用户总数时，按以下步骤操作：

1. **查看语义模型**：调用 `list_semantic_models` 工具，检查是否有包含用户数或客户数指标的语义模型。
2. **选取指标**：如果存在，直接使用该指标（例如“客户数”），系统会自动生成对应的 SQL（如 `SELECT COUNT(DISTINCT customer_id) FROM ...`）。
3. **执行查询**：调用 `sql_execute` 执行生成的 SQL，获取用户总数。
4. **展示结果**：将结果以卡片或面板形式呈现，并注明数据来源与口径。

## 涉及工具

- `list_semantic_models`
- `sql_execute`

---
_来源会话：sess-3f6c8a7b，沉淀于 2026-07-21T02:33:58+08:00_
