## Why

需要一个标准的Python基础服务脚手架，以支持快速搭建新服务。目前缺乏统一的项目结构和最佳实践指导，导致每个新项目都需要从零开始配置，增加开发成本和维护难度。

## What Changes

- 建立标准Python服务项目结构
- 配置开发环境依赖管理（poetry/uv）
- 集成代码质量工具（linting, formatting, type checking）
- 配置测试框架和CI/CD模板
- 添加日志、配置管理、错误处理等基础模块

## Capabilities

### New Capabilities

- `project-structure`: 标准Python项目目录结构和文件组织
- `dependency-management`: 使用uv进行依赖管理和虚拟环境管理
- `code-quality`: 集成ruff（linting + formatting）和mypy（type checking）
- `testing-framework`: pytest配置和测试组织结构
- `logging-config`: 结构化日志配置（JSON格式输出）
- `config-management`: 多环境配置管理（dev/test/prod）
- `api-framework`: FastAPI/Flask基础框架搭建
- `docker-support`: Dockerfile和docker-compose配置
- `ci-cd`: GitHub Actions工作流模板

### Modified Capabilities

- None（新项目搭建）

## Impact

- **新建服务**: 所有新Python服务可基于此脚手架快速初始化
- **开发工具链**: 统一团队开发工具配置（代码格式、类型检查、测试）
- **部署流程**: 标准化容器化部署流程
- **依赖**: 新增uv、ruff、mypy、pytest等开发依赖

