---
name: doc-qa
description: 文档问答技能，从知识库中语义检索相关文档片段并据此作答，附带来源引用。
when_to_use: 当用户的问题需要从知识库/文档中查找答案（如“文档里怎么说的”“检索一下相关资料”），依赖非结构化文本内容而非数据库取数时使用。
category: retrieval
tags:
  - rag
  - doc-qa
  - retrieval
  - knowledge-base
version: "1.0.0"
author: Link Team
allowed_tools:
  - rag_query
model: inherit
context_mode: inline
effort: 5
---

# 文档问答（Doc QA）

面向「从知识库检索文档片段并据此回答」的场景。区别于图谱检索（查实体关系）与取数（查数据库）。

## 何时用 / 何时不用

- ✅ 用：答案在文档/资料里，需要语义检索文本片段后归纳作答。
- ❌ 不用：
  - 查实体之间的关系 / 血缘 / 上下游 → `graph-retrieval`
  - 查数据库里的数字/明细 → `text2sql-adhoc` / `semantic-metric`

## 标准流程

```
1. 理解问题意图，提炼检索 query
   ↓
2. rag_query 语义检索相关文档片段
   ↓
3. 依据检索内容归纳作答，不足则换 query 重检
   ↓
4. 标注引用来源（文档/片段）
```

## 工具

### rag_query — 知识库语义检索
```json
{ "tool": "rag_query", "parameters": { "query": "退货政策的时限是多少天" } }
```

## 约束

- 回答**只基于检索到的内容**；检索为空或不足时如实说明，不要编造。
- 命中片段与用户问题弱相关时，改写 query 再检一次。
- 给出来源引用，便于溯源核对。
