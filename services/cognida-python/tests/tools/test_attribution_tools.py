"""data_attribution / data_comparison MCP 工具契约测试。

验证行集契约（{columns, rows} / records）与 ``{success, data, error}`` 信封，
即 Go 侧 result_id 编排解析行集后经 MCP 传入的形态（任务 4a.4 / 4a.5）。
"""

import json

import pytest

from tools.analytics import DataAttributionTool, DataComparisonTool


@pytest.fixture
def rowset() -> dict:
    """{columns, rows} 形态（与 Go sql_execute / Result Store 行集对齐）。"""
    return {
        "columns": ["month", "region", "gmv"],
        "rows": [
            ["2026-05", "华北", 1000],
            ["2026-05", "华东", 800],
            ["2026-06", "华北", 600],
            ["2026-06", "华东", 850],
        ],
    }


def _ok(result) -> dict:
    json.dumps(result, ensure_ascii=False)  # 必须 JSON 可序列化
    assert result["success"] is True
    return result["data"]


def _err(result) -> dict:
    json.dumps(result, ensure_ascii=False)
    assert result["success"] is False
    return result


class TestDataAttributionTool:
    @pytest.mark.asyncio
    async def test_rowset_contract(self, rowset):
        data = _ok(
            await DataAttributionTool().execute(
                data=rowset, value_col="gmv", period_col="month", dim_col="region"
            )
        )
        assert data["metric"] == "gmv"
        assert data["drivers"][0]["segment"] == "华北"
        assert data["drivers"][0]["contribution_pct"] == pytest.approx(
            -400 / -350 * 100
        )
        # 口径/置信/样本标注必须齐全（Go 侧信封拼装的依据）
        assert data["caliber"]["method"] == "additive_variance_decomposition"
        assert data["confidence"] in ("high", "medium", "low")
        assert data["sample"]["rows_used"] == 4

    @pytest.mark.asyncio
    async def test_records_contract_with_autoscan(self):
        records = [
            {"p": "05", "seg": "x", "v": 100},
            {"p": "05", "seg": "y", "v": 100},
            {"p": "06", "seg": "x", "v": 40},
            {"p": "06", "seg": "y", "v": 100},
        ]
        data = _ok(
            await DataAttributionTool().execute(
                data=records, value_col="v", period_col="p"
            )
        )
        assert data["dimension"] == "seg"
        assert data["drivers"][0]["segment"] == "x"

    @pytest.mark.asyncio
    async def test_missing_params(self, rowset):
        r = _err(await DataAttributionTool().execute(data=rowset, period_col="month"))
        assert r["error_type"] == "missing_param"
        r = _err(await DataAttributionTool().execute(data=rowset, value_col="gmv"))
        assert r["error_type"] == "missing_param"

    @pytest.mark.asyncio
    async def test_bad_column(self, rowset):
        r = _err(
            await DataAttributionTool().execute(
                data=rowset, value_col="nope", period_col="month"
            )
        )
        assert "nope" in r["error"]

    @pytest.mark.asyncio
    async def test_empty_data(self):
        r = _err(
            await DataAttributionTool().execute(
                data=None, value_col="v", period_col="p"
            )
        )
        assert "data" in r["error"]


class TestDataComparisonTool:
    @pytest.mark.asyncio
    async def test_rowset_contract(self, rowset):
        data = _ok(
            await DataComparisonTool().execute(
                data=rowset, value_col="gmv", dim_col="region"
            )
        )
        assert [g["group"] for g in data["groups"]] == ["华东", "华北"]
        assert "pairwise" in data  # 恰两组附差异对比

    @pytest.mark.asyncio
    async def test_missing_params(self, rowset):
        r = _err(await DataComparisonTool().execute(data=rowset, value_col="gmv"))
        assert r["error_type"] == "missing_param"
