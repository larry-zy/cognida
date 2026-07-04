"""data_attribution 工具：指标变化的维度归因（方差分解 + 驱动排序 + 隐藏因子）。

按 result_id 契约对接：Go 侧编排负责把 result_id 解析成行集后经 MCP 传入
（``data`` 为 {columns, rows} 或 records 数组），计算侧只做确定性归因算法，
文字洞察与 result_id 信封由 Go 侧拼装。
"""

from __future__ import annotations

from typing import Any

from core import get_logger
from services.analytics import AttributionAnalyzer

from ._common import (
    AnalyticsTool,
    error_result,
    records_to_df,
    to_jsonable,
    truncate,
)

logger = get_logger(__name__)


class DataAttributionTool(AnalyticsTool):
    """对指标两期变化按维度切片做可加归因分解。"""

    @property
    def name(self) -> str:
        return "data_attribution"

    @property
    def description(self) -> str:
        return (
            "归因/根因分析：对指标在基期→现期的变化按维度切片做可加方差分解，"
            "输出驱动因子排序（delta/贡献占比/方向）、维度解释力排名、隐藏因子提示"
            "与口径/置信标注。需 value_col 与 period_col；dim_col 省略时自动扫描"
            "最具解释力的维度。"
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
                "period_col": {
                    "type": "string",
                    "description": "期间列名（必填），用于切分基期/现期",
                },
                "dim_col": {
                    "type": "string",
                    "description": "（可选）归因维度列；省略则自动扫描最具解释力维度",
                },
                "baseline": {
                    "description": "（可选）基期取值；省略取倒数第二期",
                },
                "current": {
                    "description": "（可选）现期取值；省略取最后一期",
                },
                "top_n": {
                    "type": "integer",
                    "description": "（可选）驱动因子保留数，默认 10",
                    "default": 10,
                },
            },
            "required": ["data", "value_col", "period_col"],
        }

    async def _run(self, **kwargs: Any) -> Any:
        value_col = kwargs.get("value_col")
        period_col = kwargs.get("period_col")
        if not value_col:
            return error_result("缺少 value_col 参数", error_type="missing_param")
        if not period_col:
            return error_result("缺少 period_col 参数", error_type="missing_param")

        try:
            df = records_to_df(kwargs.get("data"))
        except ValueError as e:
            return error_result(str(e))

        df, truncated = truncate(df)

        try:
            result = AttributionAnalyzer.decompose(
                df,
                metric_col=value_col,
                period_col=period_col,
                dim_col=kwargs.get("dim_col"),
                baseline=kwargs.get("baseline"),
                current=kwargs.get("current"),
                top_n=int(kwargs.get("top_n") or AttributionAnalyzer.DEFAULT_TOP_N),
            )
        except ValueError as e:
            return error_result(str(e))

        out = to_jsonable(result)
        out["truncated"] = truncated
        return out
