"""data_insight 工具：综合洞察 + 行动建议。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import InsightGenerator, TimeSeriesDetector

from ._common import (
    AnalyticsTool,
    error_result,
    numeric_columns,
    records_to_df,
    to_jsonable,
    truncate,
)

logger = get_logger(__name__)


class DataInsightTool(AnalyticsTool):
    """综合趋势/异常/相关性，产出洞察与可执行建议。"""

    @property
    def name(self) -> str:
        return "data_insight"

    @property
    def description(self) -> str:
        return (
            "对行集做综合洞察：自动识别时间列与数值指标，汇总趋势、异常、相关性洞察，"
            "按严重度排序并给出行动建议（recommendations）。用于产出有计算依据的结论。"
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
                "time_col": {
                    "type": "string",
                    "description": "（可选）时间列名；省略则自动识别",
                },
                "value_cols": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "（可选）要分析的数值指标列；省略则用全部数值列",
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

        # 时间列：显式优先，否则自动识别（无则为 None）
        time_col = kwargs.get("time_col")
        if not time_col:
            try:
                time_col = TimeSeriesDetector.detect_time_column(df)
            except Exception:  # noqa: BLE001 — 无时间列不致命
                time_col = None

        value_cols = numeric_columns(df, kwargs.get("value_cols"))
        if not value_cols:
            return error_result("未找到可分析的数值列", error_type="no_numeric_column")

        insights = InsightGenerator().generate(
            df, time_col=time_col, value_cols=value_cols
        )

        recommendations: list[str] = []
        for ins in insights:
            recommendations.extend(getattr(ins, "recommendations", None) or [])
        # 去重保序
        seen: set[str] = set()
        recommendations = [
            r for r in recommendations if not (r in seen or seen.add(r))
        ]

        return {
            "time_col": time_col,
            "value_cols": value_cols,
            "row_count": int(len(df)),
            "insight_count": len(insights),
            "insights": [to_jsonable(i) for i in insights],
            "recommendations": recommendations,
            "truncated": truncated,
        }
