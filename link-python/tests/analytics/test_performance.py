"""性能测试 - 大数据集测试。

验证分析服务在处理大量数据时的性能表现。
"""

import time

import numpy as np
import pandas as pd

from services.analytics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    InsightGenerator,
)


class TestPerformance:
    """性能测试类"""

    def test_descriptive_stats_large_dataset(self):
        """测试描述统计在大数据集上的性能"""
        np.random.seed(42)
        # 10万条数据
        data = pd.Series(np.random.normal(100, 15, 100_000))

        start = time.time()
        result = DescriptiveStats.describe(data)
        elapsed = time.time() - start

        # 应该在1秒内完成
        assert elapsed < 1.0
        assert result.count == 100_000
        assert 95 < result.mean < 105

    def test_correlation_large_dataset(self):
        """测试相关性分析在大数据集上的性能"""
        np.random.seed(42)
        # 10万行 x 10列
        df = pd.DataFrame(np.random.randn(100_000, 10))

        start = time.time()
        result = CorrelationAnalysis.compute_correlation(df, threshold=0.5)
        elapsed = time.time() - start

        # 应该在5秒内完成
        assert elapsed < 5.0
        assert len(result.matrix) == 10

    def test_trend_analysis_large_dataset(self):
        """测试趋势分析在大数据集上的性能"""
        np.random.seed(42)
        # 创建带趋势的大数据集
        t = np.arange(50_000)
        trend = 2 * t + 100
        noise = np.random.normal(0, 10, 50_000)
        data = pd.Series(trend + noise)

        start = time.time()
        result = LinearTrendAnalyzer.analyze(data)
        elapsed = time.time() - start

        # 应该在1秒内完成
        assert elapsed < 1.0
        assert result.direction == "up"
        assert result.strength == "strong"

    def test_moving_average_large_dataset(self):
        """测试移动平均在大数据集上的性能"""
        np.random.seed(42)
        data = pd.Series(np.random.randn(50_000))

        start = time.time()
        result = MovingAverageAnalyzer.sma(data, window=100)
        elapsed = time.time() - start

        # 应该在1秒内完成
        assert elapsed < 1.0
        assert len(result) == 50_000

    def test_insight_generator_large_dataset(self):
        """测试洞察生成在大数据集上的性能"""
        np.random.seed(42)
        # 创建大数据集
        df = pd.DataFrame({
            "A": np.random.randn(10_000),
            "B": np.random.randn(10_000),
            "C": np.random.randn(10_000),
        })

        start = time.time()
        generator = InsightGenerator()
        insights = generator.generate(df, options={"include_correlation": True})
        elapsed = time.time() - start

        # 应该在5秒内完成
        assert elapsed < 5.0
        assert isinstance(insights, list)

    def test_distribution_analysis_large_dataset(self):
        """测试分布分析在大数据集上的性能"""
        np.random.seed(42)
        # 正态分布数据
        data = pd.Series(np.random.normal(0, 1, 100_000))

        start = time.time()
        result = DistributionAnalysis.test_normality(data, method="ks")
        elapsed = time.time() - start

        # 应该在2秒内完成
        assert elapsed < 2.0
        assert result.test_type == "ks"

    def test_seasonality_decompose_large_dataset(self):
        """测试季节性分解在大数据集上的性能"""
        np.random.seed(42)
        t = np.arange(1000)
        seasonal = 10 * np.sin(2 * np.pi * t / 12)
        trend_line = 0.5 * t
        noise = np.random.normal(0, 2, 1000)
        data = pd.Series(trend_line + seasonal + noise)

        start = time.time()
        result = SeasonalityAnalyzer.decompose(data, period=12)
        elapsed = time.time() - start

        # 应该在3秒内完成
        assert elapsed < 3.0
        assert result.period == 12


class TestMemoryEfficiency:
    """内存效率测试"""

    def test_memory_efficient_correlation(self):
        """测试相关性分析的内存效率"""
        np.random.seed(42)
        # 中等规模数据集
        df = pd.DataFrame(np.random.randn(50_000, 20))

        # 执行相关性分析
        result = CorrelationAnalysis.compute_correlation(df, threshold=0.3)

        # 验证结果完整性
        assert len(result.matrix) == 20
        for col in result.matrix:
            assert len(result.matrix[col]) == 20

    def test_memory_efficient_insights(self):
        """测试洞察生成的内存效率"""
        np.random.seed(42)
        df = pd.DataFrame({
            f"col_{i}": np.random.randn(5000)
            for i in range(20)
        })

        generator = InsightGenerator()
        insights = generator.generate(df, options={
            "include_correlation": True,
            "correlation_threshold": 0.8,
        })

        # 洞察数量应该合理
        assert len(insights) < 1000
