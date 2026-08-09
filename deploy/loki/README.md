# Loki 原始日志采集栈

链路追踪的两条腿之一：**MySQL 审计表**（结构化的每请求审计，Go 侧 `audit_logs` 表）
负责“可查询、可统计”；**Loki**（本目录）负责“原始 stdout 日志的集中检索”。二者用同一个
`request_id` 串起来——拿到审计表里的 `request_id`，就能在 Loki 里 `|= "<request_id>"`
捞出该请求在 Go / Python 各服务打印的所有原始日志行。

## 组成

| 组件 | 端口 | 作用 |
|------|------|------|
| loki | 3100 | 日志聚合/查询后端（filesystem 存储，保留 7 天）|
| promtail | — | 采集器：抓取宿主机 `cognida-go/logs` 与 `cognida-python/logs` 下的 `*.log` |
| grafana | 3300 | 查询/可视化，已预置 Loki 数据源（匿名 Admin，免登录）|

## 前置：让服务把日志落到被采集目录

- **Python**：`cognida-python/scripts/dev-all.sh` 已把四服务日志写到
  `cognida-python/logs/{grpc,http,eval,mcp}.log`，开箱即用。
- **Go**：stdout 需重定向到 `cognida-go/logs/`：
  ```bash
  cd cognida-go
  go run ./cmd/server 2>&1 | tee logs/server.log
  # 或后台： go run ./cmd/server > logs/server.log 2>&1 &
  ```

## 启动

```bash
cd deploy/loki
docker compose up -d
# 打开 http://localhost:3300 → Explore → 数据源 Loki
```

## LogQL 检索示例

```logql
# 某个 request_id 的全链路日志（Go + Python 所有服务）
{job="cognida"} |= "550e8400e29b41d4"

# 仅评测服务
{service="eval"}

# 仅 Go 栈的错误
{stack="go"} |= "error"
```

## 设计说明

- **request_id 不做 Loki label**：它是高基数字段，作为 label 会导致 stream 数量爆炸。
  request_id 保留在原始日志行内，用 LogQL 行过滤 `|= "<rid>"` 检索即可。
- **低基数 label**：`job=cognida` / `stack=go|python` / `service=<文件名>`。
- request_id 的产生与透传：Go 入口 `TraceMiddleware` 注入 → gRPC metadata `x-request-id`
  / HTTP 头 `X-Request-ID` → Python `core.request_context` 绑定 structlog contextvars，
  于是各服务日志行自动带上 `request_id=...`。

## 关闭

```bash
cd deploy/loki
docker compose down          # 保留数据卷
docker compose down -v       # 连同 Loki/Grafana 数据一起清除
```
