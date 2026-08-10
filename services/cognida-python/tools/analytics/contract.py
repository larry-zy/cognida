"""Analytics 工具名共享契约〔M13 P1〕。

把原先散落在 7 个 ``*_tool.py`` 的裸字符串工具名收敛为**单一真源 + 类型化**：
各工具的 ``name`` 属性、bootstrap/registry 注册键均以本枚举为准，wire 值保持不变。

Go 侧对照真源见 services/cognida-go/internal/service/agent/tools/contract.go 的
``MCPToolName`` 常量集；两侧锁定测试互为跨语言对照锚点，任一侧改 wire 值都会被打红。
"""

from __future__ import annotations

from enum import StrEnum


class ToolName(StrEnum):
    """7 个 analytics MCP 工具名（wire 值不可变）。

    继承 ``StrEnum``：成员本身即等于其字符串值，可直接用作 dict 键 / JSON 序列化，
    与既有 registry（以 ``tool.name`` 作 key）的行为完全一致。
    """

    DESCRIBE = "data_describe"
    TREND = "data_trend"
    ANOMALY = "data_anomaly"
    CORRELATION = "data_correlation"
    INSIGHT = "data_insight"
    COMPARISON = "data_comparison"
    ATTRIBUTION = "data_attribution"
