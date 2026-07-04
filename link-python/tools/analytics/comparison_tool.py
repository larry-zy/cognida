"""data_comparison 工具：维度分组对比（组间聚合、差异与显著性）。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import GroupComparison

from ._common import (
    AnalyticsTool,
    error_result,
    records_to_df,
    to_jsonable,
    truncate,
)

logger = get_logger(__name__)


class DataComparisonTool(AnalyticsTool):
    """按维度分组对比指标：sum/mean/count、占比排名，两组时附差异与 t 检验。"""

    @property
    def name(self) -> str:
        return "data_comparison"

    @property
    def description(self) -> str:
        return (
            "对比分析：按维度列分组对比指标（sum/mean/count 与总量占比，按 sum 排名）。"
            "恰好两组时补充差值、差异百分比与独立样本 t 检验显著性。"
            "需 value_col 与 dim_col；groups 可选定要对比的分组取值。"
        )

    @property
    def input_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "data": {
                    "type": ["object", "array"],
                    "description": "行集，{columns, rows} 或 records 数组",
                },
                "value_col": {
                    "type": "string",
                    "description": "指标列名（必填）",
                },
                "dim_col": {
                    "type": "string",
                    "description": "分组维度列名（必填）",
                },
                "groups": {
                    "type": "array",
                    "description": "（可选）只对比这些分组取值；恰两组时附显著性检验",
                },
            },
            "required": ["data", "value_col", "dim_col"],
        }

    async def _run(self, **kwargs: Any) -> Any:
        value_col = kwargs.get("value_col")
        dim_col = kwargs.get("dim_col")
        if not value_col:
            return error_result("缺少 value_col 参数", error_type="missing_param")
        if not dim_col:
            return error_result("缺少 dim_col 参数", error_type="missing_param")

        try:
            df = records_to_df(kwargs.get("data"))
        except ValueError as e:
            return error_result(str(e))

        df, truncated = truncate(df)

        try:
            result = GroupComparison.compare(
                df,
                metric_col=value_col,
                dim_col=dim_col,
                groups=kwargs.get("groups"),
            )
        except ValueError as e:
            return error_result(str(e))

        out = to_jsonable(result)
        out["truncated"] = truncated
        return out
