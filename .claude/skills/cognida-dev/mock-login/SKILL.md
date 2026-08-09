---
name: mock-login
description: Cognida 项目测试环境模拟登录技能，使用数据库已有用户数据直接登录，无需密码验证
when_to_use: 当需要测试需要用户身份的 API、功能测试需要特定角色、或自动化测试需要快速登录时使用
category: testing
tags:
  - authentication
  - testing
  - login
version: "1.0.0"
---

# Mock Login Skill

Cognida 项目测试环境模拟登录技能。

## 可用测试用户

根据 Cognida 项目的用户数据库，以下用户可用于模拟登录：

### 租户 1 (tenant_id=1)

| 邮箱 | 用户名 | 用途 |
|------|--------|------|
| admin@example.com | admin | 管理员账号 |
| testuser@example.com | testuser | 普通用户测试 |

### 如何使用

当需要测试需要登录的 API 时，告知用户标识即可：

```
用户: 测试用户资料更新 API
Agent: 我需要先登录。使用 admin@example.com 登录...
```

## 数据库表结构

users 表关键字段：
- id: 用户ID
- tenant_id: 租户ID  
- username: 用户名
- email: 邮箱
- status: 状态 (1=正常, 0=禁用)

## 查询用户

如需查询可用用户：

```sql
SELECT id, username, email, tenant_id, status 
FROM users 
WHERE status = 1 
ORDER BY tenant_id, id;
```

## 注意事项

- 此技能仅用于测试环境
- 生产环境禁止使用
- 登录后获取的 token 可用于 API 测试
