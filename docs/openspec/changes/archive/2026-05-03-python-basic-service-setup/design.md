## Context

当前团队在创建新的Python服务时缺乏标准化流程。每个项目都需要单独配置开发环境、代码质量工具、测试框架等，导致：
- 项目结构不一致，难以维护
- 开发环境配置重复，效率低下
- 代码风格不统一，review成本高
- 缺乏统一的日志和配置管理模式

## Goals / Non-Goals

**Goals:**
- 提供可复制的Python服务脚手架
- 统一开发工具链和代码质量标准
- 简化新服务的初始化流程（5分钟内启动）
- 支持微服务和单体架构两种模式

**Non-Goals:**
- 不替代cookiecutter等模板工具（可作为其模板源）
- 不包含业务逻辑代码
- 不包含数据库迁移工具（由具体服务决定）

## Decisions

### 依赖管理：uv over poetry/pip

**选择**: 使用 uv 作为依赖管理工具

**理由**:
- 速度比poetry快10-100倍（Rust实现）
- 兼容pyproject.toml标准格式
- 支持锁文件和虚拟环境管理
- 自动解决依赖冲突

**替代方案**: Poetry（成熟但较慢）、pip+venv（缺乏锁文件）

### 代码质量：ruff统一方案

**选择**: 使用 ruff 替代 black/flake8/isort

**理由**:
- 一站式解决方案（linting + formatting + import sorting）
- 速度快（Rust实现）
- 兼容flake8/black规则
- 减少工具配置复杂度

### 类型检查：mypy严格模式

**选择**: mypy with strict mode

**理由**:
- Python生态最成熟的类型检查器
- strict模式提供最佳类型安全保障
- 与VS Code/PyCharm深度集成

### API框架：FastAPI优先

**选择**: FastAPI 作为默认API框架

**理由**:
- 原生异步支持
- 自动生成OpenAPI文档
- 内置数据校验（Pydantic）
- 类型提示友好

**替代方案**: Flask（同步，需要额外扩展）

### 日志：structlog

**选择**: structlog for structured logging

**理由**:
- JSON格式输出便于日志聚合
- 上下文信息自动绑定
- 开发环境支持可读输出

### 测试：pytest + pytest-cov + pytest-asyncio

**选择**: pytest生态

**理由**:
- 最流行的Python测试框架
- 插件生态丰富
- 原生支持异步测试

### 容器化：多阶段构建

**选择**: Multi-stage Dockerfile

**理由**:
- 减小最终镜像大小（不含构建工具）
- 分离构建和运行环境
- 支持开发热重载模式

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| uv较新，生态可能不成熟 | 保留poetry作为备选方案 |
| FastAPI学习曲线 | 提供完整示例和文档 |
| strict type checking可能影响开发速度 | 提供--no-strict选项 |
| 多环境配置管理复杂 | 使用pydantic-settings统一管理 |

## Migration Plan

**Phase 1: 基础结构**（1天）
- 创建项目目录结构
- 配置uv和pyproject.toml
- 设置ruff和mypy

**Phase 2: 核心功能**（1天）
- 实现日志模块
- 实现配置管理
- 实现错误处理中间件

**Phase 3: API和测试**（1天）
- 搭建FastAPI应用
- 配置pytest
- 编写示例测试

**Phase 4: 容器化和CI**（1天）
- 编写Dockerfile
- 配置docker-compose
- 设置GitHub Actions

**Rollback策略**: 保留原有项目结构，新服务可选择性采用

## Open Questions

1. 是否需要支持gRPC服务？ → 后续评估，当前仅支持REST
2. 数据库选择（PostgreSQL/MySQL）？ → 不在脚手架范围，由具体服务决定
3. 消息队列集成？ → 作为可选扩展模块提供

