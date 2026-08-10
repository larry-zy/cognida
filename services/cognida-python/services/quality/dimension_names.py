"""质量评估维度的单一真源（Go↔Python 跨服务 wire 契约）。

本模块是「质量评估维度」这一跨服务字符串契约在 Python 侧的**唯一真源**：
servicer 的维度清单、registry 装饰器 key、evaluator 的字符串分派、
rules/builtins 的 ``dimension`` 归属，一律从这里引用，杜绝各处硬编码字面量
各自漂移。

wire 值（枚举成员的字符串值）必须与 Go 侧
``services/cognida-go/internal/service/quality/dimensions.go`` 的 ``Dimension``
常量集逐一对应，二者互为跨语言锚点——任一侧新增/改名/删除维度都必须同步
另一侧，并由双侧锁定测试守护（Python: ``tests/quality/test_dimension_names.py``，
Go: ``dimensions_test.go``）。**切勿改动既有 wire 值**（向后兼容）。
"""

from __future__ import annotations

from enum import StrEnum


class Dimension(StrEnum):
    """质量评估维度（结构化 6 + 非结构化 6）。

    继承 :class:`enum.StrEnum`：成员既是枚举又是 ``str``，故
    ``Dimension.COMPLETENESS == "completeness"`` 为真、``str(Dimension.COMPLETENESS)``
    即 ``"completeness"``，可无缝用于比较；作 dict key / 注册键时统一取 ``.value``
    以保持「纯字符串」字面结果不变。
    """

    # ---- 结构化维度 ----
    COMPLETENESS = "completeness"
    ACCURACY = "accuracy"
    CONSISTENCY = "consistency"
    VALIDITY = "validity"
    UNIQUENESS = "uniqueness"
    TIMELINESS = "timeliness"

    # ---- 非结构化维度 ----
    READABILITY = "readability"
    INFORMATION_DENSITY = "information_density"
    LANGUAGE_QUALITY = "language_quality"
    DUPLICATION = "duplication"
    PII_DETECTOR = "pii_detector"
    RELEVANCE = "relevance"


#: 结构化数据评估维度（有序，供 ListDimensions 保持稳定展示顺序）
STRUCTURED_DIMENSIONS: tuple[Dimension, ...] = (
    Dimension.COMPLETENESS,
    Dimension.ACCURACY,
    Dimension.CONSISTENCY,
    Dimension.VALIDITY,
    Dimension.UNIQUENESS,
    Dimension.TIMELINESS,
)

#: 非结构化文本评估维度（有序）
UNSTRUCTURED_DIMENSIONS: tuple[Dimension, ...] = (
    Dimension.READABILITY,
    Dimension.INFORMATION_DENSITY,
    Dimension.LANGUAGE_QUALITY,
    Dimension.DUPLICATION,
    Dimension.PII_DETECTOR,
    Dimension.RELEVANCE,
)


__all__ = [
    "Dimension",
    "STRUCTURED_DIMENSIONS",
    "UNSTRUCTURED_DIMENSIONS",
]
