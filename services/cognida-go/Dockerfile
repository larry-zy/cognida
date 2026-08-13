# syntax=docker/dockerfile:1
# =====================================================================
# Cognida Go 服务 —— 多阶段镜像
# 构建阶段编译 server 与 migrate-db 两个二进制；运行阶段仅带二进制 + CA。
# config.yaml 经 //go:embed 打进二进制，运行时无需挂载配置文件。
# =====================================================================

# ---------- 构建阶段 ----------
FROM golang:1.26-bookworm AS builder
WORKDIR /src

# 依赖层缓存：先只拷贝 go.mod/go.sum 下载依赖，源码变动不触发重新下载
COPY go.mod go.sum ./
RUN go mod download

# 拷贝源码并编译两个入口
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate-db ./cmd/migrate-db

# ---------- 运行阶段 ----------
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /out/migrate-db /app/migrate-db

# 非 root 运行；/data/uploads 为与 Python 服务共享的上传目录挂载点
RUN useradd -m -u 1000 appuser \
 && mkdir -p /data/uploads \
 && chown -R appuser:appuser /data /app
USER appuser

EXPOSE 8080
# 默认启动 HTTP 服务；数据库迁移由独立的一次性 migrate 服务执行（见 docker-compose.yml）。
CMD ["/app/server"]
