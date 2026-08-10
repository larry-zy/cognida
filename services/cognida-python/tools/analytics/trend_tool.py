"""data_trend 工具：线性趋势 + 预测 + 增长率。"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import GrowthRateAnalyzer, LinearTrendAnalyzer
from services.analytics import TimeSeriesDetector

from ._common import (
    AnalyticsTool,
    error_result,
    records_to_df,
    to_jsonable,
    truncate,
)
from .contract import ToolName

logger = get_logger(__name__)


class DataTrendTool(AnalyticsTool):
    """对时间序列做线性趋势分析、预测与增长率计算。"""

    @property
    def name(self) -> str:
        return ToolName.TREND.value

    @property
    def description(self) -> str:
        return (
            "对时间序列数值列做线性趋势分析（方向/强度/斜率/显著性）、未来预测与增长率"
            "（环比/同比/CAGR）。需指定 value_col；time_col 省略时自动识别时间列。"
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
                    "description": "要分析的数值列名（必填）",
                },
                "time_col": {
                    "type": "string",
                    "description": "（可选）时间列名；省略则自动识别",
                },
                "forecast_steps": {
                    "type": "integer",
                    "description": "（可选）向前预测的步数，默认 5",
                    "default": 5,
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

        # 时间列：显式指定优先，否则自动识别后按时间排序
        time_col = kwargs.get("time_col")
        try:
            df, time_col = TimeSeriesDetector.validate_time_series(df, time_col)
        except Exception:  # noqa: BLE001 — 无时间列不致命，退化为按行序分析
            time_col = None
        if time_col and time_col in df.columns:
            df = df.sort_values(time_col)

        series = df[value_col].dropna()
        if len(series) < 3:
            return error_result(
                "有效数据点不足（<3），无法分析趋势", error_type="insufficient_data"
            )

        steps = int(kwargs.get("forecast_steps") or 5)
        trend = LinearTrendAnalyzer.analyze(series)
        forecast = LinearTrendAnalyzer.forecast(series, steps=steps)

        growth: dict[str, Any] = {}
        try:
            growth["period_over_period"] = to_jsonable(
                GrowthRateAnalyzer.period_over_period(series)
            )
            growth["cagr"] = to_jsonable(GrowthRateAnalyzer.cagr(series))
        except Exception as e:  # noqa: BLE001 — 增长率为附加信息，失败不致命
            logger.debug("growth rate computation skipped: %s", e)

        return {
            "value_col": value_col,
            "time_col": time_col,
            "row_count": int(len(series)),
            "trend": to_jsonable(trend),
            "forecast": to_jsonable(forecast),
            "growth": growth,
            "truncated": truncated,
        }
