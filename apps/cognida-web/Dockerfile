# syntax=docker/dockerfile:1
# =====================================================================
# Cognida Web 前端 —— 多阶段镜像
# 构建阶段用 Node 产出静态资源；运行阶段用 nginx 托管，并把 /api 反代到 Go 服务。
# 生产构建下前端 API base 恒为相对路径 /api/v1（见 src/utils/request.ts），
# 故同源部署无需 CORS，一切经 nginx 反代到 cognida-go:8080。
# =====================================================================

# ---------- 构建阶段 ----------
FROM node:20-bookworm AS builder
WORKDIR /app

# 依赖层缓存
COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# 部署镜像只需产物：直接用 vite build（跳过 vue-tsc 类型检查，类型检查归 CI 负责，
# 避免与运行时无关的类型告警阻断镜像构建）。
RUN npx vite build

# ---------- 运行阶段 ----------
FROM nginx:1.27-alpine AS runtime
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost/ || exit 1
