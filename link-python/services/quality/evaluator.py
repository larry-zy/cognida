"""数据质量评估器总入口。

提供统一的质量评估接口。
"""

from pathlib import Path
from typing import Any

import pandas as pd

from .models import QualityReport, UnstructuredQualityReport
from .registry import EvaluatorRegistry
from .rules import RuleEngine
from .rules.engine import get_engine

# 显式导入内置维度包，触发各维度的 @register_evaluator 装饰器完成注册。
# 结构化维度过去仅因 pipeline.executor 间接 import dimensions.base 才“碰巧”注册，
# 而非结构化维度无人导入 → EvaluatorRegistry 为空 → evaluate_unstructured 得到 0 维度、
# overall_score=0。这里在评估器自身模块内做显式注册，使两条评估路径都不依赖导入顺序。
from . import dimensions as _dimensions  # noqa: F401  # 注册结构化维度（副作用导入）
from . import unstructured as _unstructured  # noqa: F401  # 注册非结构化维度（副作用导入）


class DataQualityEvaluator:
    """数据质量评估器。

    提供结构化和非结构化数据的质量评估能力。
    """

    def __init__(self, config_dir: str | None = None) -> None:
        """初始化评估器。

        Args:
            config_dir: 规则配置目录
        """
        self.rule_engine = RuleEngine(config_dir)

    def load_data(
        self,
        source: str | Path,
        source_type: str | None = None,
        **kwargs: Any,
    ) -> pd.DataFrame:
        """加载数据。

        Args:
            source: 数据源（文件路径或URL）
            source_type: 数据源类型 (csv, json, excel, parquet)
            **kwargs: 额外参数

        Returns:
            加载的数据

        Raises:
            ValueError: 不支持的数据源类型
        """
        source_path = Path(source)

        if source_type is None:
            source_type = source_path.suffix.lstrip(".").lower()

        loaders = {
            "csv": self._load_csv,
            "json": self._load_json,
            "excel": self._load_excel,
            "xlsx": self._load_excel,
            "parquet": self._load_parquet,
        }

        if source_type not in loaders:
            raise ValueError(f"Unsupported source type: {source_type}")

        return loaders[source_type](source_path, **kwargs)

    def _load_csv(self, path: Path, **kwargs: Any) -> pd.DataFrame:
        """加载CSV文件。"""
        return pd.read_csv(path, **kwargs)

    def _load_json(self, path: Path, **kwargs: Any) -> pd.DataFrame:
        """加载JSON文件。"""
        return pd.read_json(path, **kwargs)

    def _load_excel(self, path: Path, **kwargs: Any) -> pd.DataFrame:
        """加载Excel文件。"""
        return pd.read_excel(path, **kwargs)

    def _load_parquet(self, path: Path, **kwargs: Any) -> pd.DataFrame:
        """加载Parquet文件。"""
        return pd.read_parquet(path, **kwargs)

    def evaluate_structured(
        self,
        data: pd.DataFrame | str | Path,
        dimensions: list[str] | None = None,
        rule_file: str | None = None,
        config: dict[str, Any] | None = None,
    ) -> QualityReport:
        """评估结构化数据质量。

        Args:
            data: 数据（DataFrame或文件路径）
            dimensions: 要评估的维度列表，None表示全部
            rule_file: 规则文件名
            config: 额外配置

        Returns:
            质量报告
        """
        config = config or {}

        # 如果是文件路径，加载数据
        if isinstance(data, (str, Path)):
            data = self.load_data(data)

        # 加载规则
        rules = self.rule_engine.load_rules(rule_file)

        # 交由规则引擎统一编排：按维度分桶执行规则、加权汇总、排序问题。
        # 维度选择、维度配置、权重均来自 rules.dimension_rules 元数据。
        return get_engine().assemble_report(
            data=data,
            field_rules=rules.field_rules,
            dimension_metas=rules.dimension_rules,
            config=config,
            dimensions=dimensions,
        )

    def evaluate_unstructured(
        self,
        text: str,
        dimensions: list[str] | None = None,
        rule_file: str | None = None,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredQualityReport:
        """评估非结构化数据质量。

        Args:
            text: 文本内容
            dimensions: 要评估的维度列表，None表示全部
            rule_file: 规则文件名
            config: 额外配置

        Returns:
            质量报告
        """
        config = config or {}

        # 加载规则
        rules = self.rule_engine.load_rules(rule_file)

        # 默认维度列表
        if dimensions is None:
            dimensions = [
                "readability",
                "information_density",
                "language_quality",
                "duplication",
                "pii_detector",
            ]

        # 执行评估
        all_dimensions = []
        all_issues = []
        text_stats = {
            "length": len(text),
            "word_count": len(text.split()),
        }

        for dim_name in dimensions:
            evaluator = EvaluatorRegistry.get_instance(dim_name)
            if evaluator:
                result = evaluator.evaluate(text, config)
                all_dimensions.append(result)
                all_issues.extend(result.issues)
                if result.details:
                    text_stats.update(result.details)

        # 计算总体分数
        overall_score = (
            sum(d.score for d in all_dimensions) / len(all_dimensions)
            if all_dimensions
            else 0
        )

        return UnstructuredQualityReport(
            overall_score=overall_score,
            dimensions=all_dimensions,
            text_stats=text_stats,
            issues=all_issues,
        )

    def get_recommendations(self, report: QualityReport | UnstructuredQualityReport) -> list[str]:
        """生成改进建议。

        Args:
            report: 质量报告

        Returns:
            建议列表
        """
        recommendations = []

        if isinstance(report, QualityReport):
            for dim in report.dimensions:
                if not dim.passed:
                    if dim.name == "completeness":
                        recommendations.append(
                            "建议：检查并补充缺失的字段值，特别是必填字段"
                        )
                    elif dim.name == "accuracy":
                        recommendations.append(
                            "建议：检查数据范围和格式，修正异常值"
                        )
                    elif dim.name == "consistency":
                        recommendations.append(
                            "建议：检查跨字段的逻辑关系，修正不一致的数据"
                        )
                    elif dim.name == "validity":
                        recommendations.append(
                            "建议：检查数据类型和值约束，确保数据格式正确"
                        )
                    elif dim.name == "uniqueness":
                        recommendations.append(
                            "建议：去除重复记录，确保主键唯一性"
                        )
                    elif dim.name == "timeliness":
                        recommendations.append(
                            "建议：更新过期数据，确保数据时效性"
                        )

        elif isinstance(report, UnstructuredQualityReport):
            for dim in report.dimensions:
                if not dim.passed:
                    if dim.name == "readability":
                        recommendations.append(
                            "建议：检查文本编码和格式，修复乱码问题"
                        )
                    elif dim.name == "information_density":
                        recommendations.append(
                            "建议：增加文本内容，提高信息密度"
                        )
                    elif dim.name == "language_quality":
                        recommendations.append(
                            "建议：检查并修正语法和标点符号错误"
                        )
                    elif dim.name == "duplication":
                        recommendations.append(
                            "建议：去除重复内容"
                        )
                    elif dim.name == "pii_detector":
                        recommendations.append(
                            "建议：对敏感信息进行脱敏处理"
                        )

        return recommendations
