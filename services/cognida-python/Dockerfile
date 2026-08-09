# 多阶段构建 - 开发阶段
FROM python:3.11-slim AS development

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# 安装 uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# 设置工作目录
WORKDIR /app

# 复制依赖文件
COPY pyproject.toml ./

# 创建虚拟环境并安装依赖
RUN uv sync --frozen --no-dev

# 非 root 用户
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

# 暴露端口：HTTP 8000 / 评测 HTTP 18888 / gRPC 50051
EXPOSE 8000 18888 50051

# 开发服务器：单进程多协议统一入口（core.main）
CMD ["uv", "run", "python", "-m", "core.main"]


# 多阶段构建 - 生产阶段
FROM python:3.11-slim AS production

# 安装运行时依赖
RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 安装 uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# 设置工作目录
WORKDIR /app

# 复制依赖文件
COPY pyproject.toml ./

# 仅安装生产依赖
RUN uv sync --frozen --no-dev --no-editable

# 复制源代码
COPY --chown=appuser:appuser . .

# 非 root 用户
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# 暴露端口：HTTP 8000 / 评测 HTTP 18888 / gRPC 50051
EXPOSE 8000 18888 50051

# 生产服务器：单进程多协议统一入口（core.main）
# 注意为单进程拓扑，多协议共享一个事件循环，不使用 uvicorn --workers 多进程。
CMD ["uv", "run", "python", "-m", "core.main"]
