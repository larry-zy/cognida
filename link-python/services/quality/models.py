"""数据质量服务核心数据模型。

定义质量评估、清洗和流程编排中使用的所有数据类。
"""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class SeverityLevel(str, Enum):
    """质量问题严重级别。"""

    CRITICAL = "critical"
    WARNING = "warning"
    INFO = "info"


class Decision(str, Enum):
    """质量决策。"""

    ACCEPT = "accept"  # 自动接受
    REJECT = "reject"  # 自动拒绝
    REVIEW = "review"  # 人工审核


@dataclass(frozen=True)
class QualityIssue:
    """质量问题。

    Attributes:
        severity: 严重级别
        dimension: 问题所属维度
        field: 相关字段
        description: 问题描述
        count: 影响的记录数
        sample: 样本数据（最多5条）
    """

    severity: SeverityLevel
    dimension: str
    field: str | None
    description: str
    count: int
    sample: list[dict[str, Any]] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "severity": self.severity.value,
            "dimension": self.dimension,
            "field": self.field,
            "description": self.description,
            "count": self.count,
            "sample": self.sample,
        }


@dataclass(frozen=True)
class DimensionScore:
    """维度评分。

    Attributes:
        name: 维度名称
        score: 分数 (0-100)
        passed: 是否通过阈值
        issues: 相关质量问题列表
    """

    name: str
    score: float
    passed: bool
    issues: list[QualityIssue] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "name": self.name,
            "score": self.score,
            "passed": self.passed,
            "issues": [issue.to_dict() for issue in self.issues],
        }


@dataclass(frozen=True)
class QualityReport:
    """结构化数据质量报告。

    Attributes:
        overall_score: 总体质量分数 (0-100)
        dimensions: 各维度评分
        issues: 所有质量问题（按严重级别排序）
        record_count: 评估的记录数
        timestamp: 评估时间戳
        metadata: 额外元数据
    """

    overall_score: float
    dimensions: list[DimensionScore]
    issues: list[QualityIssue]
    record_count: int
    timestamp: datetime = field(default_factory=datetime.now)
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "overall_score": self.overall_score,
            "dimensions": [d.to_dict() for d in self.dimensions],
            "issues": [issue.to_dict() for issue in self.issues],
            "record_count": self.record_count,
            "timestamp": self.timestamp.isoformat(),
            "metadata": self.metadata,
        }

    def get_issues_by_severity(self, severity: SeverityLevel) -> list[QualityIssue]:
        """按严重级别获取问题。"""
        return [issue for issue in self.issues if issue.severity == severity]

    def get_critical_issues(self) -> list[QualityIssue]:
        """获取所有严重问题。"""
        return self.get_issues_by_severity(SeverityLevel.CRITICAL)


@dataclass(frozen=True)
class TextQualityIssue:
    """非结构化数据质量问题。

    Attributes:
        type: 问题类型
        severity: 严重级别
        description: 问题描述
        position: 问题位置（可选）
        snippet: 问题文本片段
        suggestion: 修复建议（可选）
    """

    type: str
    severity: SeverityLevel
    description: str
    position: tuple[int, int] | None = None
    snippet: str | None = None
    suggestion: str | None = None


@dataclass(frozen=True)
class UnstructuredDimensionScore:
    """非结构化数据维度评分。

    Attributes:
        name: 维度名称
        score: 分数 (0-100)
        passed: 是否通过阈值
        issues: 相关质量问题列表
        details: 额外详情（如语言、编码等）
    """

    name: str
    score: float
    passed: bool
    issues: list[TextQualityIssue] = field(default_factory=list)
    details: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class UnstructuredQualityReport:
    """非结构化数据质量报告。

    Attributes:
        overall_score: 总体质量分数 (0-100)
        dimensions: 各维度评分
        text_stats: 文本统计信息
        issues: 所有质量问题
        timestamp: 评估时间戳
    """

    overall_score: float
    dimensions: list[UnstructuredDimensionScore]
    text_stats: dict[str, Any]
    issues: list[TextQualityIssue]
    timestamp: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "overall_score": self.overall_score,
            "dimensions": [
                {
                    "name": d.name,
                    "score": d.score,
                    "passed": d.passed,
                    "issues": [
                        {
                            "type": i.type,
                            "severity": i.severity.value,
                            "description": i.description,
                            "position": i.position,
                            "snippet": i.snippet,
                            "suggestion": i.suggestion,
                        }
                        for i in d.issues
                    ],
                    "details": d.details,
                }
                for d in self.dimensions
            ],
            "text_stats": self.text_stats,
            "issues": [
                {
                    "type": i.type,
                    "severity": i.severity.value,
                    "description": i.description,
                    "position": i.position,
                    "snippet": i.snippet,
                    "suggestion": i.suggestion,
                }
                for i in self.issues
            ],
            "timestamp": self.timestamp.isoformat(),
        }


@dataclass(frozen=True)
class CleaningOperation:
    """清洗操作记录。

    Attributes:
        type: 操作类型
        field: 相关字段
        count: 影响的记录数
        description: 操作描述
    """

    type: str
    field: str | None
    count: int
    description: str


@dataclass(frozen=True)
class CleaningResult:
    """数据清洗结果。

    Attributes:
        original_count: 原始记录数
        cleaned_count: 清洗后记录数
        removed_count: 移除的记录数
        operations: 应用的清洗操作列表
        metadata: 额外元数据
    """

    original_count: int
    cleaned_count: int
    removed_count: int
    operations: list[CleaningOperation]
    metadata: dict[str, Any] = field(default_factory=dict)
    timestamp: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "original_count": self.original_count,
            "cleaned_count": self.cleaned_count,
            "removed_count": self.removed_count,
            "operations": [
                {
                    "type": op.type,
                    "field": op.field,
                    "count": op.count,
                    "description": op.description,
                }
                for op in self.operations
            ],
            "metadata": self.metadata,
            "timestamp": self.timestamp.isoformat(),
        }


@dataclass(frozen=True)
class PipelineResult:
    """端到端流程执行结果。

    Attributes:
        decision: 最终决策
        quick_score: 快速评估分数
        deep_score: 深度评估分数
        cleaning_result: 清洗结果
        quality_report: 质量报告
        stage_times: 各阶段耗时（毫秒）
        metadata: 额外元数据
    """

    decision: Decision
    quick_score: float | None
    deep_score: float
    cleaning_result: CleaningResult | None
    quality_report: QualityReport | UnstructuredQualityReport
    stage_times: dict[str, float]
    metadata: dict[str, Any] = field(default_factory=dict)
    timestamp: datetime = field(default_factory=datetime.now)

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "decision": self.decision.value,
            "quick_score": self.quick_score,
            "deep_score": self.deep_score,
            "cleaning_result": (
                self.cleaning_result.to_dict() if self.cleaning_result else None
            ),
            "quality_report": self.quality_report.to_dict(),
            "stage_times": self.stage_times,
            "metadata": self.metadata,
            "timestamp": self.timestamp.isoformat(),
        }


@dataclass(frozen=True)
class DriftMetric:
    """数据漂移指标。

    Attributes:
        field: 字段名
        drift_score: 漂移分数 (0-1)
        p_value: 统计检验p值
        threshold: 阈值
        drifted: 是否发生漂移
    """

    field: str
    drift_score: float
    p_value: float
    threshold: float
    drifted: bool


@dataclass(frozen=True)
class DriftResult:
    """数据漂移检测结果。

    Attributes:
        has_drift: 是否检测到漂移
        drift_metrics: 各字段漂移指标
        overall_drift_score: 总体漂移分数
        baseline_timestamp: 基线数据时间戳
        current_timestamp: 当前数据时间戳
    """

    has_drift: bool
    drift_metrics: list[DriftMetric]
    overall_drift_score: float
    baseline_timestamp: datetime
    current_timestamp: datetime

    def to_dict(self) -> dict[str, Any]:
        """转换为字典。"""
        return {
            "has_drift": self.has_drift,
            "drift_metrics": [
                {
                    "field": m.field,
                    "drift_score": m.drift_score,
                    "p_value": m.p_value,
                    "threshold": m.threshold,
                    "drifted": m.drifted,
                }
                for m in self.drift_metrics
            ],
            "overall_drift_score": self.overall_drift_score,
            "baseline_timestamp": self.baseline_timestamp.isoformat(),
            "current_timestamp": self.current_timestamp.isoformat(),
        }

    def get_summary(self) -> dict[str, Any]:
        """获取漂移检测摘要。

        Returns:
            摘要字典
        """
        return {
            "overall_drift_score": self.overall_drift_score,
            "drifted_field_count": sum(1 for m in self.drift_metrics if m.drifted),
            "total_field_count": len(self.drift_metrics),
            "has_drift": self.has_drift,
        }

    def get_details(self) -> dict[str, Any]:
        """获取漂移检测详细信息。

        Returns:
            详细信息字典
        """
        return {
            "has_drift": self.has_drift,
            "drift_metrics": [
                {
                    "field": m.field,
                    "drift_score": m.drift_score,
                    "p_value": m.p_value,
                    "threshold": m.threshold,
                    "drifted": m.drifted,
                }
                for m in self.drift_metrics
            ],
            "overall_drift_score": self.overall_drift_score,
            "baseline_timestamp": self.baseline_timestamp.isoformat(),
            "current_timestamp": self.current_timestamp.isoformat(),
            "drifted_fields": [m.field for m in self.drift_metrics if m.drifted],
        }


# 规则配置相关数据类


class FieldType(str, Enum):
    """字段类型。"""

    STRING = "string"
    INTEGER = "integer"
    FLOAT = "float"
    BOOLEAN = "boolean"
    DATE = "date"
    DATETIME = "datetime"
    ARRAY = "array"


@dataclass(frozen=True)
class FieldRule:
    """字段规则。

    Attributes:
        name: 字段名
        type: 字段类型
        required: 是否必填
        unique: 是否唯一
        pattern: 正则表达式模式
        min_value: 最小值
        max_value: 最大值
        min_length: 最小长度
        max_length: 最大长度
        allowed_values: 允许的值列表
        description: 字段描述
    """

    name: str
    type: FieldType
    required: bool = False
    unique: bool = False
    pattern: str | None = None
    min_value: float | None = None
    max_value: float | None = None
    min_length: int | None = None
    max_length: int | None = None
    allowed_values: list[Any] | None = None
    description: str = ""


@dataclass(frozen=True)
class DimensionRule:
    """维度规则配置。

    Attributes:
        name: 维度名称
        enabled: 是否启用
        threshold: 通过阈值
        weight: 权重（用于计算总体分数）
        config: 额外配置
    """

    name: str
    enabled: bool = True
    threshold: float = 70.0
    weight: float = 1.0
    config: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class QualityRules:
    """质量规则配置。

    Attributes:
        version: 规则版本
        field_rules: 字段规则列表
        dimension_rules: 维度规则列表
        cleaning_rules: 清洗规则
        metadata: 元数据
    """

    version: str = "1.0"
    field_rules: list[FieldRule] = field(default_factory=list)
    dimension_rules: list[DimensionRule] = field(default_factory=list)
    cleaning_rules: dict[str, Any] = field(default_factory=dict)
    metadata: dict[str, Any] = field(default_factory=dict)


# Pydantic 模型（用于 gRPC 接口转换）


class QualityIssueModel(BaseModel):
    """质量问题 Pydantic 模型。"""

    severity: SeverityLevel
    dimension: str
    field: str | None = None
    description: str
    count: int = 0
    sample: list[dict[str, Any]] = Field(default_factory=list)


class DimensionScoreModel(BaseModel):
    """维度评分 Pydantic 模型。"""

    name: str
    score: float
    passed: bool
    issues: list[QualityIssueModel] = Field(default_factory=list)


class QualityReportModel(BaseModel):
    """质量报告 Pydantic 模型。"""

    overall_score: float
    dimensions: list[DimensionScoreModel]
    issues: list[QualityIssueModel] = Field(default_factory=list)
    record_count: int
    timestamp: datetime = Field(default_factory=datetime.now)
    metadata: dict[str, Any] = Field(default_factory=dict)


class CleaningOperationModel(BaseModel):
    """清洗操作 Pydantic 模型。"""

    type: str
    field: str | None = None
    count: int
    description: str


class CleaningResultModel(BaseModel):
    """清洗结果 Pydantic 模型。"""

    original_count: int
    cleaned_count: int
    removed_count: int
    operations: list[CleaningOperationModel] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)
    timestamp: datetime = Field(default_factory=datetime.now)


class PipelineResultModel(BaseModel):
    """流程结果 Pydantic 模型。"""

    decision: Decision
    quick_score: float | None = None
    deep_score: float
    cleaning_result: CleaningResultModel | None = None
    quality_report: QualityReportModel
    stage_times: dict[str, float] = Field(default_factory=dict)
    metadata: dict[str, Any] = Field(default_factory=dict)
    timestamp: datetime = Field(default_factory=datetime.now)
