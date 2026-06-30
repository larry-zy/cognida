"""gRPC服务测试。"""

import io
import pandas as pd
import pytest
from unittest.mock import Mock, patch
from grpc import ServicerContext

import quality_pb2
import quality_pb2_grpc
from services.quality.servicer import QualityServicer


@pytest.fixture
def servicer():
    """创建服务实例。"""
    return QualityServicer()


@pytest.fixture
def sample_csv_data():
    """创建示例CSV数据。"""
    data = pd.DataFrame({
        "id": [1, 2, 3, 4, 5],
        "name": ["Alice", "Bob", None, "David", "Eve"],
        "age": [25, 30, 35, -5, 150],
    })
    output = io.BytesIO()
    data.to_csv(output, index=False)
    return output.getvalue()


@pytest.fixture
def mock_context():
    """创建模拟gRPC上下文。"""
    context = Mock(spec=ServicerContext)
    return context


class TestQualityServicer:
    """质量服务测试。"""

    def test_initialization(self):
        """测试初始化。"""
        servicer = QualityServicer()
        assert servicer.evaluator is not None
        assert servicer.pipeline is not None
        assert servicer.drift_detector is not None
        assert servicer.rule_engine is not None

    def test_evaluate_quality_with_file(self, servicer, mock_context, tmp_path):
        """测试通过文件路径评估质量。"""
        # 创建测试文件
        data = pd.DataFrame({
            "id": [1, 2, 3],
            "name": ["Alice", "Bob", "Charlie"],
        })
        file_path = tmp_path / "test.csv"
        data.to_csv(file_path, index=False)

        request = quality_pb2.EvaluateQualityRequest(
            file_path=str(file_path),
            dimensions=["completeness", "accuracy"],
        )

        response = servicer.EvaluateQuality(request, mock_context)

        assert response.success is True
        assert response.report is not None
        assert response.report.record_count == 3

    def test_evaluate_quality_with_csv_data(self, servicer, mock_context, sample_csv_data):
        """测试通过CSV数据评估质量。"""
        request = quality_pb2.EvaluateQualityRequest(
            csv_data=sample_csv_data,
            dimensions=["completeness"],
        )

        response = servicer.EvaluateQuality(request, mock_context)

        assert response.success is True
        assert response.report is not None

    def test_evaluate_quality_no_data(self, servicer, mock_context):
        """测试没有数据的情况。"""
        request = quality_pb2.EvaluateQualityRequest()

        response = servicer.EvaluateQuality(request, mock_context)

        assert response.success is False
        assert "No data provided" in response.error_message

    def test_evaluate_quality_with_config(self, servicer, mock_context, sample_csv_data):
        """测试带配置的评估。"""
        request = quality_pb2.EvaluateQualityRequest(
            csv_data=sample_csv_data,
            config={"threshold": "0.9", "mode": "strict"},
        )

        response = servicer.EvaluateQuality(request, mock_context)

        assert response.success is True

    def test_evaluate_unstructured_quality(self, servicer, mock_context):
        """测试非结构化数据评估。"""
        text = """
        人工智能是计算机科学的重要分支。
        机器学习使计算机能够从数据中学习。
        """

        request = quality_pb2.EvaluateUnstructuredQualityRequest(
            text=text,
            dimensions=["readability", "information_density"],
        )

        response = servicer.EvaluateUnstructuredQuality(request, mock_context)

        assert response.success is True
        assert response.report is not None
        assert response.report.overall_score >= 0

    def test_evaluate_unstructured_quality_empty(self, servicer, mock_context):
        """测试空文本评估。"""
        request = quality_pb2.EvaluateUnstructuredQualityRequest(
            text="",
        )

        response = servicer.EvaluateUnstructuredQuality(request, mock_context)

        # 空文本应该返回成功但分数较低
        assert response.success is True
        assert response.report.overall_score < 50

    def test_clean_data(self, servicer, mock_context, sample_csv_data):
        """测试数据清洗。"""
        request = quality_pb2.CleanDataRequest(
            csv_data=sample_csv_data,
            cleaners=["text_cleaner"],
        )

        response = servicer.CleanData(request, mock_context)

        assert response.success is True
        assert response.result is not None
        assert response.cleaned_data is not None

    def test_clean_data_no_data(self, servicer, mock_context):
        """测试清洗没有数据的情况。"""
        request = quality_pb2.CleanDataRequest()

        response = servicer.CleanData(request, mock_context)

        assert response.success is False
        assert "No data provided" in response.error_message

    def test_process_pipeline_structured(self, servicer, mock_context, sample_csv_data):
        """测试结构化数据流程处理。"""
        request = quality_pb2.ProcessPipelineRequest(
            csv_data=sample_csv_data,
            is_structured=True,
            config={"accept_threshold": "80"},
        )

        response = servicer.ProcessPipeline(request, mock_context)

        assert response.success is True
        assert response.result is not None
        assert response.result.HasField("structured_report")

    def test_process_pipeline_unstructured(self, servicer, mock_context):
        """测试非结构化数据流程处理。"""
        text = "这是一段测试文本，用于评估质量。"

        request = quality_pb2.ProcessPipelineRequest(
            text=text,
            is_structured=False,
        )

        response = servicer.ProcessPipeline(request, mock_context)

        assert response.success is True
        assert response.result is not None
        assert response.result.HasField("unstructured_report")

    def test_process_pipeline_no_data(self, servicer, mock_context):
        """测试流程处理没有数据的情况。"""
        request = quality_pb2.ProcessPipelineRequest(
            is_structured=True,
        )

        response = servicer.ProcessPipeline(request, mock_context)

        assert response.success is False
        assert "No data provided" in response.error_message

    def test_detect_drift(self, servicer, mock_context):
        """测试漂移检测。"""
        baseline_data = pd.DataFrame({
            "value": [10, 20, 30, 40, 50] * 20,
        })
        current_data = pd.DataFrame({
            "value": [15, 25, 35, 45, 55] * 20,  # 有漂移
        })

        baseline_output = io.BytesIO()
        baseline_data.to_csv(baseline_output, index=False)
        current_output = io.BytesIO()
        current_data.to_csv(current_output, index=False)

        request = quality_pb2.DetectDriftRequest(
            baseline_data=baseline_output.getvalue(),
            current_data=current_output.getvalue(),
            columns=["value"],
            threshold=0.05,
        )

        response = servicer.DetectDrift(request, mock_context)

        assert response.success is True
        assert response.overall_drift_score >= 0
        assert len(response.drift_metrics) > 0

    def test_list_dimensions(self, servicer, mock_context):
        """测试列出维度。"""
        request = quality_pb2.ListDimensionsRequest()

        response = servicer.ListDimensions(request, mock_context)

        assert response.success is True
        assert len(response.dimensions) > 0

        # 检查是否有结构化和非结构化维度
        has_structured = any(d.supports_structured for d in response.dimensions)
        has_unstructured = any(d.supports_unstructured for d in response.dimensions)
        assert has_structured is True
        assert has_unstructured is True

    def test_get_quality_rules(self, servicer, mock_context):
        """测试获取质量规则。"""
        request = quality_pb2.GetQualityRulesRequest()

        response = servicer.GetQualityRules(request, mock_context)

        assert response.success is True
        assert response.rules is not None
        assert isinstance(response.available_rule_files, list)

    def test_get_quality_rules_with_file(self, servicer, mock_context):
        """测试获取指定文件的质量规则。"""
        request = quality_pb2.GetQualityRulesRequest(
            rule_file="default_rules.yaml",
        )

        response = servicer.GetQualityRules(request, mock_context)

        # 即使文件不存在，也应该返回成功（使用默认规则）
        assert response.success is True

    def test_update_quality_rules(self, servicer, mock_context):
        """测试更新质量规则。"""
        rules = quality_pb2.QualityRules(
            version="1.0",
        )

        request = quality_pb2.UpdateQualityRulesRequest(
            rule_file="test_rules.yaml",
            rules=rules,
        )

        response = servicer.UpdateQualityRules(request, mock_context)

        # 当前实现返回成功（简化版）
        assert response.success is True

    def test_register_custom_evaluator(self, servicer, mock_context):
        """测试注册自定义评估器。"""
        # 使用内置模块测试
        request = quality_pb2.RegisterCustomEvaluatorRequest(
            name="test_evaluator",
            module_path="services.quality.dimensions.completeness",
            class_name="CompletenessEvaluator",
        )

        response = servicer.RegisterCustomEvaluator(request, mock_context)

        assert response.success is True

    def test_register_custom_evaluator_invalid(self, servicer, mock_context):
        """测试注册无效的评估器。"""
        request = quality_pb2.RegisterCustomEvaluatorRequest(
            name="invalid_evaluator",
            module_path="invalid.module.path",
            class_name="InvalidClass",
        )

        response = servicer.RegisterCustomEvaluator(request, mock_context)

        assert response.success is False
        assert response.error_message is not None

    def test_register_custom_cleaner(self, servicer, mock_context):
        """测试注册自定义清洗器。"""
        request = quality_pb2.RegisterCustomCleanerRequest(
            name="test_cleaner",
            module_path="services.quality.cleaning.text_cleaner",
            class_name="TextCleaner",
        )

        response = servicer.RegisterCustomCleaner(request, mock_context)

        assert response.success is True

    def test_register_custom_cleaner_invalid(self, servicer, mock_context):
        """测试注册无效的清洗器。"""
        request = quality_pb2.RegisterCustomCleanerRequest(
            name="invalid_cleaner",
            module_path="invalid.module.path",
            class_name="InvalidClass",
        )

        response = servicer.RegisterCustomCleaner(request, mock_context)

        assert response.success is False

    def test_convert_quality_report(self, servicer):
        """测试质量报告转换。"""
        from services.quality.models import QualityReport, DimensionScore, QualityIssue

        report = QualityReport(
            overall_score=85.0,
            dimensions=[
                DimensionScore(
                    name="completeness",
                    score=90.0,
                    passed=True,
                    issues=[],
                )
            ],
            issues=[],
            record_count=100,
            metadata={"key": "value"},
        )

        pb_report = servicer._convert_quality_report(report)

        assert pb_report.overall_score == 85.0
        assert pb_report.record_count == 100
        assert len(pb_report.dimensions) == 1
        assert pb_report.metadata["key"] == "value"

    def test_convert_unstructured_report(self, servicer):
        """测试非结构化报告转换。"""
        from services.quality.models import (
            UnstructuredQualityReport,
            UnstructuredDimensionScore,
            TextQualityIssue,
        )

        report = UnstructuredQualityReport(
            overall_score=75.0,
            dimensions=[
                UnstructuredDimensionScore(
                    name="readability",
                    score=80.0,
                    passed=True,
                    issues=[],
                    details={},
                )
            ],
            text_stats={"length": 100},
            issues=[],
        )

        pb_report = servicer._convert_unstructured_report(report)

        assert pb_report.overall_score == 75.0
        assert len(pb_report.dimensions) == 1
        assert pb_report.text_stats["length"] == "100"

    def test_convert_severity(self, servicer):
        """测试严重级别转换。"""
        from services.quality.models import SeverityLevel

        # 测试各个级别
        assert servicer._convert_severity(SeverityLevel.CRITICAL) == quality_pb2.SeverityLevel.CRITICAL
        assert servicer._convert_severity(SeverityLevel.WARNING) == quality_pb2.SeverityLevel.WARNING
        assert servicer._convert_severity(SeverityLevel.INFO) == quality_pb2.SeverityLevel.INFO

    def test_convert_decision(self, servicer):
        """测试决策转换。"""
        from services.quality.models import Decision

        assert servicer._convert_decision(Decision.ACCEPT) == quality_pb2.Decision.ACCEPT
        assert servicer._convert_decision(Decision.REJECT) == quality_pb2.Decision.REJECT
        assert servicer._convert_decision(Decision.REVIEW) == quality_pb2.Decision.REVIEW


class TestServicerIntegration:
    """服务集成测试。"""

    def test_full_evaluation_workflow(self, servicer, mock_context, sample_csv_data):
        """测试完整评估工作流。"""
        # 评估
        eval_request = quality_pb2.EvaluateQualityRequest(
            csv_data=sample_csv_data,
        )
        eval_response = servicer.EvaluateQuality(eval_request, mock_context)

        assert eval_response.success is True

        # 获取规则
        rules_request = quality_pb2.GetQualityRulesRequest()
        rules_response = servicer.GetQualityRules(rules_request, mock_context)

        assert rules_response.success is True

        # 列出维度
        dim_request = quality_pb2.ListDimensionsRequest()
        dim_response = servicer.ListDimensions(dim_request, mock_context)

        assert dim_response.success is True

    def test_pipeline_workflow(self, servicer, mock_context):
        """测试流程工作流。"""
        text = "人工智能是计算机科学的重要分支。"

        # 处理流程
        pipeline_request = quality_pb2.ProcessPipelineRequest(
            text=text,
            is_structured=False,
        )
        pipeline_response = servicer.ProcessPipeline(pipeline_request, mock_context)

        assert pipeline_response.success is True
        assert pipeline_response.result.decision in [
            quality_pb2.Decision.ACCEPT,
            quality_pb2.Decision.REJECT,
            quality_pb2.Decision.REVIEW,
        ]

    def test_error_handling(self, servicer, mock_context):
        """测试错误处理。"""
        # 发送无效的CSV数据
        request = quality_pb2.EvaluateQualityRequest(
            csv_data=b"invalid,csv,data\nwith mismatched,columns",
        )

        response = servicer.EvaluateQuality(request, mock_context)

        # 应该返回错误或处理异常
        assert response is not None
        if not response.success:
            assert response.error_message is not None
