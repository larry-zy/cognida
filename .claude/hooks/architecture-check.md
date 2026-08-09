# 架构规范检查

## 1. Go 服务 3-Layer 架构

### 架构图
```
services/cognida-go/internal/
├── handler/          # Handler 层 - HTTP/gRPC 接口处理
├── service/          # Service 层 - 业务逻辑编排
├── repository/       # Repository 层 - 数据访问实现
└── model/            # Model 层 - 领域实体和接口定义
```

### 依赖方向
```
handler → service → model ← repository
```

**核心原则**：
- **model**：无依赖，定义领域实体和 Repository 接口
- **service**：仅依赖 model，编排业务逻辑
- **repository**：实现 model 定义的接口，仅依赖 model
- **handler**：依赖 service，处理 HTTP 协议和响应

### 各层职责检查

#### handler 层检查
- ✅ 只处理 HTTP/gRPC 协议相关（参数解析、响应格式）
- ✅ 调用 service 处理业务逻辑
- ❌ 不直接访问 database/external API
- ❌ 不包含业务逻辑

```go
// ✅ 正确
func (h *Handler) CreateUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    user, err := h.service.CreateUser(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(201, user)
}

// ❌ 错误 - handler 直接访问数据库
func (h *Handler) CreateUser(c *gin.Context) {
    // 直接 SQL 查询
    result := h.db.Exec(...)
}
```

#### service 层检查
- ✅ 业务逻辑编排
- ✅ 可调用多个 repository
- ❌ 不处理 HTTP 协议
- ❌ 不依赖 infrastructure（通过 model 接口）

```go
// ✅ 正确
func (s *UserService) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    if err := s.validate(req); err != nil {
        return nil, err
    }
    user := &User{...}
    if err := s.repo.Save(ctx, user); err != nil {
        return nil, err
    }
    return user, nil
}

// ❌ 错误 - service 直接使用 redis
func (s *UserService) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    s.redis.Set(...) // 应该通过 CacheRepository 接口
}
```

#### model 层检查
- ✅ 无任何依赖
- ✅ 定义实体和接口
- ❌ 不依赖任何其他层

```go
// ✅ 正确 - model 完全独立
package user

type Entity struct {
    ID       string
    Username string
}

type Repository interface {
    Save(ctx context.Context, user *Entity) error
}

// ❌ 错误 - model 依赖其他包
package user
import "link/internal/service" // 禁止！
```

#### repository 层检查
- ✅ 实现 model 定义的接口
- ✅ 可依赖 infrastructure（database client）
- ❌ 不依赖 handler/service

```go
// ✅ 正确
package mysql

import "link/internal/model/user"

type UserRepository struct {
    db *gorm.DB
}

func (r *UserRepository) Save(ctx context.Context, u *user.Entity) error {
    return r.db.Save(u).Error
}
```

### 包划分检查

| 包名 | 所属层 | 能力域 |
|------|--------|--------|
| handler/user | handler | 用户 HTTP 处理 |
| handler/agent | handler | Agent HTTP 处理 |
| service/user | service | 用户业务逻辑 |
| service/agent | service | Agent 业务逻辑 |
| repository/mysql/user | repository | 用户 MySQL 持久化 |
| model/user | model | 用户领域模型 |

### 禁止的依赖关系

```bash
# ❌ 禁止
handler → repository
handler → model
service → infrastructure
service → repository
model → 任何层

# ✅ 允许
handler → service
service → model
repository → model
```

## 2. Python 服务分层架构

### 架构图
```
services/cognida-python/
├── grpc/           # gRPC 服务层
│   ├── server.py
│   └── servicer/
├── services/       # 业务逻辑层
│   ├── document/
│   ├── llm/
│   └── ...
├── core/           # 核心模块
│   ├── logger.py
│   ├── exceptions.py
│   └── config.py
└── config/         # 配置管理
```

### 依赖方向
```
grpc → services → core
grpc → core
services → core
core 无依赖
```

### 各层职责检查

#### grpc 层检查
- ✅ 只处理 gRPC 协议
- ✅ 调用 services 处理业务
- ❌ 不包含业务逻辑

```python
# ✅ 正确
class DocumentServicer(document_pb2_grpc.DocumentServicer):
    def Parse(self, request, context):
        result = self.document_service.parse(request.url)
        return document_pb2.ParseResponse(result=result)

# ❌ 错误 - servicer 包含业务逻辑
class DocumentServicer(document_pb2_grpc.DocumentServicer):
    def Parse(self, request, context):
        # 直接下载、解析文档
        content = download(request.url)
        # 业务逻辑在 servicer 中
```

#### services 层检查
- ✅ 业务逻辑实现
- ✅ 可调用 core 模块
- ❌ 不处理 gRPC 协议细节

```python
# ✅ 正确
class DocumentService:
    def parse(self, url: str) -> ParseResult:
        content = self.downloader.download(url)
        return self.parser.parse(content)

# ❌ 错误 - service 直接处理协议
class DocumentService:
    def parse(self, grpc_request):
        # 处理 gRPC 请求细节
```

#### core 层检查
- ✅ 基础设施（logger、异常）
- ✅ 无业务逻辑
- ✅ 无任何依赖

```python
# ✅ 正确 - core/exceptions.py
class DocumentError(Exception):
    """文档处理异常"""
    pass

# ❌ 错误 - core 依赖其他层
from link.python.services import document  # 禁止！
```

## 3. 领域划分检查

### 能力域划分

| 能力域 | Go 包 | Python Services |
|--------|-------|-----------------|
| 用户 | handler/user, service/user, model/user | - |
| Agent | handler/agent, service/agent, model/agent | llm/ |
| 知识库 | handler/kb, service/kb, model/knowledge | - |
| 评测 | handler/evaluation, service/evaluation, model/evaluation | evaluation/ |
| 文档 | handler/document, service/document, model/document | document/ |
| 图谱 | handler/graph, service/graph, model/graph | - |
| 向量 | handler/vector, service/vector, model/vector | - |

### 检查规则

1. **包名与能力域一致**
   - 用户相关代码必须在 user 包中
   - Agent 相关代码必须在 agent 包中

2. **跨域引用检查**
   ```go
   // ❌ 错误 - user service 直接依赖 agent service
   package user
   import "link/internal/service/agent"  // 禁止跨域直接依赖

   // ✅ 正确 - 通过接口隔离
   package user
   import "link/internal/model/agent"  // 可依赖 model 层接口
   ```

3. **数据结构归位**
   - User 相关结构在 model/user 中
   - Agent 相关结构在 model/agent 中

## 4. 接口定义位置检查

### 接口在 model 层定义

```go
// ✅ 正确 - internal/model/user/repository.go
package user

type Repository interface {
    Save(ctx context.Context, user *Entity) error
    FindByID(ctx context.Context, id string) (*Entity, error)
}

// ✅ 正确 - internal/repository/mysql/user/repo.go
package user

import "link/internal/model/user"

type MySQLRepository struct {
    db *gorm.DB
}

func (r *MySQLRepository) Save(ctx context.Context, u *user.Entity) error {
    return r.db.Save(u).Error
}

// ❌ 错误 - 接口在 repository 层定义
package user

type Repository interface { ... }  // 应该在 model 中定义
```

## 5. 循环依赖检查

### 检查命令

```bash
# Go 循环依赖检查
go list -json ./... | jq -r 'select(.Deps != null) | .ImportPath + " -> " + (.Deps | join(", "))' | grep 循环模式

# 使用工具
go mod graph | grep 循环
```

### 禁止的循环依赖

```go
// ❌ 禁止 - service A 依赖 service B，service B 依赖 service A
// internal/service/a/user.go
import "link/internal/service/b"

// internal/service/b/order.go
import "link/internal/service/a"
```

## 6. 文件组织检查

### 单一职责文件

```bash
# ✅ 正确
internal/service/user/
├── user.go          # UserService
├── user_service.go  # UserService 实现
└── user_test.go     # UserService 测试

# ❌ 错误 - 文件职责混乱
internal/service/user/
├── everything.go     # 包含所有内容
```

### 导入分组

```go
// ✅ 正确
import (
    // 标准库
    "context"
    "fmt"

    // 项目内部
    "link/internal/model/user"
    "link/internal/service"

    // 第三方库
    "github.com/gin-gonic/gin"
)
```

## 7. 架构一致性检查

### 检查点

1. **新增代码是否遵循现有架构**
   - Handler 是否只处理协议
   - Service 是否只编排业务
   - Repository 是否只访问数据

2. **跨层调用是否正确**
   - 是否跳过中间层
   - 是否依赖了禁止的层

3. **领域划分是否清晰**
   - 代码是否放在正确的域包中
   - 是否有跨域直接依赖

### 自动化检查脚本

```bash
#!/bin/bash
# 架构检查脚本

# 检查 handler 不依赖 repository
if grep -r "link/internal/repository" services/cognida-go/internal/handler/; then
    echo "❌ 错误: handler 直接依赖 repository"
    exit 1
fi

# 检查 service 不依赖 infrastructure
if grep -r "link/internal/infrastructure" services/cognida-go/internal/service/ | grep -v "_test.go"; then
    echo "❌ 错误: service 直接依赖 infrastructure"
    exit 1
fi

# 检查 model 无依赖
if grep -r "^import" services/cognida-go/internal/model/ | grep "link/internal" | grep -v "model"; then
    echo "❌ 错误: model 依赖其他层"
    exit 1
fi

echo "✅ 架构检查通过"
```
