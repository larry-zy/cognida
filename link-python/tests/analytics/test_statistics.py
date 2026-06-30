"""数理统计模块单元测试。"""

import numpy as np
import pandas as pd
import pytest

from services.analytics.statistics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    HypothesisTest,
)


@pytest.fixture
def sample_data():
    """创建测试数据"""
    np.random.seed(42)
    return pd.Series([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])


@pytest.fixture
def sample_dataframe():
    """创建测试数据框"""
    np.random.seed(42)
    return pd.DataFrame({
        "A": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
        "B": [2, 4, 6, 8, 10, 12, 14, 16, 18, 20],
        "C": [1, 3, 2, 4, 3, 5, 4, 6, 5, 7],
    })


class TestDescriptiveStats:
    """描述统计测试"""

    def test_describe(self, sample_data):
        """测试描述统计"""
        result = DescriptiveStats.describe(sample_data)

        assert result.count == 10
        assert result.mean == 5.5
        assert result.median == 5.5
        assert result.min == 1.0
        assert result.max == 10.0
        assert result.q25 == 3.25
        assert result.q75 == 7.75

    def test_describe_empty(self):
        """测试空序列"""
        with pytest.raises(ValueError, match="序列为空"):
            DescriptiveStats.describe(pd.Series([]))

    def test_describe_dataframe(self, sample_dataframe):
        """测试数据框描述统计"""
        results = DescriptiveStats.describe_df(sample_dataframe)

        assert "A" in results
        assert "B" in results
        assert results["A"].mean == 5.5


class TestDistributionAnalysis:
    """分布分析测试"""

    def test_normality_test(self, sample_data):
        """测试正态性检验"""
        result = DistributionAnalysis.test_normality(sample_data)

        assert isinstance(result.is_normal, bool)
        assert isinstance(result.p_value, float)
        assert result.test_type in ["shapiro", "ks", "normaltest"]

    def test_normality_test_small_sample(self):
        """测试小样本"""
        data = pd.Series([1, 2, 3])
        result = DistributionAnalysis.test_normality(data)

        # 小样本应使用 shapiro
        assert result.test_type == "shapiro"

    def test_percentile(self, sample_data):
        """测试分位数计算"""
        result = DistributionAnalysis.percentile(sample_data)

        assert "p50" in result
        assert result["p50"] == 5.5


class TestCorrelationAnalysis:
    """相关性分析测试"""

    def test_correlation_pearson(self, sample_dataframe):
        """测试 Pearson 相关性"""
        result = CorrelationAnalysis.compute_correlation(
            sample_dataframe,
            method="pearson",
            threshold=0.7
        )

        assert result.method == "pearson"
        assert "A" in result.matrix
        assert "B" in result.matrix
        # A 和 B 应该强相关
        assert result.matrix["A"]["B"] > 0.9

    def test_correlation_significant_pairs(self, sample_dataframe):
        """测试显著相关对"""
        result = CorrelationAnalysis.compute_correlation(
            sample_dataframe,
            method="pearson",
            threshold=0.7
        )

        # A 和 B 应该被检测为显著相关
        ab_corr = next(
            (c for c in result.significant_pairs if set(c[:2]) == {"A", "B"}),
            None
        )
        assert ab_corr is not None
        assert ab_corr[2] > 0.7


class TestHypothesisTest:
    """假设检验测试"""

    def test_t_test_independent(self):
        """测试独立样本 t 检验"""
        sample1 = pd.Series([1, 2, 3, 4, 5])
        sample2 = pd.Series([2, 3, 4, 5, 6])

        result = HypothesisTest.t_test_ind(sample1, sample2)

        assert isinstance(result.statistic, float)
        assert isinstance(result.p_value, float)
        assert isinstance(result.significant, bool)

    def test_t_test_insufficient_data(self):
        """测试数据不足的情况"""
        sample1 = pd.Series([1])
        sample2 = pd.Series([2])

        result = HypothesisTest.t_test_ind(sample1, sample2)

        # 数据不足应返回默认值
        assert result.p_value == 1.0

    def test_chi_square(self):
        """测试卡方检验"""
        observed = pd.DataFrame({
            "A": [10, 20],
            "B": [20, 40]
        })

        result = HypothesisTest.chi_square(observed)

        assert isinstance(result.chi2, float)
        assert isinstance(result.p_value, float)
        assert isinstance(result.dof, int)
