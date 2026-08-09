# Cognida 本地开发环境搭建完整指南

> 面向「一台新电脑从零把整个 Cognida 项目跑起来」的完整清单。涵盖硬件建议、系统依赖（运行时 + 中间件）、逐服务安装与配置、启动顺序、验证方法与常见排错。
>
> 项目由 4 个可运行部分组成：
> - **cognida-go** — Go 主后端（REST / gRPC 网关 / SSE），端口 `8080`
> - **cognida-python** — Python 计算/分析服务（gRPC 50051、基础 HTTP 8000、评测 HTTP 18888、MCP 3000）
> - **cognida-web** — Vue 3 + Vite 前端，端口 `5173`
> - **中间件** — MySQL、Redis、Milvus、Neo4j

---

## 1. 硬件配置建议

> 下表内存一栏是**实测数据**，不是估算。测量环境：48 GB M4 Pro，cognida-go + cognida-python 四服务 + cognida-web + MySQL/Redis/Milvus/Neo4j 全部同时运行、**空载/轻载**状态下各进程 RSS。

### 实测内存占用（空载）

| 组件 | 实测 RSS | 备注 |
|------|----------|------|
| Milvus（standalone embed 单容器，含 etcd+minio）| **~300 MB** | 空库；数据/索引 load 进内存后会涨 |
| Neo4j（Homebrew Community，默认 `-Xmx128m`）| **~58 MB** | 默认堆很小，跑真实图谱要调大 |
| MySQL 8（空载）| **~17 MB** | 默认 `innodb_buffer_pool_size` 128 MB |
| Redis 7 | **~3 MB** | `used_memory` 1.8 M |
| cognida-go server | **~41 MB** | |
| cognida-python 四服务合计 | **~150 MB** | gRPC / HTTP / 评测 / MCP |
| cognida-web（vite + node + esbuild）| **~150 MB** | dev server |
| **应用 + 中间件进程 RSS 合计** | **≈ 360 MB** | 全栈空载不到 1 GB |

### 配置建议

| 项目 | 够用 | 舒服 | 本项目参考机 |
|------|------|------|--------------|
| CPU | 4 核 | 8 核以上 | Apple M4 Pro（14 核） |
| 内存 | **8 GB**（空载栈 ~1 GB）| **16 GB** | 48 GB |
| 磁盘 | 20 GB 可用 SSD | 50 GB+ SSD | — |
| 系统 | macOS 13+ / Ubuntu 20.04+ / Windows 11 + WSL2 | macOS / Linux | macOS 26.5 (arm64) |

说明（内存瓶颈在负载态，不在服务本身）：
- 空载整套栈占用不到 1 GB，**16 GB 是「舒服线」而非「硬门槛」**，留余量给 IDE / 浏览器 / 数据加载即可；8 GB 也能开发。
- 真正吃内存的是**负载态**，且随配置和数据量变化，不是固定值：
  - **Milvus**：内存随向量集合大小 + 加载进内存的索引增长。空库 300 MB，灌几十万条向量并 load 索引后可到 GB 级。
  - **Neo4j**：Homebrew 默认堆才 128 MB，跑真实图谱/血缘查询需手动调大 `server.memory.heap.max_size` 与 page cache，那时才占几百 MB ~ GB。
  - **MySQL**：`innodb_buffer_pool_size` 默认 128 MB，数据量大时需调优。
- ⚠️ **macOS 上的隐藏成本**：Milvus 容器跑在 Docker Desktop 的 VM 里，`docker stats` 会显示该 VM 上限被分到 ~15 GB——那是**预留上限，不是实际占用**（Milvus 实占仅 300 MB）。内存小的机器要去 Docker Desktop 设置里把 VM 内存调低，否则它会预留一大块；或改用 `standalone_embed.sh` 直接跑 / 换 OrbStack 规避此预留。
- Apple Silicon（arm64）与 x86_64 均可，容器镜像都有对应架构。
- 若内存实在不足，可把 Milvus/Neo4j 放到远程或按需关闭对应功能（见 §7）。

---

## 2. 需要安装的运行时（宿主机）

| 工具 | 版本要求 | 本项目实际 | 用途 |
|------|----------|-----------|------|
| **Go** | **1.25+**（go.mod 声明 `go 1.25.8`）| 1.26.4 | 编译/运行 cognida-go |
| **Python** | **3.11+**（pyproject `requires-python>=3.11`）| 3.11（uv 管理的 venv）| cognida-python |
| **uv** | 最新 | 0.11.x | Python 依赖与虚拟环境管理（推荐）|
| **Node.js** | 18+，推荐 **20 LTS 或更高** | v26 | cognida-web 构建/开发 |
| **npm** | 随 Node | 11.x | 前端包管理 |
| **Docker (+ Compose)** | 最新 | 29.x | 运行 Milvus / 可选中间件 |
| **Git** | 任意近期版本 | — | 拉取代码 |
| **buf**（可选）| 1.7x | 1.71 | 改 proto 时重新生成代码 |

> ⚠️ 版本注意：README 里写的「Go 1.21+ / Python 3.10+」已过时，以本表为准——`go.mod` 要求 Go **1.25+**，`pyproject.toml` 要求 Python **3.11+**，低于此版本无法编译/安装。

### macOS 一键安装（Homebrew）

```bash
# 运行时
brew install go python@3.11 node git
brew install uv                      # 或 curl -LsSf https://astral.sh/uv/install.sh | sh

# 中间件（宿主机原生方式，见 §3 可选路线 A）
brew install mysql redis
brew install --cask docker           # Docker Desktop，用于 Milvus / Neo4j

# 可选：改 proto 才需要
brew install bufbuild/buf/buf
```

### Ubuntu/Debian

```bash
# Go 1.25+（apt 版本通常偏旧，建议官方包）
wget https://go.dev/dl/go1.25.8.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.25.8.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && source ~/.bashrc

# Python 3.11 + uv
sudo apt install -y python3.11 python3.11-venv
curl -LsSf https://astral.sh/uv/install.sh | sh

# Node 20 LTS
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash - && sudo apt install -y nodejs

# Docker
sudo apt install -y docker.io docker-compose-plugin
```

---

## 3. 需要准备的中间件（4 个）

| 中间件 | 版本 | 默认端口 | 项目内配置项 |
|--------|------|----------|--------------|
| **MySQL** | 8.0+ | 3306 | `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME` |
| **Redis** | 7.0+ | 6379 | `REDIS_ADDR/REDIS_PASSWORD/REDIS_DB` |
| **Milvus** | 2.6+ | 19530 | `MILVUS_HOST`（如 `localhost:19530`）|
| **Neo4j** | 5.0+ | 7687（bolt）/ 7474（浏览器）| `NEO4J_URI/NEO4J_USERNAME/NEO4J_PASSWORD` |

> 注意：仓库 README 里提到的根目录 `docker-compose.yml` 和 `docker/docker-deps.yml` **当前并不存在**。因此下面给出两条可实操的路线，任选其一。

### 路线 A：MySQL/Redis 原生 + Milvus/Neo4j 用 Docker（macOS 推荐）

```bash
# --- MySQL ---
brew services start mysql
mysql -uroot -e "CREATE DATABASE IF NOT EXISTS link CHARACTER SET utf8mb4;"
# 如需密码：mysql_secure_installation，然后把密码写进 cognida-go/.env 的 DB_PASSWORD

# --- Redis ---
brew services start redis

# --- Milvus 2.6 standalone（官方 embed 脚本，数据落在 ~/milvus）---
# 本机已使用该方式，数据/配置目录：~/milvus（含 embedEtcd.yaml、volumes/）
mkdir -p ~/milvus && cd ~/milvus
curl -sfL https://raw.githubusercontent.com/milvus-io/milvus/master/scripts/standalone_embed.sh -o standalone_embed.sh
bash standalone_embed.sh start        # 监听 19530
# 若遇 etcd "leader changed" 崩溃循环，在 ~/milvus/embedEtcd.yaml 里补
#   heartbeat-interval / election-timeout / snapshot-count 三个参数后重启（见团队记忆）

# --- Neo4j 5 ---
docker run -d --name neo4j \
  -p 7474:7474 -p 7687:7687 \
  -e NEO4J_AUTH=neo4j/your_neo4j_password \
  -v $HOME/neo4j/data:/data \
  neo4j:5
```

### 路线 B：全部用 Docker（Linux/统一环境更省心）

在 `deploy/` 下自建 `docker-deps.yml`（仓库暂未内置，可按此模板创建）：

```yaml
# deploy/docker-deps.yml
services:
  mysql:
    image: mysql:8.0
    ports: ["3306:3306"]
    environment:
      MYSQL_ROOT_PASSWORD: your_password
      MYSQL_DATABASE: link
    volumes: ["mysql_data:/var/lib/mysql"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  neo4j:
    image: neo4j:5
    ports: ["7474:7474", "7687:7687"]
    environment:
      NEO4J_AUTH: neo4j/your_neo4j_password
    volumes: ["neo4j_data:/data"]

  # Milvus 用官方 milvus-standalone-docker-compose.yml 更稳妥（含 etcd+minio）
  # 见 https://milvus.io/docs/install_standalone-docker-compose.md

volumes:
  mysql_data:
  neo4j_data:
```

```bash
docker compose -f deploy/docker-deps.yml up -d
```

> Milvus 建议单独用官方 standalone compose（它需要 etcd + MinIO 两个伴生容器），不要塞进上面这个文件里手写。

---

## 4. 外部 API 凭证（LLM / Embedding / 搜索）

Cognida 是 AI 应用，多数功能依赖外部大模型服务。这些是 **必须提前拿到 Key** 的部分（本机当前用的是 DeepSeek + 阿里云 DashScope）：

| 能力 | 变量 | 供应商示例 | 是否必需 |
|------|------|-----------|----------|
| 对话 LLM | `CHAT_PROVIDER/CHAT_MODEL_NAME/CHAT_API_KEY/CHAT_BASE_URL` | DeepSeek、OpenAI 等 | ✅ 核心 |
| Python 侧 LLM | `LLM_PROVIDER/LLM_MODEL/LLM_API_KEY/LLM_BASE_URL` | 同上 | ✅ 评测/分析用 |
| Embedding 向量 | `EMBEDDING_PROVIDER/EMBEDDING_MODEL/EMBEDDING_API_KEY/EMBEDDING_BASE_URL` | 阿里云 DashScope（`text-embedding-v4`）| ✅ 检索必需 |
| 联网搜索 | `METASO_API_KEY/SEARCH_API_ENDPOINT` | Metaso | ⭕ 可选 |

此外还有两个 **本地生成** 的密钥（不是第三方，但必须自己生成，否则启动失败）：

```bash
# JWT 签名密钥：至少 32 字节，占位/过短会 fail-closed 启动失败
openssl rand -hex 32          # 填入 cognida-go/.env 的 JWT_SECRET

# 外部数据源凭证加密密钥（多数据源功能必需）
openssl rand -base64 32       # 填入 cognida-go/.env 的 DATASOURCE_SECRET_KEY
```

---

## 5. 逐服务安装与配置

### 5.0 拉取代码

```bash
git clone <repo-url> link && cd link
```

### 5.1 cognida-go（后端）

```bash
cd cognida-go

# 1) 生成 .env（以示例为模板）
cp .env.example .env
# 编辑 .env：DB_PASSWORD、NEO4J_PASSWORD、JWT_SECRET、DATASOURCE_SECRET_KEY、
#            CHAT_*/LLM_*/EMBEDDING_* 各类 Key，MILVUS_HOST=localhost:19530
# macOS 注意：把 UPLOAD_DIR / PYTHON_ALLOWED_PATHS 从 "D:/cognida/uploads" 改成本机绝对路径，
#            例如 /Users/<you>/Downloads/link/uploads

# 2) 拉依赖
make deps                       # = go mod download && go mod tidy

# 3) 同步数据库表结构（从 GORM model 幂等建表，替代手写 ALTER）
set -a && source .env && set +a && go run ./cmd/migrate-db

# 4)（可选）灌演示数据
go run ./cmd/seed-ecommerce     # 电商演示库
go run ./cmd/seed-eval-datasets # 评测数据集
```

关键 `.env` 项（cognida-go）：

```ini
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=<你的MySQL密码>
DB_NAME=cognida

NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=<你的Neo4j密码>

MILVUS_HOST=localhost:19530

REDIS_ADDR=localhost:6379

JWT_SECRET=<openssl rand -hex 32>
DATASOURCE_SECRET_KEY=<openssl rand -base64 32>

CORS_ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
PYTHON_GRPC_TARGET=localhost:50051
PYTHON_EVALUATION_ENDPOINT=http://localhost:18888
```

### 5.2 cognida-python（计算/分析服务）

```bash
cd cognida-python

# 1) 用 uv 创建 venv 并装全部功能依赖（含评测/文档处理/向量模型）
uv venv --python 3.11
uv pip install -e ".[all]"      # 或按需：".[evaluation]" ".[document]" 等

# 2) 生成 .env
cp .env.example .env
# 编辑：LLM_API_KEY / LLM_BASE_URL / LLM_MODEL、MILVUS_HOST、NEO4J_URI
#      UPLOAD_BASE_DIR / ALLOWED_PATHS 同样改成本机绝对路径
```

> `.[all]` 里包含 `paddleocr`、`sentence-transformers` 等较大依赖，首次安装耗时较久，且首次运行会联网下载模型权重。若不用 OCR/本地向量模型，可只装 `.[evaluation]`。

### 5.3 cognida-web（前端）

```bash
cd cognida-web
npm install
# .env.development 已内置：VITE_API_BASE_URL=http://localhost:8080/api/v1
# Vite 会把 /api 代理到 127.0.0.1:8080，无需额外改动
```

---

## 6. 启动顺序与命令

严格按 **中间件 → Go 后端 → Python 服务 → 前端** 顺序启动。

```bash
# ① 中间件（见 §3，确保 3306 / 6379 / 19530 / 7687 都在监听）

# ② Go 后端（端口 8080）
cd cognida-go && make run                 # = go run ./cmd/server/main.go
#   生产方式：make build && ./bin/cognida

# ③ Python 四服务一键并行启动（gRPC 50051 / HTTP 8000 / 评测 18888 / MCP 3000）
cd cognida-python && bash scripts/dev-all.sh
#   Ctrl+C 会优雅终止全部子进程；日志在 cognida-python/logs/*.log

# ④ 前端（端口 5173）
cd cognida-web && npm run dev
```

浏览器打开 **http://localhost:5173**。

### 端口一览

| 服务 | 端口 | 说明 |
|------|------|------|
| cognida-go 后端 | 8080 | REST / gRPC 网关 / SSE |
| cognida-python gRPC | 50051（+分析 50053）| 主计算服务 |
| cognida-python 基础 HTTP | 8000 | FastAPI 基础/调试接口 |
| cognida-python 评测 HTTP | 18888 | 独立 FastAPI，Go 经 `PYTHON_EVALUATION_ENDPOINT` 调用 |
| cognida-python MCP | 3000 | HTTP 模式下（默认 stdio 不占端口）|
| cognida-web 前端 | 5173 | Vite dev server（strictPort）|
| MySQL / Redis / Milvus / Neo4j | 3306 / 6379 / 19530 / 7687 | 中间件 |

> ⚠️ 评测服务 :18888 必须启动，否则 Go 调用 graders/compute-metrics 会连接被拒（500）。`dev-all.sh` 已包含它。

---

## 7. 验证与冒烟测试

```bash
# 中间件连通性
mysql -uroot -p -e "SHOW DATABASES;" | grep link
redis-cli ping                                   # PONG
curl -s localhost:9091/healthz || nc -vz localhost 19530   # Milvus 端口
curl -s http://localhost:7474                    # Neo4j 浏览器

# 应用健康检查
curl -s http://localhost:8080/api/v1/health      # Go 后端
curl -s http://localhost:8000/health             # Python 基础服务
curl -s http://localhost:18888/health            # Python 评测服务

# 单元测试
cd cognida-go && go test ./internal/... -short
cd cognida-python && uv run pytest tests/ -v
cd cognida-web && npm run test
```

若某中间件暂时不可用，可临时关闭对应功能位（如 `.env` 里 `PYTHON_GRPC_ENABLED=false`、留空 `MILVUS_HOST`）来先跑通主链路，但检索/图谱/评测等功能会降级。

---

## 8. 常见问题排错

| 现象 | 原因 | 解决 |
|------|------|------|
| Go 启动即报 JWT 相关 fatal | `JWT_SECRET` 是占位符或不足 32 字节 | `openssl rand -hex 32` 重新生成 |
| Go 编译报 `go: requires go >= 1.25` | Go 版本过低 | 升级到 Go 1.25+ |
| `pip install` 报 Python 版本不符 | 用了系统 Python 3.9 | 用 `uv venv --python 3.11` |
| 上传/文件路径报错找不到目录 | `.env` 里还是 Windows 路径 `D:/cognida/uploads` | 改成本机绝对路径并 `mkdir -p` |
| 前端 502 / `/api` 代理失败 | Go 未起或只监听 IPv6 | 确认 8080 在 `127.0.0.1` 监听（Vite 已固定走 IPv4）|
| 页面覆盖/按钮失灵 | 5173 端口被两个 vite 占用 | `lsof -i:5173` 排查后 kill 多余进程 |
| Milvus 反复重启（etcd leader changed）| 内嵌 etcd 配置缺参 | 在 `~/milvus/embedEtcd.yaml` 补 heartbeat/election/snapshot-count |
| 评测接口 500 连接被拒 | :18888 未启动 | 用 `dev-all.sh` 启动或手动起评测 FastAPI |
| 端口占用启动失败 | 残留进程 | `lsof -i:<port>` → `kill`；任务完成后记得终止服务进程 |

---

## 9. 一页速查（TL;DR）

```bash
# 0. 装运行时（macOS）
brew install go python@3.11 node git uv mysql redis && brew install --cask docker

# 1. 起中间件
brew services start mysql redis
bash ~/milvus/standalone_embed.sh start
docker run -d --name neo4j -p7474:7474 -p7687:7687 -e NEO4J_AUTH=neo4j/pass neo4j:5
mysql -uroot -e "CREATE DATABASE IF NOT EXISTS link CHARACTER SET utf8mb4;"

# 2. Go
cd cognida-go && cp .env.example .env   # 填 Key/密码/路径
make deps && set -a && source .env && set +a && go run ./cmd/migrate-db
make run

# 3. Python
cd cognida-python && uv venv --python 3.11 && uv pip install -e ".[all]"
cp .env.example .env                 # 填 LLM Key
bash scripts/dev-all.sh

# 4. Web
cd cognida-web && npm install && npm run dev
# → http://localhost:5173
```
