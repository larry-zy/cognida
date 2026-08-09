"""规则引擎测试。

锁定「声明式可插拔规则引擎」的核心契约：
- 注册表：@register_rule 注册、单例实例、按维度分桶（新增规则 = 一个类 + 一行装饰器，
  无需改动引擎/维度核心）；
- 打分口径：通用比例、completeness 单元格、uniqueness 不截断、score_override 钉死、
  counts_toward_score 排除注解；
- 编排：assemble_report 的维度选择、加权汇总、按严重级别排序、metadata。
"""

import pandas as pd
import pytest

# 导入维度包以触发内置维度评估器 + 内置规则的注册（副作用）
import services.quality.dimensions  # noqa: F401
from services.quality.models import DimensionRule, FieldRule, FieldType, SeverityLevel
from services.quality.rules.engine import (
    Rule,
    RuleRegistry,
    RuleResult,
    get_engine,
    register_rule,
)


@pytest.fixture
def temp_registry():
    """快照并在测试后还原全局注册表，避免自定义规则污染内置规则。"""
    rules = dict(RuleRegistry._rules)
    by_dim = {k: list(v) for k, v in RuleRegistry._by_dimension.items()}
    instances = dict(RuleRegistry._instances)
    try:
        yield
    finally:
        RuleRegistry._rules = rules
        RuleRegistry._by_dimension = by_dim
        RuleRegistry._instances = instances


class TestRuleRegistry:
    """规则注册表测试。"""

    def test_register_custom_rule_extends_without_core_change(self, temp_registry):
        """新增一条规则 = 一个类 + 一行装饰器，引擎无需改动即可发现并执行。"""

        @register_rule("always_flag")
        class AlwaysFlagRule(Rule):
            dimension = "custom_dim"

            def run(self, data, field_rules, config):
                return [
                    RuleResult(
                        field=None,
                        count=len(data),
                        severity=SeverityLevel.WARNING,
                        description="flag all",
                    )
                ]

        assert "always_flag" in RuleRegistry.list_rules()
        assert "custom_dim" in RuleRegistry.list_dimensions()

        inst = RuleRegistry.get_instance("always_flag")
        assert inst is not None
        assert inst.name == "always_flag"
        # 单例
        assert RuleRegistry.get_instance("always_flag") is inst
        assert RuleRegistry.rules_for_dimension("custom_dim") == [inst]

        data = pd.DataFrame({"a": [1, 2, 3, 4]})
        score = get_engine().evaluate_dimension("custom_dim", data)
        assert score.name == "custom_dim"
        assert score.score == 0.0  # 4 命中 / 4 记录
        assert len(score.issues) == 1
        assert score.issues[0].dimension == "custom_dim"

    def test_get_instance_unknown_returns_none(self):
        """未注册的规则名返回 None。"""
        assert RuleRegistry.get_instance("__not_a_rule__") is None

    def test_builtin_rules_registered_by_dimension(self):
        """内置规则应按维度分桶注册。"""
        assert "range" in RuleRegistry._by_dimension["accuracy"]
        assert "pattern" in RuleRegistry._by_dimension["accuracy"]
        assert "missing_scan" in RuleRegistry._by_dimension["completeness"]
        assert "staleness" in RuleRegistry._by_dimension["timeliness"]


class TestScoring:
    """打分口径测试。"""

    def test_generic_ratio_score(self, temp_registry):
        """通用维度：score = 100 * (1 - 命中/记录数)。"""

        @register_rule("half_bad")
        class HalfBad(Rule):
            dimension = "ratio_dim"

            def run(self, data, field_rules, config):
                return [RuleResult("a", 2, SeverityLevel.WARNING, "bad")]

        score = get_engine().evaluate_dimension(
            "ratio_dim", pd.DataFrame({"a": [1, 2, 3, 4]})
        )
        assert score.score == pytest.approx(50.0)  # 100 * (1 - 2/4)

    def test_score_override_pins_dimension_score(self, temp_registry):
        """score_override 直接钉死维度分数（用于 timeliness 特判）。"""

        @register_rule("pin_50")
        class Pin50(Rule):
            dimension = "pin_dim"

            def run(self, data, field_rules, config):
                return [
                    RuleResult(
                        None, 0, SeverityLevel.INFO, "pinned", score_override=50.0
                    )
                ]

        score = get_engine().evaluate_dimension("pin_dim", pd.DataFrame({"a": [1, 2]}))
        assert score.score == 50.0

    def test_non_counting_annotation_excluded(self, temp_registry):
        """counts_toward_score=False 的结果不参与扣分。"""

        @register_rule("annotation_only")
        class AnnotationOnly(Rule):
            dimension = "anno_dim"

            def run(self, data, field_rules, config):
                return [
                    RuleResult(
                        "a", 5, SeverityLevel.INFO, "info", counts_toward_score=False
                    )
                ]

        score = get_engine().evaluate_dimension(
            "anno_dim", pd.DataFrame({"a": [1, 2, 3, 4, 5]})
        )
        assert score.score == 100.0
        assert len(score.issues) == 1

    def test_completeness_uses_cell_ratio(self):
        """completeness 按缺失单元格 / 总单元格计分，与规则命中解耦。"""
        data = pd.DataFrame({"a": [1, None], "b": [None, None]})  # 3/4 缺失
        score = get_engine().evaluate_dimension("completeness", data, [])
        assert score.score == pytest.approx(25.0)

    def test_uniqueness_score_not_clamped(self):
        """uniqueness 沿用旧口径不截断，可为负。"""
        data = pd.DataFrame({"id": [1, 1], "v": ["a", "a"]})
        rules = [FieldRule(name="id", type=FieldType.INTEGER, unique=True)]
        score = get_engine().evaluate_dimension("uniqueness", data, rules)
        # 唯一字段重复 2 + 完全重复行 2 = 4，记录 2 → 100 * (1 - 4/2) = -100
        assert score.score == -100.0

    def test_empty_data_generic_zero(self, temp_registry):
        """空数据的通用维度返回 0 分。"""

        @register_rule("noop")
        class Noop(Rule):
            dimension = "empty_dim"

            def run(self, data, field_rules, config):
                return []

        score = get_engine().evaluate_dimension("empty_dim", pd.DataFrame({"a": []}))
        assert score.score == 0.0


class TestAssembleReport:
    """多维度编排测试。"""

    @pytest.fixture
    def data(self):
        return pd.DataFrame(
            {
                "id": [1, 2, 3],
                "name": ["a", None, "c"],  # 必填空值 → CRITICAL
                "age": [25, 200, 30],  # 200 越界 → WARNING
                "note": ["x", None, "y"],  # 低缺失率 → INFO
            }
        )

    @pytest.fixture
    def field_rules(self):
        return [
            FieldRule(name="name", type=FieldType.STRING, required=True),
            FieldRule(name="age", type=FieldType.INTEGER, min_value=0, max_value=120),
        ]

    def test_dimension_selection_and_metadata(self, data, field_rules):
        """只评估 enabled 维度，metadata 记录已评估维度数。"""
        metas = [
            DimensionRule(name="completeness", enabled=True, weight=2.0),
            DimensionRule(name="accuracy", enabled=True, weight=1.0),
            DimensionRule(name="uniqueness", enabled=False, weight=1.0),
        ]
        report = get_engine().assemble_report(data, field_rules, metas)

        assert report.metadata["dimensions_evaluated"] == 2
        assert {d.name for d in report.dimensions} == {"completeness", "accuracy"}
        assert report.record_count == 3

    def test_weighted_overall(self, data, field_rules):
        """总体分数按维度权重加权平均。"""
        metas = [
            DimensionRule(name="completeness", enabled=True, weight=2.0),
            DimensionRule(name="accuracy", enabled=True, weight=1.0),
        ]
        report = get_engine().assemble_report(data, field_rules, metas)
        by_name = {d.name: d.score for d in report.dimensions}
        expected = (by_name["completeness"] * 2.0 + by_name["accuracy"] * 1.0) / 3.0
        assert report.overall_score == pytest.approx(expected)

    def test_issues_sorted_by_severity(self, data, field_rules):
        """问题按 critical → warning → info 排序。"""
        metas = [
            DimensionRule(name="completeness", enabled=True, weight=1.0),
            DimensionRule(name="accuracy", enabled=True, weight=1.0),
        ]
        report = get_engine().assemble_report(data, field_rules, metas)

        priority = {"critical": 1, "warning": 2, "info": 3}
        seq = [priority[i.severity.value] for i in report.issues]
        assert seq == sorted(seq)
        # 三种级别都应出现
        assert set(seq) == {1, 2, 3}

    def test_explicit_dimensions_override(self, data, field_rules):
        """显式传入 dimensions 时忽略 enabled，只评估指定维度。"""
        metas = [
            DimensionRule(name="completeness", enabled=False, weight=1.0),
            DimensionRule(name="accuracy", enabled=True, weight=1.0),
        ]
        report = get_engine().assemble_report(
            data, field_rules, metas, dimensions=["completeness"]
        )
        assert [d.name for d in report.dimensions] == ["completeness"]
