# 多阶段构建 - 开发阶段
FROM python:3.11-slim AS development

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# 安装 uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# 设置工作目录
WORKDIR /app

# 复制依赖文件
COPY pyproject.toml uv.lock ./

# 创建虚拟环境并安装依赖
RUN uv sync --frozen --no-dev --extra document --no-install-project

# 非 root 用户
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

# 暴露端口：HTTP 8000 / 评测 HTTP 18888 / gRPC 50051
EXPOSE 8000 18888 50051

# 开发服务器：单进程多协议统一入口（core.main）
CMD ["uv", "run", "--no-sync", "python", "-m", "core.main"]


# 多阶段构建 - 生产阶段
FROM python:3.11-slim AS production

# 安装 uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# 设置工作目录
WORKDIR /app

# 复制依赖文件
COPY pyproject.toml uv.lock ./

# 仅安装生产依赖。ARM64 上部分 document 依赖没有预编译 wheel，安装期需要
# build-essential；编译完成后立即清理，避免把编译工具带入运行镜像。
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl build-essential \
    && uv sync --frozen --no-dev --extra document --no-install-project \
    && apt-get purge -y --auto-remove build-essential \
    && rm -rf /var/lib/apt/lists/*

# 非 root 用户
RUN useradd -m -u 1000 appuser && chown -R appuser:appuser /app

# 复制源代码
COPY --chown=appuser:appuser . .

USER appuser

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# 暴露端口：HTTP 8000 / 评测 HTTP 18888 / gRPC 50051
EXPOSE 8000 18888 50051

# 生产服务器：单进程多协议统一入口（core.main）
# 注意为单进程拓扑，多协议共享一个事件循环，不使用 uvicorn --workers 多进程。
CMD ["uv", "run", "--no-sync", "python", "-m", "core.main"]
