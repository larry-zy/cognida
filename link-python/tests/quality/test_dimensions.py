"""结构化数据评估器测试。"""

import pandas as pd
import pytest

from services.quality.dimensions.completeness import CompletenessEvaluator
from services.quality.dimensions.accuracy import AccuracyEvaluator
from services.quality.dimensions.consistency import ConsistencyEvaluator
from services.quality.dimensions.validity import ValidityEvaluator
from services.quality.dimensions.uniqueness import UniquenessEvaluator
from services.quality.dimensions.timeliness import TimelinessEvaluator
from services.quality.models import FieldRule, FieldType, SeverityLevel


@pytest.fixture
def sample_data():
    """创建测试数据。"""
    return pd.DataFrame({
        "id": [1, 2, 3, 4, 5],
        "name": ["Alice", "Bob", None, "David", "Eve"],
        "age": [25, 30, 35, -5, 150],  # -5 和 150 是异常值
        "email": ["alice@example.com", "invalid-email", "charlie@example.com", None, "eve@example.com"],
        "score": [85.5, 90.0, 78.5, 92.0, 88.5],
    })


@pytest.fixture
def field_rules():
    """创建字段规则。"""
    return [
        FieldRule(name="id", type=FieldType.INTEGER, required=True),
        FieldRule(name="name", type=FieldType.STRING, required=True),
        FieldRule(name="age", type=FieldType.INTEGER, min_value=0, max_value=120),
        FieldRule(name="email", type=FieldType.STRING, pattern=r"^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$"),
        FieldRule(name="score", type=FieldType.FLOAT, min_value=0, max_value=100),
    ]


class TestCompletenessEvaluator:
    """完整性评估器测试。"""

    def test_evaluate_completeness(self, sample_data, field_rules):
        """测试完整性评估。"""
        evaluator = CompletenessEvaluator()
        result = evaluator.evaluate(sample_data, field_rules)

        assert result.name == "completeness"
        assert 0 <= result.score <= 100
        assert len(result.issues) > 0

        # 应该检测到 name 和 email 的空值
        critical_issues = [i for i in result.issues if i.severity == SeverityLevel.CRITICAL]
        assert len(critical_issues) > 0

    def test_required_field_detection(self, sample_data):
        """测试必填字段检测。"""
        evaluator = CompletenessEvaluator()
        rules = [
            FieldRule(name="id", type=FieldType.INTEGER, required=True),
            FieldRule(name="name", type=FieldType.STRING, required=True),
        ]
        result = evaluator.evaluate(sample_data, rules)

        # name 有空值
        name_issues = [i for i in result.issues if i.field == "name"]
        assert len(name_issues) == 1
        assert name_issues[0].count == 1  # 一个空值

    def test_high_missing_rate_detection(self):
        """测试高缺失率检测。"""
        data = pd.DataFrame({
            "field1": [1, 2, None, None, None, None],  # 67% 缺失
            "field2": [1, 2, 3, 4, 5, 6],
        })
        evaluator = CompletenessEvaluator()
        result = evaluator.evaluate(data, {"high_missing_threshold": 0.5})

        # field1 应该被标记为高缺失率
        high_missing_issues = [
            i for i in result.issues
            if i.field == "field1" and i.severity == SeverityLevel.WARNING
        ]
        assert len(high_missing_issues) > 0


class TestAccuracyEvaluator:
    """准确性评估器测试。"""

    def test_range_check(self, sample_data):
        """测试范围检查。"""
        evaluator = AccuracyEvaluator()
        rules = [
            FieldRule(name="age", type=FieldType.INTEGER, min_value=0, max_value=120),
        ]
        result = evaluator.evaluate(sample_data, rules)

        # age 有 -5 和 150 两个异常值
        age_issues = [i for i in result.issues if i.field == "age"]
        assert len(age_issues) == 1
        assert age_issues[0].count == 2

    def test_pattern_validation(self, sample_data):
        """测试模式验证。"""
        evaluator = AccuracyEvaluator()
        rules = [
            FieldRule(
                name="email",
                type=FieldType.STRING,
                pattern=r"^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$"
            ),
        ]
        result = evaluator.evaluate(sample_data, rules)

        # 有一个无效邮箱
        email_issues = [i for i in result.issues if i.field == "email"]
        assert len(email_issues) == 1


class TestConsistencyEvaluator:
    """一致性评估器测试。"""

    def test_end_after_start(self):
        """测试结束时间晚于开始时间。"""
        data = pd.DataFrame({
            "start_date": pd.to_datetime(["2024-01-01", "2024-02-01", "2024-03-01"]),
            "end_date": pd.to_datetime(["2024-01-15", "2024-01-20", "2024-03-15"]),  # 第二条记录的 end_date 早于 start_date
        })
        evaluator = ConsistencyEvaluator()
        config = {
            "consistency_rules": [
                {"rule": "end_after_start", "params": {"start_field": "start_date", "end_field": "end_date"}},
            ]
        }
        result = evaluator.evaluate(data, config=config)

        # 应该检测到一条不一致的记录
        assert len(result.issues) > 0

    def test_positive_value_check(self):
        """测试正数值检查。"""
        data = pd.DataFrame({
            "amount": [100, -50, 200, -10, 300],
        })
        evaluator = ConsistencyEvaluator()
        config = {
            "consistency_rules": [
                {"rule": "positive_value", "params": {"field": "amount"}},
            ]
        }
        result = evaluator.evaluate(data, config=config)

        # 应该检测到两个负值
        amount_issues = [i for i in result.issues if i.field == "amount"]
        assert len(amount_issues) == 1
        assert amount_issues[0].count == 2

    def test_enum_consistency(self):
        """测试枚举值一致性。"""
        data = pd.DataFrame({
            "status": ["active", "inactive", "pending", "unknown", "active"],
        })
        evaluator = ConsistencyEvaluator()
        rules = [
            FieldRule(name="status", type=FieldType.STRING, allowed_values=["active", "inactive", "pending"]),
        ]
        result = evaluator.evaluate(data, rules)

        # 应该检测到 "unknown" 不在允许列表中
        status_issues = [i for i in result.issues if i.field == "status"]
        assert len(status_issues) == 1


class TestValidityEvaluator:
    """有效性评估器测试。"""

    def test_type_validation(self):
        """测试类型验证。"""
        data = pd.DataFrame({
            "number": ["1", "2", "abc", "4", "5"],  # "abc" 无法转换为数字
        })
        evaluator = ValidityEvaluator()
        rules = [
            FieldRule(name="number", type=FieldType.INTEGER),
        ]
        result = evaluator.evaluate(data, rules)

        # 应该检测到 "abc" 无法转换
        number_issues = [i for i in result.issues if i.field == "number"]
        assert len(number_issues) == 1

    def test_length_validation(self):
        """测试长度验证。"""
        data = pd.DataFrame({
            "code": ["AB", "ABC", "A", "ABCD", "ABCDE"],  # "A" 太短，"ABCDE" 太长
        })
        evaluator = ValidityEvaluator()
        rules = [
            FieldRule(name="code", type=FieldType.STRING, min_length=2, max_length=4),
        ]
        result = evaluator.evaluate(data, rules)

        # 应该检测到长度问题
        code_issues = [i for i in result.issues if i.field == "code"]
        assert len(code_issues) >= 1


class TestUniquenessEvaluator:
    """唯一性评估器测试。"""

    def test_full_row_duplication(self):
        """测试完全重复行检测。"""
        data = pd.DataFrame({
            "id": [1, 2, 3, 2, 4],
            "value": ["a", "b", "c", "b", "d"],
        })
        evaluator = UniquenessEvaluator()
        result = evaluator.evaluate(data)

        # 应该检测到重复的行
        assert len(result.issues) > 0
        assert result.score < 100

    def test_unique_field_duplication(self):
        """测试唯一字段重复检测。"""
        data = pd.DataFrame({
            "id": [1, 2, 3, 2, 5],  # id=2 重复
            "name": ["Alice", "Bob", "Charlie", "David", "Eve"],
        })
        evaluator = UniquenessEvaluator()
        # 添加自定义 unique 属性
        from services.quality.models import FieldRule, FieldType
        rules = [
            FieldRule(name="id", type=FieldType.INTEGER, unique=True),
        ]

        result = evaluator.evaluate(data, rules)

        # 应该检测到 id 重复
        id_issues = [i for i in result.issues if i.field == "id"]
        assert len(id_issues) > 0


class TestTimelinessEvaluator:
    """时效性评估器测试。"""

    def test_stale_data_detection(self):
        """测试过期数据检测。"""
        from datetime import datetime, timedelta

        now = datetime.now()
        old_date = now - timedelta(days=60)  # 60 天前
        recent_date = now - timedelta(days=10)  # 10 天前

        data = pd.DataFrame({
            "id": [1, 2, 3],
            "timestamp": [old_date, recent_date, None],
        })
        evaluator = TimelinessEvaluator()
        config = {"timestamp_field": "timestamp", "max_age_days": 30}
        result = evaluator.evaluate(data, config=config)

        # 应该检测到过期数据
        assert len(result.issues) > 0

    def test_missing_timestamp(self):
        """测试缺失时间戳。"""
        data = pd.DataFrame({
            "id": [1, 2, 3],
            "value": [100, 200, 300],
        })
        evaluator = TimelinessEvaluator()
        config = {"timestamp_field": "nonexistent_field"}
        result = evaluator.evaluate(data, config=config)

        # 应该返回信息级别的问题
        assert len(result.issues) > 0
        assert result.issues[0].severity == SeverityLevel.INFO
