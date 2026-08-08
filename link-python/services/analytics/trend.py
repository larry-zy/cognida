"""趋势分析模块。

提供线性趋势、季节分解、增长率分析、移动平均等时序分析能力。
"""

from dataclasses import dataclass

import numpy as np
import pandas as pd
from scipy import stats
from statsmodels.tsa.seasonal import seasonal_decompose
from statsmodels.tsa.stattools import acf


@dataclass
class TrendResult:
    """趋势分析结果"""
    direction: str  # up, down, flat
    strength: str  # strong, moderate, weak
    slope: float
    r_squared: float
    p_value: float
    confidence_interval: tuple[float, float]
    forecast: list[float] | None = None


@dataclass
class SeasonalityResult:
    """季节性分析结果"""
    has_seasonality: bool
    period: int | None
    strength: float
    seasonal_component: list[float]
    trend_component: list[float]
    residual: list[float]


@dataclass
class GrowthRateResult:
    """增长率结果"""
    period_over_period: list[float]
    yoy: list[float] | None
    cagr: float


class LinearTrendAnalyzer:
    """线性趋势分析"""

    @staticmethod
    def analyze(
        series: pd.Series,
        alpha: float = 0.05
    ) -> TrendResult:
        """分析线性趋势

        Args:
            series: 输入时间序列
            alpha: 显著性水平

        Returns:
            TrendResult: 趋势分析结果
        """
        series = series.dropna()

        if len(series) < 2:
            return TrendResult(
                direction="flat",
                strength="weak",
                slope=0.0,
                r_squared=0.0,
                p_value=1.0,
                confidence_interval=(0.0, 0.0)
            )

        y = series.values
        x = np.arange(len(y))

        try:
            slope, intercept, r_value, p_value, std_err = stats.linregress(x, y)
        except Exception:
            return TrendResult(
                direction="flat",
                strength="weak",
                slope=0.0,
                r_squared=0.0,
                p_value=1.0,
                confidence_interval=(0.0, 0.0)
            )

        # 计算置信区间
        try:
            t_critical = stats.t.ppf(1 - alpha / 2, len(y) - 2)
            margin = t_critical * std_err
            ci = (float(slope - margin), float(slope + margin))
        except Exception:
            ci = (0.0, 0.0)

        # 判断方向
        if p_value < 0.05:
            direction = "up" if slope > 0 else "down"
        else:
            direction = "flat"

        # 判断强度
        r_squared = r_value ** 2
        if r_squared > 0.7:
            strength = "strong"
        elif r_squared > 0.3:
            strength = "moderate"
        else:
            strength = "weak"

        return TrendResult(
            direction=direction,
            strength=strength,
            slope=float(slope),
            r_squared=float(r_squared),
            p_value=float(p_value),
            confidence_interval=ci
        )

    @staticmethod
    def forecast(
        series: pd.Series,
        steps: int = 5
    ) -> list[float]:
        """线性预测

        Args:
            series: 输入时间序列
            steps: 预测步数

        Returns:
            list[float]: 预测值
        """
        series = series.dropna()

        if len(series) < 2:
            return []

        y = series.values
        x = np.arange(len(y))

        try:
            slope, intercept, _, _, _ = stats.linregress(x, y)
            future_x = np.arange(len(y), len(y) + steps)
            predictions = slope * future_x + intercept
            return [float(p) for p in predictions]
        except Exception:
            return []


class MovingAverageAnalyzer:
    """移动平均分析"""

    @staticmethod
    def sma(series: pd.Series, window: int) -> pd.Series:
        """简单移动平均

        Args:
            series: 输入序列
            window: 窗口大小

        Returns:
            pd.Series: 移动平均结果
        """
        return series.rolling(window=window, min_periods=1).mean()

    @staticmethod
    def ema(series: pd.Series, span: int) -> pd.Series:
        """指数移动平均

        Args:
            series: 输入序列
            span: 跨度

        Returns:
            pd.Series: EMA 结果
        """
        return series.ewm(span=span, adjust=False).mean()

    @staticmethod
    def double_ema(series: pd.Series, span: int) -> pd.Series:
        """双指数移动平均 (Holt's method)

        Args:
            series: 输入序列
            span: 跨度

        Returns:
            pd.Series: 双 EMA 结果
        """
        ema1 = series.ewm(span=span, adjust=False).mean()
        ema2 = ema1.ewm(span=span, adjust=False).mean()
        return 2 * ema1 - ema2


class SeasonalityAnalyzer:
    """季节性分析"""

    @staticmethod
    def decompose(
        series: pd.Series,
        period: int | None = None,
        model: str = "additive"
    ) -> SeasonalityResult:
        """季节分解

        Args:
            series: 输入时间序列
            period: 季节周期
            model: 模型类型 (additive/multiplicative)

        Returns:
            SeasonalityResult: 季节分解结果
        """
        series = series.dropna()

        if len(series) < 2:
            return SeasonalityResult(
                has_seasonality=False,
                period=None,
                strength=0.0,
                seasonal_component=[],
                trend_component=[],
                residual=[]
            )

        # 自动检测周期
        if period is None:
            period = SeasonalityAnalyzer.detect_period(series)

        if period is None or len(series) < 2 * period:
            return SeasonalityResult(
                has_seasonality=False,
                period=None,
                strength=0.0,
                seasonal_component=[],
                trend_component=[],
                residual=[]
            )

        try:
            result = seasonal_decompose(
                series,
                model=model,
                period=period
            )

            # 计算季节性强度
            total_var = series.var()
            seasonal_var = result.seasonal.var()
            strength = float(seasonal_var / total_var) if total_var > 0 else 0.0

            return SeasonalityResult(
                has_seasonality=strength > 0.1,
                period=period,
                strength=strength,
                seasonal_component=result.seasonal.dropna().tolist(),
                trend_component=result.trend.dropna().tolist(),
                residual=result.resid.dropna().tolist()
            )
        except Exception:
            return SeasonalityResult(
                has_seasonality=False,
                period=None,
                strength=0.0,
                seasonal_component=[],
                trend_component=[],
                residual=[]
            )

    @staticmethod
    def detect_period(series: pd.Series, max_period: int = 50) -> int | None:
        """检测周期长度

        Args:
            series: 输入时间序列
            max_period: 最大周期

        Returns:
            int | None: 检测到的周期
        """
        series = series.dropna()

        if len(series) < 4:
            return None

        try:
            autocorr = acf(series, nlags=min(max_period, len(series) - 1), fft=True)

            # 找第一个显著峰值（除了 lag=0）
            for lag in range(2, len(autocorr)):
                if autocorr[lag] > 0.3:
                    return lag
        except Exception:
            pass

        return None


class GrowthRateAnalyzer:
    """增长率分析"""

    @staticmethod
    def period_over_period(
        series: pd.Series,
        periods: int = 1
    ) -> pd.Series:
        """周期增长率

        Args:
            series: 输入序列
            periods: 周期数

        Returns:
            pd.Series: 增长率
        """
        return series.pct_change(periods=periods)

    @staticmethod
    def yoy(series: pd.Series, freq: int = 12) -> pd.Series:
        """同比增长率

        Args:
            series: 输入序列
            freq: 频率（月度数据为12，季度为4）

        Returns:
            pd.Series: 同比增长率
        """
        return series.pct_change(periods=freq)

    @staticmethod
    def cagr(series: pd.Series, years: int | None = None) -> float:
        """复合年增长率

        Args:
            series: 输入序列
            years: 年数，如果为None则根据序列长度计算

        Returns:
            float: CAGR
        """
        series = series.dropna()

        if len(series) < 2:
            return 0.0

        start_val = series.iloc[0]
        end_val = series.iloc[-1]

        if start_val <= 0 or end_val <= 0:
            return 0.0

        n = years if years is not None else (len(series) - 1)
        if n == 0:
            return 0.0

        try:
            return float((end_val / start_val) ** (1 / n) - 1)
        except Exception:
            return 0.0
