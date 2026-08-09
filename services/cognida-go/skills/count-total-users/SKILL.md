---
name: "count-total-users"
description: "Count the total number of users"
when_to_use: "When the user needs the total number of users"
category: experience
experimental: true
author: experience-distill
tags:
  - "data-query"
  - "ecommerce-analytics"
  - "user-stats"
disable_model_invocation: true
---

# Count Total Users

> This skill was automatically distilled from session experience, for reuse on similar problems.

## When to use

The user needs the total number of users.

## Procedure

### Count total users

When the user needs to count the total number of users, follow these steps:

1. **Inspect semantic models**: Call the `list_semantic_models` tool and check whether any semantic model exposes a user-count or customer-count metric.
2. **Pick the metric**: If one exists, use that metric directly (e.g. "customer count"); the system automatically generates the corresponding SQL (e.g. `SELECT COUNT(DISTINCT customer_id) FROM ...`).
3. **Run the query**: Call `sql_execute` to run the generated SQL and obtain the total user count.
4. **Present the result**: Render the result as a card or panel, and state the data source and definition.

## Tools involved

- `list_semantic_models`
- `sql_execute`

---
_Source session: sess-3f6c8a7b, distilled at 2026-07-21T02:33:58+08:00_
