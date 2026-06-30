## Why

Link 项目当前缺乏数据分析能力，无法为 Agent 提供数据统计、趋势分析和洞察发现功能。实现这些能力是构建"AI 数据专家"的基础，也是支持智能问数、洞察报告生成等上层功能的关键。

## What Changes

在 `link-python` 服务中新增数据分析能力：

- 新增 `services/analytics/` 模块，包含数据分析核心功能
- 新增 gRPC 服务 `AnalyticsService` (端口 50053)
- 实现三类核心分析能力：
  - **数理统计**：描述统计、分布分析、相关性分析、假设检验
  - **趋势分析**：线性趋势、季节分解、增长率分析、移动平均
  - **数据洞察**：趋势洞察、异常洞察、相关性洞察

## Capabilities

### New Capabilities

- `analytics-statistics`: 数理统计能力 - 提供描述统计、分布检验、相关性分析等统计计算
- `analytics-trend`: 趋势分析能力 - 提供线性趋势检测、季节分解、增长率计算等时序分析
- `analytics-insight`: 数据洞察发现 - 基于统计和趋势分析自动生成洞察结论

### Modified Capabilities

None

## Impact

**新增代码**:
- `link-python/services/analytics/` - 数据分析核心模块
- `link-python/grpc_service/analytics_servicer.py` - gRPC 服务实现
- `link-go/api/proto/analytics.proto` - Protobuf 定义

**新增依赖**:
- `pandas >= 2.0.0` - 数据处理
- `numpy >= 1.24.0` - 数值计算
- `scipy >= 1.10.0` - 科学计算
- `statsmodels >= 0.14.0` - 统计建模
- `scikit-learn >= 1.3.0` - 异常检测

**集成点**:
- Go Agent 调用 Python AnalyticsService 获取分析结果
- 分析结果通过 GenUI 协议返回前端渲染
