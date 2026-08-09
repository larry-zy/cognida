# Text2SQL Agent 全流程测试指南

## 一、测试准备

### 1.1 确保服务运行

```bash
cd D:\link\cognida-go
go run cmd/server/main.go
```

### 1.2 确认数据库有测试数据

```sql
-- 检查表是否存在
SHOW TABLES LIKE 'integration_%';

-- 检查数据
SELECT * FROM integration_test_users;
SELECT * FROM integration_test_orders;
```

## 二、测试流程

### 2.1 通过 Chat API 调用 Text2SQL Agent

#### 测试1：简单查询

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "messages": [
      {"role": "user", "content": "查询 integration_test_users 表有多少用户？"}
    ]
  }'
```

**预期结果**：返回 5 个用户

#### 测试2：条件查询

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "messages": [
      {"role": "user", "content": "查询状态为1的活跃用户有多少？"}
    ]
  }'
```

**预期结果**：返回 4 个活跃用户

#### 测试3：JOIN 查询

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "messages": [
      {"role": "user", "content": "查询 integration_Alice 有哪些订单？"}
    ]
  }'
```

**预期结果**：返回 2 个订单（INT001, INT002）

#### 测试4：聚合统计

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "messages": [
      {"role": "user", "content": "查询每个用户的订单总金额"}
    ]
  }'
```

**预期结果**：显示每个用户的订单总额

#### 测试5：多轮对话

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "session_id": "test-session-001",
    "messages": [
      {"role": "user", "content": "查询有哪些产品？"},
      {"role": "assistant", "content": "有笔记本电脑、无线鼠标、机械键盘、显示器、办公桌"},
      {"role": "user", "content": "按价格从高到低排序"}
    ]
  }'
```

#### 测试6：危险操作被拒绝

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-text2sql-001",
    "messages": [
      {"role": "user", "content": "删除所有用户"}
    ]
  }'
```

**预期结果**：Agent 拒绝执行，说明只能查询

## 三、验证 Agent 内部流程

### 3.1 查看 Agent 日志

服务器日志应该显示类似：

```
[Agent] ✓ Text2SQL agent registered: id=agent-text2sql-001
[Agent] 执行工具: get_schema
[Agent] 执行工具: sql_execute
```

### 3.2 工具调用流程

```
用户问题 → Text2SQL Agent
           ↓
    1. get_schema (获取表结构)
           ↓
    2. 生成 SQL (LLM)
           ↓
    3. sql_execute (执行查询)
           ↓
    4. 解释结果 (LLM)
           ↓
    返回用户
```

## 四、运行测试

### 4.1 使用测试脚本（推荐）

测试脚本会自动加载 `.env` 配置：

```bash
cd D:\link\cognida-go
bash scripts/test-text2sql.sh
```

### 4.2 单元测试（mock）

单元测试使用 sqlmock，不需要数据库连接：

```bash
cd D:\link\cognida-go
go test ./internal/application/usecases/agent/tools/... -v
```

### 4.3 集成测试（真实 MySQL）

**前置条件**：确保 `.env` 文件配置了数据库连接信息

```bash
# 运行集成测试（自动读取 .env 配置）
go test -tags=integration ./internal/application/usecases/agent/tools/... -v -run Integration
```

### 4.4 端到端测试

**前置条件**：确保 `.env` 文件配置了数据库和 LLM 连接信息

```bash
# 运行 E2E 测试（自动读取 .env 配置）
go test -tags=integration ./internal/interface/http/handler/... -v -run E2E
```

## 五、测试检查清单

- [ ] Agent 注册成功
- [ ] get_schema 工具能正确获取表结构
- [ ] sql_execute 工具能正确执行查询
- [ ] 简单 SELECT 查询正常
- [ ] WHERE 条件查询正常
- [ ] JOIN 查询正常
- [ ] 聚合查询正常
- [ ] ORDER BY / LIMIT 正常
- [ ] 危险 SQL 被阻止
- [ ] 多轮对话上下文保持
- [ ] 错误处理正确（如表不存在）
- [ ] 超时控制生效
- [ ] 结果截断警告正确

## 六、常见问题

### Q1: Agent 返回 "数据库未初始化"
**A**: 检查 main.go 中是否调用了 `ragtool.InitSQLExecuteTool(db)` 和 `ragtool.InitGetSchemaTool(db)`

### Q2: 查询没有结果
**A**: 检查表名是否正确，使用 `integration_test_users` 而不是 `users`

### Q3: Agent 说无法执行该操作
**A**: 这是正常的，Agent 的 Prompt 限制了只执行 SELECT 查询

### Q4: LLM 返回超时
**A**: 检查 CHAT_API_KEY 和 CHAT_BASE_URL 是否正确
