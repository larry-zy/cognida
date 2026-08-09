"""评分器基类和结果类型。

参考 OpenJudge 设计，提供统一的评分器接口和结果类型。
核心规则：同一 Grader 的多个指标必须基于同一类型。
"""

import logging
from abc import ABC, abstractmethod
from enum import Enum
from typing import Any, Callable, Dict, List, Optional

from pydantic import BaseModel, Field, field_validator

logger = logging.getLogger(__name__)


# ============================================================
# 指标类型枚举
# ============================================================

class MetricType(str, Enum):
    """指标类型枚举。

    同一 Grader 的所有指标必须是同一种类型。
    """
    SCORE = "score"           # 数值分数（如 1-5, 0-1）
    BINARY = "binary"         # 二值（{0, 1}）
    RANK = "rank"             # 排序（List[int]）
    PENALTY = "penalty"       # 惩罚（≤0）
    PERCENTAGE = "percentage" # 百分比（[0, 100]）

    def validate_value(self, value: float) -> bool:
        """验证值是否符合该类型的范围。"""
        if self == MetricType.SCORE:
            return 1 <= value <= 5
        elif self == MetricType.BINARY:
            return value in {0.0, 1.0}
        elif self == MetricType.PENALTY:
            return value <= 0
        elif self == MetricType.PERCENTAGE:
            return 0 <= value <= 100
        elif self == MetricType.RANK:
            return isinstance(value, int) and value >= 0
        return True


class GraderMode(str, Enum):
    """评分器模式。"""
    POINTWISE = "pointwise"  # 单样本评分
    LISTWISE = "listwise"    # 多样本排序


class EvalType(str, Enum):
    """评测类型。

    每个 grader 声明其适用的评测类型;创建某类型任务时只展示该类型可用的指标。
    `qa` 是 `llm` 的别名(向后兼容),规范化后统一为 `llm`。
    """
    LLM = "llm"       # 大模型/通用 QA 生成评测(含 qa 别名)
    RAG = "rag"       # 检索增强生成评测
    AGENT = "agent"   # 智能体评测
    SQL = "sql"       # Text2SQL / SQL 生成评测（结构匹配 + 执行准确率）


# `qa` 历史命名等价于 `llm`
_EVAL_TYPE_ALIASES = {"qa": EvalType.LLM}


def normalize_eval_type(value: str | EvalType) -> EvalType:
    """将外部传入的评测类型规范化为 EvalType(处理 qa->llm 别名)。

    Raises:
        ValueError: 无法识别的评测类型
    """
    if isinstance(value, EvalType):
        return value
    key = str(value).strip().lower()
    if key in _EVAL_TYPE_ALIASES:
        return _EVAL_TYPE_ALIASES[key]
    return EvalType(key)  # 非法值抛 ValueError


# ============================================================
# 结果类型
# ============================================================

class GraderResult(BaseModel):
    """评分器结果基类。"""
    name: str = Field(..., description="评分器名称")
    metric_type: MetricType = Field(..., description="指标类型（同一 Grader 所有指标必须一致）")
    reason: str = Field("", description="评分理由")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="元数据")


class GraderScore(GraderResult):
    """数值型评分结果（Pointwise）。

    所有指标必须是同一类型（SCORE/BINARY/PENALTY/PERCENTAGE）。
    """
    metrics: Dict[str, float] = Field(
        default_factory=dict,
        description="多个指标，如 {'accuracy': 4.0, 'completeness': 3.5}"
    )

    @field_validator("metrics")
    @classmethod
    def validate_same_type(cls, v: Dict[str, float], info) -> Dict[str, float]:
        """验证所有指标是否同类型。"""
        if not v:
            return v

        metric_type = info.data.get("metric_type", MetricType.SCORE)

        for key, value in v.items():
            if isinstance(value, (int, float)):
                value = float(value)
                v[key] = value

                if not metric_type.validate_value(value):
                    type_ranges = {
                        MetricType.SCORE: "1-5",
                        MetricType.BINARY: "0 或 1",
                        MetricType.PENALTY: "≤ 0",
                        MetricType.PERCENTAGE: "0-100",
                    }
                    raise ValueError(
                        f"{key}={value} 不符合 {metric_type.value} 类型范围 "
                        f"({type_ranges.get(metric_type, '未知')})"
                    )
        return v

    @property
    def score(self) -> float:
        """获取主分数（第一个指标）。"""
        return next(iter(self.metrics.values())) if self.metrics else 0.0

    @property
    def has_multiple_metrics(self) -> bool:
        """是否有多个指标。"""
        return len(self.metrics) > 1


class GraderRank(GraderResult):
    """排序结果（Listwise）。

    所有排序指标必须是 RANK 类型。
    """
    metrics: Dict[str, List[int]] = Field(
        default_factory=dict,
        description="多个排序维度，如 {'relevance': [2,1,3], 'quality': [1,2,3]}"
    )

    @field_validator("metrics")
    @classmethod
    def validate_rank_type(cls, v: Dict[str, List[int]]) -> Dict[str, List[int]]:
        """验证排序指标。"""
        for key, value_list in v.items():
            if not isinstance(value_list, list):
                raise ValueError(f"{key} 的值必须是列表")

            for i, value in enumerate(value_list):
                if not isinstance(value, int) or value < 0:
                    raise ValueError(f"{key}[{i}]={value} 必须是非负整数")
        return v


class GraderError(GraderResult):
    """错误结果。"""
    error: str = Field(..., description="错误信息")


# ============================================================
# 评分器基类
# ============================================================

class BaseGrader(ABC):
    """评分器基类。

    所有评分器必须继承此类并实现 _aevaluate 方法。
    """

    def __init__(
        self,
        name: str,
        mode: GraderMode = GraderMode.POINTWISE,
        description: str = "",
        label: str = "",
        group: str = "general",
        eval_types: Optional[List[EvalType | str]] = None,
        requires_reference: bool = False,
        requires_contexts: bool = False,
    ):
        """初始化评分器。

        Args:
            name: 评分器名称（唯一标识，如 "rouge"）
            mode: 评分器模式（pointwise/listwise）
            description: 评分器描述
            label: 展示名称（面向用户，缺省时回退到 name）
            group: 所属分组（用于前端目录归类，如 "generation"/"retrieval"/"rag"）
            eval_types: 适用的评测类型列表（llm/rag/agent,可多值共用）；缺省视为通用(全类型)
            requires_reference: 是否需要参考答案
            requires_contexts: 是否需要检索上下文
        """
        self.name = name
        self.mode = mode
        self.description = description
        self.label = label or name
        self.group = group
        # 规范化 eval_types（处理 qa->llm 别名）；缺省表示适用于所有类型
        if eval_types:
            self.eval_types = [normalize_eval_type(t) for t in eval_types]
        else:
            self.eval_types = list(EvalType)
        self.requires_reference = requires_reference
        self.requires_contexts = requires_contexts

    @abstractmethod
    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore | GraderRank | GraderError:
        """评估逻辑，子类必须实现。

        Args:
            query: 问题/查询
            response: 生成的回答
            reference: 参考答案（可选）
            context: 上下文列表（可选，如检索到的文档）
            **kwargs: 其他参数

        Returns:
            评分结果
        """
        pass

    async def aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore | GraderRank | GraderError:
        """评估入口，可添加前置处理。

        Args:
            query: 问题/查询
            response: 生成的回答
            reference: 参考答案（可选）
            context: 上下文列表（可选）
            **kwargs: 其他参数

        Returns:
            评分结果
        """
        try:
            return await self._aevaluate(query, response, reference, context, **kwargs)
        except Exception as e:
            # 显式记录：aevaluate 把异常收敛成 GraderError 对象返回（不抛出），而通用聚合路径
            # 只保留 GraderScore、静默丢弃 GraderError——若不落日志，某 grader 全批失败时该指标会
            # 无声从聚合中消失（既不在结果里也不在 unsupported 里），无从排查。
            logger.warning("grader %s 评估失败: %s", self.name, e, exc_info=True)
            return GraderError(
                name=self.name,
                metric_type=MetricType.SCORE,
                reason=f"评估失败: {str(e)}",
                error=str(e)
            )

    def get_metadata(self) -> Dict[str, Any]:
        """返回评分器元数据。

        Returns:
            包含 name, description, mode, parameters 等信息的字典
        """
        return {
            "name": self.name,
            "label": self.label,
            "description": self.description,
            "mode": self.mode.value if isinstance(self.mode, GraderMode) else self.mode,
            "group": self.group,
            "eval_types": [
                t.value if isinstance(t, EvalType) else t for t in self.eval_types
            ],
            "requires_reference": self.requires_reference,
            "requires_contexts": self.requires_contexts,
        }

    def applies_to(self, eval_type: EvalType | str) -> bool:
        """判断该 grader 是否适用于指定评测类型。"""
        try:
            target = normalize_eval_type(eval_type)
        except ValueError:
            return False
        return target in self.eval_types

    @classmethod
    def from_config(cls, config: Dict[str, Any]) -> "BaseGrader":
        """从配置创建评分器实例。

        Args:
            config: 配置字典

        Returns:
            评分器实例
        """
        return cls(**config)


# ============================================================
# 函数式评分器装饰器
# ============================================================

def function_grader(
    name: str,
    mode: GraderMode = GraderMode.POINTWISE,
    description: str = "",
):
    """将函数转换为评分器的装饰器。

    Args:
        name: 评分器名称
        mode: 评分器模式
        description: 评分器描述

    Returns:
        装饰器函数
    """
    def decorator(func: Callable[..., Any]) -> BaseGrader:
        class FunctionGrader(BaseGrader):
            def __init__(self, func: Callable[..., Any]) -> None:
                super().__init__(name, mode, description or func.__doc__)
                self._func = func

            async def _aevaluate(self, **kwargs) -> GraderScore | GraderRank | GraderError:
                result = self._func(**kwargs)
                if isinstance(result, (GraderScore, GraderRank, GraderError)):
                    return result
                # 假设返回的是分数，包装为 GraderScore
                return GraderScore(
                    name=self.name,
                    metric_type=MetricType.SCORE,
                    reason="",
                    metrics={"score": float(result)}
                )

        return FunctionGrader(func)
    return decorator
