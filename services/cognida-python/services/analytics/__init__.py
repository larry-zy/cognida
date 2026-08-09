"""数据分析服务。

提供数理统计、趋势分析、数据洞察等核心分析能力。
"""

__version__ = "0.1.0"

from .statistics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    HypothesisTest,
    DescribeResult,
    DistributionResult,
    CorrelationResult,
)
from .trend import (
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    GrowthRateAnalyzer,
    TrendResult,
    SeasonalityResult,
)
from .insight import (
    TrendInsightFinder,
    AnomalyInsightFinder,
    CorrelationInsightFinder,
    InsightGenerator,
    Insight,
    InsightType,
    InsightSeverity,
)
from .attribution import (
    AttributionAnalyzer,
    AttributionResult,
    GroupComparison,
    SegmentDriver,
    UNKNOWN_SEGMENT,
)
from .validation import (
    DataValidator,
    TimeSeriesDetector,
    DataChecker,
    DEFAULT_MAX_ROWS,
    DEFAULT_MIN_ROWS,
)

__all__ = [
    # Version
    "__version__",
    # Statistics
    "DescriptiveStats",
    "DistributionAnalysis",
    "CorrelationAnalysis",
    "HypothesisTest",
    "DescribeResult",
    "DistributionResult",
    "CorrelationResult",
    # Trend
    "LinearTrendAnalyzer",
    "MovingAverageAnalyzer",
    "SeasonalityAnalyzer",
    "GrowthRateAnalyzer",
    "TrendResult",
    "SeasonalityResult",
    # Insight
    "TrendInsightFinder",
    "AnomalyInsightFinder",
    "CorrelationInsightFinder",
    "InsightGenerator",
    "Insight",
    "InsightType",
    "InsightSeverity",
    # Attribution
    "AttributionAnalyzer",
    "AttributionResult",
    "GroupComparison",
    "SegmentDriver",
    "UNKNOWN_SEGMENT",
    # Validation
    "DataValidator",
    "TimeSeriesDetector",
    "DataChecker",
    "DEFAULT_MAX_ROWS",
    "DEFAULT_MIN_ROWS",
]
