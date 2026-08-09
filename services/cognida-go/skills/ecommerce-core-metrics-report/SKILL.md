---
name: "ecommerce-core-metrics-report"
description: "Generate a comprehensive report of core e-commerce operating metrics (GMV, orders, average order value, repurchase)"
when_to_use: "When the user needs a comprehensive report of core e-commerce operating metrics (GMV, orders, average order value, repurchase rate)"
category: experience
experimental: true
author: experience-distill
tags:
  - "ecommerce-analytics"
  - "metrics-report"
  - "repurchase-rate"
  - "trend-analysis"
disable_model_invocation: true
---

# E-commerce Core Operating Metrics Report

> This skill was automatically distilled from session experience, for reuse on similar problems.

## When to use

The user needs a comprehensive report of core e-commerce operating metrics (GMV, orders, average order value, repurchase).

## Procedure

### E-commerce core operating metrics report

1. **Discover available data**: First look for a governed semantic model (e.g. an e-commerce sales model) and pull data from it directly.
2. **Align terminology**: Confirm the meaning of the model's fields (e.g. GMV = revenue, order count = number of orders), and define derived metrics as needed (e.g. average order value = GMV / order count).
3. **Pull the core-metrics overview**: Extract GMV, order count, and average order value from the semantic model, and fetch the underlying order-level detail.
4. **Compute repurchase rate**: Group orders by customer ID and count purchases; formula: repurchasing customers (≥ 2 purchases) / total customers.
5. **Trend analysis**: Aggregate the core metrics over time (by month) to obtain the trend series.
6. **Generate the report**: Call the data-analysis engine (e.g. `data_analysis`) to combine the data with insights and produce the comprehensive report.

## Tools involved

- `data_analysis`

---
_Source session: sess-149374f1, distilled at 2026-07-20T04:28:27+08:00_
