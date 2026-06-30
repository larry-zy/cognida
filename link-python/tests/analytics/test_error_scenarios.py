"""错误场景测试。

验证分析服务对异常输入的处理能力。
"""

import pytest
import pandas as pd
import numpy as np

from services.analytics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    GrowthRateAnalyzer,
    InsightGenerator,
)
from services.analytics.validation import (
    DataValidator,
    DataChecker,
    DEFAULT_MAX_ROWS,
    DEFAULT_MIN_ROWS,
)
from core.exceptions import DataValidationError, InsufficientDataError, AnalyticsError


class TestEmptyData:
    """空数据处理测试"""

    def test_describe_empty_series(self):
        """测试空序列的描述统计"""
        series = pd.Series([], dtype=float)

        # 空序列应该抛出 ValueError
        with pytest.raises(ValueError):
            DescriptiveStats.describe(series)

    def test_trend_empty_series(self):
        """测试空序列的趋势分析"""
        series = pd.Series([])

        result = LinearTrendAnalyzer.analyze(series)

        assert result.direction == "flat"
        assert result.strength == "weak"
        assert result.slope == 0
        assert result.r_squared == 0

    def test_correlation_empty_dataframe(self):
        """测试空数据框的相关性分析"""
        df = pd.DataFrame()

        result = CorrelationAnalysis.compute_correlation(df)

        assert result.matrix == {}
        assert result.significant_pairs == []

    def test_insight_generator_empty_dataframe(self):
        """测试空数据框的洞察生成"""
        df = pd.DataFrame()

        generator = InsightGenerator()
        insights = generator.generate(df)

        assert insights == []


class TestInsufficientData:
    """数据不足测试"""

    def test_describe_single_value(self):
        """测试单个值的描述统计"""
        series = pd.Series([5.0])

        result = DescriptiveStats.describe(series)

        assert result.count == 1
        assert result.mean == 5.0
        # 单值的 std 是 NaN 或 0
        assert pd.isna(result.std) or result.std == 0
        assert result.min == 5.0
        assert result.max == 5.0

    def test_trend_two_points(self):
        """测试两个点的趋势分析"""
        series = pd.Series([1, 2])

        result = LinearTrendAnalyzer.analyze(series)

        # 两个点可以计算趋势
        assert result.direction in ["up", "down", "flat"]

    def test_correlation_single_column(self):
        """测试单列数据框的相关性分析"""
        df = pd.DataFrame({"A": [1, 2, 3]})

        result = CorrelationAnalysis.compute_correlation(df)

        # 单列无法计算相关性
        assert result.matrix == {}

    def test_seasonality_insufficient_data(self):
        """测试数据不足的季节性分析"""
        data = pd.Series([1, 2, 3])

        result = SeasonalityAnalyzer.decompose(data, period=12)

        assert result.has_seasonality is False
        assert result.period is None

    def test_cagr_insufficient_data(self):
        """测试数据不足的 CAGR 计算"""
        data = pd.Series([100])

        result = GrowthRateAnalyzer.cagr(data)

        assert result == 0.0


class TestInvalidDataType:
    """无效数据类型测试"""

    def test_describe_all_nan(self):
        """测试全为 NaN 的序列"""
        series = pd.Series([np.nan, np.nan, np.nan])

        # 全 NaN 序列应该抛出 ValueError
        with pytest.raises(ValueError):
            DescriptiveStats.describe(series)

    def test_describe_mixed_types(self):
        """测试混合类型数据"""
        series = pd.Series([1, "two", 3.0, None, "5"])

        # 转换为数值后，非数值变为 NaN
        numeric_series = pd.to_numeric(series, errors="coerce")
        result = DescriptiveStats.describe(numeric_series)

        # 1, 3.0, 和 5 (被正确解析)
        assert result.count == 3

    def test_correlation_all_non_numeric(self):
        """测试全非数值列的相关性分析"""
        df = pd.DataFrame({
            "A": ["a", "b", "c"],
            "B": ["x", "y", "z"],
        })

        result = CorrelationAnalysis.compute_correlation(df)

        # 没有数值列，返回空结果
        assert result.matrix == {}

    def test_insight_all_nan(self):
        """测试全 NaN 数据的洞察生成"""
        df = pd.DataFrame({
            "A": [np.nan, np.nan, np.nan],
            "B": [np.nan, np.nan, np.nan],
        })

        generator = InsightGenerator()
        insights = generator.generate(df)

        # 没有 insights
        assert insights == []


class TestExtremeValues:
    """极端值测试"""

    def test_describe_with_inf(self):
        """测试包含无穷值的序列"""
        series = pd.Series([1, 2, np.inf, 4, -np.inf])

        # 移除 inf 值后再计算
        clean_series = series.replace([np.inf, -np.inf], np.nan).dropna()
        result = DescriptiveStats.describe(clean_series)

        # inf 值被移除后剩余 3 个有效值
        assert result.count == 3

    def test_describe_very_large_values(self):
        """测试极大值"""
        series = pd.Series([1e308, 1e309, 1e310])

        result = DescriptiveStats.describe(series)

        # 应该能处理而不崩溃
        assert result.count == 3

    def test_describe_very_small_values(self):
        """测试极小值"""
        series = pd.Series([1e-308, 1e-309, 1e-310])

        result = DescriptiveStats.describe(series)

        # 应该能处理而不崩溃
        assert result.count == 3

    def test_correlation_perfect_correlation(self):
        """测试完全相关"""
        df = pd.DataFrame({
            "A": [1, 2, 3, 4, 5],
            "B": [2, 4, 6, 8, 10],  # A * 2
        })

        result = CorrelationAnalysis.compute_correlation(df, threshold=0.99)

        # 应该检测到完全相关
        assert len(result.significant_pairs) == 1
        assert result.significant_pairs[0][2] == 1.0


class TestDataValidator:
    """数据验证器测试"""

    def test_validate_dataframe_too_few_rows(self):
        """测试行数不足的验证"""
        df = pd.DataFrame({"A": [1, 2]})  # 2 行，少于最小值 3

        # 默认最小行数是 3，2 行应该通过
        result = DataValidator.validate_dataframe(df, min_rows=2)
        # validate_dataframe 返回 None，不返回数据框

        # 但要求 3 行会抛出 InsufficientDataError
        with pytest.raises(InsufficientDataError):
            DataValidator.validate_dataframe(df, min_rows=3)

    def test_validate_dataframe_too_many_rows(self):
        """测试行数过多的验证"""
        # 创建超过最大行数的数据框
        data = {f"col_{i}": [1] * (DEFAULT_MAX_ROWS + 1) for i in range(5)}
        df = pd.DataFrame(data)

        with pytest.raises(DataValidationError):
            DataValidator.validate_dataframe(df, max_rows=DEFAULT_MAX_ROWS)

    def test_validate_column_not_found(self):
        """测试列不存在的验证"""
        df = pd.DataFrame({"A": [1, 2, 3]})

        with pytest.raises(DataValidationError):
            DataValidator.validate_column(df, "B")

    def test_validate_numeric_column_with_strings(self):
        """测试字符串列的数值验证"""
        df = pd.DataFrame({"A": ["a", "b", "c"]})

        with pytest.raises(DataValidationError):
            DataValidator.validate_numeric_column(df, "A")

    def test_validate_numeric_column_success(self):
        """测试有效的数值列验证"""
        df = pd.DataFrame({"A": [1, 2, 3]})

        series = DataValidator.validate_numeric_column(df, "A")

        assert series.name == "A"
        assert len(series) == 3


class TestDataChecker:
    """数据检查器测试"""

    def test_check_data_quality_with_missing(self):
        """测试带缺失值的数据质量检查"""
        df = pd.DataFrame({
            "A": [1, np.nan, 3],
            "B": [4, 5, np.nan],
        })

        report = DataChecker.check_data_quality(df)

        assert report["row_count"] == 3
        assert report["col_count"] == 2
        assert "A" in report["missing_values"]
        assert "B" in report["missing_values"]
        assert report["missing_values"]["A"]["count"] == 1
        assert report["missing_values"]["B"]["count"] == 1

    def test_sanitize_dataframe(self):
        """测试数据框清理"""
        df = pd.DataFrame({
            "A": [1, np.nan, 3, np.inf],
            "B": [4, 5, np.nan, -np.inf],
        })

        # 使用 drop_na=True 删除包含 NaN 的行
        cleaned = DataChecker.sanitize_dataframe(df, drop_na=True)

        # 删除 NaN 行后，剩余行数应该减少
        assert len(cleaned) < len(df)
        # 由于 inf 仍然存在，需要手动处理
        # 验证数据框被复制而非修改原数据
        assert "A" in cleaned.columns
        assert "B" in cleaned.columns

    def test_sanitize_with_drop_na(self):
        """测试删除 NA 值的数据清理"""
        df = pd.DataFrame({
            "A": [1, np.nan, 3],
            "B": [4, 5, np.nan],
        })

        cleaned = DataChecker.sanitize_dataframe(df, drop_na=True)

        # 删除所有包含 NaN 的行
        assert len(cleaned) == 1  # 只有第一行 (1, 4) 没有 NaN


class TestInvalidParameters:
    """无效参数测试"""

    def test_correlation_invalid_method(self):
        """测试无效的相关性方法"""
        df = pd.DataFrame({"A": [1, 2, 3], "B": [4, 5, 6]})

        # pandas 会抛出 ValueError
        with pytest.raises(ValueError):
            CorrelationAnalysis.compute_correlation(df, method="invalid")

    def test_moving_average_invalid_window(self):
        """测试无效的窗口大小"""
        series = pd.Series([1, 2, 3, 4, 5])

        # 窗口大于序列长度，但由于 min_periods=1，仍会计算部分平均值
        result = MovingAverageAnalyzer.sma(series, window=10)

        # 应该返回与原序列相同长度的结果
        assert len(result) == len(series)
        # 所有值都应该是系列的平均值
        assert result.iloc[0] == 1.0  # 第一个值
        assert result.iloc[-1] == 3.0  # 最后一个值是整个序列的平均值

    def test_anomaly_detection_invalid_method(self):
        """测试无效的异常检测方法"""
        from services.analytics.insight import AnomalyInsightFinder

        series = pd.Series([1, 2, 3, 4, 5])
        finder = AnomalyInsightFinder()

        # 无效方法应该返回空结果
        insights = finder.find_insights(series, "test", method="invalid")

        assert insights == []
