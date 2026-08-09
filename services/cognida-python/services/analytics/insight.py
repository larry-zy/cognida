"""数据洞察模块。

基于统计和趋势分析自动生成洞察结论。
"""

from dataclasses import dataclass
from enum import Enum
from typing import Any

import numpy as np
import pandas as pd

from .statistics import CorrelationAnalysis
from .trend import LinearTrendAnalyzer


class InsightType(Enum):
    """洞察类型"""
    TREND_UP = "trend_up"
    TREND_DOWN = "trend_down"
    TREND_FLAT = "trend_flat"
    ANOMALY_SPIKE = "anomaly_spike"
    ANOMALY_DIP = "anomaly_dip"
    CORRELATION = "correlation"
    OUTLIER = "outlier"
    CHANGE_POINT = "change_point"
    RANKING_CHANGE = "ranking_change"


class InsightSeverity(Enum):
    """洞察严重程度"""
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"


@dataclass
class Insight:
    """单条洞察"""
    type: InsightType | str
    severity: InsightSeverity | str
    title: str
    description: str
    confidence: float
    evidence: dict[str, Any]
    recommendations: list[str]
    affected_metrics: list[str]


class TrendInsightFinder:
    """趋势洞察发现器"""

    def __init__(self) -> None:
        self.trend_analyzer = LinearTrendAnalyzer()

    def find_insights(
        self,
        series: pd.Series,
        name: str = "指标"
    ) -> list[Insight]:
        """发现趋势相关洞察

        Args:
            series: 输入序列
            name: 指标名称

        Returns:
            list[Insight]: 趋势洞察列表
        """
        insights = []

        # 线性趋势分析
        trend_result = self.trend_analyzer.analyze(series)

        if trend_result.direction != "flat" and trend_result.p_value < 0.05:
            # 趋势显著
            if trend_result.strength == "strong":
                severity = InsightSeverity.HIGH
            elif trend_result.strength == "moderate":
                severity = InsightSeverity.MEDIUM
            else:
                severity = InsightSeverity.LOW

            if trend_result.direction == "up":
                direction_text = "上升"
                change_pct = (series.iloc[-1] - series.iloc[0]) / series.iloc[0] * 100
            else:
                direction_text = "下降"
                change_pct = (series.iloc[0] - series.iloc[-1]) / series.iloc[0] * 100

            insights.append(Insight(
                type=InsightType.TREND_UP if trend_result.direction == "up" else InsightType.TREND_DOWN,
                severity=severity,
                title=f"{name}呈{direction_text}趋势",
                description=(
                    f"{name}在分析期内呈{trend_result.strength}{direction_text}趋势，"
                    f"整体变化约{change_pct:.1f}%，"
                    f"趋势显著性(p={trend_result.p_value:.4f})。"
                ),
                confidence=min(1 - trend_result.p_value, trend_result.r_squared),
                evidence={
                    "slope": trend_result.slope,
                    "r_squared": trend_result.r_squared,
                    "p_value": trend_result.p_value,
                    "change_pct": change_pct
                },
                recommendations=self._get_trend_recommendations(trend_result.direction, name),
                affected_metrics=[name]
            ))

        # 加速/减速检测
        if len(series) >= 6:
            insights.extend(self._check_acceleration(series, name))

        return insights

    def _check_acceleration(
        self,
        series: pd.Series,
        name: str
    ) -> list[Insight]:
        """检测趋势加速/减速

        Args:
            series: 输入序列
            name: 指标名称

        Returns:
            list[Insight]: 加速/减速洞察
        """
        insights = []

        mid = len(series) // 2
        first_half = series.iloc[:mid]
        second_half = series.iloc[mid:]

        trend1 = self.trend_analyzer.analyze(first_half)
        trend2 = self.trend_analyzer.analyze(second_half)

        # 判断是否加速
        if trend1.direction == trend2.direction == "up":
            if trend2.slope > trend1.slope * 1.5 and trend2.slope > 0:
                insights.append(Insight(
                    type=InsightType.TREND_UP,
                    severity=InsightSeverity.MEDIUM,
                    title=f"{name}呈加速上升趋势",
                    description=(
                        f"{name}近期上升速度加快，"
                        f"前期斜率为{trend1.slope:.2f}，"
                        f"后期斜率为{trend2.slope:.2f}。"
                    ),
                    confidence=0.7,
                    evidence={"slope1": trend1.slope, "slope2": trend2.slope},
                    recommendations=["关注加速原因，确认是否可持续"],
                    affected_metrics=[name]
                ))

        return insights

    def _get_trend_recommendations(
        self,
        direction: str,
        metric: str
    ) -> list[str]:
        """生成趋势建议

        Args:
            direction: 趋势方向
            metric: 指标名称

        Returns:
            list[str]: 建议列表
        """
        if direction == "up":
            return [
                f"分析{metric}上升的驱动因素",
                "评估是否需要调整资源以应对增长",
                "监控上升趋势是否可持续"
            ]
        else:
            return [
                f"分析{metric}下降的原因",
                "识别下降是否为风险信号",
                "制定止损或改善措施"
            ]


class AnomalyInsightFinder:
    """异常洞察发现器"""

    def find_insights(
        self,
        series: pd.Series,
        name: str = "指标",
        method: str = "iqr",
        threshold: float = 3.0
    ) -> list[Insight]:
        """发现异常洞察

        Args:
            series: 输入序列
            name: 指标名称
            method: 检测方法 (iqr/zscore)
            threshold: 阈值 (zscore 方法使用)

        Returns:
            list[Insight]: 异常洞察列表
        """
        insights = []

        if method == "iqr":
            outliers = self._detect_iqr(series)
        elif method == "zscore":
            outliers = self._detect_zscore(series, threshold)
        else:
            outliers = pd.Series(dtype=float)

        if not outliers.empty:
            # 分类异常类型
            spikes = outliers[outliers > series.median()]
            dips = outliers[outliers < series.median()]

            if not spikes.empty:
                insights.append(Insight(
                    type=InsightType.ANOMALY_SPIKE,
                    severity=InsightSeverity.HIGH if len(spikes) > 1 else InsightSeverity.MEDIUM,
                    title=f"{name}检测到突增异常",
                    description=(
                        f"检测到{len(spikes)}个突增异常点，"
                        f"最高值为{spikes.max():.2f}，"
                        f"发生在{spikes.idxmax()}。"
                    ),
                    confidence=0.8,
                    evidence={
                        "count": len(spikes),
                        "max_value": float(spikes.max()),
                        "dates": [str(d) for d in spikes.index]
                    },
                    recommendations=[
                        "核查异常点是否为数据错误",
                        "分析突增的业务原因",
                        "评估是否需要调整阈值"
                    ],
                    affected_metrics=[name]
                ))

            if not dips.empty:
                insights.append(Insight(
                    type=InsightType.ANOMALY_DIP,
                    severity=InsightSeverity.HIGH if len(dips) > 1 else InsightSeverity.MEDIUM,
                    title=f"{name}检测到突降异常",
                    description=(
                        f"检测到{len(dips)}个突降异常点，"
                        f"最低值为{dips.min():.2f}。"
                    ),
                    confidence=0.8,
                    evidence={
                        "count": len(dips),
                        "min_value": float(dips.min()),
                        "dates": [str(d) for d in dips.index]
                    },
                    recommendations=[
                        "核查异常点是否为数据错误",
                        "分析突降的业务原因",
                        "检查是否发生了系统故障"
                    ],
                    affected_metrics=[name]
                ))

        return insights

    def _detect_iqr(self, series: pd.Series) -> pd.Series:
        """IQR 方法检测异常值

        Args:
            series: 输入序列

        Returns:
            pd.Series: 异常值
        """
        series = series.dropna()
        if len(series) == 0:
            return pd.Series(dtype=float)

        Q1 = series.quantile(0.25)
        Q3 = series.quantile(0.75)
        IQR = Q3 - Q1
        lower = Q1 - 1.5 * IQR
        upper = Q3 + 1.5 * IQR
        return series[(series < lower) | (series > upper)]

    def _detect_zscore(
        self,
        series: pd.Series,
        threshold: float
    ) -> pd.Series:
        """Z-score 方法检测异常值

        Args:
            series: 输入序列
            threshold: 阈值

        Returns:
            pd.Series: 异常值
        """
        series = series.dropna()
        if len(series) == 0:
            return pd.Series(dtype=float)

        z_scores = np.abs((series - series.mean()) / series.std())
        return series[z_scores > threshold]


class CorrelationInsightFinder:
    """相关性洞察发现器"""

    def find_insights(
        self,
        df: pd.DataFrame,
        threshold: float = 0.7
    ) -> list[Insight]:
        """发现相关性洞察

        Args:
            df: 输入数据框
            threshold: 相关性阈值

        Returns:
            list[Insight]: 相关性洞察列表
        """
        insights = []

        result = CorrelationAnalysis.compute_correlation(df, method="pearson", threshold=threshold)

        for col1, col2, corr_val in result.significant_pairs:
            direction = "正相关" if corr_val > 0 else "负相关"
            strength = "强" if abs(corr_val) > 0.8 else "中等"

            insights.append(Insight(
                type=InsightType.CORRELATION,
                severity=InsightSeverity.MEDIUM,
                title=f"{col1} 与 {col2} 存在{strength}{direction}",
                description=(
                    f"{col1} 与 {col2} 的相关系数为 {corr_val:.3f}，"
                    f"呈现{strength}{direction}关系。"
                ),
                confidence=abs(corr_val),
                evidence={
                    "correlation": corr_val,
                    "method": "pearson"
                },
                recommendations=[
                    f"分析{col1}和{col2}的业务关联性",
                    "考虑是否可利用相关性进行预测",
                    "注意区分相关性与因果关系"
                ],
                affected_metrics=[col1, col2]
            ))

        return insights


class InsightGenerator:
    """洞察生成器 - 整合所有发现器"""

    def __init__(self) -> None:
        self.trend_finder = TrendInsightFinder()
        self.anomaly_finder = AnomalyInsightFinder()
        self.correlation_finder = CorrelationInsightFinder()

    def generate(
        self,
        df: pd.DataFrame,
        time_col: str | None = None,
        value_cols: list[str] | None = None,
        options: dict[str, Any] | None = None
    ) -> list[Insight]:
        """生成所有洞察

        Args:
            df: 输入数据框
            time_col: 时间列名
            value_cols: 要分析的数值列
            options: 洞察选项

        Returns:
            list[Insight]: 所有洞察列表
        """
        if options is None:
            options = {}

        all_insights = []

        if value_cols is None:
            value_cols = df.select_dtypes(include=[np.number]).columns.tolist()

        # 对每个指标做趋势和异常分析
        for col in value_cols:
            series = df[col].dropna()

            if len(series) < 3:
                continue

            # 趋势洞察
            if options.get("include_trend", True):
                all_insights.extend(self.trend_finder.find_insights(series, col))

            # 异常洞察
            if options.get("include_anomaly", True):
                anomaly_method = options.get("anomaly_method", "iqr")
                anomaly_threshold = options.get("anomaly_threshold", 3.0)
                all_insights.extend(
                    self.anomaly_finder.find_insights(
                        series,
                        col,
                        method=anomaly_method,
                        threshold=anomaly_threshold
                    )
                )

        # 相关性洞察
        if options.get("include_correlation", True):
            correlation_threshold = options.get("correlation_threshold", 0.7)
            numeric_df = df[value_cols].select_dtypes(include=[np.number])
            if numeric_df.shape[1] >= 2:
                all_insights.extend(
                    self.correlation_finder.find_insights(
                        numeric_df,
                        threshold=correlation_threshold
                    )
                )

        # 按严重程度和置信度排序
        severity_order = {"high": 0, "medium": 1, "low": 2}
        all_insights.sort(
            key=lambda x: (
                severity_order.get(str(x.severity), 3),
                -x.confidence
            )
        )

        return all_insights
