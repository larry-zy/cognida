"""data_describe 工具：描述统计 + 分布分析。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import DescriptiveStats, DistributionAnalysis

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


class DataDescribeTool(AnalyticsTool):
    """对行集中的数值列计算描述统计与正态性。"""

    @property
    def name(self) -> str:
        return ToolName.DESCRIBE.value

    @property
    def description(self) -> str:
        return (
            "对行集中的数值列计算描述统计（count/mean/median/std/min/max/Q25/Q75/IQR、偏度峰度）"
            "及正态性检验。用于了解数据分布概况。"
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
                    "description": "（可选）要分析的数值列；省略则分析全部数值列",
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
        if not cols:
            return error_result("未找到可分析的数值列", error_type="no_numeric_column")

        describe: dict[str, Any] = {}
        for col, res in DescriptiveStats.describe_df(df[cols]).items():
            entry = to_jsonable(res)
            entry["normality"] = to_jsonable(
                DistributionAnalysis.test_normality(df[col])
            )
            describe[col] = entry

        return {
            "row_count": int(len(df)),
            "columns": cols,
            "describe": describe,
            "truncated": truncated,
        }
