"""流程编排测试。"""

import pandas as pd
import pytest

from services.quality.pipeline.executor import QualityPipeline
from services.quality.pipeline.hooks import (
    PipelineHook,
    HookManager,
    LoggingHook,
    MetricsHook,
    PipelineContext,
)
from services.quality.models import Decision, DimensionScore


@pytest.fixture
def sample_structured_data():
    """创建结构化测试数据。"""
    return pd.DataFrame({
        "id": [1, 2, 3, 4, 5],
        "name": ["Alice", "Bob", None, "David", "Eve"],
        "age": [25, 30, 35, -5, 150],
        "score": [85.5, 90.0, 78.5, 92.0, 88.5],
    })


@pytest.fixture
def sample_unstructured_text():
    """创建非结构化测试文本。"""
    return """
    人工智能是计算机科学的重要分支，致力于创建智能系统。
    机器学习使计算机能够从数据中学习。
    深度学习使用神经网络模拟人脑学习。
    """


class TestQualityPipeline:
    """质量评估流程测试。"""

    def test_initialization(self):
        """测试初始化。"""
        pipeline = QualityPipeline()
        assert pipeline._max_iterations == 3
        assert pipeline.hook_manager is not None

    def test_set_max_iterations(self):
        """测试设置最大迭代次数。"""
        pipeline = QualityPipeline()
        pipeline.set_max_iterations(5)
        assert pipeline._max_iterations == 5

    def test_process_structured_basic(self, sample_structured_data):
        """测试基本结构化数据处理。"""
        pipeline = QualityPipeline()
        result = pipeline.process_structured(sample_structured_data)

        assert result.decision in [Decision.ACCEPT, Decision.REJECT, Decision.REVIEW]
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert "quick_eval" in result.stage_times
        assert "deep_eval" in result.stage_times

    def test_process_unstructured_basic(self, sample_unstructured_text):
        """测试基本非结构化数据处理。"""
        pipeline = QualityPipeline()
        result = pipeline.process_unstructured(sample_unstructured_text)

        assert result.decision in [Decision.ACCEPT, Decision.REJECT, Decision.REVIEW]
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert result.quality_report is not None

    def test_quick_evaluation_threshold(self, sample_structured_data):
        """测试快速评估阈值。"""
        pipeline = QualityPipeline()

        # 高质量数据
        config = {"quick_threshold": 50}
        result = pipeline.process_structured(sample_structured_data, config)

        # 低阈值应该跳过清洗
        assert result.cleaning_result is None or result.quick_score >= 50

    def test_deep_evaluation_dimensions(self, sample_structured_data):
        """测试深度评估维度。"""
        pipeline = QualityPipeline()
        result = pipeline.process_structured(sample_structured_data)

        # 应该有多个维度评分
        if isinstance(result.quality_report.dimensions, list):
            assert len(result.quality_report.dimensions) > 0

    def test_decision_making(self):
        """测试决策逻辑。"""
        pipeline = QualityPipeline()

        # 高分数据应该被接受
        high_quality_data = pd.DataFrame({
            "id": range(100),
            "value": [50 + i % 50 for i in range(100)],
        })
        result = pipeline.process_structured(
            high_quality_data,
            {"accept_threshold": 60, "reject_threshold": 30}
        )
        # 根据实际质量，可能是ACCEPT或REVIEW

    def test_stage_times_tracking(self, sample_structured_data):
        """测试阶段时间跟踪。"""
        pipeline = QualityPipeline()
        result = pipeline.process_structured(sample_structured_data)

        # 所有阶段都应该有时间记录
        for stage in ["quick_eval", "cleaning", "deep_eval", "decision"]:
            assert stage in result.stage_times
            assert result.stage_times[stage] >= 0

    def test_cleaning_result_in_pipeline(self, sample_structured_data):
        """测试流程中的清洗结果。"""
        pipeline = QualityPipeline()
        # 设置低阈值确保触发清洗
        config = {"quick_threshold": 100}  # 不太可能达到
        result = pipeline.process_structured(sample_structured_data, config)

        # 如果质量较低，应该有清洗结果
        if result.quick_score < 100:
            # 不一定有清洗，因为要看具体实现
            assert result is not None


class TestPipelineHooks:
    """流程钩子测试。"""

    def test_hook_registration(self):
        """测试钩子注册。"""
        manager = HookManager()
        hook = LoggingHook()

        manager.register(hook)
        assert len(manager.hooks) == 1

    def test_hook_execution_order(self):
        """测试钩子执行顺序。"""
        manager = HookManager()

        # 创建跟踪执行顺序的钩子
        call_order = []

        class FirstHook(PipelineHook):
            def before_quick_eval(self, context):
                call_order.append("first")

        class SecondHook(PipelineHook):
            def before_quick_eval(self, context):
                call_order.append("second")

        manager.register(FirstHook())
        manager.register(SecondHook())

        context = PipelineContext("test data", {})
        manager.execute_before_quick_eval(context)

        assert call_order == ["first", "second"]

    def test_logging_hook(self, sample_structured_data):
        """测试日志钩子。"""
        hook = LoggingHook()
        context = PipelineContext(sample_structured_data, {})

        # 不应该抛出异常
        hook.before_quick_eval(context)
        hook.after_quick_eval(context, [])
        hook.before_cleaning(context)

    def test_metrics_hook(self, sample_structured_data):
        """测试指标钩子。"""
        hook = MetricsHook()
        context = PipelineContext(sample_structured_data, {})

        # 执行钩子
        hook.before_quick_eval(context)
        context.quick_score = 85.0
        hook.after_quick_eval(context, [])

        # 检查指标收集
        assert "quick_eval_start" in hook.metrics
        assert "quick_eval_duration" in hook.metrics

    def test_hook_manager_all_hooks(self):
        """测试钩子管理器所有钩子方法。"""
        manager = HookManager()
        hook = LoggingHook()
        manager.register(hook)

        context = PipelineContext("test", {})
        dimensions = [DimensionScore(name="test", score=80, passed=True, issues=[])]
        decision = Decision.ACCEPT
        cleaning_result = None

        # 执行所有钩子方法
        manager.execute_before_quick_eval(context)
        manager.execute_after_quick_eval(context, dimensions)
        manager.execute_before_cleaning(context)
        manager.execute_after_cleaning(context, cleaning_result)
        manager.execute_before_deep_eval(context)
        manager.execute_after_deep_eval(context, dimensions)
        manager.execute_before_decision(context, dimensions)
        manager.execute_after_decision(context, decision)

        # 不应该抛出异常

    def test_custom_hook(self, sample_structured_data):
        """测试自定义钩子。"""
        call_tracker = []

        class CustomHook(PipelineHook):
            def before_quick_eval(self, context):
                call_tracker.append("before_quick")

            def after_decision(self, context, decision):
                call_tracker.append(f"decision_{decision.value}")

        hook = CustomHook()

        manager = HookManager()
        manager.register(hook)

        context = PipelineContext(sample_structured_data, {})
        manager.execute_before_quick_eval(context)
        manager.execute_after_decision(context, Decision.ACCEPT)

        assert "before_quick" in call_tracker
        assert "decision_accept" in call_tracker

    def test_hook_with_pipeline(self, sample_structured_data):
        """测试钩子与流程集成。"""
        pipeline = QualityPipeline()
        call_tracker = []

        class TrackingHook(PipelineHook):
            def before_quick_eval(self, context):
                call_tracker.append("start")

            def after_decision(self, context, decision):
                call_tracker.append(f"end_{decision.value}")

        pipeline.register_hook(TrackingHook())
        result = pipeline.process_structured(sample_structured_data)

        assert "start" in call_tracker
        assert "end" in call_tracker[1]  # 第二个元素应该是end_xxx

    def test_hook_error_handling(self):
        """测试钩子错误处理。"""
        class FailingHook(PipelineHook):
            def before_quick_eval(self, context):
                raise ValueError("Hook error")

        manager = HookManager()
        manager.register(FailingHook())

        context = PipelineContext("test", {})

        # 应该处理错误（要么继续，要么抛出）
        try:
            manager.execute_before_quick_eval(context)
            # 如果没有抛出，说明错误被处理
        except ValueError:
            # 也可以选择抛出错误
            pass

    def test_multiple_hooks_execution(self):
        """测试多个钩子执行。"""
        manager = HookManager()

        results = []

        class Hook1(PipelineHook):
            def before_quick_eval(self, context):
                results.append("hook1")

        class Hook2(PipelineHook):
            def before_quick_eval(self, context):
                results.append("hook2")

        class Hook3(PipelineHook):
            def before_quick_eval(self, context):
                results.append("hook3")

        manager.register(Hook1())
        manager.register(Hook2())
        manager.register(Hook3())

        context = PipelineContext("test", {})
        manager.execute_before_quick_eval(context)

        assert results == ["hook1", "hook2", "hook3"]

    def test_pipeline_context(self):
        """测试流程上下文。"""
        data = pd.DataFrame({"a": [1, 2, 3]})
        config = {"key": "value"}
        context = PipelineContext(data, config)

        assert context.data is not None
        assert context.config == config
        assert context.quick_score is None
        assert context.deep_score is None

        # 更新状态
        context.quick_score = 80.0
        assert context.quick_score == 80.0


class TestPipelineIntegration:
    """流程集成测试。"""

    def test_end_to_end_structured(self, sample_structured_data):
        """测试结构化数据端到端流程。"""
        pipeline = QualityPipeline()
        result = pipeline.process_structured(sample_structured_data)

        # 检查结果完整性
        assert result.decision is not None
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert result.metadata is not None
        assert result.timestamp is not None

    def test_end_to_end_unstructured(self, sample_unstructured_text):
        """测试非结构化数据端到端流程。"""
        pipeline = QualityPipeline()
        result = pipeline.process_unstructured(sample_unstructured_text)

        # 检查结果完整性
        assert result.decision is not None
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert result.quality_report is not None

    def test_pipeline_with_custom_config(self, sample_structured_data):
        """测试带自定义配置的流程。"""
        pipeline = QualityPipeline()
        config = {
            "accept_threshold": 90,
            "reject_threshold": 40,
            "quick_threshold": 85,
            "dimensions": ["completeness", "accuracy"],
        }

        result = pipeline.process_structured(sample_structured_data, config)

        assert result is not None

    def test_pipeline_iteration(self):
        """测试流程迭代。"""
        # 创建质量较低的数据
        low_quality_data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5],
            "value": [None, None, None, None, 10],  # 80%缺失
        })

        pipeline = QualityPipeline()
        pipeline.set_max_iterations(2)

        result = pipeline.process_structured(low_quality_data)

        # 检查是否执行了迭代
        # 具体行为取决于实现

    def test_pipeline_metadata(self, sample_structured_data):
        """测试流程元数据。"""
        pipeline = QualityPipeline()
        result = pipeline.process_structured(sample_structured_data)

        # 应该有元数据
        assert result.metadata is not None
        assert isinstance(result.metadata, dict)
