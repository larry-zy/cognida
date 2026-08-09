"""归因引擎单元测试 + driver ranking 稳定性集成测试。

覆盖（openspec data-agent-evolution 任务 4a.5 / 4a.6 计算侧）：
- 可加分解：各切片 delta 之和恰等于总 delta
- driver ranking：按 |delta| 降序且稳定
- 隐藏因子：驱动分散 / 新增切片提示
- 口径与置信标注
- 维度自动扫描
- 对比：分组聚合 + 两组显著性
- 集成：真实规模数据乱序重排后 ranking 完全一致
"""

import random

import pandas as pd
import pytest

from services.analytics import (
    AttributionAnalyzer,
    GroupComparison,
    UNKNOWN_SEGMENT,
)


@pytest.fixture
def region_df() -> pd.DataFrame:
    """两期 × 三区域的汇总行集：华北大幅下降，华东微涨，华南持平。"""
    return pd.DataFrame(
        [
            {"month": "2026-05", "region": "华北", "gmv": 1000},
            {"month": "2026-05", "region": "华东", "gmv": 800},
            {"month": "2026-05", "region": "华南", "gmv": 500},
            {"month": "2026-06", "region": "华北", "gmv": 600},
            {"month": "2026-06", "region": "华东", "gmv": 850},
            {"month": "2026-06", "region": "华南", "gmv": 500},
        ]
    )


class TestAttributionDecompose:
    def test_additive_decomposition(self, region_df):
        """可加性：切片 delta 之和 == 总 delta；总量与期间取值正确。"""
        r = AttributionAnalyzer.decompose(
            region_df, metric_col="gmv", period_col="month", dim_col="region"
        )
        assert r.baseline["label"] == "2026-05"
        assert r.current["label"] == "2026-06"
        assert r.total_delta == pytest.approx(-350)
        assert sum(d.delta for d in r.drivers) == pytest.approx(r.total_delta)
        # 贡献占比也应可加至 100%
        assert sum(d.contribution_pct for d in r.drivers) == pytest.approx(100)

    def test_driver_ranking_order(self, region_df):
        """ranking：|delta| 降序，华北（-400）应为头号驱动且方向 down。"""
        r = AttributionAnalyzer.decompose(
            region_df, metric_col="gmv", period_col="month", dim_col="region"
        )
        assert [d.segment for d in r.drivers] == ["华北", "华东", "华南"]
        top = r.drivers[0]
        assert top.delta == pytest.approx(-400)
        assert top.direction == "down"
        assert r.drivers[2].direction == "flat"

    def test_caliber_annotation(self, region_df):
        """口径标注：指标/维度/期间/聚合/方法齐全（任务 4a.3 契约的计算侧）。"""
        r = AttributionAnalyzer.decompose(
            region_df, metric_col="gmv", period_col="month", dim_col="region"
        )
        assert r.caliber == {
            "metric_col": "gmv",
            "dim_col": "region",
            "period_col": "month",
            "baseline": "2026-05",
            "current": "2026-06",
            "agg": "sum",
            "method": "additive_variance_decomposition",
        }
        assert r.confidence in ("high", "medium", "low")
        assert r.sample["rows_used"] == 6

    def test_explicit_periods(self, region_df):
        """显式指定基期/现期。"""
        r = AttributionAnalyzer.decompose(
            region_df,
            metric_col="gmv",
            period_col="month",
            dim_col="region",
            baseline="2026-06",
            current="2026-05",
        )
        assert r.total_delta == pytest.approx(350)

    def test_new_segment_flagged(self):
        """现期新增切片应标记 new_segment 并进入隐藏因子提示。"""
        df = pd.DataFrame(
            [
                {"month": "05", "ch": "直营", "v": 100},
                {"month": "06", "ch": "直营", "v": 100},
                {"month": "06", "ch": "新渠道", "v": 300},
            ]
        )
        r = AttributionAnalyzer.decompose(
            df, metric_col="v", period_col="month", dim_col="ch"
        )
        top = r.drivers[0]
        assert top.segment == "新渠道" and top.new_segment
        assert r.hidden_factor["suspected"]
        assert any("新增/消失" in n for n in r.hidden_factor["notes"])

    def test_diffuse_drivers_hint_hidden_factor(self):
        """驱动高度分散（每切片贡献≈均匀）→ 隐藏因子提示。"""
        rows = []
        for i in range(20):
            rows.append({"p": "a", "seg": f"s{i:02d}", "v": 100})
            rows.append({"p": "b", "seg": f"s{i:02d}", "v": 110})
        df = pd.DataFrame(rows)
        r = AttributionAnalyzer.decompose(
            df, metric_col="v", period_col="p", dim_col="seg"
        )
        assert r.hidden_factor["suspected"]
        assert any("驱动分散" in n for n in r.hidden_factor["notes"])

    def test_unknown_segment_kept_additive(self):
        """维度空值归入未知切片，可加性不丢行。"""
        df = pd.DataFrame(
            [
                {"p": "a", "seg": "x", "v": 10},
                {"p": "a", "seg": None, "v": 5},
                {"p": "b", "seg": "x", "v": 12},
                {"p": "b", "seg": None, "v": 30},
            ]
        )
        r = AttributionAnalyzer.decompose(
            df, metric_col="v", period_col="p", dim_col="seg"
        )
        assert sum(d.delta for d in r.drivers) == pytest.approx(r.total_delta)
        assert r.drivers[0].segment == UNKNOWN_SEGMENT

    def test_offsetting_change_contribution_none(self):
        """正负对冲总 delta≈0：contribution_pct 为 None，share_of_change 仍可用。"""
        df = pd.DataFrame(
            [
                {"p": "a", "seg": "x", "v": 100},
                {"p": "a", "seg": "y", "v": 100},
                {"p": "b", "seg": "x", "v": 150},
                {"p": "b", "seg": "y", "v": 50},
            ]
        )
        r = AttributionAnalyzer.decompose(
            df, metric_col="v", period_col="p", dim_col="seg"
        )
        assert r.total_delta == pytest.approx(0)
        assert all(d.contribution_pct is None for d in r.drivers)
        assert sum(d.share_of_change for d in r.drivers) == pytest.approx(1.0)

    def test_dimension_autoscan_picks_explanatory(self):
        """维度自动扫描：应选中集中解释变化的维度而非噪声维度。"""
        rows = []
        # region 维度：变化全部来自华北；channel 维度：变化均匀分散
        channels = ["c1", "c2", "c3", "c4"]
        for i, ch in enumerate(channels):
            rows.append({"month": "05", "region": "华北", "channel": ch, "v": 100})
            rows.append({"month": "06", "region": "华北", "channel": ch, "v": 50})
            rows.append({"month": "05", "region": "华东", "channel": ch, "v": 100})
            rows.append({"month": "06", "region": "华东", "channel": ch, "v": 100})
        df = pd.DataFrame(rows)
        r = AttributionAnalyzer.decompose(df, metric_col="v", period_col="month")
        assert r.dimension == "region"
        assert r.dimension_ranking[0]["dimension"] == "region"
        assert {x["dimension"] for x in r.dimension_ranking} == {"region", "channel"}

    def test_input_errors(self, region_df):
        with pytest.raises(ValueError, match="指标列"):
            AttributionAnalyzer.decompose(
                region_df, metric_col="nope", period_col="month", dim_col="region"
            )
        with pytest.raises(ValueError, match="不足两期"):
            AttributionAnalyzer.decompose(
                region_df[region_df["month"] == "2026-05"],
                metric_col="gmv",
                period_col="month",
                dim_col="region",
            )


class TestGroupComparison:
    def test_ranking_and_share(self, region_df):
        r = GroupComparison.compare(region_df, metric_col="gmv", dim_col="region")
        assert [g["group"] for g in r["groups"]] == ["华东", "华北", "华南"]
        assert sum(g["share_pct"] for g in r["groups"]) == pytest.approx(100)
        assert r["caliber"]["method"] == "group_comparison"

    def test_two_group_pairwise(self):
        df = pd.DataFrame(
            [{"g": "A", "v": v} for v in [10, 12, 11, 13, 12]]
            + [{"g": "B", "v": v} for v in [5, 6, 5, 7, 6]]
        )
        r = GroupComparison.compare(df, metric_col="v", dim_col="g")
        pw = r["pairwise"]
        assert pw["leader"] == "A"
        assert pw["diff"] == pytest.approx(58 - 29)
        assert pw["significant"] is True

    def test_groups_filter(self, region_df):
        r = GroupComparison.compare(
            region_df, metric_col="gmv", dim_col="region", groups=["华北", "华东"]
        )
        assert len(r["groups"]) == 2
        assert "pairwise" in r

    def test_missing_group_values(self, region_df):
        with pytest.raises(ValueError, match="无数据"):
            GroupComparison.compare(
                region_df, metric_col="gmv", dim_col="region", groups=["不存在"]
            )


@pytest.mark.integration
class TestDriverRankingStability:
    """任务 4a.6：真实规模数据端到端，driver ranking 稳定性。"""

    def _make_df(self, seed: int) -> pd.DataFrame:
        """3000 行明细：华北植入 -40% 降幅，其余区域 ±5% 噪声。"""
        rng = random.Random(seed)
        regions = ["华北", "华东", "华南", "西南", "东北"]
        rows = []
        for month, scale in (("2026-05", 1.0), ("2026-06", None)):
            for _ in range(300):
                for region in regions:
                    if scale is None:  # 现期：华北 -40%，其余 ±5% 噪声
                        factor = 0.6 if region == "华北" else rng.uniform(0.95, 1.05)
                    else:
                        factor = 1.0
                    rows.append(
                        {
                            "month": month,
                            "region": region,
                            "channel": rng.choice(["线上", "线下"]),
                            "gmv": round(100 * factor, 2),
                        }
                    )
        return pd.DataFrame(rows)

    def test_ranking_stable_under_row_shuffle(self):
        """同一数据不同行序 → ranking 与贡献完全一致。"""
        df = self._make_df(seed=42)
        shuffled = df.sample(frac=1, random_state=7).reset_index(drop=True)

        r1 = AttributionAnalyzer.decompose(
            df, metric_col="gmv", period_col="month", dim_col="region"
        )
        r2 = AttributionAnalyzer.decompose(
            shuffled, metric_col="gmv", period_col="month", dim_col="region"
        )
        assert [d.segment for d in r1.drivers] == [d.segment for d in r2.drivers]
        for d1, d2 in zip(r1.drivers, r2.drivers):
            assert d1.delta == pytest.approx(d2.delta)
            assert d1.contribution_pct == pytest.approx(d2.contribution_pct)

    def test_planted_driver_recovered(self):
        """不同随机种子生成的数据里，植入的华北降幅都应稳定排第一。"""
        for seed in (1, 2, 3):
            df = self._make_df(seed=seed)
            r = AttributionAnalyzer.decompose(
                df, metric_col="gmv", period_col="month"
            )
            assert r.dimension == "region"
            assert r.drivers[0].segment == "华北"
            assert r.drivers[0].direction == "down"
            assert r.confidence == "high"
