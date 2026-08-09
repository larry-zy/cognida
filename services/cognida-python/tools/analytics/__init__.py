"""Analytics MCP 工具包。

把 services/analytics 引擎包装为可经 MCP 调用的 BaseTool 工具。
"""

from .anomaly_tool import DataAnomalyTool
from .attribution_tool import DataAttributionTool
from .comparison_tool import DataComparisonTool
from .correlation_tool import DataCorrelationTool
from .describe_tool import DataDescribeTool
from .insight_tool import DataInsightTool
from .trend_tool import DataTrendTool

#: 全部 analytics 工具类，供 bootstrap 统一注册
ANALYTICS_TOOLS = [
    DataDescribeTool,
    DataTrendTool,
    DataAnomalyTool,
    DataCorrelationTool,
    DataInsightTool,
    DataAttributionTool,
    DataComparisonTool,
]

__all__ = [
    "ANALYTICS_TOOLS",
    "DataAnomalyTool",
    "DataAttributionTool",
    "DataComparisonTool",
    "DataCorrelationTool",
    "DataDescribeTool",
    "DataInsightTool",
    "DataTrendTool",
]
