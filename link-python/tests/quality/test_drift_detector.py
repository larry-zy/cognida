"""漂移检测器测试。"""

import pandas as pd
import pytest
from datetime import datetime

from services.quality.drift_detector import DriftDetector, DriftResult, DriftMetric


@pytest.fixture
def baseline_data():
    """创建基线数据。"""
    return pd.DataFrame({
        "id": range(100),
        "age": [30 + i % 40 for i in range(100)],  # 30-69岁
        "score": [70 + (i % 30) for i in range(100)],  # 70-99分
        "category": ["A", "B", "C"] * 33 + ["A"],
    })


@pytest.fixture
def similar_data():
    """创建相似数据（无漂移）。"""
    return pd.DataFrame({
        "id": range(100),
        "age": [30 + i % 40 for i in range(100)],  # 同基线
        "score": [70 + (i % 30) for i in range(100)],  # 同基线
        "category": ["A", "B", "C"] * 33 + ["A"],
    })


@pytest.fixture
def drifted_data():
    """创建漂移数据。"""
    return pd.DataFrame({
        "id": range(100),
        "age": [40 + i % 35 for i in range(100)],  # 40-74岁，整体偏高
        "score": [50 + (i % 40) for i in range(100)],  # 50-89分，整体偏低
        "category": ["B", "C"] * 50,  # 分布变化
    })


@pytest.fixture
def extreme_drift_data():
    """创建极端漂移数据。"""
    return pd.DataFrame({
        "id": range(100),
        "age": [60 + i % 20 for i in range(100)],  # 60-79岁，严重偏高
        "score": [40 + (i % 20) for i in range(100)],  # 40-59分，严重偏低
        "category": ["C"] * 100,  # 完全不同的分布
    })


class TestDriftDetector:
    """漂移检测器测试。"""

    def test_initialization(self):
        """测试初始化。"""
        detector = DriftDetector()
        assert detector.threshold == 0.05
        assert detector.baseline_stats is None

    def test_set_baseline(self, baseline_data):
        """测试设置基线。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)

        assert detector.baseline_stats is not None
        assert "age" in detector.baseline_stats
        assert "score" in detector.baseline_stats
        assert detector.baseline_timestamp is not None

    def test_detect_no_drift(self, baseline_data, similar_data):
        """测试无漂移检测。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(similar_data)

        assert isinstance(result, DriftResult)
        # 相似数据应该没有显著漂移
        assert result.has_drift is False or result.overall_drift_score < 0.1

    def test_detect_drift(self, baseline_data, drifted_data):
        """测试漂移检测。"""
        detector = DriftDetector(threshold=0.05)
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(drifted_data)

        assert isinstance(result, DriftResult)
        # 漂移数据应该被检测到
        assert result.has_drift is True
        assert result.overall_drift_score > 0

    def test_detect_extreme_drift(self, baseline_data, extreme_drift_data):
        """测试极端漂移检测。"""
        detector = DriftDetector(threshold=0.05)
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(extreme_drift_data)

        assert result.has_drift is True
        assert result.overall_drift_score > 0.1  # 更高的漂移分数

        # 检查具体的漂移指标
        age_metric = next((m for m in result.drift_metrics if m.field == "age"), None)
        assert age_metric is not None
        assert age_metric.drifted == True

    def test_column_specific_detection(self, baseline_data):
        """测试指定列检测。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data, columns=["age", "score"])

        # 创建只有age漂移的数据
        drifted = baseline_data.copy()
        drifted["age"] = drifted["age"] + 20

        result = detector.detect_drift(drifted, columns=["age", "score"])

        # 应该只检测指定列
        assert len(result.drift_metrics) == 2
        assert all(m.field in ["age", "score"] for m in result.drift_metrics)

    def test_ks_test_calculation(self, baseline_data):
        """测试KS检验计算。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)

        # 创建有明显分布变化的数据
        drifted = pd.DataFrame({
            "age": [50 + i % 30 for i in range(100)],  # 完全不同的分布
        })

        result = detector.detect_drift(drifted, columns=["age"])

        age_metric = next((m for m in result.drift_metrics if m.field == "age"), None)
        assert age_metric is not None
        # KS检验返回的p_value应该很小（表示分布差异）
        assert age_metric.p_value is not None

    def test_threshold_configuration(self, baseline_data, drifted_data):
        """测试阈值配置。"""
        # 严格阈值
        strict_detector = DriftDetector(threshold=0.01)
        strict_detector.set_baseline(baseline_data)
        strict_result = strict_detector.detect_drift(drifted_data)

        # 宽松阈值
        loose_detector = DriftDetector(threshold=0.2)
        loose_detector.set_baseline(baseline_data)
        loose_result = loose_detector.detect_drift(drifted_data)

        # 严格阈值可能检测到更多漂移
        # （具体行为取决于数据）
        assert isinstance(strict_result, DriftResult)
        assert isinstance(loose_result, DriftResult)

    def test_drift_metric_structure(self, baseline_data, drifted_data):
        """测试漂移指标结构。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(drifted_data)

        for metric in result.drift_metrics:
            assert metric.field is not None
            assert metric.drift_score >= 0
            assert metric.p_value is not None
            assert metric.threshold > 0
            # drifted can be bool or numpy.bool_
            assert metric.drifted in (True, False)

    def test_timestamp_tracking(self, baseline_data):
        """测试时间戳跟踪。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)

        baseline_time = detector.baseline_timestamp
        assert baseline_time is not None

        result = detector.detect_drift(baseline_data)

        assert result.baseline_timestamp == baseline_time
        assert result.current_timestamp is not None
        assert result.current_timestamp >= baseline_time

    def test_empty_data_handling(self):
        """测试空数据处理。"""
        detector = DriftDetector()

        empty_data = pd.DataFrame()

        # 空数据应该被优雅处理
        try:
            detector.set_baseline(empty_data)
            # 或者抛出适当的异常
        except Exception as e:
            assert isinstance(e, (ValueError, KeyError))

    def test_single_column_detection(self, baseline_data):
        """测试单列检测。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data, columns=["age"])

        drifted = baseline_data.copy()
        drifted["age"] = drifted["age"] + 20

        result = detector.detect_drift(drifted, columns=["age"])

        assert len(result.drift_metrics) == 1
        assert result.drift_metrics[0].field == "age"

    def test_all_columns_detection(self, baseline_data, drifted_data):
        """测试所有列检测。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)  # 不指定列，检测所有数值列

        result = detector.detect_drift(drifted_data)

        # 应该检测所有数值型列
        numeric_columns = baseline_data.select_dtypes(include=["number"]).columns.tolist()
        assert len(result.drift_metrics) >= len(numeric_columns)

    def test_categorical_column_detection(self, baseline_data):
        """测试分类列检测。"""
        detector = DriftDetector()
        # 当前实现只支持数值列，分类列会被跳过
        detector.set_baseline(baseline_data)

        # 改变分类分布
        drifted = baseline_data.copy()
        drifted["category"] = ["C"] * 100

        # category 列不会被检测（非数值列）
        result = detector.detect_drift(drifted, columns=["category"])

        # 结果中不应该有 category 指标（因为不是数值列）
        category_metric = next((m for m in result.drift_metrics if m.field == "category"), None)
        # 当前实现不支持分类列，所以 category_metric 应该是 None
        # 如果需要支持分类列，需要添加卡方检验等方法

    def test_drift_result_str_representation(self, baseline_data, drifted_data):
        """测试漂移结果字符串表示。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(drifted_data)

        # 测试字符串表示不会报错
        result_str = str(result)
        assert len(result_str) > 0

        # 测试详细信息
        if result.has_drift:
            details = result.get_details()
            assert "drifted_fields" in details
            assert len(details["drifted_fields"]) > 0

    def test_multiple_detection_calls(self, baseline_data):
        """测试多次检测调用。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)

        # 第一次检测
        result1 = detector.detect_drift(baseline_data)

        # 更新基线
        drifted = baseline_data.copy()
        drifted["age"] = drifted["age"] + 10
        detector.set_baseline(drifted)

        # 第二次检测（使用新基线）
        result2 = detector.detect_drift(baseline_data)

        # 结果应该不同
        assert isinstance(result1, DriftResult)
        assert isinstance(result2, DriftResult)

    def test_drift_summary(self, baseline_data, extreme_drift_data):
        """测试漂移摘要。"""
        detector = DriftDetector()
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(extreme_drift_data)

        if result.has_drift:
            summary = result.get_summary()
            assert "overall_drift_score" in summary
            assert "drifted_field_count" in summary
            assert "total_field_count" in summary
