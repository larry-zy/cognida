## Context

Link 是一个基于知识图谱和 RAG 的智能知识管理系统，采用 Go 后端 + Vue 前端架构。项目正在从传统 MVC 结构向 Clean Architecture + DDD 模式迁移。

**当前状态：**
- 新的 Clean Architecture 结构已部分完成（`internal/domain/`、`internal/application/`、`internal/infrastructure/`、`internal/interface/`）
- 旧的 `internal/application/service/` 目录仍存在，包含 agent 和 rag 子模块
- `pkg/` 目录混入了基础设施代码（gorm、middleware），不符合 Clean Architecture 原则
- Git 工作区混乱：200+ 文件处于暂存状态，部分文件同时有暂存和未暂存更改
- 前端 `component/` 目录命名不符合 Vue 社区约定（应为复数 `components/`）

**约束：**
- 必须保持 HTTP API 兼容性（无破坏性变更）
- 需要确保 Wire 依赖注入正确工作
- 不能影响正在运行的部署环境

## Goals / Non-Goals

**Goals:**
1. 删除遗留的 `internal/application/service/` 目录，完成 Clean Architecture 迁移
2. 重组 `pkg/` 目录，使其仅包含纯工具函数
3. 标准化前端目录命名，符合 Vue 项目约定
4. 清理 Git 工作区，建立清晰的提交历史
5. 更新 .gitignore 防止临时文件被提交

**Non-Goals:**
- 不改变任何 API 接口或数据模型
- 不重写业务逻辑代码（仅移动文件）
- 不修改前端组件的实现（仅目录重组）
- 不涉及数据库迁移或数据变更

## Decisions

### 1. pkg 目录重构策略

**决策**：将 `pkg/` 拆分为两部分
- 保留在 `pkg/`：纯函数工具类（convert、crypto、errors、jwt、page、parser、response）
- 移至 `internal/infrastructure/`：基础设施相关（gorm、middleware）

**理由**：
- `pkg/` 应该是可被外部项目复用的独立工具包
- gorm、middleware 与项目内部实现耦合，不属于通用工具
- 符合 Go 社区对 `pkg/` 目录的约定

**替代方案**：完全移除 `pkg/`，将所有内容移至 `internal/`
- 被拒绝：jwt、crypto 等确实是通用工具，值得保留在 pkg 中

### 2. 删除旧 service/ 的时机

**决策**：先验证新结构可编译运行，再删除旧代码

**理由**：
- 当前 `agent_adapter.go` 仍在引用旧代码
- 需要先将依赖关系理清，确保没有遗漏的 import
- 降低风险，避免编译失败导致工作阻塞

**步骤**：
1. 分析 `internal/application/service/` 中哪些函数仍在被使用
2. 将使用的代码迁移到新位置或更新引用
3. 运行 `go build ./...` 验证编译
4. 删除旧目录

### 3. 前端目录重命名方案

**决策**：使用 `git mv` 重命名，保留 Git 历史

**理由**：
- Git mv 能正确追踪文件历史
- 比直接删除+新增更安全，可回滚
- IDE 的 refactoring 功能通常能正确更新引用

**步骤**：
1. 使用 IDE 重命名功能或 `git mv` 命令
2. 全局搜索替换 import 路径 `@/component/` → `@/components/`
3. 运行 `npm run build` 验证

### 4. Git 工作区清理策略

**决策**：分阶段提交，按模块分组

**理由**：
- 单次提交 200+ 文件难以 review
- 按模块分组便于定位问题
- 可以选择性 revert 某个模块的更改

**分组计划**：
1. 后端 domain 层重构
2. 后端 application 层重构
3. 后端 infrastructure 层重构
4. 后端 interface 层重构
5. pkg 目录重组
6. 前端目录规范化
7. .gitignore 和孤立文件清理

### 5. .gitignore 更新内容

**决策**：添加以下规则
```
# 临时文件
tmp/
*.tmp

# 上传文件（保留 .gitkeep）
uploads/*
!uploads/.gitkeep

# 编译产物
*.exe
*.dll
*.so
*.dylib
bin/

# IDE
.idea/
.vscode/
*.swp
*.swo

# 空文件
nul
```

**理由**：
- `uploads/` 存储用户上传文件，不应提交到版本控制
- `*.exe` 是 Windows 编译产物
- `nul` 是意外创建的空文件

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|----------|
| 删除 service/ 后发现仍有代码引用 | 在删除前运行 `grep -r "application/service" internal/` 全局搜索 |
| Wire 依赖注入配置错误导致运行时 panic | 在删除旧代码前运行完整集成测试 |
| 前端 import 路径更新遗漏导致运行时错误 | 使用 TypeScript 的 `noUnusedLocals` 和 `noUnusedParameters` 检测 |
| Git 提交后丢失工作 | 在开始前创建 `backup/refactor-backup` 分支 |
| 前端构建失败 | 在每个步骤后运行 `npm run build` 验证 |

**权衡**：
- 更长的重构时间 vs 更清晰的架构：选择清晰架构，长期收益大于短期成本
- 一次性提交 vs 分阶段提交：选择分阶段，便于 review 和回滚

## Migration Plan

### Phase 0: 准备（安全网）

1. 创建备份分支：`git checkout -b backup/refactor-backup`
2. 返回开发分支：`git checkout dev`
3. 运行现有测试套件，确保基线可用：`go test ./...`

### Phase 1: 后端 pkg 目录重组

1. 移动基础设施代码：
   ```bash
   git mv pkg/gorm.go internal/infrastructure/persistence/gorm.go
   git mv pkg/middleware internal/infrastructure/http/middleware
   ```
2. 更新 import 路径
3. 验证编译：`go build ./...`
4. 提交：`git commit -m "refactor: move gorm and middleware to infrastructure layer"`

### Phase 2: 分析并迁移 service/ 依赖

1. 搜索引用：`grep -r "application/service" internal/`
2. 分析每个引用的代码，决定是迁移还是更新引用
3. 更新 `cmd/wire/wire.go` 移除旧的 provider
4. 重新生成 Wire：`cd cmd/wire && wire`
5. 验证编译和测试
6. 提交：`git commit -m "refactor: remove legacy application/service directory"`

### Phase 3: 前端目录规范化

1. 重命名目录：
   ```bash
   cd web/img
   git mv component components
   ```
2. 更新引用（IDE 重命名或全局替换）：
   - 搜索：`from '@/component/`
   - 替换为：`from '@/components/`
3. 移动或删除 pic/：
   ```bash
   git mv img/pic public/assets/wallpaper  # 或直接删除
   ```
4. 验证构建：`cd web && npm run build`
5. 提交：`git commit -m "refactor: standardize frontend directory naming"`

### Phase 4: 清理孤立文件和更新 .gitignore

1. 删除孤立文件：
   ```bash
   rm nul
   git rm nul 2>/dev/null || true
   ```
2. 更新 .gitignore（见决策 5）
3. 创建 uploads/.gitkeep：`touch uploads/.gitkeep && git add uploads/.gitkeep`
4. 提交：`git commit -m "chore: update .gitignore and remove orphan files"`

### Phase 5: 更新文档

1. 更新 README.md 中的目录结构
2. 更新 docs/ 下的架构文档（如需要）
3. 提交：`git commit -m "docs: update architecture documentation"`

### Rollback 策略

如果任何阶段出现问题：
1. 使用 `git revert <commit-hash>` 回滚特定提交
2. 或恢复到备份分支：`git reset --hard backup/refactor-backup`

## Open Questions

1. **Q**: `pkg/chunker` 是否应该保留在 pkg 中？
   - **A**: 需要检查其依赖关系，如果依赖内部 domain 层，应移至 application 层

2. **Q**: `cmd/gentoken/` 工具是否应该保留？
   - **A**: 需要确认其用途，如果是开发工具应移至 `tools/` 目录

3. **Q**: `node_modules` 在根目录的原因？
   - **A**: 需要确认是否误放，应该只在 `web/` 目录下
