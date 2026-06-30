# Go 语言规范检查

## 1. 命名规范

### 包名
- ✅ 小写单数名词：`package user`, `package agent`
- ❌ 下划线：`package user_repo`
- ❌ 复数：`package users`

### 接口名
- ✅ 动名词组合：`AgentRepository`, `DocumentParser`
- ✅ -er 结尾：`Reader`, `Writer`
- ❌ `I` 前缀：`IUserRepository`

### 常量
- ✅ 驼峰命名：`MaxRetries`, `DefaultTimeout`
- ❌ 下划线：`max_retries`

### 缩写词
- ✅ 保持大写：`UserID`, `GetURL`, `ParseJSON`
- ❌ 小写缩写：`UserId`, `GetUrl`

### 私有变量
- ✅ 驼峰命名：`userCache`, `calculateScore()`
- ❌ 下划线：`user_cache`

## 2. 注释规范

### 导出元素必须有注释
```go
// ✅ 正确
// Chat 处理消息并返回完整响应。
func (a *Agent) Chat(ctx context.Context, message string) (*Response, error) {}

// ❌ 错误 - 缺少注释
func (a *Agent) Chat(ctx context.Context, message string) (*Response, error) {}
```

### 包注释
```go
// Package agent 提供统一的 Agent 接口和构建器。
package agent
```

## 3. 错误处理

### 立即处理错误
```go
// ✅ 正确
result, err := someFunc()
if err != nil {
    return nil, fmt.Errorf("operation failed: %w", err)
}

// ❌ 错误 - 忽略错误
result, err := someFunc()
// 使用 result，没有检查 err
```

### 定义包级错误变量
```go
// ✅ 正确
var (
    ErrDocumentNotFound = errors.New("document not found")
    ErrInvalidInput    = errors.New("invalid input")
)
```

### 错误检查
```go
// ✅ 正确
if errors.Is(err, ErrDocumentNotFound) {
    // 处理未找到
}

// ✅ 正确
var ve *ValidationError
if errors.As(err, &ve) {
    // 处理验证错误
}
```

## 4. Context 使用

### 第一个参数必须是 context.Context
```go
// ✅ 正确
func (r *Repository) Save(ctx context.Context, doc *Document) error {}

// ❌ 错误
func (r *Repository) Save(doc *Document, ctx context.Context) error {}
```

### 传递 context 到下游
```go
// ✅ 正确
func (s *Service) ProcessDocument(ctx context.Context, id string) error {
    doc, err := s.repo.Get(ctx, id)
    if err != nil {
        return err
    }
    return s.indexer.Index(ctx, doc)
}
```

## 5. 并发安全

### 使用 sync.WaitGroup
```go
// ✅ 正确
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func(i Item) {
        defer wg.Done()
        process(i)
    }(item)
}
wg.Wait()
```

### 使用 sync.Mutex
```go
// ✅ 正确
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}
```

## 6. 依赖方向检查

### 3-Layer 架构
```
handler → service → model ← repository
```

### 禁止的依赖
- ❌ handler → repository（必须通过 service）
- ❌ service → infrastructure（必须通过 model 接口）
- ❌ model → 任何其他层（model 无依赖）

### 检查规则
| 层 | 可依赖 | 禁止依赖 |
|----|--------|----------|
| handler | service | model, repository, infrastructure |
| service | model | handler, repository, infrastructure |
| model | 无 | handler, service, repository, infrastructure |
| repository | model | handler, service |

## 7. 常见错误模式

### 资源泄漏
```go
// ❌ 错误 - 可能泄漏
func readFile() string {
    f, _ := os.Open("file.txt")
    data, _ := io.ReadAll(f)
    return string(data)
    // f 未关闭
}

// ✅ 正确
func readFile() (string, error) {
    f, err := os.Open("file.txt")
    if err != nil {
        return "", err
    }
    defer f.Close()
    data, err := io.ReadAll(f)
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

### Goroutine 泄漏
```go
// ❌ 错误 - 可能泄漏
func process(ch chan<- Result) {
    ch <- heavyComputation()
    // 如果接收方已关闭，goroutine 永久阻塞
}

// ✅ 正确 - 使用 select
func process(ch chan<- Result) {
    select {
    case ch <- heavyComputation():
    case <-ctx.Done():
        return
    }
}
```

## 8. 文件组织

### 一个文件一个类型
- ✅ 每个主要类型有自己的文件
- ✅ 相关小类型可以放在同一文件

### 导入分组
```go
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

## 9. 接口设计

### 接口在 model 层定义
- ✅ `internal/model/user/repository.go` 定义 `UserRepository`
- ❌ 不在 repository 实现层定义

### 接口小而专
- ✅ 每个接口 1-3 个方法
- ❌ 大而全的接口

## 10. 编译前检查

### 必须通过
```bash
go vet ./...
go build ./...
golangci-lint run
```
