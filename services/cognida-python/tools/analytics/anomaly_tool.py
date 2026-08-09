"""data_anomaly 工具：异常点检测（IQR / zscore）。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import AnomalyInsightFinder

from ._common import (
    AnalyticsTool,
    error_result,
    records_to_df,
    to_jsonable,
    truncate,
)

logger = get_logger(__name__)


class DataAnomalyTool(AnalyticsTool):
    """对单个数值列做异常检测。"""

    @property
    def name(self) -> str:
        return "data_anomaly"

    @property
    def description(self) -> str:
        return (
            "对指定数值列检测异常点（method=iqr 或 zscore），返回异常洞察与受影响指标。"
            "需指定 value_col。"
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
                    "description": "要检测异常的数值列名（必填）",
                },
                "method": {
                    "type": "string",
                    "enum": ["iqr", "zscore"],
                    "description": "（可选）检测方法，默认 iqr",
                    "default": "iqr",
                },
                "threshold": {
                    "type": "number",
                    "description": "（可选）zscore 阈值，默认 3.0",
                    "default": 3.0,
                },
            },
            "required": ["data", "value_col"],
        }

    async def _run(self, **kwargs: Any) -> Any:
        value_col = kwargs.get("value_col")
        if not value_col:
            return error_result("缺少 value_col 参数", error_type="missing_param")

        try:
            df = records_to_df(kwargs.get("data"))
        except ValueError as e:
            return error_result(str(e))

        df, truncated = truncate(df)
        if value_col not in df.columns:
            return error_result(
                f"列 {value_col} 不存在", error_type="column_not_found"
            )

        series = df[value_col].dropna()
        if len(series) < 3:
            return error_result(
                "有效数据点不足（<3），无法检测异常", error_type="insufficient_data"
            )

        method = str(kwargs.get("method") or "iqr")
        threshold = float(kwargs.get("threshold") or 3.0)
        insights = AnomalyInsightFinder().find_insights(
            series, value_col, method=method, threshold=threshold
        )

        return {
            "value_col": value_col,
            "method": method,
            "row_count": int(len(series)),
            "anomaly_count": len(insights),
            "insights": [to_jsonable(i) for i in insights],
            "truncated": truncated,
        }
