"""质量维度契约锁定测试（防跨服务字符串契约漂移）。

「质量评估维度」是 Go↔Python 的跨服务 wire 契约。本测试把维度取值钉死为一份
显式期望字面量，作为 **跨语言锚点**——Go 侧
``services/cognida-go/internal/service/quality/dimensions_test.go`` 里断言的是
**同一份** 期望字面量。任一侧改动 wire 值都会触发对应侧测试失败，从而强制两侧同步。

同时校验：单一真源枚举 ⇔ registry 实际注册键 ⇔ 规则维度归属 ⇔ servicer 维度清单
四者一致，杜绝各处硬编码各自漂移。
"""

from services.quality.dimension_names import (
    Dimension,
    STRUCTURED_DIMENSIONS,
    UNSTRUCTURED_DIMENSIONS,
)

# 跨语言锚点：与 Go 侧 dimensions_test.go 的期望字面量逐字对应，切勿改动。
EXPECTED_STRUCTURED = {
    "completeness",
    "accuracy",
    "consistency",
    "validity",
    "uniqueness",
    "timeliness",
}
EXPECTED_UNSTRUCTURED = {
    "readability",
    "information_density",
    "language_quality",
    "duplication",
    "pii_detector",
    "relevance",
}
EXPECTED_ALL = EXPECTED_STRUCTURED | EXPECTED_UNSTRUCTURED


class TestDimensionEnumContract:
    """枚举取值集合 == 期望 12 字面量。"""

    def test_all_dimension_values(self):
        assert {d.value for d in Dimension} == EXPECTED_ALL

    def test_structured_grouping(self):
        assert {d.value for d in STRUCTURED_DIMENSIONS} == EXPECTED_STRUCTURED

    def test_unstructured_grouping(self):
        assert {d.value for d in UNSTRUCTURED_DIMENSIONS} == EXPECTED_UNSTRUCTURED

    def test_groups_partition_all(self):
        # 两组不相交且并集恰为全体，杜绝遗漏/重复归类
        struct = set(STRUCTURED_DIMENSIONS)
        unstruct = set(UNSTRUCTURED_DIMENSIONS)
        assert struct.isdisjoint(unstruct)
        assert struct | unstruct == set(Dimension)

    def test_strenum_wire_value_identity(self):
        # StrEnum：成员按值等于其字符串 wire 值（比较/dict key 语义不变）
        assert Dimension.COMPLETENESS == "completeness"
        assert str(Dimension.PII_DETECTOR) == "pii_detector"


class TestRegistryMatchesEnum:
    """registry 已注册的评估器键集合 == 枚举取值。"""

    def test_registered_evaluators_equal_enum(self):
        # 触发内置结构化 / 非结构化评估器的 @register_evaluator 副作用注册
        import services.quality.dimensions  # noqa: F401
        import services.quality.unstructured  # noqa: F401
        from services.quality.registry import EvaluatorRegistry

        registered = set(EvaluatorRegistry.list_evaluators())
        enum_values = {d.value for d in Dimension}
        # 内置维度全部注册；允许运行时另有自定义评估器（故用 ⊆ + 覆盖断言）
        assert enum_values <= registered
        builtin_registered = registered & enum_values
        assert builtin_registered == enum_values
        # 注册键必须是纯字符串（装饰器传 .value），保持 wire 字面结果不变
        assert all(type(k) is str for k in EvaluatorRegistry._evaluators)

    def test_rule_dimensions_subset_of_enum(self):
        # rules/builtins 里各规则声明的归属维度 ⊆ 结构化维度枚举
        import services.quality.rules.builtins  # noqa: F401
        from services.quality.rules.engine import RuleRegistry

        rule_dims = set(RuleRegistry.list_dimensions())
        assert rule_dims <= EXPECTED_STRUCTURED
        assert all(type(d) is str for d in rule_dims)


class TestServicerListDimensions:
    """servicer.ListDimensions 输出 == 枚举，且结构化/非结构化归属正确。"""

    def test_list_dimensions_matches_enum(self):
        from proto import quality_pb2
        from services.quality.servicer import QualityServicer

        servicer = QualityServicer()
        resp = servicer.ListDimensions(quality_pb2.ListDimensionsRequest(), None)

        assert resp.success
        names = {d.name for d in resp.dimensions}
        assert names == EXPECTED_ALL

        structured = {d.name for d in resp.dimensions if d.supports_structured}
        unstructured = {d.name for d in resp.dimensions if d.supports_unstructured}
        assert structured == EXPECTED_STRUCTURED
        assert unstructured == EXPECTED_UNSTRUCTURED
