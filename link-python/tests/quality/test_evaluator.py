"""评估器集成测试。"""

import pandas as pd
import pytest
from pathlib import Path

from services.quality.evaluator import DataQualityEvaluator
from services.quality.models import QualityReport, UnstructuredQualityReport


@pytest.fixture
def evaluator():
    """创建评估器实例。"""
    return DataQualityEvaluator()


@pytest.fixture
def sample_csv_path(tmp_path):
    """创建示例CSV文件。"""
    data = pd.DataFrame({
        "id": [1, 2, 3, 4, 5],
        "name": ["Alice", "Bob", None, "David", "Eve"],
        "age": [25, 30, 35, -5, 150],
        "email": [
            "alice@example.com",
            "bob@example.com",
            "invalid-email",
            "david@example.com",
            "eve@example.com",
        ],
        "score": [85.5, 90.0, 78.5, 92.0, 88.5],
    })
    file_path = tmp_path / "test_data.csv"
    data.to_csv(file_path, index=False)
    return file_path


@pytest.fixture
def sample_text():
    """创建示例文本。"""
    return """
    人工智能是计算机科学的重要分支，它致力于创建能够执行通常需要人类智能的任务的系统。
    机器学习是人工智能的核心技术之一，它使计算机能够从数据中学习并改进性能。
    深度学习是机器学习的一个子领域，它使用多层神经网络来模拟人脑的学习过程。
    自然语言处理是人工智能的另一个重要应用领域，它涉及计算机与人类语言之间的交互。
    """


class TestDataQualityEvaluator:
    """数据质量评估器测试。"""

    def test_initialization(self):
        """测试初始化。"""
        evaluator = DataQualityEvaluator()
        assert evaluator.rule_engine is not None

    def test_load_csv(self, sample_csv_path):
        """测试加载CSV文件。"""
        evaluator = DataQualityEvaluator()
        data = evaluator.load_data(sample_csv_path)

        assert isinstance(data, pd.DataFrame)
        assert len(data) == 5
        assert "id" in data.columns

    def test_load_data_by_extension(self, tmp_path):
        """测试通过扩展名加载数据。"""
        # CSV
        csv_path = tmp_path / "test.csv"
        pd.DataFrame({"a": [1, 2, 3]}).to_csv(csv_path, index=False)

        evaluator = DataQualityEvaluator()
        data = evaluator.load_data(csv_path)

        assert len(data) == 3

    def test_load_unsupported_format(self, tmp_path):
        """测试加载不支持的格式。"""
        unsupported_path = tmp_path / "test.xyz"
        unsupported_path.write_text("some data")

        evaluator = DataQualityEvaluator()

        with pytest.raises(ValueError, match="Unsupported source type"):
            evaluator.load_data(unsupported_path)

    def test_evaluate_structured_with_dataframe(self, evaluator):
        """测试评估DataFrame。"""
        data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5],
            "name": ["Alice", "Bob", "Charlie", "David", "Eve"],
            "age": [25, 30, 35, 40, 45],
        })

        result = evaluator.evaluate_structured(data)

        assert isinstance(result, QualityReport)
        assert 0 <= result.overall_score <= 100
        assert result.record_count == 5
        assert len(result.dimensions) > 0

    def test_evaluate_structured_with_file(self, evaluator, sample_csv_path):
        """测试评估文件。"""
        result = evaluator.evaluate_structured(sample_csv_path)

        assert isinstance(result, QualityReport)
        assert result.record_count == 5

    def test_evaluate_structured_with_dimensions(self, evaluator):
        """测试指定维度评估。"""
        data = pd.DataFrame({
            "id": [1, 2, 3],
            "name": ["Alice", None, "Charlie"],
        })

        result = evaluator.evaluate_structured(
            data,
            dimensions=["completeness"]
        )

        # 应该只评估指定的维度
        assert len(result.dimensions) == 1
        assert result.dimensions[0].name == "completeness"

    def test_evaluate_structured_with_config(self, evaluator):
        """测试带配置的评估。"""
        data = pd.DataFrame({
            "id": [1, 2, 3],
            "value": [100, 200, 300],
        })

        config = {"completeness_threshold": 1.0}
        result = evaluator.evaluate_structured(data, config=config)

        assert isinstance(result, QualityReport)

    def test_evaluate_unstructured(self, evaluator, sample_text):
        """测试非结构化文本评估。"""
        result = evaluator.evaluate_unstructured(sample_text)

        assert isinstance(result, UnstructuredQualityReport)
        assert 0 <= result.overall_score <= 100
        assert len(result.dimensions) > 0
        assert "length" in result.text_stats

    def test_evaluate_unstructured_with_dimensions(self, evaluator, sample_text):
        """测试指定维度评估非结构化数据。"""
        result = evaluator.evaluate_unstructured(
            sample_text,
            dimensions=["readability", "information_density"]
        )

        # 应该只评估指定的维度
        assert len(result.dimensions) == 2

    def test_evaluate_unstructured_short_text(self, evaluator):
        """测试短文本评估。"""
        short_text = "短文本。"

        result = evaluator.evaluate_unstructured(short_text)

        # 短文本应该有较低的分数
        assert result.overall_score < 100

    def test_get_recommendations_structured(self, evaluator):
        """测试获取结构化数据建议。"""
        data = pd.DataFrame({
            "id": [1, 2, 3],
            "name": ["Alice", None, "Charlie"],  # 有缺失
            "age": [25, -5, 150],  # 有异常值
        })

        report = evaluator.evaluate_structured(data)
        recommendations = evaluator.get_recommendations(report)

        assert isinstance(recommendations, list)
        assert len(recommendations) > 0

    def test_get_recommendations_unstructured(self, evaluator):
        """测试获取非结构化数据建议。"""
        # 创建有问题的文本
        text = "短。"
        report = evaluator.evaluate_unstructured(text)
        recommendations = evaluator.get_recommendations(report)

        assert isinstance(recommendations, list)

    def test_issue_severity_sorting(self, evaluator):
        """测试问题按严重级别排序。"""
        data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5],
            "name": [None, None, None, None, None],  # 大量缺失
            "age": [25, 30, 35, 40, 45],
        })

        report = evaluator.evaluate_structured(data)

        # 严重级别高的问题应该排在前面
        # 定义优先级顺序
        severity_order = {"critical": 1, "warning": 2, "info": 3}
        if len(report.issues) > 1:
            for i in range(len(report.issues) - 1):
                current_severity = report.issues[i].severity.value
                next_severity = report.issues[i + 1].severity.value
                # CRITICAL(1) > WARNING(2) > INFO(3)
                current_priority = severity_order.get(current_severity, 99)
                next_priority = severity_order.get(next_severity, 99)
                assert current_priority <= next_priority

    def test_overall_score_calculation(self, evaluator):
        """测试总体分数计算。"""
        # 创建高质量数据
        good_data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5],
            "name": ["Alice", "Bob", "Charlie", "David", "Eve"],
            "age": [25, 30, 35, 40, 45],
            "email": [
                "alice@example.com",
                "bob@example.com",
                "charlie@example.com",
                "david@example.com",
                "eve@example.com",
            ],
        })

        report = evaluator.evaluate_structured(good_data)

        # 高质量数据应该有较高的分数
        assert report.overall_score > 50

    def test_metadata_in_report(self, evaluator):
        """测试报告元数据。"""
        data = pd.DataFrame({"a": [1, 2, 3]})
        report = evaluator.evaluate_structured(data)

        assert report.metadata is not None
        assert isinstance(report.metadata, dict)

    def test_empty_dataframe_evaluation(self, evaluator):
        """测试空DataFrame评估。"""
        data = pd.DataFrame()

        report = evaluator.evaluate_structured(data)

        # 空数据应该有低分或返回默认报告
        assert isinstance(report, QualityReport)

    def test_single_row_evaluation(self, evaluator):
        """测试单行数据评估。"""
        data = pd.DataFrame({
            "id": [1],
            "name": ["Alice"],
        })

        report = evaluator.evaluate_structured(data)

        assert report.record_count == 1
        assert isinstance(report, QualityReport)

    def test_large_dataset_evaluation(self, evaluator):
        """测试大数据集评估。"""
        # 创建1000行数据
        data = pd.DataFrame({
            "id": range(1000),
            "value": [i % 100 for i in range(1000)],
        })

        report = evaluator.evaluate_structured(data)

        assert report.record_count == 1000

    def test_text_stats_in_unstructured_report(self, evaluator, sample_text):
        """测试非结构化报告中的文本统计。"""
        report = evaluator.evaluate_unstructured(sample_text)

        # 应该有基本的文本统计
        assert "length" in report.text_stats
        assert "word_count" in report.text_stats

    def test_dimension_score_structure(self, evaluator):
        """测试维度分数结构。"""
        data = pd.DataFrame({"a": [1, 2, 3]})
        report = evaluator.evaluate_structured(data)

        for dimension in report.dimensions:
            assert dimension.name is not None
            assert 0 <= dimension.score <= 100
            # passed can be bool or numpy.bool_
            assert dimension.passed in (True, False)
            assert isinstance(dimension.issues, list)

    def test_multiple_evaluations(self, evaluator):
        """测试多次评估。"""
        data1 = pd.DataFrame({"a": [1, 2, 3]})
        data2 = pd.DataFrame({"b": [4, 5, 6]})

        report1 = evaluator.evaluate_structured(data1)
        report2 = evaluator.evaluate_structured(data2)

        # 两次评估应该独立
        assert report1 is not report2

    def test_config_dir_initialization(self, tmp_path):
        """测试指定配置目录初始化。"""
        evaluator = DataQualityEvaluator(config_dir=str(tmp_path))
        assert evaluator.rule_engine is not None


class TestEvaluatorIntegration:
    """评估器集成测试。"""

    def test_full_workflow_structured(self, sample_csv_path):
        """测试完整结构化数据工作流。"""
        evaluator = DataQualityEvaluator()

        # 加载数据
        data = evaluator.load_data(sample_csv_path)

        # 评估
        report = evaluator.evaluate_structured(data)

        # 获取建议
        recommendations = evaluator.get_recommendations(report)

        # 检查完整流程
        assert len(data) > 0
        assert report is not None
        assert isinstance(recommendations, list)

    def test_full_workflow_unstructured(self):
        """测试完整非结构化数据工作流。"""
        evaluator = DataQualityEvaluator()

        text = """
        这是一段关于人工智能的文本。机器学习和深度学习是重要的技术。
        请联系：13812345678，邮箱：test@example.com
        """

        # 评估
        report = evaluator.evaluate_unstructured(text)

        # 获取建议
        recommendations = evaluator.get_recommendations(report)

        # 检查
        assert report is not None
        assert isinstance(recommendations, list)

    def test_mixed_quality_data(self):
        """测试混合质量数据。"""
        evaluator = DataQualityEvaluator()

        # 创建有各种质量问题的数据
        data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5, 6],
            "name": ["Alice", None, "  Bob  ", "Charlie", "David", "Eve"],
            "age": [25, 30, -5, 150, 35, 40],
            "email": [
                "alice@example.com",
                "invalid-email",
                "bob@example.com",
                None,
                "david@example.com",
                "eve@example.com",
            ],
        })

        report = evaluator.evaluate_structured(data)

        # 应该检测到多个问题
        assert len(report.issues) > 0
        assert report.overall_score < 100

    def test_recommendation_relevance(self):
        """测试建议相关性。"""
        evaluator = DataQualityEvaluator()

        # 创建有明显完整性问题的数据
        data = pd.DataFrame({
            "name": ["Alice", None, None, None, None],
            "age": [25, 30, 35, 40, 45],
        })

        report = evaluator.evaluate_structured(data)
        recommendations = evaluator.get_recommendations(report)

        # 应该有关于完整性的建议
        assert any("缺失" in r or "补充" in r for r in recommendations)
