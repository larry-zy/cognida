"""Analytics MCP 工具单元测试：正常 / 边界 / 错误路径。

所有工具经 ``AnalyticsTool`` 基类统一返回 ``{success, data, error}`` 信封：
- 成功：``{"success": True, "data": {...}}``
- 失败：``{"success": False, "error": ..., "error_type": ...}``
"""

import json

import pytest

from tools.analytics import (
    DataAnomalyTool,
    DataCorrelationTool,
    DataDescribeTool,
    DataInsightTool,
    DataTrendTool,
)


@pytest.fixture
def rowset() -> dict:
    """{columns, rows} 形态的样例行集，含一个明显异常点。"""
    return {
        "columns": ["month", "sales", "cost"],
        "rows": [
            ["2024-01", 100, 80],
            ["2024-02", 120, 85],
            ["2024-03", 150, 90],
            ["2024-04", 170, 95],
            ["2024-05", 999, 100],
            ["2024-06", 210, 110],
        ],
    }


@pytest.fixture
def records() -> list:
    """records 数组形态的样例行集。"""
    return [
        {"x": 1, "y": 2},
        {"x": 2, "y": 4},
        {"x": 3, "y": 6},
        {"x": 4, "y": 8},
    ]


def _assert_jsonable(obj) -> None:
    """结果必须 JSON 可序列化。"""
    json.dumps(obj, ensure_ascii=False)


def _ok(result) -> dict:
    """断言成功信封并返回内层 data。"""
    _assert_jsonable(result)
    assert result["success"] is True
    return result["data"]


def _err(result) -> dict:
    """断言失败信封并返回错误体。"""
    _assert_jsonable(result)
    assert result["success"] is False
    return result


@pytest.mark.unit
class TestDataDescribeTool:
    async def test_describe_rowset(self, rowset) -> None:
        data = _ok(await DataDescribeTool().execute(data=rowset))
        assert data["row_count"] == 6
        assert set(data["columns"]) == {"sales", "cost"}
        assert "mean" in data["describe"]["sales"]
        assert "normality" in data["describe"]["sales"]

    async def test_describe_records(self, records) -> None:
        data = _ok(await DataDescribeTool().execute(data=records))
        assert set(data["columns"]) == {"x", "y"}

    async def test_column_filter(self, rowset) -> None:
        data = _ok(await DataDescribeTool().execute(data=rowset, columns=["sales"]))
        assert data["columns"] == ["sales"]

    async def test_empty_rows_error(self) -> None:
        result = _err(await DataDescribeTool().execute(data={"columns": ["a"], "rows": []}))
        assert result["error_type"] == "invalid_input"

    async def test_missing_data_error(self) -> None:
        result = _err(await DataDescribeTool().execute())
        assert "error" in result

    async def test_no_numeric_column(self) -> None:
        data = {"columns": ["name"], "rows": [["a"], ["b"], ["c"]]}
        result = _err(await DataDescribeTool().execute(data=data))
        assert result["error_type"] == "no_numeric_column"


@pytest.mark.unit
class TestDataTrendTool:
    async def test_trend_normal(self, rowset) -> None:
        data = _ok(await DataTrendTool().execute(data=rowset, value_col="sales"))
        assert data["value_col"] == "sales"
        assert "trend" in data
        assert isinstance(data["forecast"], list)

    async def test_missing_value_col(self, rowset) -> None:
        result = _err(await DataTrendTool().execute(data=rowset))
        assert result["error_type"] == "missing_param"

    async def test_unknown_column(self, rowset) -> None:
        result = _err(await DataTrendTool().execute(data=rowset, value_col="nope"))
        assert result["error_type"] == "column_not_found"

    async def test_insufficient_data(self) -> None:
        data = {"columns": ["v"], "rows": [[1], [2]]}
        result = _err(await DataTrendTool().execute(data=data, value_col="v"))
        assert result["error_type"] == "insufficient_data"


@pytest.mark.unit
class TestDataAnomalyTool:
    async def test_detects_spike(self, rowset) -> None:
        data = _ok(await DataAnomalyTool().execute(data=rowset, value_col="sales"))
        assert data["anomaly_count"] >= 1

    async def test_missing_value_col(self, rowset) -> None:
        result = _err(await DataAnomalyTool().execute(data=rowset))
        assert result["error_type"] == "missing_param"

    async def test_unknown_column(self, rowset) -> None:
        result = _err(await DataAnomalyTool().execute(data=rowset, value_col="nope"))
        assert result["error_type"] == "column_not_found"


@pytest.mark.unit
class TestDataCorrelationTool:
    async def test_correlation_normal(self, rowset) -> None:
        data = _ok(await DataCorrelationTool().execute(data=rowset))
        assert "matrix" in data["correlation"]

    async def test_requires_two_columns(self) -> None:
        data = {"columns": ["only"], "rows": [[1], [2], [3]]}
        result = _err(await DataCorrelationTool().execute(data=data))
        assert result["error_type"] == "insufficient_columns"


@pytest.mark.unit
class TestDataInsightTool:
    async def test_insight_normal(self, rowset) -> None:
        data = _ok(await DataInsightTool().execute(data=rowset))
        assert "insights" in data
        assert "recommendations" in data
        assert isinstance(data["recommendations"], list)

    async def test_no_numeric_column(self) -> None:
        data = {"columns": ["name"], "rows": [["a"], ["b"], ["c"]]}
        result = _err(await DataInsightTool().execute(data=data))
        assert result["error_type"] == "no_numeric_column"

    async def test_empty_error(self) -> None:
        result = _err(await DataInsightTool().execute(data={"columns": ["a"], "rows": []}))
        assert "error" in result


@pytest.mark.unit
class TestInputSchema:
    @pytest.mark.parametrize(
        "tool_cls",
        [
            DataDescribeTool,
            DataTrendTool,
            DataAnomalyTool,
            DataCorrelationTool,
            DataInsightTool,
        ],
    )
    def test_schema_declares_data_required(self, tool_cls) -> None:
        schema = tool_cls().input_schema
        assert schema["type"] == "object"
        assert "data" in schema["properties"]
        assert "data" in schema["required"]
