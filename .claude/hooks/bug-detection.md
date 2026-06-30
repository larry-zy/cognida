# Bug 检测规则

## 1. 编译错误检测

### Go 编译错误

```bash
# 检查是否可以编译
check_go_compile() {
    echo "🔨 检查 Go 编译..."
    if ! go build ./...; then
        echo "❌ Go 编译失败"
        return 1
    fi
    echo "✅ Go 编译通过"
}
```

### 常见编译错误模式

```go
// ❌ 错误 - 导入包未使用
import (
    "fmt"
    "os"  // 未使用
)

// ❌ 错误 - 类型不匹配
var userID int = "123"  // string 不能赋值给 int

// ❌ 错误 - 未定义变量
func process() {
    fmt.Println(user)  // user 未定义
}

// ❌ 错误 - 方法签名不匹配
type Handler interface {
    Handle(ctx context.Context) error
}

func (h *MyHandler) Handle() error {  // 缺少 ctx 参数
    return nil
}
```

### Python 编译错误

```bash
# 类型检查
check_python_types() {
    echo "🔍 检查 Python 类型..."
    if ! mypy link-python/; then
        echo "❌ Python 类型检查失败"
        return 1
    fi
}
```

```python
# ❌ 错误 - 类型不匹配
def get_user(user_id: str) -> int:  # 返回类型应该是 User
    return db.query(user_id)

# ❌ 错误 - 参数类型错误
def process(count: int):
    pass

process("string")  # 应该传 int
```

## 2. 空指针/None 检测

### Go 空指针

```go
// ❌ 危险 - 未检查 nil
func processUser(id string) {
    user := getUser(id)
    fmt.Println(user.Name)  // user 可能是 nil
}

// ✅ 正确
func processUser(id string) error {
    user := getUser(id)
    if user == nil {
        return ErrUserNotFound
    }
    fmt.Println(user.Name)
    return nil
}
```

### Go 空指针检查规则

```bash
# 检查可能的空指针解引用
check_nil_dereference() {
    echo "🔍 检查空指针风险..."

    # 检查指针解引用前是否有 nil 检查
    grep -rn '\.Name' link-go/internal/ | while read line; do
        # 检查附近是否有 nil 检查
        if ! echo "$line" | grep -B5 'if.*==.*nil\|if.*!=.*nil'; then
            echo "⚠️  可能的空指针: $line"
        fi
    done
}
```

### Python None 检测

```python
# ❌ 危险 - 未检查 None
def process_user(user_id: str) -> str:
    user = db.get_user(user_id)
    return user.name  # user 可能是 None

# ✅ 正确
def process_user(user_id: str) -> str:
    user = db.get_user(user_id)
    if user is None:
        raise UserNotFoundError(user_id)
    return user.name
```

## 3. 并发问题检测

### Race Condition

```go
// ❌ 危险 - 数据竞争
var counter int

func increment() {
    counter++  // 非原子操作
}

// ✅ 正确
var (
    counter int
    mu      sync.Mutex
)

func increment() {
    mu.Lock()
    counter++
    mu.Unlock()
}

// ✅ 正确 - 使用 atomic
var counter int64

func increment() {
    atomic.AddInt64(&counter, 1)
}
```

```bash
# Go race 检测
check_race() {
    echo "🏃 检查数据竞争..."
    go test -race ./... || {
        echo "❌ 检测到数据竞争"
        return 1
    }
}
```

### Goroutine 泄漏

```go
// ❌ 危险 - Goroutine 泄漏
func process() {
    ch := make(chan Result)
    go func() {
        ch <- heavyComputation()  // 如果没人接收，永久阻塞
    }()
    // 忘记接收，或提前返回
}

// ✅ 正确 - 使用 context
func process(ctx context.Context) error {
    ch := make(chan Result)
    go func() {
        select {
        case ch <- heavyComputation():
        case <-ctx.Done():
            return
        }
    }()

    select {
    case result := <-ch:
        return processResult(result)
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Python 并发问题

```python
# ❌ 危险 - 共享状态
class Cache:
    def __init__(self):
        self.data = {}  # 非线程安全

    def get(self, key):
        return self.data.get(key)

    def set(self, key, value):
        self.data[key] = value

# ✅ 正确 - 使用锁
import threading

class Cache:
    def __init__(self):
        self.data = {}
        self.lock = threading.Lock()

    def get(self, key):
        with self.lock:
            return self.data.get(key)

    def set(self, key, value):
        with self.lock:
            self.data[key] = value
```

## 4. 资源泄漏检测

### 文件描述符泄漏

```go
// ❌ 危险 - 文件未关闭
func readFile() string {
    f, _ := os.Open("file.txt")
    data, _ := io.ReadAll(f)
    return string(data)  // f 未关闭
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

### 数据库连接泄漏

```go
// ❌ 危险 - 连接未关闭
func query() {
    db := getDB()
    db.Query("SELECT * FROM users")
    // db 未关闭
}

// ✅ 正确
func query() error {
    db := getDB()
    defer db.Close()
    return db.Query("SELECT * FROM users")
}
```

```python
# ❌ 危险 - 数据库连接未关闭
def get_user(user_id: str) -> User:
    session = Session()
    return session.query(User).filter_by(id=user_id).one()

# ✅ 正确
def get_user(user_id: str) -> User:
    with Session() as session:
        return session.query(User).filter_by(id=user_id).one()
```

## 5. SQL 注入检测

```go
// ❌ 危险 - SQL 注入
func getUser(name string) {
    query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name)
    db.Exec(query)
}

// ✅ 正确 - 使用参数化查询
func getUser(name string) {
    query := "SELECT * FROM users WHERE name = ?"
    db.Query(query, name)
}
```

```python
# ❌ 危险 - SQL 注入
def get_user(name: str) -> User:
    query = f"SELECT * FROM users WHERE name = '{name}'"
    return db.execute(query)

# ✅ 正确
def get_user(name: str) -> User:
    query = "SELECT * FROM users WHERE name = %s"
    return db.execute(query, (name,))
```

```bash
# 检查可能的 SQL 注入
check_sql_injection() {
    echo "💉 检查 SQL 注入风险..."

    # 检查字符串拼接 SQL
    grep -rn 'fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT' link-go/internal/
    grep -rn 'f".*SELECT\|f".*INSERT' link-python/

    echo "请确认以上 SQL 查询使用参数化"
}
```

## 6. 错误处理缺失

### Go 错误忽略

```go
// ❌ 错误 - 忽略错误
result, _ := someFunc()  // 忽略错误

// ❌ 错误 - 错误未处理
result, err := someFunc()
fmt.Println(result)  // 没有检查 err

// ✅ 正确
result, err := someFunc()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}
fmt.Println(result)
```

```bash
# 检查忽略的错误
check_ignored_errors() {
    echo "⚠️  检查忽略的错误..."

    # 检查使用 _ 忽略错误的情况
    grep -rn ', _ := ' link-go/internal/
    grep -rn ', _ = ' link-go/internal/
}
```

### Python 异常处理缺失

```python
# ❌ 错误 - 不使用裸 except
try:
    risky_operation()
except:
    pass  # 吞掉所有异常

# ✅ 正确
try:
    risky_operation()
except ValueError as e:
    logger.error(f"Invalid value: {e}")
    raise
except Exception as e:
    logger.error(f"Unexpected error: {e}")
    raise
```

## 7. 逻辑错误检测

### Off-by-one 错误

```go
// ❌ 错误 - 循环边界
for i := 0; i <= len(items); i++ {  // 应该是 <
    process(items[i])  // 最后一项会越界
}

// ✅ 正确
for i := 0; i < len(items); i++ {
    process(items[i])
}
```

### 条件判断错误

```go
// ❌ 错误 - 赋值而非比较
if user.ID = "123" {  // 应该是 ==
    fmt.Println("Admin")
}

// ✅ 正确
if user.ID == "123" {
    fmt.Println("Admin")
}
```

### 死循环

```go
// ❌ 错误 - 死循环
func process() {
    for {
        // 没有退出条件
        doSomething()
    }
}

// ✅ 正确
func process(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            doSomething()
        }
    }
}
```

## 8. 类型转换错误

```go
// ❌ 危险 - 潜在 panic
var value interface{} = "123"
number := value.(int)  // panic: 无法转换

// ✅ 正确
var value interface{} = "123"
number, ok := value.(int)
if !ok {
    return fmt.Errorf("not an int")
}
```

```python
# ❌ 危险 - 类型错误
value: Any = "123"
number = int(value)  # 如果 value 不是数字会抛异常

# ✅ 正确
def safe_int(value: Any) -> Optional[int]:
    try:
        return int(value)
    except (ValueError, TypeError):
        return None
```

## 9. 整数溢出检测

```go
// ❌ 危险 - 可能溢出
func multiply(a, b int) int {
    return a * b  // 可能溢出
}

// ✅ 正确
func multiply(a, b int) (int, error) {
    if a != 0 && b > math.MaxInt64/a {
        return 0, errors.New("overflow")
    }
    return a * b, nil
}

// ✅ 正确 - 使用 math/big
func multiplyBig(a, b int64) *big.Int {
    return new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
}
```

## 10. 无限递归检测

```go
// ❌ 错误 - 无限递归导致栈溢出
func factorial(n int) int {
    return n * factorial(n-1)  // 没有基准情况
}

// ✅ 正确
func factorial(n int) int {
    if n <= 1 {
        return 1
    }
    return n * factorial(n-1)
}
```

## 11. 配置错误检测

### 硬编码配置

```go
// ❌ 错误 - 硬编码敏感信息
const API_KEY = "sk-1234567890"
const DB_PASSWORD = "password123"

// ✅ 正确 - 从环境变量读取
apiKey := os.Getenv("API_KEY")
dbPassword := os.Getenv("DB_PASSWORD")
```

```bash
# 检查硬编码配置
check_hardcoded_config() {
    echo "🔑 检查硬编码配置..."

    # 检查可能的硬编码密钥
    grep -rn 'sk-.*"\|password.*"\|secret.*"' link-go/internal/
    grep -rn 'sk-.*"\|password.*"\|secret.*"' link-python/

    echo "请确认以上敏感信息使用环境变量"
}
```

## 12. 自动化检测脚本

```bash
#!/bin/bash
# Bug 检测脚本

echo "🐛 Bug 检测"
echo "==========="

FAILED=0

# 1. 编译检查
echo "📦 [1/8] 编译检查"
check_go_compile || FAILED=1

# 2. Go vet
echo "🔍 [2/8] Go vet"
go vet ./... || FAILED=1

# 3. Race 检测
echo "🏃 [3/8] 数据竞争检测"
go test -race ./... || FAILED=1

# 4. SQL 注入
echo "💉 [4/8] SQL 注入风险"
check_sql_injection || FAILED=1

# 5. 忽略的错误
echo "⚠️  [5/8] 忽略的错误"
check_ignored_errors || FAILED=1

# 6. 硬编码配置
echo "🔑 [6/8] 硬编码配置"
check_hardcoded_config || FAILED=1

# 7. Python 类型检查
echo "🐍 [7/8] Python 类型检查"
check_python_types || FAILED=1

# 8. 静态分析
echo "📊 [8/8] 静态分析"
golangci-lint run || FAILED=1

if [ $FAILED -eq 1 ]; then
    echo "❌ Bug 检测发现问题"
    exit 1
fi

echo "✅ Bug 检测通过"
```

## 13. 常见 Bug 模式总结

| Bug 类型 | 检测方法 | 严重程度 |
|----------|----------|----------|
| 空指针解引用 | 代码审查 + static analysis | 🔴 高 |
| 数据竞争 | go test -race | 🔴 高 |
| SQL 注入 | 静态分析 | 🔴 高 |
| 资源泄漏 | 代码审查 + go vet | 🟡 中 |
| 错误忽略 | grep 检查 | 🟡 中 |
| 整数溢出 | 代码审查 | 🟡 中 |
| 类型断言 panic | 代码审查 | 🟡 中 |
| 硬编码配置 | grep 检查 | 🟡 中 |
| Off-by-one | 单元测试 | 🟢 低 |
| 死循环 | 代码审查 + 超时测试 | 🟢 低 |
