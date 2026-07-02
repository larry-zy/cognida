"""结构化数据评估器基类。"""

from abc import ABC, abstractmethod
from typing import Any

import pandas as pd

from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel


class DimensionEvaluator(ABC):
    """结构化数据维度评估器抽象基类。

    每个维度评估器负责评估数据质量的某一个维度。
    """

    dimension_name: str = ""
    description: str = ""

    @abstractmethod
    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据质量。

        Args:
            data: 要评估的数据
            rules: 字段规则列表
            config: 额外配置参数

        Returns:
            维度评分结果
        """

    def _create_issue(
        self,
        severity: SeverityLevel,
        field: str | None,
        description: str,
        count: int,
        sample: list[dict[str, Any]] | None = None,
    ) -> QualityIssue:
        """创建质量问题。

        Args:
            severity: 严重级别
            field: 相关字段
            description: 问题描述
            count: 影响的记录数
            sample: 样本数据

        Returns:
            质量问题对象
        """
        return QualityIssue(
            severity=severity,
            dimension=self.dimension_name,
            field=field,
            description=description,
            count=count,
            sample=sample or [],
        )

    def _get_sample_rows(
        self, data: pd.DataFrame, indices: list[int], max_samples: int = 5
    ) -> list[dict[str, Any]]:
        """获取样本行。

        Args:
            data: 数据框
            indices: 行索引列表
            max_samples: 最大样本数

        Returns:
            样本数据列表
        """
        if not indices:
            return []
        sample_indices = indices[:max_samples]
        return data.iloc[sample_indices].to_dict("records")

    def _calculate_score(
        self, total_issues: int, total_records: int, base_score: float = 100.0
    ) -> float:
        """计算维度分数。

        Args:
            total_issues: 问题总数
            total_records: 总记录数
            base_score: 基础分数

        Returns:
            维度分数 (0-100)
        """
        if total_records == 0:
            return 0.0
        issue_ratio = total_issues / total_records
        return max(0.0, base_score * (1 - issue_ratio))

    def is_passed(self, score: float, threshold: float) -> bool:
        """判断是否通过阈值。

        Args:
            score: 维度分数
            threshold: 阈值

        Returns:
            是否通过
        """
        # config 经 proto map<string,string> 传入时 threshold 可能为字符串,
        # 统一转为 float 后再比较, 避免 float 与 str 的比较报错。
        try:
            threshold = float(threshold)
        except (TypeError, ValueError):
            threshold = 80.0
        return score >= threshold
