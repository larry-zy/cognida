---
name: testing
description: Cognida 项目测试技能，涵盖单元测试、集成测试、E2E 测试、自动化测试的完整指导和最佳实践
when_to_use: 当需要编写测试、运行测试、设计测试策略、进行 E2E 测试或自动化测试时使用此技能
category: development
tags:
  - testing
  - unit-test
  - integration-test
  - e2e
  - browser-automation
  - ui-testing
  - browseract
  - ci-cd
version: "1.0.0"
---

# Testing Skill

Cognida 项目完整测试技能，涵盖从代码级测试到端到端用户流程验证。

## 能力概览

| 能力 | 工具/技术 | 适用场景 |
|------|----------|---------|
| **单元测试** | Go testing, Python pytest | 测试单个函数/方法 |
| **集成测试** | testify, pytest fixtures | 测试多组件协同 |
| **API 测试** | httptest, test client | 测试 HTTP 接口 |
| **E2E 测试** | BrowserAct | 浏览器自动化全流程 |
| **UI 测试** | BrowserAct | 用户交互验证 |
| **CI/CD 集成** | GitHub Actions | 自动化测试流水线 |

---

## 第一部分：代码级测试

### 1. 单元测试 (Unit Tests)

测试单个函数、方法或类的行为，隔离外部依赖。

**Go 单元测试**：
```go
// user_test.go
package user

import "testing"

func TestUser_IsActive(t *testing.T) {
    user := &User{Status: 1}
    if !user.IsActive() {
        t.Errorf("expected user to be active")
    }
}

// 表驱动测试
func TestUser_ValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
    }{
        {"valid email", "user@example.com", false},
        {"invalid email", "not-an-email", true},
        {"empty email", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEmail(tt.email)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

**Python 单元测试 (pytest)**：
```python
import pytest

def test_user_is_active():
    user = User(status=1)
    assert user.is_active() is True

@pytest.mark.parametrize("email,expected", [
    ("user@example.com", True),
    ("not-an-email", False),
    ("", False),
])
def test_validate_email(email, expected):
    assert validate_email(email) == expected
```

### 2. 集成测试 (Integration Tests)

测试多个组件协同工作，涉及数据库、外部服务等。

**Go 集成测试（使用 build tag）**：
```go
//go:build integration
package user

func TestUserRepository_Integration(t *testing.T) {
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)

    repo := NewUserRepository(db)
    user := &User{Username: "testuser", Email: "test@example.com"}

    err := repo.Create(context.Background(), user)
    assert.NoError(t, err)
}
```

**运行集成测试**：`go test -tags=integration ./...`

**Python 集成测试**：
```python
@pytest.fixture(scope="session")
def test_db():
    db = setup_test_database()
    yield db
    cleanup_test_database(db)

def test_user_repository_create(test_db):
    repo = UserRepository(test_db)
    user = User(username="test", email="test@example.com")
    repo.create(user)
    found = repo.find_by_id(user.id)
    assert found.email == user.email
```

### 3. API 测试

**Go API 测试**：
```go
func TestUserAPI_CreateUser(t *testing.T) {
    router := setupTestRouter(t)
    body := `{"username":"test","email":"test@example.com"}`
    req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 201, w.Code)
}
```

**Python API 测试**：
```python
def test_create_user(client):
    response = client.post("/api/v1/users", json={
        "username": "test",
        "email": "test@example.com"
    })
    assert response.status_code == 201
```

### 测试命令

#### Go 服务
```bash
# 运行所有测试
go test ./...

# 运行特定包
go test ./internal/service/user

# 显示覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 运行集成测试
go test -tags=integration ./...
```

#### Python 服务
```bash
# 运行所有测试
pytest

# 显示覆盖率
pytest --cov=services --cov-report=html

# 运行特定测试
pytest tests/test_api.py

# 只运行集成测试
pytest -m integration
```

### 测试最佳实践

#### AAA 模式 (Arrange-Act-Assert)
```go
func TestUser_UpdateStatus(t *testing.T) {
    // Arrange - 准备测试数据
    user := &User{ID: 1, Status: 1}
    repo := NewMockRepository()

    // Act - 执行操作
    err := user.UpdateStatus(repo, 2)

    // Assert - 验证结果
    assert.NoError(t, err)
    assert.Equal(t, int8(2), user.Status)
}
```

#### 测试命名
- Go: `Test<Function>_<Scenario>`
- Python: `test_<function>_<scenario>`

#### 覆盖率目标
| 类型 | 目标 |
|------|------|
| 核心业务逻辑 | > 80% |
| 一般代码 | > 70% |
| 工具函数 | > 90% |

#### 常用工具
| 语言 | 工具 | 用途 |
|------|------|------|
| Go | testing | 内置框架 |
| Go | testify | 断言/Mock |
| Python | pytest | 测试框架 |
| Python | pytest-cov | 覆盖率 |

---

## 第二部分：E2E 测试

### BrowserAct 简介

BrowserAct 是专为 AI Agent 设计的浏览器自动化 CLI：
- 突破反机器人限制（指纹伪装、验证码求解）
- 多浏览器并行执行，零干扰
- 人类接管模式（卡住时切换人工）
- Agent 友好设计（索引化交互、语义化记忆）

### 安装 BrowserAct

```
安装 browser-act。技能来源：https://github.com/browser-act/skills/tree/main/browser-act
安装后验证是否工作正常。
```

### 核心命令

#### 基础命令
```bash
# 获取环境状态和可用命令
browser-act get-skills core --skill-version 2.0.2

# 快速提取受保护页面内容
browser-act stealth-extract https://example.com

# 完整浏览器自动化流程
browser-act --session my-task browser open <id> https://example.com
browser-act --session my-task state          # 查看可点击元素
browser-act --session my-task click 3        # 按索引点击
browser-act --session my-task input 2 "text" # 按索引输入
```

#### 浏览器模式
| 模式 | 场景 | 特点 |
|------|------|------|
| chrome | 复用本地 Chrome 登录态 | 导入 Profile 或 CDP 连接 |
| stealth（隐私模式） | 无登录批量抓取 | 每会话新指纹 + 代理轮换 |
| stealth（固定身份） | 登录账号·多浏览器并行 | 稳定指纹 + 稳定 IP |

### E2E 测试场景

#### 场景 1：用户注册登录流程
```bash
# 1. 打开注册页面
browser-act --session user-register browser open stealth http://localhost:8080/register

# 2. 查看页面状态
browser-act --session user-register state

# 3. 填写注册表单（假设索引：username=2, email=3, password=4）
browser-act --session user-register input 2 "testuser"
browser-act --session user-register input 3 "testuser@example.com"
browser-act --session user-register input 4 "password123"

# 4. 提交表单（假设提交按钮索引=5）
browser-act --session user-register click 5

# 5. 验证跳转到首页或登录页
browser-act --session user-register wait --selector=".welcome-message"
```

#### 场景 2：知识库管理流程
```bash
# 1. 登录（使用 mock-login 技能获取 token）
# 假设已登录状态

# 2. 打开知识库页面
browser-act --session kb-manage browser open stealth http://localhost:8080/knowledge

# 3. 点击"创建知识库"按钮
browser-act --session kb-manage state
browser-act --session kb-manage click 1  # 假设创建按钮索引为 1

# 4. 填写知识库信息
browser-act --session kb-manage input 2 "测试知识库"
browser-act --session kb-manage input 3 "测试描述"

# 5. 提交创建
browser-act --session kb-manage click 4  # 提交按钮

# 6. 验证创建成功
browser-act --session kb-manage wait --text="创建成功"
```

#### 场景 3：RAG 问答完整流程
```bash
# 1. 登录系统
browser-act --session rag-test browser open stealth http://localhost:8080/login
browser-act --session rag-test input 2 "admin@example.com"
browser-act --session rag-test input 3 "password"
browser-act --session rag-test click 4

# 2. 上传文档
browser-act --session rag-test navigate http://localhost:8080/documents/upload
browser-act --session rag-test state
browser-act --session rag-test input-file 1 "/path/to/test.pdf"
browser-act --session rag-test click 2  # 上传按钮

# 3. 等待文档解析完成
browser-act --session rag-test wait --text="解析完成"

# 4. 进入问答页面
browser-act --session rag-test navigate http://localhost:8080/chat

# 5. 发送问题
browser-act --session rag-test input 1 "文档的主要内容是什么？"
browser-act --session rag-test click 2  # 发送按钮

# 6. 验证收到回复
browser-act --session rag-test wait --selector=".answer"
```

#### 场景 4：多租户隔离测试
```bash
# 租户 1 用户操作
browser-act --session tenant1 browser open stealth http://localhost:8080/login
browser-act --session tenant1 input 2 "user1@tenant1.com"
browser-act --session tenant1 input 3 "password"
browser-act --session tenant1 click 4
# 创建租户 1 的数据...

# 租户 2 用户操作（独立会话）
browser-act --session tenant2 browser open stealth http://localhost:8080/login
browser-act --session tenant2 input 2 "user1@tenant2.com"
browser-act --session tenant2 input 3 "password"
browser-act --session tenant2 click 4
# 验证只能看到租户 2 的数据...

# 验证租户 1 无法访问租户 2 数据
```

### 常用操作

#### 导航操作
```bash
browser-act --session <name> navigate <url>
browser-act --session <name> back
browser-act --session <name> forward
browser-act --session <name> refresh
```

#### 元素交互
```bash
# 点击
browser-act --session <name> click <index>

# 输入文本
browser-act --session <name> input <index> "<text>"

# 文件上传
browser-act --session <name> input-file <index> "<file-path>"

# 下拉选择
browser-act --session <name> select <index> "<option>"
```

#### 等待与验证
```bash
# 等待选择器
browser-act --session <name> wait --selector="<css>"

# 等待文本
browser-act --session <name> wait --text="<text>"

# 等待时间
browser-act --session <name> wait --ms=1000
```

#### 信息获取
```bash
# 获取页面文本
browser-act --session <name> extract --text

# 获取页面截图
browser-act --session <name> screenshot <path>

# 获取页面状态
browser-act --session <name> state
```

### E2E 测试最佳实践

#### 1. 会话命名规范
- `e2e-<feature>-<scenario>`：如 `e2e-user-register`
- 使用描述性名称便于调试

#### 2. 等待策略
- 优先使用语义等待（wait --text/--selector）
- 避免固定时间等待
- 设置合理的超时时间

#### 3. 错误处理
```bash
# 检查页面状态是否异常
browser-act --session test state
# 如果出现错误页面，使用 remote-assist 人工接管
```

#### 4. 数据清理
- 测试前创建专用测试数据
- 测试后清理产生的数据
- 使用独立测试环境

#### 5. 并行测试
```bash
# 多个测试场景并行执行
browser-act --session test1 browser open stealth http://localhost:8080/test1 &
browser-act --session test2 browser open stealth http://localhost:8080/test2 &
```

### E2E 测试清单

#### 用户模块
- [ ] 用户注册流程
- [ ] 用户登录流程
- [ ] 密码重置流程
- [ ] 用户资料修改
- [ ] 用户登出

#### 知识库模块
- [ ] 创建知识库
- [ ] 上传文档
- [ ] 文档解析验证
- [ ] 删除文档
- [ ] 知识库共享

#### RAG 模块
- [ ] 发送问题
- [ ] 接收回答
- [ ] 引用来源验证
- [ ] 多轮对话

#### 多租户模块
- [ ] 租户隔离验证
- [ ] 租户切换
- [ ] 跨租户访问拒绝

#### Agent 模块
- [ ] Agent 执行任务
- [ ] 工具调用验证
- [ ] 结果展示

---

## 第三部分：CI/CD 配置

### GitHub Actions - Go
```yaml
name: Go Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - run: go test -v -race -coverprofile=coverage.out ./...
```

### GitHub Actions - Python
```yaml
name: Python Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-python@v4
        with:
          python-version: '3.11'
      - run: pip install pytest pytest-cov
      - run: pytest --cov --cov-report=xml
```

---

## 测试检查清单

### 代码级测试
- [ ] 每个公开函数都有测试
- [ ] 边界条件已测试
- [ ] 错误情况已测试
- [ ] API 状态码正确
- [ ] 数据库操作正确
- [ ] 测试数据已清理

### E2E 测试
- [ ] 环境准备完成
- [ ] 测试数据已创建
- [ ] 用户流程完整
- [ ] 断言验证充分
- [ ] 测试后数据已清理

---

## 与其他技能配合

### 配合 mock-login
```
1. 使用 mock-login 获取测试用户 token
2. BrowserAct 携带 token 访问需要认证的页面
3. 验证页面功能正常
```

---

## 常见问题

### Q: 如何找到元素索引？
A: 使用 `state` 命令查看页面所有可交互元素的索引列表。

### Q: 页面加载慢怎么办？
A: 使用 `wait --selector` 或 `wait --text` 等待特定元素出现。

### Q: 如何处理弹窗？
A: 使用 `state` 查看弹窗元素索引，然后 `click` 关闭或确认。

### Q: 测试失败后如何调试？
A: 使用 `screenshot` 保存失败时的页面状态，或使用 `remote-assist` 人工接管。

---

## 注意事项

1. **测试环境**：E2E 测试应在独立测试环境运行
2. **数据隔离**：使用专门的测试数据，避免污染生产数据
3. **资源清理**：测试后清理浏览器会话和测试数据
4. **并发安全**：不同会话使用不同名称，避免冲突
5. **人类接管**：遇到验证码或复杂操作时，使用 remote-assist
