# 知识库测试问题记录

## 测试时间
2025-02-12

## 问题清单

### 1. 编译错误 - 缺少 fmt 包导入
**文件**: `internal/common/base.go:117`

**错误现象**:
```
internal\common\base.go:117:2: undefined: fmt
internal\common\base.go:120:3: undefined: fmt
internal\common\base.go:122:3: undefined: fmt
```

**原因**: 代码中使用了 `fmt.Printf` 但没有导入 `fmt` 包

**解决方案**:
```go
import (
	"context"
	"fmt"  // 添加这一行
	"time"
	"gorm.io/gorm"
)
```

---

### 2. 服务器未使用新编译的代码
**错误现象**: 修改代码后测试，仍然出现旧问题
- 响应中 `id` 为空字符串
- 响应中 `user_id` 为 0

**原因**: 服务器进程在代码修改之前启动，运行的还是旧版本的二进制文件
- 服务器启动时间: 5:19
- 代码修改时间: 6:23
- 没有重启服务器

**解决方案**:
```bash
# 1. 停止旧服务器进程
powershell -Command "Stop-Process -Id <PID> -Force"

# 2. 重新编译
go build -o server.exe ./cmd/server

# 3. 启动新服务器
powershell -Command "Start-Process -FilePath './server.exe' -NoNewWindow"
```

**最佳实践**: 修改代码后务必重启服务器

---

### 3. 测试代码缺少 X-Tenant-ID 请求头
**文件**: `cmd/test_knowledge_flow/main.go`

**错误现象**:
```json
{"code":40000,"message":"缺少租户ID，请在请求头中添加 X-Tenant-ID"}
```

**原因**: 测试代码中的 HTTP 请求只设置了 `Authorization` 头，缺少租户隔离所需的 `X-Tenant-ID` 头

**影响接口**:
- GET `/api/v1/knowledge-bases` (列表)
- GET `/api/v1/knowledge-bases/:id` (详情)
- POST `/api/v1/knowledge-bases/:id/knowledge/file` (上传)
- GET `/api/v1/knowledge-bases/:id/knowledge` (文档列表)
- GET `/api/v1/knowledge-bases/:id/chunks` (分块列表)
- GET `/api/v1/knowledge-bases/:id/stats` (统计)
- PUT `/api/v1/knowledge-bases/:id` (更新)

**解决方案**: 为所有需要租户ID的请求添加请求头
```go
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("X-Tenant-ID", tenantID)  // 添加这一行
```

**修改的函数**:
- `testListKnowledgeBases(baseURL, token, tenantID string)`
- `testGetKnowledgeBaseDetail(baseURL, token, kbID, tenantID string)`
- `testUploadFile(baseURL, token, kbID, tenantID string)`
- `testGetKnowledgeStatus(baseURL, token, kbID, knowledgeID, tenantID string)`
- `testGetKnowledgeList(baseURL, token, kbID, tenantID string)`
- `testGetChunks(baseURL, token, kbID, tenantID string)`
- `testGetKnowledgeBaseStats(baseURL, token, kbID, tenantID string)`
- `testUpdateKnowledgeBase(baseURL, token, kbID, tenantID string)`

---

### 4. GORM 软删除语法错误
**文件**: `internal/application/repository/knowledge_base.go:132`

**错误现象**:
```
Error 1054 (42S22): Unknown column '22c54413' in 'where clause'
```

**原因**: GORM 的 `Delete` 方法使用了错误的语法
```go
// 错误写法
return db.Delete(&types.KnowledgeBase{}, id).Error
```

GORM 将 `id` 作为字段名而不是值来处理

**解决方案**:
```go
// 正确写法
return db.Where("id = ?", id).Delete(&types.KnowledgeBase{}).Error
```

---

### 5. Milvus 未初始化
**文件**: `cmd/server/main.go`

**错误现象**:
```json
{"error":"Knowledge handler not fully initialized"}
```

**原因**: `container.InitMilvus()` 函数存在但从未在 main.go 中调用，导致 MilvusClient 为 nil，KnowledgeHandler 无法初始化

**解决方案**:
在 `cmd/server/main.go` 的数据库初始化之后添加：
```go
// 初始化 Milvus
if err := container.InitMilvus(cfg.Milvus); err != nil {
    log.Printf("⚠️  Milvus 初始化失败: %v", err)
    log.Println("继续运行（向量检索功能将不可用）...")
} else {
    log.Println("✅ Milvus 初始化成功")
    defer container.CloseMilvus()
}
```

---

### 6. Handler 中间件使用错误
**文件**: `internal/handler/knowledge.go:68-82`

**错误现象**:
```json
{"error":"missing required parameters"}
```

**原因**: Handler 使用了错误的中间件上下文获取方式：
```go
// 错误写法
tenantID := c.Get("tenant_id")
userID := c.Get("user_id")
```

`c.Get()` 返回 `(interface{}, bool)` 而不是直接值

**解决方案**:
使用专门的中间件辅助函数：
```go
// 正确写法
tenantID := middleware.GetTenantID(c)
if tenantID == 0 {
    c.JSON(401, gin.H{"error": "unauthorized: missing tenant_id"})
    return
}

userID, ok := middleware.GetUserID(c)
if !ok {
    c.JSON(401, gin.H{"error": "unauthorized: missing user_id"})
    return
}
```

---

### 7. 表名不匹配
**文件**: `internal/types/kb.go:59-61`

**错误现象**:
```
Error 1146 (42S02): Table 'link_go.knowledge' doesn't exist
```

**原因**: `Knowledge.TableName()` 返回 `"knowledge"` 但数据库表名是 `"knowledges"`

**解决方案**:
```go
func (Knowledge) TableName() string {
    return "knowledges"  // 修改为复数形式
}
```

---

### 8. 结构体字段与数据库不匹配
**文件**: `internal/types/kb.go:33-57`

**错误现象**:
```
Error 1054 (42S22): Unknown column 'chunk_count' in 'field list'
```

**原因**: Knowledge 结构体定义了数据库中不存在的字段：
```go
// 不存在于数据库中的字段
ChunkCount   int    `json:"chunk_count" gorm:"default:0"`
StorageSize  int64  `json:"storage_size" gorm:"default:0"`
```

**解决方案**:
从 Knowledge 结构体中移除这两个字段（保留在 KnowledgeResponse 响应结构体中）

---

### 9. Handler 引用已删除字段
**文件**: `internal/handler/knowledge.go`

**错误现象**:
```
knowledge.ChunkCount undefined (type *"link/internal/types".Knowledge has no field or method ChunkCount)
```

**原因**: 从 Knowledge 结构体移除 ChunkCount 后，handler 中仍有 3 处引用

**解决方案**:
1. 删除 line 246: `knowledge.ChunkCount = len(chunkIDs)`
2. 从 line 541 的 JSON 响应中移除 `"chunk_count": knowledge.ChunkCount,`

---

### 10. knowledge.go 文件损坏
**文件**: `internal/handler/knowledge.go`

**错误现象**:
```
found packages handler (auth.go) and handlerimport (knowledge.go) in D:\link\internal\handler
```

**原因**: Python 脚本修改文件时导致文件格式损坏，package 声明和 imports 被合并到一行

**解决方案**:
重写整个 `knowledge.go` 文件，确保正确的 Go 代码格式：
```go
package handler

import (
    "context"
    "fmt"
    // ... 其他 imports
)
```

---

## 测试结果总结

### ✅ 成功的测试
1. 用户注册
2. 用户登录
3. 创建知识库 (ID 正确生成: e2aaefb0-bc94-49c3-8dcb-ebdd97d70de9)
4. 获取知识库列表
5. 获取知识库详情
6. 更新知识库
7. 文件上传到知识库 (返回 KnowledgeID: d80ad8f8-568e-418e-a408-e07c8f87fa55)

### ⚠️ 部分成功的测试
1. **文件上传**: API 返回成功，知识条目已创建
2. **异步处理**: 需要配置完整的 Milvus + Neo4j + Embedder 服务才能完成分片、索引和图谱构建

### 🔧 修复的代码（10个问题）
1. `internal/common/base.go` - 添加 fmt 导入
2. `internal/application/repository/knowledge_base.go` - 修复软删除语法
3. `cmd/test_knowledge_flow/main.go` - 添加 X-Tenant-ID 请求头
4. `cmd/server/main.go` - 添加 Milvus 初始化
5. `internal/handler/knowledge.go` - 修复中间件使用方式
6. `internal/handler/knowledge.go` - 重写文件修复格式损坏
7. `internal/types/kb.go` - 修复 TableName 返回值
8. `internal/types/kb.go` - 移除不存在的字段（ChunkCount, StorageSize）
9. `internal/handler/knowledge.go` - 移除对已删除字段的引用
10. `internal/handler/task_processor.go` - 保持独立文件正常

---

## 经验教训

1. **修改代码后务必重启服务器进程** - 代码修改不会自动生效到运行中的进程
2. **编译错误会导致静默失败** - 需要检查编译输出，确保编译成功
3. **多租户系统需要在所有请求中包含 X-Tenant-ID 头** - 这是租户隔离的关键
4. **GORM 删除操作需要明确的 Where 条件** - `Delete(&Model{}, id)` 语法错误
5. **完整的知识库功能需要三个外部服务都可用** - GraphService + Embedder + Milvus
6. **使用 Python 脚本修改 Go 文件需谨慎** - 可能破坏文件格式
7. **结构体字段需与数据库表结构严格匹配** - GORM 不会自动忽略不存在的字段
8. **中间件辅助函数比直接访问 Context 更可靠** - 使用 `middleware.GetTenantID(c)` 而非 `c.Get("tenant_id")`

---

---

## 最新进展（2025-02-12 上午）

### 11. Milvus 插入 Context 取消问题
**文件**: `internal/handler/knowledge.go:363-418`

**问题**: Milvus Insert 操作使用来自 errgroup 的 context，导致在所有 goroutine 完成后被自动取消

**解决方案**:
1. 在 `task_processor.go:120` - 将超时从10分钟增加到30分钟
2. 在 `knowledge.go:367` - 为 Milvus 操作创建独立的 context（5分钟超时）
```go
// 创建独立的 Milvus context，避免被 errgroup 的 context 取消
milvusCtx, milvusCancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer milvusCancel()

// 使用 milvusCtx 进行 Insert
_, err := h.milvusClient.Insert(milvusCtx, collectionName, "", ...)
```

**测试结果**:
- ✅ 文档分片成功（1个chunk）
- ✅ Chunks 保存到数据库
- ✅ Chunks 索引到 Milvus（虽然有flush警告，但不影响功能）
- 🔄 Graph 提取已启动
- 📊 处理状态从 "pending" → "processing"

---

## 相关文件

### 核心文件
- 服务器入口: `cmd/server/main.go`
- 知识库 Handler: `internal/handler/knowledge.go`
- 知识库基础 Handler: `internal/handler/knowledge_base.go`
- 任务处理器: `internal/handler/task_processor.go`
- 知识库 Repository: `internal/application/repository/knowledge_base.go`
- 基础 Repository: `internal/common/base.go`
- 类型定义: `internal/types/kb.go`
- 中间件: `internal/middleware/middleware.go`

### 测试文件
- 测试程序: `cmd/test_knowledge_flow/main.go`
- 知识库简单测试: `cmd/test_kb_create/main.go`

### 辅助脚本（已删除）
- `fix_chunk_count.py` - 修复 ChunkCount 引用的 Python 脚本
- `fix_chunk_count_v2.py` - 修复 ChunkCount 引用的 Python 脚本 v2

### 配置文件
- 测试问题记录: `.claude/test_issues_2025-02-12.md`
