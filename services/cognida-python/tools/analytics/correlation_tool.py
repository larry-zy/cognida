"""data_correlation 工具：相关性分析。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import CorrelationAnalysis

from ._common import (
    AnalyticsTool,
    error_result,
    numeric_columns,
    records_to_df,
    to_jsonable,
    truncate,
)
from .contract import ToolName

logger = get_logger(__name__)


class DataCorrelationTool(AnalyticsTool):
    """对 ≥2 个数值列计算相关性矩阵与显著相关对。"""

    @property
    def name(self) -> str:
        return ToolName.CORRELATION.value

    @property
    def description(self) -> str:
        return (
            "计算多个数值列之间的相关性矩阵（pearson/spearman/kendall），"
            "并返回超过阈值的显著相关变量对。需至少 2 个数值列。"
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
                "columns": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "（可选）参与相关性分析的数值列；省略则用全部数值列",
                },
                "method": {
                    "type": "string",
                    "enum": ["pearson", "spearman", "kendall"],
                    "description": "（可选）相关系数方法，默认 pearson",
                    "default": "pearson",
                },
                "threshold": {
                    "type": "number",
                    "description": "（可选）显著相关阈值（绝对值），默认 0.7",
                    "default": 0.7,
                },
            },
            "required": ["data"],
        }

    async def _run(self, **kwargs: Any) -> Any:
        try:
            df = records_to_df(kwargs.get("data"))
        except ValueError as e:
            return error_result(str(e))

        df, truncated = truncate(df)
        cols = numeric_columns(df, kwargs.get("columns"))
        if len(cols) < 2:
            return error_result(
                "数值列不足（需 ≥2 列）以计算相关性", error_type="insufficient_columns"
            )

        method = str(kwargs.get("method") or "pearson")
        threshold = float(kwargs.get("threshold") or 0.7)
        result = CorrelationAnalysis.compute_correlation(
            df[cols], method=method, threshold=threshold
        )

        return {
            "columns": cols,
            "row_count": int(len(df)),
            "correlation": to_jsonable(result),
            "truncated": truncated,
        }
