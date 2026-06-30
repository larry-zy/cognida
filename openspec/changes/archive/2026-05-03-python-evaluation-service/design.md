# Python Evaluation Service - Technical Design (Updated with OpenJudge Patterns)

## Context

**Current State:**
- Go 服务有评测系统框架，但存在生产级别问题
- Python 生态在 NLP 领域更成熟（jieba、sentence-transformers、rouge）
- 当前架构：Go 通过 gRPC 调用 Python 进行文档处理
- 已实现基础评测指标：检索（Precision/Recall/NDCG/MRR/MAP）、生成（ROUGE/BLEU）、语义相似度、LLM-as-Judge

**OpenJudge Reference:**
- OpenJudge 提供 50+ 生产级评测器，覆盖 9 大类别
- 统一的 Grader 接口设计，支持 Pointwise 和 Listwise 两种模式
- 标准化的结果类型：GraderScore、GraderRank、GraderError
- 1-5 级别评分替代 0-100 数值评分

**Constraints:**
- Go 作为 API Gateway，必须保持对外的唯一入口
- Python 服务必须是无状态的（可水平扩展）
- 评测数据集较大，不适合在 gRPC 请求中传输

**Stakeholders:**
- Go 团队：负责任务管理、鉴权、结果存储
- Python 团队：负责评测执行和指标计算
- 前端：需要实时展示评测进度

## Goals / Non-Goals

**Goals:**
- Python 实现完整的生产级评测系统
- 支持流式返回评测进度
- 正确的中文分词和指标计算
- 支持语义相似度计算
- 评测数据集本地化管理
- **插件式评测器（Grader）系统，参考 OpenJudge 设计**
- **支持 Pointwise（单样本评分）和 Listwise（多样本排序）两种模式**
- **统一的返回类型：GraderScore、GraderRank、GraderError**
- **1-5 级别评分，替代 0-100 数值评分**
- **字段映射机制，自动转换 Go 数据格式到 Python 格式**

**Non-Goals:**
- Python 不负责任务持久化（由 Go 负责）
- Python 不负责用户鉴权（由 Go 负责）
- Python 不负责评测任务的生命周期管理（由 Go 负责）

## Decisions

### 1. 使用 gRPC 流式接口

**选择理由：**
- 评测任务耗时较长（分钟级），需要实时进度反馈
- gRPC 流式是原生支持，无需额外协议
- 连接断开双方都能感知，容错性好

**替代方案：**
- HTTP 轮询：延迟高，空请求浪费资源
- WebSocket：需要额外的连接管理

### 2. 评测数据集存储在 Python 本地

**选择理由：**
- 数据集文件较大（可能 MB 到 GB 级别）
- 避免每次评测都传输数据集
- Python 可以预先加载和索引

**替代方案：**
- 存储在 Go 数据库：增加传输开销，Python 需要 DB 连接
- 存储在共享文件系统：需要额外的存储配置

### 3. Grader 架构设计（参考 OpenJudge）

**核心规则：同一 Grader 的多个指标必须基于同一类型**

这是 OpenJudge 的核心设计原则，确保：
- **类型一致性**：一个 Grader 返回的所有指标必须是同一种类型
- **语义关联性**：同一 Grader 的指标在语义上应该相关
- **可组合性**：同类型指标可以方便地进行聚合、比较

**不允许的示例（混合类型）：**
```python
# ❌ 错误：混合了 SCORE 和 BINARY 类型
GraderScore(
    name="bad_grader",
    metrics={
        "accuracy": 4.0,      # SCORE 类型
        "passed": 1.0,        # BINARY 类型
    }
)
```

**允许的示例（单一类型）：**
```python
# ✅ 正确：所有指标都是 SCORE 类型
GraderScore(
    name="relevance_grader",
    metric_type=MetricType.SCORE,
    metrics={
        "relevance": 4.0,
        "clarity": 3.5,
        "completeness": 4.2,
    }
)
```

**核心概念：**

```
Evaluation Scenario（评测场景）
    ↓
Grader（评分器）
    ↓
Metrics（指标）/ Score（分数）/ Rank（排序）
```

**Grader 类型：**

1. **Pointwise Grader（逐点评分器）**
   - 输入：单个样本（query, response, reference, context）
   - 输出：`GraderScore`（分数 + 原因 + 元数据）
   - 适用场景：质量评估、准确性检查、幻觉检测

2. **Listwise Grader（列表排序器）**
   - 输入：多个样本（同一 query 的多个 response）
   - 输出：`GraderRank`（排序 + 原因 + 元数据）
   - 适用场景：模型对比、响应排序、竞技场模式

**统一返回类型（核心规则：同一 Grader 的多个指标必须同类型）：**

```python
# 指标类型枚举
class MetricType(str, Enum):
    """指标类型，同一 Grader 的所有指标必须是同一种类型"""
    SCORE = "score"           # 数值分数（如 1-5, 0-1）
    BINARY = "binary"         # 二值（{0, 1}）
    RANK = "rank"             # 排序（List[int]）
    PENALTY = "penalty"       # 惩罚（≤0）
    PERCENTAGE = "percentage" # 百分比（[0, 100]）

# 基础结果
class GraderResult(BaseModel):
    name: str              # 评分器名称
    metric_type: MetricType # 指标类型（同一 Grader 所有指标必须一致）
    reason: str            # 评分理由
    metadata: dict         # 元数据

# 评分结果（Pointwise）- 支持多个同类型指标
class GraderScore(GraderResult):
    """数值型评分结果，所有指标必须是同一类型（SCORE/BINARY/PENALTY/PERCENTAGE）"""
    metrics: Dict[str, float]  # 多个指标，如 {"accuracy": 4.0, "completeness": 3.5}
                               # 键为指标名，值为同类型的数值

    @field_validator("metrics")
    @classmethod
    def validate_same_type(cls, v: Dict[str, float], info) -> Dict[str, float]:
        """验证所有指标是否同类型（通过值范围推断）"""
        if not v:
            return v
        # 根据指标类型检查值范围
        metric_type = info.data.get("metric_type", MetricType.SCORE)
        for key, value in v.items():
            if metric_type == MetricType.SCORE:
                if not (1 <= value <= 5):
                    raise ValueError(f"SCORE 类型指标必须在 1-5 范围，{key}={value}")
            elif metric_type == MetricType.BINARY:
                if value not in {0.0, 1.0}:
                    raise ValueError(f"BINARY 类型指标必须是 0 或 1，{key}={value}")
            elif metric_type == MetricType.PENALTY:
                if value > 0:
                    raise ValueError(f"PENALTY 类型指标必须 ≤ 0，{key}={value}")
            elif metric_type == MetricType.PERCENTAGE:
                if not (0 <= value <= 100):
                    raise ValueError(f"PERCENTAGE 类型指标必须在 0-100 范围，{key}={value}")
        return v

# 单指标便捷属性（向后兼容）
    @property
    def score(self) -> float:
        """获取主分数（第一个指标）"""
        return next(iter(self.metrics.values())) if self.metrics else 0.0

# 排序结果（Listwise）- 支持多个同类型排序
class GraderRank(GraderResult):
    """排序结果，所有排序指标必须是同一类型（RANK）"""
    metrics: Dict[str, List[int]]  # 多个排序维度，如 {"relevance": [2,1,3], "quality": [1,2,3]}

# 错误结果
class GraderError(GraderResult):
    error: str         # 错误信息
```

**评分标准（按指标类型）：**

| 指标类型 | 值范围 | 说明 | 示例 |
|----------|--------|------|------|
| SCORE | 1-5 | 级别评分，越高越好 | `{"accuracy": 4.0, "clarity": 3.5}` |
| BINARY | {0, 1} | 二值，通过/不通过 | `{"passed": 1.0, "correct": 0.0}` |
| PERCENTAGE | 0-100 | 百分比，越高越好 | `{"precision": 85.0, "recall": 90.0}` |
| PENALTY | ≤0 | 惩罚值，越低越好 | `{"length_penalty": -0.5, "repetition": -1.0}` |
| RANK | List[int] | 排序，排列组合 | `{"relevance": [2,1,3]}` |

**SCORE 类型（1-5）的含义：**

| 级别 | 含义 | 百分比等效 |
|------|------|------------|
| 5 | 优秀 | 90-100% |
| 4 | 良好 | 75-89% |
| 3 | 一般 | 50-74% |
| 2 | 较差 | 25-49% |
| 1 | 很差 | 0-24% |

### 4. Grader 分类体系（9 大类别）

参考 OpenJudge，我们将实现以下 9 类评分器：

#### 4.1 General Graders（通用评分器）

| 评分器 | 描述 | 指标类型 | 返回指标示例 |
|--------|------|----------|-------------|
| `RelevanceGrader` | 评估回答与问题的相关性 | SCORE (1-5) | `{"relevance": 4.0, "topic_alignment": 3.5}` |
| `HallucinationGrader` | 检测幻觉内容（无依据信息） | SCORE (1-5) | `{"faithfulness": 4.5, "groundedness": 4.0}` |
| `HarmfulnessGrader` | 检测有害/不当内容 | SCORE (1-5) | `{"safety": 5.0, "bias_free": 4.5}` |
| `InstructionFollowingGrader` | 检查是否遵循指令 | BINARY | `{"followed_length": 1.0, "followed_format": 1.0}` |
| `CorrectnessGrader` | 验证答案正确性 | SCORE (1-5) | `{"accuracy": 4.0, "completeness": 3.0}` |

**示例：RelevanceGrader 返回多个同类型指标**
```python
# 同一 Grader 的所有指标都是 SCORE 类型（1-5）
GraderScore(
    name="relevance_grader",
    metric_type=MetricType.SCORE,
    metrics={
        "relevance": 4.0,        # 相关性
        "topic_alignment": 3.5,  # 主题一致性
        "clarity": 4.2,          # 清晰度
    },
    reason="回答与问题高度相关，主题一致，表达清晰",
)
```

#### 4.2 Agent Graders（Agent 评分器）

**Action 评分器：**
| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `ActionAlignmentGrader` | 评估动作与目标一致性 | LLM | {0, 1} |
| `ActionLoopDetectionGrader` | 检测重复动作循环 | Code | {0, 1} |

**Tool 评分器：**
| 评分器 | 描述 | 指标类型 | 返回指标示例 |
|--------|------|----------|-------------|
| `ToolSelectionGrader` | 评估工具选择合理性 | SCORE (1-5) | `{"selection_quality": 4.0, "efficiency": 3.5}` |
| `ToolCallAccuracyGrader` | 检查工具调用正确性 | BINARY | `{"correct_tool": 1.0, "correct_params": 1.0}` |
| `ToolCallSequenceMatchGrader` | 多步工具序列匹配 | PERCENTAGE (0-100) | `{"precision": 80.0, "recall": 100.0}` |
| `ToolCallSuccessGrader` | 检查工具调用是否成功 | BINARY | `{"api_success": 1.0, "execution_success": 0.0}` |
| `ToolParameterCheckGrader` | 验证工具参数 | BINARY | `{"required_params": 1.0, "optional_params": 1.0}` |

**示例：ToolSelectionGrader 返回多个同类型指标**
```python
# 同一 Grader 的所有指标都是 SCORE 类型（1-5）
GraderScore(
    name="tool_selection_grader",
    metric_type=MetricType.SCORE,
    metrics={
        "selection_quality": 4.0,  # 工具选择质量
        "efficiency": 3.5,          # 效率（是否选择了最少步骤）
        "appropriateness": 4.2,     # 适用性
    },
    reason="工具选择合理，但可优化效率",
)
```

**Memory 评分器：**
| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `MemoryAccuracyGrader` | 评估记忆准确性 | LLM | {0, 1} |
| `MemoryDetailPreservationGrader` | 检查重要细节保留 | LLM | {0, 1} |
| `MemoryRetrievalEffectivenessGrader` | 评估记忆检索质量 | LLM | {0, 1} |

**Plan & Reflection 评分器：**
| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `PlanFeasibilityGrader` | 评估计划可行性 | LLM | {0, 1} |
| `ReflectionAccuracyGrader` | 检查反思准确性 | LLM | {0, 1} |
| `ReflectionOutcomeUnderstandingGrader` | 评估对结果的理解 | LLM | {0, 1} |

**Trajectory 评分器：**
| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `TrajectoryAccuracyGrader` | 评估轨迹准确性 | LLM | 1-3 |
| `TrajectoryComprehensiveGrader` | 综合轨迹评估 | LLM | {0, 1} |

#### 4.3 Multi-turn Graders（多轮对话评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `ContextMemoryGrader` | 评估上下文记忆能力 | LLM | 1-5 |
| `AnaphoraResolutionGrader` | 评估代词解析能力 | LLM | 1-5 |
| `TopicSwitchGrader` | 评估话题切换处理 | LLM | 1-5 |
| `SelfCorrectionGrader` | 评估自我纠正能力 | LLM | 1-5 |
| `InstructionClarificationGrader` | 评估请求澄清能力 | LLM | 1-5 |
| `ProactiveInteractionGrader` | 评估主动交互能力 | LLM | 1-5 |

#### 4.4 Text Graders（文本评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `SimilarityGrader` | 文本相似度（15+ 算法） | Code | [0, 1] |
| `StringMatchGrader` | 字符串匹配 | Code | {0, 1} |
| `NumberAccuracyGrader` | 数值比较（容差） | Code | {0, 1} |

#### 4.5 Code Graders（代码评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `CodeExecutionGrader` | 执行代码测试 | Code | [0, 1] |
| `SyntaxCheckGrader` | Python 语法检查 | Code | {0, 1} |
| `CodeStyleGrader` | 代码风格检查 | Code | [0, 1] |
| `PatchSimilarityGrader` | 代码补丁相似度 | Code | [0, 1] |

#### 4.6 Math Graders（数学评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `MathExpressionVerifyGrader` | 数学表达式验证 | Code | {0, 1} |

#### 4.7 Format Graders（格式评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `JsonValidatorGrader` | JSON 语法验证 | Code | {0, 1} |
| `JsonMatchGrader` | JSON 深度比较 | Code | {0, 1} |
| `LengthPenaltyGrader` | 长度惩罚 | Code | ≤0 |
| `NgramRepetitionPenaltyGrader` | N-gram 重复惩罚 | Code | ≤0 |
| `ReasoningFormatGrader` | 推理格式检查 | Code | {0, 1} |

#### 4.8 Multimodal Graders（多模态评分器）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `ImageCoherenceGrader` | 图文一致性评估 | LLM | 1-5 |
| `ImageHelpfulnessGrader` | 图像有用性评估 | LLM | 1-5 |
| `TextToImageGrader` | 文生图质量评估 | LLM | 1-5 |

#### 4.9 Skill Graders（技能评分器 - Agentic）

| 评分器 | 描述 | 类型 | 分数范围 |
|--------|------|------|----------|
| `AgenticGrader` | 可使用工具的评分器 | LLM | 变化 |

### 5. 基础指标架构

**选择模块化设计：**
```
services/evaluation/
├── service.py           # gRPC 服务层
├── runner.py            # 评测编排器
├── metrics/             # 基础指标计算模块
│   ├── retrieval.py     # 检索指标
│   ├── generation.py    # 生成指标
│   ├── llm_judge.py     # LLM 裁判
│   └── semantic.py      # 语义相似度
├── graders/             # Grader 插件系统
│   ├── base.py          # BaseGrader 抽象类
│   ├── schema.py        # GraderResult, GraderScore, GraderRank, GraderError
│   ├── registry.py      # 评分器注册表
│   ├── mapping.py       # 字段映射工具
│   ├── builtin/         # 内置评分器
│   │   ├── general/     # 通用评分器
│   │   ├── agent/       # Agent 评分器
│   │   ├── multi_turn/  # 多轮对话评分器
│   │   ├── text/        # 文本评分器
│   │   ├── code/        # 代码评分器
│   │   ├── math/        # 数学评分器
│   │   ├── format/      # 格式评分器
│   │   ├── multimodal/  # 多模态评分器
│   │   └── skills/      # 技能评分器
│   └── custom/          # 自定义评分器目录
├── strategies/          # 评测策略
│   ├── base.py          # BaseStrategy 抽象类
│   ├── zero_shot.py     # 零样本评测策略
│   └── data_driven.py   # 数据驱动评测策略
└── datasets/            # 数据集管理
    ├── manager.py
    └── data/
```

### 6. 字段映射机制

**问题：** Go 端数据格式与 Python Grader 期望格式不一致

**解决方案：** 实现 `FieldMapper` 工具类

```python
class FieldMapper:
    """字段映射器，自动转换 Go 数据格式到 Python 格式"""

    # Go -> Python 字段映射
    MAPPINGS = {
        "llm": {
            "query": "question",
            "response": "output",
            "reference": "answer",
        },
        "agent": {
            "query": "question",
            "response": "trajectory",
            "reference": "expected_tools",
        },
        "rag": {
            "query": "question",
            "response": "answer",
            "reference": "ground_truth",
            "retrieved_docs": "context",
        },
    }

    @classmethod
    def map(cls, data: dict, scenario: str) -> dict:
        """映射字段"""
        mapping = cls.MAPPINGS.get(scenario, {})
        return {mapping.get(k, k): v for k, v in data.items()}
```

### 7. BaseGrader 抽象类设计

```python
from enum import Enum
from abc import ABC, abstractmethod

class GraderMode(str, Enum):
    """评分器模式"""
    POINTWISE = "pointwise"  # 单样本评分
    LISTWISE = "listwise"    # 多样本排序

class BaseGrader(ABC):
    """评分器基类"""

    def __init__(
        self,
        name: str,
        mode: GraderMode = GraderMode.POINTWISE,
        description: str = "",
    ):
        self.name = name
        self.mode = mode
        self.description = description

    @abstractmethod
    async def _aevaluate(
        self, **kwargs
    ) -> GraderScore | GraderRank | GraderError:
        """评估逻辑，子类必须实现"""
        pass

    async def aevaluate(
        self, **kwargs
    ) -> GraderScore | GraderRank | GraderError:
        """评估入口，可添加前置处理"""
        # 字段映射
        mapped_kwargs = FieldMapper.map(kwargs, self._scenario)
        return await self._aevaluate(**mapped_kwargs)

    @staticmethod
    def get_metadata() -> dict:
        """返回评分器元数据"""
        return {
            "name": "",
            "description": "",
            "score_meaning": "",
            "prompt_template": "",
        }
```

### 8. 中文分词方案

**选择 jieba + 自定义处理：**
- jieba：成熟的中文分词库
- 支持自定义词典（领域相关）
- 对于混合中英文文本，分别处理

**理由：**
- Go 的 `strings.Fields` 对中文完全无效
- Python 生态有更好的中文支持

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Python 需要连接 Go 的知识库服务 | 通过环境变量配置 Go 服务地址，使用 gRPC 调用 |
| 评测数据集更新需要重启 Python 服务 | 支持热加载，定期扫描数据集目录变化 |
| 大规模评测的内存占用 | 限制并发评测任务数，使用队列 |
| gRPC 连接断开导致评测任务丢失 | 支持任务恢复机制，Python 定期写入中间状态 |
| Grader 数量多，维护成本高 | 使用 LLM-Generated Graders，自动生成评分器 |

## Migration Plan

**Phase 1: 基础框架（已完成）**
1. ✅ 创建 `services/evaluation/` 目录结构
2. ✅ 定义 `proto/evaluation.proto`
3. ✅ 实现基础 gRPC 服务
4. ✅ 实现基础指标（检索、生成、语义）

**Phase 2: Grader 核心架构（必须）**
1. 实现 `BaseGrader` 抽象类和 `GraderResult` 类型体系
2. 实现 `MetricType` 枚举和类型验证
3. 实现 `GraderRegistry` 注册表
4. 实现 `FieldMapper` 字段映射工具

**Phase 3: 基础评分器（必须 - 示例性质）**
1. 实现 `RelevanceGrader` - 相关性评分（SCORE 类型）
2. 实现 `CorrectnessGrader` - 正确性评分（SCORE 类型）
3. 实现 `StringMatchGrader` - 字符串匹配（BINARY 类型）
4. 实现 `SimilarityGrader` - 文本相似度（PERCENTAGE 类型）

> 这些基础评分器作为参考实现，展示如何：
> - 继承 `BaseGrader`
> - 实现 `_aevaluate` 方法
> - 返回正确类型的 `GraderScore`
> - 支持多个同类型指标

**Phase 4+: 扩展评分器（按需实现）**
根据实际需求逐步添加，不预先实现：

| 优先级 | 评分器类别 | 触发条件 |
|--------|-----------|----------|
| P0 | General 类其他评分器 | 需要评估安全性、指令遵循时 |
| P1 | Agent 评分器 | 需要 Agent 评测时 |
| P2 | Multi-turn 评分器 | 需要多轮对话评测时 |
| P3 | Code/Math 评分器 | 需要代码/数学评测时 |
| P4 | Format/Multimodal | 需要格式/多模态评测时 |

**Phase 6: 集成与测试**
1. Go 端集成测试
2. 性能测试
3. 灰度发布

**Rollback Strategy:**
- Go 保留原有的评测框架作为降级方案
- 通过配置开关切换评测实现
- 新功能逐步灰度

## Open Questions

1. **Go 知识库接口格式** - 需要确认 Go 端提供什么样的检索接口
2. **评测并发控制** - 是否需要队列，还是由 Go 控制并发
3. **LLM-as-Judge 的 LLM 配置** - 使用哪个模型，API Key 如何管理
4. **大模型评测的日志** - 是否需要保存所有中间结果用于分析
5. **LLM-Generated Graders** - 是否需要支持自动生成评分器
