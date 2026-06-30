"""数据洞察模块单元测试。"""

import numpy as np
import pandas as pd
import pytest

from services.analytics.insight import (
    TrendInsightFinder,
    AnomalyInsightFinder,
    CorrelationInsightFinder,
    InsightGenerator,
    InsightType,
    InsightSeverity,
)


@pytest.fixture
def sample_series_with_anomaly():
    """创建带异常值的测试序列"""
    np.random.seed(42)
    normal_data = list(np.random.normal(10, 1, 20))
    # 添加突增异常
    normal_data.append(50)
    # 添加突降异常
    normal_data.append(2)
    return pd.Series(normal_data)


@pytest.fixture
def sample_dataframe():
    """创建测试数据框"""
    np.random.seed(42)
    return pd.DataFrame({
        "A": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
        "B": [2, 4, 6, 8, 10, 12, 14, 16, 18, 20],
        "C": [1, 3, 2, 4, 3, 5, 4, 6, 5, 7],
    })


class TestTrendInsightFinder:
    """趋势洞察发现器测试"""

    def test_find_upward_trend_insights(self):
        """测试上升趋势洞察"""
        data = pd.Series(range(1, 11))  # 1-10 上升序列
        finder = TrendInsightFinder()
        insights = finder.find_insights(data, "测试指标")

        assert len(insights) > 0
        # 应该检测到上升趋势
        trend_insights = [i for i in insights if i.type == InsightType.TREND_UP]
        assert len(trend_insights) > 0

    def test_find_insights_with_recommendations(self):
        """测试洞察包含建议"""
        data = pd.Series(range(1, 11))
        finder = TrendInsightFinder()
        insights = finder.find_insights(data, "测试指标")

        for insight in insights:
            assert isinstance(insight.recommendations, list)
            assert len(insight.recommendations) > 0

    def test_insight_structure(self):
        """测试洞察数据结构"""
        data = pd.Series(range(1, 11))
        finder = TrendInsightFinder()
        insights = finder.find_insights(data, "测试指标")

        for insight in insights:
            assert insight.title
            assert insight.description
            assert 0 <= insight.confidence <= 1
            assert insight.affected_metrics == ["测试指标"]


class TestAnomalyInsightFinder:
    """异常洞察发现器测试"""

    def test_find_spike_anomalies(self, sample_series_with_anomaly):
        """测试突增异常检测"""
        finder = AnomalyInsightFinder()
        insights = finder.find_insights(sample_series_with_anomaly, "测试指标", method="iqr")

        spike_insights = [i for i in insights if i.type == InsightType.ANOMALY_SPIKE]
        assert len(spike_insights) > 0

    def test_find_dip_anomalies(self, sample_series_with_anomaly):
        """测试突降异常检测"""
        finder = AnomalyInsightFinder()
        insights = finder.find_insights(sample_series_with_anomaly, "测试指标", method="iqr")

        dip_insights = [i for i in insights if i.type == InsightType.ANOMALY_DIP]
        assert len(dip_insights) > 0

    def test_zscore_method(self, sample_series_with_anomaly):
        """测试 Z-score 方法"""
        finder = AnomalyInsightFinder()
        insights = finder.find_insights(
            sample_series_with_anomaly,
            "测试指标",
            method="zscore",
            threshold=2.0
        )

        # Z-score 应该检测到异常
        assert len(insights) > 0


class TestCorrelationInsightFinder:
    """相关性洞察发现器测试"""

    def test_find_correlation_insights(self, sample_dataframe):
        """测试相关性洞察"""
        finder = CorrelationInsightFinder()
        insights = finder.find_insights(sample_dataframe, threshold=0.9)

        # A 和 B 完全相关，应该被检测到
        corr_insights = [i for i in insights if i.type == InsightType.CORRELATION]
        assert len(corr_insights) > 0

        # 检查是否有 A-B 相关性洞察
        ab_insights = [
            i for i in corr_insights
            if set(i.affected_metrics) == {"A", "B"}
        ]
        assert len(ab_insights) > 0

    def test_correlation_evidence(self, sample_dataframe):
        """测试相关性证据"""
        finder = CorrelationInsightFinder()
        insights = finder.find_insights(sample_dataframe, threshold=0.9)

        for insight in insights:
            assert "correlation" in insight.evidence
            assert "method" in insight.evidence


class TestInsightGenerator:
    """洞察生成器测试"""

    def test_generate_all_insights(self, sample_dataframe):
        """测试生成所有洞察"""
        generator = InsightGenerator()
        options = {
            "include_trend": True,
            "include_anomaly": True,
            "include_correlation": True,
        }
        insights = generator.generate(sample_dataframe, options=options)

        # 应该有趋势洞察或相关性洞察
        trend_insights = [
            i for i in insights
            if i.type in (InsightType.TREND_UP, InsightType.TREND_DOWN, InsightType.TREND_FLAT)
        ]
        # 检查相关性洞察
        corr_insights = [i for i in insights if i.type == InsightType.CORRELATION]

        assert len(trend_insights) > 0 or len(corr_insights) > 0

    def test_insights_sorted_by_priority(self, sample_dataframe):
        """测试洞察按优先级排序"""
        generator = InsightGenerator()
        insights = generator.generate(sample_dataframe)

        # 检查是否按严重程度和置信度排序
        if len(insights) > 1:
            severity_order = {"high": 0, "medium": 1, "low": 2}
            for i in range(len(insights) - 1):
                current_severity = severity_order.get(str(insights[i].severity), 3)
                next_severity = severity_order.get(str(insights[i + 1].severity), 3)
                # 当前洞察的严重程度应该 <= 下一个（优先级更高）
                assert current_severity <= next_severity

    def test_insight_types_are_enums(self, sample_dataframe):
        """测试洞察类型是枚举"""
        generator = InsightGenerator()
        insights = generator.generate(sample_dataframe)

        for insight in insights:
            # 类型应该是枚举或字符串
            assert isinstance(insight.type, (InsightType, str))
            assert isinstance(insight.severity, (InsightSeverity, str))
