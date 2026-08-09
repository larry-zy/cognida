# 部署指南

## Docker 部署

### 构建镜像

```bash
docker build -t cognida-python:latest .
```

### 运行容器

```bash
docker run -d \
  --name cognida-python \
  -p 8000:8000 \
  -p 50051:50051 \
  --env-file .env \
  cognida-python:latest
```

### 使用 docker-compose

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

## 环境变量配置

### 必需配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `APP_ENV` | 运行环境 (dev/test/prod) | dev |
| `LOG_LEVEL` | 日志级别 | INFO |
| `API_PORT` | HTTP API 端口 | 8000 |
| `GRPC_PORT` | gRPC 端口 | 50051 |

### 可选配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `LLM_API_KEY` | LLM API 密钥 | - |
| `LLM_BASE_URL` | LLM API 地址 | https://api.openai.com/v1 |
| `LLM_MODEL` | LLM 模型名称 | gpt-4 |
| `MILVUS_HOST` | Milvus 地址 | localhost |
| `MILVUS_PORT` | Milvus 端口 | 19530 |

## 生产环境检查清单

- [ ] 设置 `APP_ENV=prod`
- [ ] 配置适当的 `LOG_LEVEL` (INFO 或 WARNING)
- [ ] 配置 `LLM_API_KEY` 等密钥
- [ ] 配置数据库连接 (如果需要)
- [ ] 配置 CORS 允许的来源
- [ ] 启用健康检查
- [ ] 配置日志聚合
- [ ] 设置资源限制 (CPU/内存)
- [ ] 配置自动重启策略

## 健康检查

### HTTP 健康检查

```bash
curl http://localhost:8000/health
```

### gRPC 健康检查

```bash
grpc_health_probe -addr=localhost:50051
```

## 监控

### 日志

容器日志输出到 stdout/stderr，使用以下命令查看：

```bash
docker logs -f cognida-python
```

### 指标

当前版本未暴露 Prometheus 指标，后续可集成。

## CI/CD

项目包含 GitHub Actions 工作流 (`.github/workflows/ci.yml`)：

- **Lint**: 代码检查 (ruff, mypy)
- **Test**: 测试 (Python 3.11, 3.12, 3.13)
- **Security**: 安全扫描 (safety, bandit)
- **Build**: Docker 镜像构建和推送

### 配置 Docker 镜像仓库

在 GitHub Secrets 中配置：

- `REGISTRY_URL`: 镜像仓库地址
- `REGISTRY_USERNAME`: 用户名
- `REGISTRY_PASSWORD`: 密码

## 性能优化

### uvicorn 工作进程数

根据 CPU 核心数调整：

```bash
uvicorn core.app:create_app --workers 4 --host 0.0.0.0 --port 8000
```

### gRPC 工作线程数

通过 `GRPC_MAX_WORKERS` 环境变量配置。

## 故障排查

### 容器启动失败

```bash
# 查看日志
docker logs cognida-python

# 检查配置
docker run --rm --env-file .env cognida-python:latest python -c "from config import get_settings; print(get_settings().model_dump())"
```

### 端口冲突

修改 `docker-compose.yml` 或环境变量中的端口配置。

### 内存不足

增加 Docker 容器内存限制或优化服务配置。
