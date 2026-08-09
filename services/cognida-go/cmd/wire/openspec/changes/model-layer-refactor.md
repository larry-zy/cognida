# Model Layer Refactor

**Status**: In Progress
**Priority**: High
**Created**: 2025-06-05

## Summary

重构 `internal/model` 层，解决架构层级混乱、实体重复定义、DTO 混入等问题。

## Background

当前 model 层存在以下问题：
1. 混合了 Domain 层和 Application 层内容（应用服务接口）
2. 实体定义重复（account vs user/tenant，chat vs conversation）
3. DTO 混入 model 层（Request/Response 类型）
4. entity.go 文件内容混杂（配置、工具、研究类型）

## Goals

1. 清晰分层：model 层只包含领域实体、值对象、Repository 接口
2. 消除重复：删除重复的实体定义
3. 规范结构：统一目录结构和命名规范
4. DTO 继承：所有 Request/Response 继承 handler 层基类

## Changes

### 1. 删除重复定义

- [ ] 删除 `internal/model/account/` 目录（与 `user/` 和 `tenant/` 重复）
- [ ] 删除 `internal/model/chat/` 目录（与 `conversation/` 重复）

### 2. 移出 DTO 文件

- [ ] 删除 `internal/model/conversation/types.go`（DTO 移至 handler）
- [ ] 删除 `internal/model/llm/types.go`（DTO 移至 handler）
- [ ] 删除 `internal/model/task/types.go`（DTO 移至 handler）
- [ ] 拆分 `internal/model/user/rbac/types.go`（实体保留，DTO 移至 handler）

### 3. 拆分混杂内容

- [ ] 拆分 `internal/model/agent/entity.go`
  - entity.go - 仅保留实体定义
  - config.go - 配置类型（AgentConfig, HookConfig, SearchConfig, MemoryConfig）

### 4. 删除应用服务接口

- [ ] 删除 `internal/model/agent/service.go`（应用服务接口移至 application 层）
- [ ] 删除 `internal/model/memory/service.go`
- [ ] 删除 `internal/model/user/service.go`
- [ ] 删除 `internal/model/tenant/service.go`

### 5. 规范 DTO 结构

所有 DTO（Request/Response）继承 handler 层基类：

```go
// BaseRequest 基础请求
type BaseRequest struct {
    TenantID int64 `json:"-"`
    UserID   int64 `json:"-"`
}

// BaseResponse 基础响应
type BaseResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}
```

### 6. 统一目录结构

```
internal/model/{module}/
├── entity.go       # 实体定义
├── repository.go   # Repository 接口
├── errors.go       # 错误定义
└── types.go        # 领域类型（枚举/配置/值对象）
```

## Impact

### Positive

- 清晰的分层架构
- 消除重复定义，降低维护成本
- 规范的代码结构

### Risk

- 大量文件变更，可能影响现有代码
- 需要更新 import 路径

## Rollback Plan

可通过 git revert 回滚所有变更。

## Tasks

1. 删除 account/ 和 chat/ 目录
2. 移出 DTO 文件
3. 拆分 agent/entity.go
4. 删除应用 service.go 文件
5. 更新所有 import 引用
6. 测试验证
