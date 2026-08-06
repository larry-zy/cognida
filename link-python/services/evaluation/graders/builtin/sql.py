"""Text2SQL / SQL 生成评测器（结构匹配 + 执行准确率）。

与 agent 评测器同一模式：这些指标在 metrics/sql.py 里有整批算子，此处登记为正式
grader（eval_types=[SQL], group="sql"），使目录成为唯一事实来源、前端能按 sql 类型
筛选出这三项。

批量端点（compute_metrics）通过 `_SQL_GRADERS` 集合优先拦截这些名字，走
compute_sql_metrics（结果集无序比对等需跨字段协同），不落到下面的逐样本 `_aevaluate`。
因此这里的 `_aevaluate` 是"单样本可执行"语义的诚实实现，供 /evaluate 单条接口
与目录"每项都对应可执行 grader"的约定使用。
"""

from typing import Any, List, Optional

from ...graders.base import (
    BaseGrader,
    EvalType,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader
from ...metrics.sql import (
    sql_component_match,
    sql_exact_match,
    sql_execution_accuracy,
)


@register_grader("sql_exact_match")
class SQLExactMatchGrader(BaseGrader):
    """SQL 精确匹配评分器（金标准与生成 SQL 归一化后相等，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="sql_exact_match",
            mode=GraderMode.POINTWISE,
            description="生成 SQL 与金标准 SQL 归一化后是否完全相等（大小写/空白/引号不敏感）",
            label="SQL 精确匹配",
            group="sql",
            eval_types=[EvalType.SQL],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        gold_sql: Optional[str] = None,
        generated_sql: Optional[str] = None,
        **kwargs,
    ) -> GraderScore:
        gold = gold_sql if gold_sql is not None else (reference or "")
        gen = generated_sql if generated_sql is not None else (response or "")
        v = sql_exact_match(gold, gen)
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"SQL 精确匹配: {v:.0f}",
            metrics={"sql_exact_match": v * 100, "sql_exact_match_raw": v},
        )


@register_grader("sql_component_match")
class SQLComponentMatchGrader(BaseGrader):
    """SQL 结构组件评分器（Spider 风格子句组件 F1，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="sql_component_match",
            mode=GraderMode.POINTWISE,
            description="生成 SQL 与金标准的结构组件（子句/表/列）集合 F1，容忍等价改写",
            label="SQL 结构匹配",
            group="sql",
            eval_types=[EvalType.SQL],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        gold_sql: Optional[str] = None,
        generated_sql: Optional[str] = None,
        **kwargs,
    ) -> GraderScore:
        gold = gold_sql if gold_sql is not None else (reference or "")
        gen = generated_sql if generated_sql is not None else (response or "")
        v = sql_component_match(gold, gen)
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"SQL 结构组件 F1: {v:.4f}",
            metrics={"sql_component_match": v * 100, "sql_component_match_raw": v},
        )


@register_grader("sql_execution_accuracy")
class SQLExecutionAccuracyGrader(BaseGrader):
    """SQL 执行准确率评分器（金标准/生成结果集无序等价，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="sql_execution_accuracy",
            mode=GraderMode.POINTWISE,
            description="金标准 SQL 与生成 SQL 只读执行后结果集是否等价（忽略行顺序）",
            label="SQL 执行准确率",
            group="sql",
            eval_types=[EvalType.SQL],
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        gold_result_set: Optional[List[Any]] = None,
        result_set: Optional[List[Any]] = None,
        **kwargs,
    ) -> GraderScore:
        v = sql_execution_accuracy(gold_result_set, result_set)
        if v is None:
            # 结果集缺失（未执行/执行失败）：单条语义下记 0 并说明，避免抛错阻断。
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少结果集，无法比对执行准确率",
                metrics={"sql_execution_accuracy": 0.0, "sql_execution_accuracy_raw": 0.0},
            )
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"SQL 执行准确率: {v:.0f}",
            metrics={"sql_execution_accuracy": v * 100, "sql_execution_accuracy_raw": v},
        )
