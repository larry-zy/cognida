"""端到端集成测试。"""

import io
import tempfile
from pathlib import Path

import pandas as pd
import pytest

from services.quality.evaluator import DataQualityEvaluator
from services.quality.models import QualityReport
from services.quality.pipeline.executor import QualityPipeline
from services.quality.drift_detector import DriftDetector
from services.quality.rules import RuleEngine


class TestEndToEndStructuredData:
    """结构化数据端到端测试。"""

    def test_complete_structured_workflow(self):
        """测试完整的结构化数据处理流程。"""
        # 1. 准备测试数据
        data = pd.DataFrame({
            "customer_id": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],
            "name": ["Alice", "Bob", None, "David", "Eve", "Frank", None, "Henry", "Isabel", "Jack"],
            "age": [25, 30, 35, -5, 150, 28, 32, 45, 29, 31],
            "email": [
                "alice@example.com",
                "bob@example.com",
                "invalid-email",
                "david@example.com",
                "eve@example.com",
                "",
                "grace@example.com",
                "another-invalid",
                "isabel@example.com",
                "jack@example.com",
            ],
            "purchase_amount": [100.50, 250.00, None, 75.25, -50.00, 300.00, 150.75, None, 200.00, 125.50],
            "signup_date": pd.to_datetime([
                "2024-01-01", "2024-02-15", "2024-03-01", "2024-04-10",
                "2024-05-20", "2024-06-05", "2024-07-12", "2024-08-18",
                "2024-09-25", "2024-10-30",
            ]),
        })

        # 2. 创建评估器
        evaluator = DataQualityEvaluator()

        # 3. 执行评估
        report = evaluator.evaluate_structured(data)

        # 4. 验证结果
        assert isinstance(report, QualityReport)
        assert report.record_count == 10
        assert 0 <= report.overall_score <= 100
        assert len(report.dimensions) > 0
        assert len(report.issues) > 0  # 应该检测到问题

        # 5. 获取建议
        recommendations = evaluator.get_recommendations(report)
        assert isinstance(recommendations, list)
        assert len(recommendations) > 0

    def test_structured_pipeline_workflow(self):
        """测试结构化数据完整流程。"""
        data = pd.DataFrame({
            "id": [1, 2, 3, 4, 5],
            "text": ["Good text", "  Text with spaces  ", "Short", "Normal text", "Duplicate"],
            "value": [100, 200, 300, 400, 500],
        })

        pipeline = QualityPipeline()
        result = pipeline.process_structured(data, {
            "accept_threshold": 70,
            "reject_threshold": 40,
        })

        # 验证流程结果
        assert result.decision is not None
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert len(result.stage_times) > 0
        assert result.quality_report is not None

    def test_structured_drift_detection_workflow(self):
        """测试结构化数据漂移检测工作流。"""
        # 基线数据
        baseline_data = pd.DataFrame({
            "product_id": range(100),
            "price": [10 + i % 50 for i in range(100)],
            "quantity": [1 + i % 10 for i in range(100)],
            "category": ["A", "B", "C"] * 33 + ["A"],
        })

        # 当前数据（有漂移）
        current_data = pd.DataFrame({
            "product_id": range(100),
            "price": [20 + i % 40 for i in range(100)],  # 价格整体上涨
            "quantity": [5 + i % 8 for i in range(100)],  # 数量变化
            "category": ["B", "C"] * 50,  # 分布变化
        })

        detector = DriftDetector(threshold=0.05)
        detector.set_baseline(baseline_data)
        result = detector.detect_drift(current_data)

        assert result.has_drift is True
        assert result.overall_drift_score > 0
        assert len(result.drift_metrics) > 0

    def test_rules_based_evaluation(self):
        """测试基于规则的评估工作流。"""
        # 创建自定义规则文件
        rules_data = r"""
version: "1.0"
field_rules:
  - name: customer_id
    type: integer
    required: true
  - name: email
    type: string
    required: true
    pattern: '^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$'
  - name: age
    type: integer
    min_value: 0
    max_value: 120
dimension_rules:
  - name: completeness
    enabled: true
    threshold: 0.95
    weight: 1.0
  - name: accuracy
    enabled: true
    threshold: 0.90
    weight: 1.0
  - name: validity
    enabled: true
    threshold: 0.85
    weight: 0.8
cleaning_rules:
  remove_duplicates: true
  normalize_whitespace: true
metadata:
  description: "客户数据质量规则"
"""

        with tempfile.TemporaryDirectory() as tmpdir:
            rules_path = Path(tmpdir) / "customer_rules.yaml"
            rules_path.write_bytes(rules_data.encode('utf-8'))

            # 使用规则评估
            data = pd.DataFrame({
                "customer_id": [1, 2, 3, 4, 5],
                "email": ["valid@example.com", "invalid", "test@example.com", None, "user@example.com"],
                "age": [25, 30, -5, 150, 35],
            })

            evaluator = DataQualityEvaluator(config_dir=tmpdir)
            report = evaluator.evaluate_structured(data, rule_file="customer_rules.yaml")

            assert report is not None
            assert len(report.issues) > 0


class TestEndToEndUnstructuredData:
    """非结构化数据端到端测试。"""

    def test_complete_unstructured_workflow(self):
        """测试完整的非结构化文本处理流程。"""
        texts = [
            "人工智能是计算机科学的重要分支，致力于创建能够执行通常需要人类智能的任务的系统。",
            "机器学习使计算机能够从数据中学习并改进性能，无需显式编程。",
            "深度学习是机器学习的一个子领域，使用多层神经网络来处理复杂的数据模式。",
            "自然语言处理（NLP）是人工智能的一个应用领域，专注于计算机与人类语言之间的交互。",
            "短文本。",
        ]

        evaluator = DataQualityEvaluator()

        for text in texts:
            report = evaluator.evaluate_unstructured(text)

            assert report is not None
            assert 0 <= report.overall_score <= 100
            assert len(report.dimensions) > 0
            assert "length" in report.text_stats
            assert "word_count" in report.text_stats

    def test_unstructured_pipeline_workflow(self):
        """测试非结构化数据完整流程。"""
        text = """
        人工智能技术正在快速发展。深度学习和机器学习是重要的技术方向。
        自然语言处理和计算机视觉也取得了显著进展。
        联系方式：13812345678，邮箱：contact@example.com
        """

        pipeline = QualityPipeline()
        result = pipeline.process_unstructured(text, {
            "dimensions": ["readability", "information_density", "pii_detector"],
        })

        assert result.decision is not None
        assert result.quick_score >= 0
        assert result.deep_score >= 0
        assert result.quality_report is not None

    def test_text_cleaning_workflow(self):
        """测试文本清洗工作流。"""
        dirty_texts = [
            "  这段文本  有  多余  空格  ",
            "<div>包含HTML标签<b>的文本</b></div>",
            "包含\t制表符\n和换行符的文本",
            "正常文本",
        ]

        evaluator = DataQualityEvaluator()
        cleaned_results = []

        for text in dirty_texts:
            report = evaluator.evaluate_unstructured(text)
            cleaned_results.append({
                "original": text,
                "score": report.overall_score,
                "issues": [i.description for i in report.issues],
            })

        assert len(cleaned_results) == 4

    def test_batch_text_evaluation(self):
        """测试批量文本评估。"""
        texts = [
            "高质量文本，内容丰富，信息密度高，语言流畅。",
            "低质量文本。短。",
            "包含敏感信息：电话13812345678，邮箱test@example.com",
            "正常的中等质量文本。",
        ] * 10  # 40个文本

        evaluator = DataQualityEvaluator()
        results = []

        for text in texts:
            report = evaluator.evaluate_unstructured(text)
            results.append({
                "score": report.overall_score,
                "word_count": report.text_stats.get("word_count", 0),
                "issue_count": len(report.issues),
            })

        assert len(results) == 40
        scores = [r["score"] for r in results]
        assert min(scores) >= 0
        assert max(scores) <= 100


class TestEndToEndDataScenarios:
    """端到端数据场景测试。"""

    def test_customer_data_quality_scenario(self):
        """测试客户数据质量场景。"""
        # 模拟客户数据
        customer_data = pd.DataFrame({
            "customer_id": [i for i in range(1, 101)],
            "first_name": [f"Customer{i}" for i in range(1, 101)] + [None] * 0,
            "last_name": [f"LastName{i}" for i in range(1, 101)],
            "email": [
                f"customer{i}@example.com" if i % 5 != 0 else "invalid"
                for i in range(1, 101)
            ],
            "phone": [
                f"138{i:08d}" if i % 10 != 0 else "invalid"
                for i in range(1, 101)
            ],
            "age": [25 + (i % 50) for i in range(100)],
            "signup_date": pd.date_range("2024-01-01", periods=100, freq="D"),
            "last_purchase": [100 + i * 10 for i in range(100)],
        })

        # 添加一些问题数据
        customer_data.loc[0, "first_name"] = None
        customer_data.loc[10, "age"] = -5
        customer_data.loc[20, "age"] = 200
        customer_data.loc[30:35, "email"] = None

        evaluator = DataQualityEvaluator()
        report = evaluator.evaluate_structured(customer_data)

        assert report.record_count == 100
        assert report.overall_score < 100  # 应该检测到问题
        recommendations = evaluator.get_recommendations(report)
        assert len(recommendations) > 0

    def test_ecommerce_product_data_scenario(self):
        """测试电商产品数据质量场景。"""
        product_data = pd.DataFrame({
            "product_id": [f"PROD{i:05d}" for i in range(1, 51)],
            "name": [f"Product {i}" for i in range(1, 51)],
            "description": [
                f"This is a detailed description for product {i}. " * (i % 5 + 1)
                for i in range(1, 51)
            ],
            "price": [10.0 + i * 2.5 for i in range(50)],
            "stock": [i % 100 for i in range(50)],
            "category": ["Electronics", "Clothing", "Books", "Home", "Sports"] * 10,
            "rating": [3.0 + (i % 3) for i in range(50)],
        })

        # 添加异常
        product_data.loc[5, "price"] = -100
        product_data.loc[10, "stock"] = -10
        product_data.loc[15, "rating"] = 6  # 超出范围
        product_data.loc[20:22, "category"] = "Unknown"

        evaluator = DataQualityEvaluator()
        report = evaluator.evaluate_structured(product_data)

        assert report.record_count == 50
        assert len(report.dimensions) > 0

    def test_log_data_quality_scenario(self):
        """测试日志数据质量场景。"""
        log_data = pd.DataFrame({
            "timestamp": pd.date_range("2024-01-01", periods=1000, freq="min"),
            "level": ["INFO", "WARNING", "ERROR", "DEBUG"] * 250,
            "message": [f"Log message {i}" for i in range(1000)],
            "user_id": [i % 50 for i in range(1000)],
            "response_time": [100 + (i % 500) for i in range(1000)],
        })

        # 添加问题
        log_data.loc[100:105, "level"] = "INVALID"
        log_data.loc[200, "response_time"] = -50
        log_data.loc[300:305, "user_id"] = None

        evaluator = DataQualityEvaluator()
        report = evaluator.evaluate_structured(log_data)

        assert report.record_count == 1000

    def test_survey_response_data_scenario(self):
        """测试问卷调查数据质量场景。"""
        survey_data = pd.DataFrame({
            "response_id": range(1, 201),
            "q1_satisfaction": [i % 6 for i in range(200)],  # 1-5 scale, 0 is invalid
            "q2_recommendation": [i % 11 for i in range(200)],  # 1-10 scale
            "q3_quality": [i % 6 for i in range(200)],
            "open_feedback": [
                f"Feedback text {i}" if i % 10 != 0 else ""
                for i in range(200)
            ],
            "age_group": ["18-25", "26-35", "36-45", "46-55", "55+"] * 40,
        })

        # 添加异常
        survey_data.loc[50:55, "q1_satisfaction"] = 10  # 超出范围
        survey_data.loc[60:65, "age_group"] = "Unknown"

        evaluator = DataQualityEvaluator()
        report = evaluator.evaluate_structured(survey_data)

        assert report.record_count == 200
        # 检测到超出范围的值
        assert len(report.issues) > 0


class TestEndToFileOperations:
    """端到端文件操作测试。"""

    def test_csv_file_roundtrip(self):
        """测试CSV文件读写往返。"""
        with tempfile.TemporaryDirectory() as tmpdir:
            # 创建测试文件
            original_data = pd.DataFrame({
                "id": [1, 2, 3, 4, 5],
                "name": ["Alice", "Bob", "Charlie", "David", "Eve"],
                "value": [100, 200, 300, 400, 500],
            })

            file_path = Path(tmpdir) / "test.csv"
            original_data.to_csv(file_path, index=False)

            # 评估
            evaluator = DataQualityEvaluator()
            loaded_data = evaluator.load_data(file_path)
            report = evaluator.evaluate_structured(loaded_data)

            assert report.record_count == 5
            assert report.overall_score > 90  # 高质量数据

    def test_json_file_evaluation(self):
        """测试JSON文件评估。"""
        with tempfile.TemporaryDirectory() as tmpdir:
            # 创建测试文件
            original_data = pd.DataFrame({
                "id": [1, 2, 3],
                "name": ["Alice", None, "Charlie"],
                "value": [100, 200, None],
            })

            file_path = Path(tmpdir) / "test.json"
            original_data.to_json(file_path, orient="records")

            # 评估
            evaluator = DataQualityEvaluator()
            report = evaluator.evaluate_structured(file_path)

            assert report.record_count == 3
            # 应该检测到缺失值
            assert report.overall_score < 100

    def test_multiple_file_batch_evaluation(self):
        """测试多文件批量评估。"""
        with tempfile.TemporaryDirectory() as tmpdir:
            # 创建多个测试文件
            evaluator = DataQualityEvaluator()
            results = []

            for i in range(5):
                data = pd.DataFrame({
                    "id": range(10),
                    "value": [j * 10 for j in range(10)],
                    "category": ["A", "B", "C"] * 3 + ["A"],
                })
                file_path = Path(tmpdir) / f"data_{i}.csv"
                data.to_csv(file_path, index=False)

                report = evaluator.evaluate_structured(file_path)
                results.append({
                    "file": f"data_{i}.csv",
                    "score": report.overall_score,
                    "record_count": report.record_count,
                })

            assert len(results) == 5
            assert all(r["record_count"] == 10 for r in results)


class TestEndToEndPerformance:
    """端到端性能测试。"""

    def test_large_dataset_performance(self):
        """测试大数据集性能。"""
        import time

        # 创建大数据集
        large_data = pd.DataFrame({
            "id": range(10000),
            "value": [i % 1000 for i in range(10000)],
            "category": ["A", "B", "C", "D", "E"] * 2000,
        })

        evaluator = DataQualityEvaluator()

        start_time = time.time()
        report = evaluator.evaluate_structured(large_data)
        duration = time.time() - start_time

        assert report.record_count == 10000
        # 应该在合理时间内完成
        assert duration < 10  # 10秒内完成

    def test_batch_text_evaluation_performance(self):
        """测试批量文本评估性能。"""
        import time

        texts = [
            "这是一段关于人工智能和机器学习的文本。" * 10
            for _ in range(100)
        ]

        evaluator = DataQualityEvaluator()

        start_time = time.time()
        for text in texts:
            evaluator.evaluate_unstructured(text)
        duration = time.time() - start_time

        # 应该在合理时间内完成
        assert duration < 30  # 30秒内完成
