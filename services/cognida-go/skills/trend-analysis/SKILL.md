---
name: trend-analysis
description: 趋势分析技能，取带时间维度的序列并量化其增长/变化幅度与异常点。
when_to_use: 当用户问某指标随时间的走势、增长/下降、环比/同比、波动或异常（如“最近半年 GMV 的走势”“订单量是不是在下滑”）时使用；只要单个静态数字不属于此技能。
category: data
tags:
  - trend
  - time-series
  - analytics
  - anomaly
version: "1.0.0"
author: Cognida Team
allowed_tools:
  - get_schema
  - semantic_query
  - sql_execute
  - data_analysis
---

# 趋势分析

面向「带时间维度的序列 → 量化趋势」的场景：取出按时间聚合的指标序列，用 `data_analysis` 算出增长/变化幅度，必要时叠加异常检测，再给出有数据支撑的结论。区别于即席取数（要一个静态值）与归因（追问为什么变化）。

## 何时用 / 何时不用

- ✅ 用：某指标随时间怎么变、涨了还是跌了、涨跌幅多少、有没有异常波动。
- ❌ 不用：
  - 只要一个当前值/明细 → `text2sql-adhoc`
  - 追问「为什么会这样变」「谁在拉动」 → `attribution-analysis`

## 标准流程

```
1. 确定时间粒度与范围（日/周/月），取带时间维度的指标序列
   ↓  受治理指标优先 semantic_query，其余 get_schema 后 sql_execute
2. data_analysis(analysis_type=trend, options.value_col=<指标列>)
   → 拿到方向、斜率、增长幅度
   ↓
3. 必要时 data_analysis(analysis_type=anomaly) 检测异常点
   ↓
4. 结论：给出量化的增长/变化幅度（如“环比 +12.3%”），标注异常拐点
```

## 工具

- `semantic_query` / `sql_execute`：取按时间聚合、时间列升序排列的序列。
- `data_analysis`：
  - `analysis_type=trend`，`options.value_col` 指定数值列，得方向与幅度。
  - `analysis_type=anomaly`：检测序列中的异常点/突变。

## 编排（按上下文重量分流）

- 取数重则委派、轻则直接做——判据是上下文重量，不是步数：
  - 需多表定位、序列很长、或 SQL 可能多轮试错 → 委派 `SQLAuthor` 取回 `result_id`，再委派 `Analysis`
    做 `analysis_type=trend`（携 `result_id`）；子代理的中间往返不进你的上下文，你只收结论摘要。
  - 序列小、表已知、单查即得 → 直接按标准流程自己做，省委派往返。
- 委派携最小 scope（read），把 `result_id` 串进下一次委派的 `inputs.result_id` 做数据接力。

## 数据传递

- 取数返回的大结果以 `result_id` 承载；分析时传 `result_id`，**不要**把整段序列复制进参数。

## 渲染

- 时序数据首选一张 `LineChart`（数字用 `{"path":"/..."}` 绑定），配一句 `Callout` 写关键结论（涨跌幅、拐点）。
- 不要为一条趋势再叠一张重复同列数字的表格。

## 约束

- 序列必须按时间升序；口径（时间列、聚合方式）在结论里写明。
- 趋势判断以 `data_analysis` 的计算结果为准，不凭样本目测下结论。
- 只做趋势量化；「为什么变化」的根因归属属于 `attribution-analysis`。
