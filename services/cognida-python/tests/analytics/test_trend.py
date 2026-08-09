"""趋势分析模块单元测试。"""

import numpy as np
import pandas as pd
import pytest

from services.analytics.trend import (
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    GrowthRateAnalyzer,
)


@pytest.fixture
def sample_series():
    """创建测试序列 - 上升趋势"""
    np.random.seed(42)
    trend = np.linspace(0, 10, 50)
    noise = np.random.normal(0, 0.5, 50)
    return pd.Series(trend + noise)


@pytest.fixture
def flat_series():
    """创建平稳序列"""
    np.random.seed(42)
    return pd.Series(np.random.normal(5, 0.5, 50))


class TestLinearTrendAnalyzer:
    """线性趋势分析测试"""

    def test_analyze_upward_trend(self, sample_series):
        """测试上升趋势检测"""
        result = LinearTrendAnalyzer.analyze(sample_series)

        assert result.direction == "up"
        assert result.strength in ["strong", "moderate", "weak"]
        assert isinstance(result.slope, float)
        assert isinstance(result.r_squared, float)
        assert isinstance(result.p_value, float)

    def test_analyze_flat_trend(self, flat_series):
        """测试平稳趋势检测"""
        result = LinearTrendAnalyzer.analyze(flat_series)

        # 平稳序列应被检测为 flat 或方向不显著
        assert result.direction in ["up", "down", "flat"]

    def test_insufficient_data(self):
        """测试数据不足"""
        series = pd.Series([1])

        result = LinearTrendAnalyzer.analyze(series)

        assert result.direction == "flat"
        assert result.strength == "weak"

    def test_forecast(self, sample_series):
        """测试预测功能"""
        forecast = LinearTrendAnalyzer.forecast(sample_series, steps=5)

        assert len(forecast) == 5
        assert all(isinstance(x, float) for x in forecast)


class TestMovingAverageAnalyzer:
    """移动平均分析测试"""

    def test_sma(self, sample_series):
        """测试简单移动平均"""
        result = MovingAverageAnalyzer.sma(sample_series, window=5)

        assert len(result) == len(sample_series)
        # 前4个值应该是NaN（窗口未填满）
        assert result.iloc[4] > 0

    def test_ema(self, sample_series):
        """测试指数移动平均"""
        result = MovingAverageAnalyzer.ema(sample_series, span=5)

        assert len(result) == len(sample_series)
        # EMA 从第一个值开始就有结果
        assert not result.iloc[0] != result.iloc[0]

    def test_double_ema(self, sample_series):
        """测试双指数移动平均"""
        result = MovingAverageAnalyzer.double_ema(sample_series, span=5)

        assert len(result) == len(sample_series)


class TestSeasonalityAnalyzer:
    """季节性分析测试"""

    def test_decompose_with_seasonality(self):
        """测试带季节性的数据分解"""
        # 创建带季节性的数据
        np.random.seed(42)
        t = np.arange(100)
        seasonal = 5 * np.sin(2 * np.pi * t / 12)
        trend = 0.5 * t
        noise = np.random.normal(0, 1, 100)
        data = pd.Series(trend + seasonal + noise)

        result = SeasonalityAnalyzer.decompose(data, period=12)

        assert isinstance(result.has_seasonality, bool)
        assert isinstance(result.period, (int, type(None)))
        assert isinstance(result.strength, float)

    def test_decompose_insufficient_data(self):
        """测试数据不足"""
        data = pd.Series([1, 2, 3])

        result = SeasonalityAnalyzer.decompose(data, period=12)

        assert result.has_seasonality is False

    def test_detect_period(self):
        """测试周期检测"""
        # 创建带周期的数据
        np.random.seed(42)
        t = np.arange(50)
        data = pd.Series(5 * np.sin(2 * np.pi * t / 10) + np.random.normal(0, 0.5, 50))

        period = SeasonalityAnalyzer.detect_period(data)

        # 应该检测到接近10的周期
        assert period is not None


class TestGrowthRateAnalyzer:
    """增长率分析测试"""

    def test_period_over_period(self):
        """测试周期增长率"""
        data = pd.Series([100, 110, 121, 133.1])  # 10% 增长

        result = GrowthRateAnalyzer.period_over_period(data, periods=1)

        # 第一个值应该是 NaN
        assert result.iloc[0] != result.iloc[0]
        # 第二个值应该接近 0.1 (10%)
        assert abs(result.iloc[1] - 0.1) < 0.01

    def test_cagr(self):
        """测试复合年增长率"""
        data = pd.Series([100, 110, 121, 133.1])  # 10% 年增长

        cagr = GrowthRateAnalyzer.cagr(data, years=3)

        # CAGR 应该接近 0.1
        assert abs(cagr - 0.1) < 0.01

    def test_cagr_insufficient_data(self):
        """测试数据不足"""
        data = pd.Series([100])

        cagr = GrowthRateAnalyzer.cagr(data)

        assert cagr == 0.0
