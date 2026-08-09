"""归因/根因分析引擎。

实现 openspec data-agent-evolution Phase 3.5（D9 归因/根因作为一等分析能力）：
- variance decomposition：指标变化按维度切片做**可加分解**（各切片 delta 之和恰等于总 delta）
- driver ranking：按 |delta| 对切片驱动因子排序，给出贡献占比与方向
- 隐藏因子：新增/消失切片、驱动分散度过低时提示可能存在未纳入的维度/因子
- 口径/置信标注：结果携带 caliber（指标/维度/期间/聚合口径）与 confidence

算法侧只做确定性计算（"Python 只做计算/分析"分工），文字洞察由 Go 侧拼装。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

import pandas as pd

from .statistics import HypothesisTest

# 未知维度值占位（维度列为空的行归入此切片，保证分解可加性不丢行）
UNKNOWN_SEGMENT = "(未知)"

# 浮点判零阈值
_EPS = 1e-9


@dataclass
class SegmentDriver:
    """单个维度切片的驱动因子。"""

    segment: str
    baseline: float
    current: float
    delta: float
    # 相对总变化的贡献百分比；总变化近零（正负对冲）时为 None
    contribution_pct: float | None
    # |delta| / Σ|delta|，对冲场景下仍可用的变化占比
    share_of_change: float
    direction: str  # up / down / flat
    new_segment: bool = False  # 基期不存在、现期新增
    disappeared: bool = False  # 基期存在、现期消失


@dataclass
class AttributionResult:
    """归因分解结果（JSON 可序列化，经 to_jsonable 输出）。"""

    metric: str
    dimension: str
    baseline: dict[str, Any]
    current: dict[str, Any]
    total_delta: float
    total_delta_pct: float | None
    drivers: list[SegmentDriver]
    dimension_ranking: list[dict[str, Any]]
    hidden_factor: dict[str, Any]
    confidence: str  # high / medium / low
    sample: dict[str, Any]
    caliber: dict[str, Any] = field(default_factory=dict)


class AttributionAnalyzer:
    """指标变化的维度切片归因（可加方差分解 + 驱动排序）。"""

    # 自动扫描候选维度时的基数范围（过高的近似主键列不具解释力）
    MIN_CARDINALITY = 2
    MAX_CARDINALITY = 50

    # top3 驱动覆盖低于此值视为"驱动分散"，提示隐藏因子
    DIFFUSE_THRESHOLD = 0.5

    # 排入 drivers 的切片数上限
    DEFAULT_TOP_N = 10

    @classmethod
    def decompose(
        cls,
        df: pd.DataFrame,
        metric_col: str,
        period_col: str,
        dim_col: str | None = None,
        baseline: Any = None,
        current: Any = None,
        top_n: int = DEFAULT_TOP_N,
    ) -> AttributionResult:
        """对 ``metric_col`` 在两期之间的变化按维度切片做可加分解。

        Args:
            df: 明细/汇总行集，须含指标列与期间列
            metric_col: 指标列（数值）
            period_col: 期间列（如月份），用于切分基期/现期
            dim_col: 归因维度列；省略则自动扫描最具解释力的维度
            baseline: 基期取值；省略取倒数第二期
            current: 现期取值；省略取最后一期
            top_n: drivers 保留的切片数上限

        Raises:
            ValueError: 列缺失、期间不足两期、指标非数值等输入问题
        """
        cls._require_column(df, metric_col, "指标列")
        cls._require_column(df, period_col, "期间列")

        work = df.copy()
        work[metric_col] = pd.to_numeric(work[metric_col], errors="coerce")
        work = work.dropna(subset=[metric_col])
        if work.empty:
            raise ValueError(f"指标列 {metric_col} 无有效数值")

        baseline, current = cls._resolve_periods(work, period_col, baseline, current)
        df_b = work[work[period_col] == baseline]
        df_c = work[work[period_col] == current]
        if df_b.empty or df_c.empty:
            raise ValueError(f"基期 {baseline!r} 或现期 {current!r} 无数据")

        # 维度：显式指定优先，否则扫描全部候选维度取最具解释力者
        dimension_ranking: list[dict[str, Any]] = []
        if dim_col is None:
            dim_col, dimension_ranking = cls._scan_dimensions(
                df_b, df_c, metric_col, period_col
            )
        else:
            cls._require_column(df, dim_col, "维度列")

        drivers, total_b, total_c = cls._decompose_dimension(
            df_b, df_c, metric_col, dim_col
        )

        total_delta = total_c - total_b
        total_delta_pct = (
            total_delta / abs(total_b) * 100 if abs(total_b) > _EPS else None
        )

        hidden = cls._detect_hidden_factor(drivers)
        confidence = cls._grade_confidence(len(df_b) + len(df_c), drivers)

        return AttributionResult(
            metric=metric_col,
            dimension=dim_col,
            baseline={"label": str(baseline), "total": float(total_b)},
            current={"label": str(current), "total": float(total_c)},
            total_delta=float(total_delta),
            total_delta_pct=(
                float(total_delta_pct) if total_delta_pct is not None else None
            ),
            drivers=drivers[:top_n],
            dimension_ranking=dimension_ranking,
            hidden_factor=hidden,
            confidence=confidence,
            sample={
                "rows_used": int(len(df_b) + len(df_c)),
                "segments": int(len(drivers)),
            },
            caliber={
                "metric_col": metric_col,
                "dim_col": dim_col,
                "period_col": period_col,
                "baseline": str(baseline),
                "current": str(current),
                "agg": "sum",
                "method": "additive_variance_decomposition",
            },
        )

    # ------------------------------------------------------------------
    # 内部步骤
    # ------------------------------------------------------------------

    @staticmethod
    def _require_column(df: pd.DataFrame, col: str, label: str) -> None:
        if not col:
            raise ValueError(f"缺少{label}参数")
        if col not in df.columns:
            raise ValueError(f"{label} {col} 不存在")

    @staticmethod
    def _resolve_periods(
        df: pd.DataFrame, period_col: str, baseline: Any, current: Any
    ) -> tuple[Any, Any]:
        """确定基期/现期：显式指定优先，否则取排序后的最后两期。"""
        periods = sorted(df[period_col].dropna().unique().tolist(), key=str)
        if baseline is not None and current is not None:
            for p, name in ((baseline, "基期"), (current, "现期")):
                if p not in periods:
                    raise ValueError(f"{name} {p!r} 不在期间列取值中")
            return baseline, current
        if len(periods) < 2:
            raise ValueError("期间列取值不足两期，无法做变化归因")
        return periods[-2], periods[-1]

    @classmethod
    def _decompose_dimension(
        cls, df_b: pd.DataFrame, df_c: pd.DataFrame, metric_col: str, dim_col: str
    ) -> tuple[list[SegmentDriver], float, float]:
        """按维度切片做可加分解，返回（按 |delta| 降序的驱动、基期总量、现期总量）。"""
        grp_b = (
            df_b.assign(_seg=df_b[dim_col].fillna(UNKNOWN_SEGMENT).astype(str))
            .groupby("_seg")[metric_col]
            .sum()
        )
        grp_c = (
            df_c.assign(_seg=df_c[dim_col].fillna(UNKNOWN_SEGMENT).astype(str))
            .groupby("_seg")[metric_col]
            .sum()
        )

        segments = grp_b.index.union(grp_c.index)
        deltas = {
            seg: float(grp_c.get(seg, 0.0)) - float(grp_b.get(seg, 0.0))
            for seg in segments
        }
        abs_sum = sum(abs(d) for d in deltas.values())
        total_b = float(grp_b.sum())
        total_c = float(grp_c.sum())
        total_delta = total_c - total_b

        drivers: list[SegmentDriver] = []
        for seg in segments:
            b = float(grp_b.get(seg, 0.0))
            c = float(grp_c.get(seg, 0.0))
            d = deltas[seg]
            drivers.append(
                SegmentDriver(
                    segment=str(seg),
                    baseline=b,
                    current=c,
                    delta=d,
                    contribution_pct=(
                        d / total_delta * 100 if abs(total_delta) > _EPS else None
                    ),
                    share_of_change=(abs(d) / abs_sum if abs_sum > _EPS else 0.0),
                    direction="up" if d > _EPS else ("down" if d < -_EPS else "flat"),
                    new_segment=seg not in grp_b.index,
                    disappeared=seg not in grp_c.index,
                )
            )

        # driver ranking：|delta| 降序；并列时按切片名保证稳定序
        drivers.sort(key=lambda x: (-abs(x.delta), x.segment))
        return drivers, total_b, total_c

    @classmethod
    def _scan_dimensions(
        cls,
        df_b: pd.DataFrame,
        df_c: pd.DataFrame,
        metric_col: str,
        period_col: str,
    ) -> tuple[str, list[dict[str, Any]]]:
        """扫描候选维度，按 top3 驱动覆盖率排序，取最具解释力的维度。"""
        combined = pd.concat([df_b, df_c])
        candidates: list[str] = []
        for col in combined.columns:
            if col in (metric_col, period_col):
                continue
            if pd.api.types.is_numeric_dtype(combined[col]):
                continue
            nunique = combined[col].nunique(dropna=True)
            if cls.MIN_CARDINALITY <= nunique <= cls.MAX_CARDINALITY:
                candidates.append(col)
        if not candidates:
            raise ValueError(
                "未找到可归因的维度列（需 2~50 个取值的非数值列），请显式指定 dim_col"
            )

        ranking: list[dict[str, Any]] = []
        for col in candidates:
            drivers, _, _ = cls._decompose_dimension(df_b, df_c, metric_col, col)
            top3 = sum(d.share_of_change for d in drivers[:3])
            # 解释力用归一化集中度（HHI ÷ 均匀基线 1/n）：度量变化比"均匀分散"
            # 集中多少倍。top3 覆盖率会天然偏向低基数维度（2 个取值恒为 1.0），
            # 不能直接用作跨维度比较。
            hhi = sum(d.share_of_change**2 for d in drivers)
            concentration = hhi * len(drivers) if drivers else 0.0
            ranking.append(
                {
                    "dimension": col,
                    "concentration": float(concentration),
                    "top3_share": float(top3),
                    "top_driver": drivers[0].segment if drivers else None,
                    "segments": len(drivers),
                }
            )
        # 集中度降序；并列按维度名保证稳定序
        ranking.sort(key=lambda x: (-x["concentration"], x["dimension"]))
        return ranking[0]["dimension"], ranking

    @classmethod
    def _detect_hidden_factor(cls, drivers: list[SegmentDriver]) -> dict[str, Any]:
        """隐藏因子提示：驱动分散 / 未知切片占比高 / 新增消失切片主导。"""
        notes: list[str] = []
        top3 = sum(d.share_of_change for d in drivers[:3])
        if drivers and top3 < cls.DIFFUSE_THRESHOLD:
            notes.append(
                f"top3 驱动仅解释 {top3 * 100:.0f}% 的变化，驱动分散，"
                "可能存在未纳入的隐藏维度/因子"
            )
        unknown = next((d for d in drivers if d.segment == UNKNOWN_SEGMENT), None)
        if unknown is not None and unknown.share_of_change > 0.3:
            notes.append("维度缺失（未知切片）承担了较大变化占比，口径可能不完整")
        structural = [d for d in drivers[:3] if d.new_segment or d.disappeared]
        if structural:
            names = "、".join(d.segment for d in structural)
            notes.append(f"头部驱动含新增/消失切片（{names}），存在结构性变化")
        return {"suspected": bool(notes), "notes": notes}

    @staticmethod
    def _grade_confidence(rows_used: int, drivers: list[SegmentDriver]) -> str:
        """置信分级：样本量 + 驱动集中度的启发式（不作为确定性事实）。"""
        top3 = sum(d.share_of_change for d in drivers[:3])
        if rows_used >= 30 and top3 >= 0.6:
            return "high"
        if rows_used >= 10 and top3 >= 0.4:
            return "medium"
        return "low"


class GroupComparison:
    """维度分组对比（对比能力：组间聚合、差异与显著性）。"""

    @staticmethod
    def compare(
        df: pd.DataFrame,
        metric_col: str,
        dim_col: str,
        groups: list[Any] | None = None,
    ) -> dict[str, Any]:
        """按 ``dim_col`` 分组对比 ``metric_col``。

        - 输出各组 sum/mean/count 与总量占比，按 sum 降序排名
        - 恰好两组时补充差值、差异百分比与独立样本 t 检验显著性
        """
        AttributionAnalyzer._require_column(df, metric_col, "指标列")
        AttributionAnalyzer._require_column(df, dim_col, "维度列")

        work = df.copy()
        work[metric_col] = pd.to_numeric(work[metric_col], errors="coerce")
        work = work.dropna(subset=[metric_col])
        if work.empty:
            raise ValueError(f"指标列 {metric_col} 无有效数值")
        work["_seg"] = work[dim_col].fillna(UNKNOWN_SEGMENT).astype(str)

        if groups:
            wanted = [str(g) for g in groups]
            work = work[work["_seg"].isin(wanted)]
            if work.empty:
                raise ValueError(f"分组取值 {groups!r} 在维度列中无数据")

        agg = work.groupby("_seg")[metric_col].agg(["sum", "mean", "count"])
        total = float(agg["sum"].sum())
        rows = [
            {
                "group": str(seg),
                "sum": float(r["sum"]),
                "mean": float(r["mean"]),
                "count": int(r["count"]),
                "share_pct": (
                    float(r["sum"]) / total * 100 if abs(total) > _EPS else None
                ),
            }
            for seg, r in agg.iterrows()
        ]
        rows.sort(key=lambda x: (-x["sum"], x["group"]))

        result: dict[str, Any] = {
            "metric": metric_col,
            "dimension": dim_col,
            "groups": rows,
            "caliber": {
                "metric_col": metric_col,
                "dim_col": dim_col,
                "agg": "sum/mean/count",
                "method": "group_comparison",
            },
            "sample": {"rows_used": int(len(work)), "groups": len(rows)},
        }

        # 恰好两组：差值 + 显著性检验
        if len(rows) == 2:
            a, b = rows[0], rows[1]
            diff = a["sum"] - b["sum"]
            t = HypothesisTest.t_test_ind(
                work[work["_seg"] == a["group"]][metric_col],
                work[work["_seg"] == b["group"]][metric_col],
            )
            result["pairwise"] = {
                "leader": a["group"],
                "diff": float(diff),
                "diff_pct": (
                    float(diff / abs(b["sum"]) * 100) if abs(b["sum"]) > _EPS else None
                ),
                "t_statistic": float(t.statistic),
                "p_value": float(t.p_value),
                "significant": bool(t.significant),
            }

        return result
