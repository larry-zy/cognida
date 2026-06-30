# Session 消息不显示问题修复记录

## 问题描述

前端调用 `/api/v1/sessions/{id}/detail` 接口后，消息列表始终为空，即使数据库中有消息记录也无法显示。

## 现象

### 前端表现
- `loadMessages` 函数调用后，`messages.value` 始终为空数组 `[]`
- 控制台输出 `loadMessages messages: []`

### 后端日志
```
📦 [GetSessionDetail] 返回数据: Session=e4a8fe91-64e8-47da-9565-7fb312fba024, Messages=0
```

## 根本原因

### 1. Service 层未实现消息查询

**文件**: `internal/application/service/session.go`

```go
// GetSessionDetail 获取会话详情（包含消息）
func (s *SessionService) GetSessionDetail(ctx context.Context, id string) (*types.SessionDetailResponse, error) {
	session, err := s.sessionRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return &types.SessionDetailResponse{
		Session:  s.toSessionResponse(session),
		// 消息列表需要通过 MessageService 获取
		Messages: []*types.MessageResponse{},  // ❌ 永远返回空数组！！！
	}, nil
}
```

**问题**: 注释写着"消息列表需要通过 MessageService 获取"，但实际代码直接返回了空数组，根本没有查询数据库。

### 2. Service 缺少 MessageRepository 依赖

```go
type SessionService struct {
	sessionRepo interfaces.SessionRepository
	// ❌ 缺少 messageRepo
}
```

## 解决方案

### 步骤 1: 修改 SessionService 结构体

**文件**: `internal/application/service/session.go`

```go
// SessionService 会话服务实现 - 多租户版本
type SessionService struct {
	sessionRepo interfaces.SessionRepository
	messageRepo interfaces.MessageRepository  // ✅ 新增
}

// NewSessionService 创建会话服务实例
func NewSessionService(sessionRepo interfaces.SessionRepository, messageRepo interfaces.MessageRepository) interfaces.SessionService {
	return &SessionService{
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,  // ✅ 新增
	}
}
```

### 步骤 2: 实现 GetSessionDetail 消息查询

```go
// GetSessionDetail 获取会话详情（包含消息）
func (s *SessionService) GetSessionDetail(ctx context.Context, id string) (*types.SessionDetailResponse, error) {
	session, err := s.sessionRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// ✅ 查询消息列表（获取所有消息，不分页）
	messageEntities, _, err := s.messageRepo.FindBySessionID(ctx, id, 1, 10000)
	if err != nil {
		return nil, fmt.Errorf("查询消息列表失败: %w", err)
	}

	// ✅ 转换为 MessageResponse
	messages := make([]*types.MessageResponse, len(messageEntities))
	for i, msg := range messageEntities {
		messages[i] = &types.MessageResponse{
			ID:                  msg.ID,
			RequestID:           msg.RequestID,
			SessionID:           msg.SessionID,
			Role:                msg.Role,
			Content:             msg.Content,
			KnowledgeReferences: msg.KnowledgeReferences,
			AgentSteps:          msg.AgentSteps,
			ToolCalls:           msg.ToolCalls,
			IsCompleted:         msg.IsCompleted,
			TokenCount:          msg.TokenCount,
			CreatedAt:           msg.CreatedAt,
		}
	}

	return &types.SessionDetailResponse{
		Session:  s.toSessionResponse(session),
		Messages: messages,  // ✅ 返回真实消息
	}, nil
}
```

### 步骤 3: 更新 main.go 依赖注入

**文件**: `cmd/server/main.go`

```go
// ❌ 修改前
sessionService := repoService.NewSessionService(sessionRepo)

// ✅ 修改后
sessionService := repoService.NewSessionService(sessionRepo, messageRepo)
```

## 相关文件

| 文件 | 修改内容 |
|------|----------|
| `internal/application/service/session.go` | 添加 messageRepo 字段、修改构造函数、实现 GetSessionDetail 消息查询 |
| `cmd/server/main.go` | 更新 NewSessionService 调用，传入 messageRepo |
| `internal/handler/session.go` | JSON 结构修复（见下方） |

## 额外发现：JSON 结构问题

### 问题 2: Handler 层 JSON 序列化问题

**文件**: `internal/handler/session.go`

**现象**:
```json
// ❌ 错误：messages 在根级别
{
  "code": 0,
  "data": {
    "session": {...}
  },
  "messages": []  // ← 位置错误
}
```

**原因**: 使用 `c.JSON(http.StatusOK, gin.H{"data": resp})` 时，Gin 会扁平化嵌套结构。

**修复**:
```go
// ❌ 修改前
c.JSON(http.StatusOK, gin.H{
    "code":    0,
    "message": "成功",
    "data":    resp,  // gin 会扁平化
})

// ✅ 修改后
c.JSON(http.StatusOK, gin.H{
    "code":    0,
    "message": "成功",
    "data": gin.H{  // 手动构建嵌套结构
        "session":  resp.Session,
        "messages": resp.Messages,
    },
})
```

## 教训总结

1. **注释不是代码** - 注释写着"需要获取消息"但实际没实现，这种情况很常见但容易被忽略

2. **Service 层要完整实现业务逻辑** - Repository 只负责数据访问，Service 负责组装完整的业务数据

3. **依赖注入要完整** - 如果 Service 需要查询关联数据，构造函数要注入所有需要的 Repository

4. **Gin 的 gin.H 会扁平化嵌套结构** - 需要注意 JSON 序列化行为

5. **先看 Service 层，再看 Handler 层** - 用户说"别光盯着 session"，问题是数据根本没查出来，Handler 改了也没用

## 验证方法

修复后，访问 `/api/v1/sessions/{id}/detail` 应该返回：

```json
{
  "code": 0,
  "message": "成功",
  "data": {
    "session": {
      "id": "...",
      "title": "对话标题",
      ...
    },
    "messages": [
      {
        "id": "...",
        "role": "user",
        "content": "消息内容",
        "created_at": "2026-02-14T..."
      }
    ]
  }
}
```

前端 `loadMessages` 函数应该能正确获取 `res.data.messages` 数组。

## 日期

2026-02-14
