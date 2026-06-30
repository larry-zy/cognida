"""数据漂移检测器。

基于统计检验检测数据分布变化。"""

from datetime import datetime, timezone
from typing import Any

import numpy as np
import pandas as pd
from scipy import stats

from .models import DriftMetric, DriftResult, SeverityLevel, QualityIssue


class DriftDetector:
    """数据漂移检测器。

    使用统计检验方法检测数据分布漂移。
    """

    def __init__(self, threshold: float = 0.05) -> None:
        """初始化漂移检测器。

        Args:
            threshold: p值阈值，低于此值认为发生漂移
        """
        self.threshold = threshold
        self._baseline_stats: dict[str, Any] | None = None
        self._baseline_timestamp: datetime | None = None

    def set_baseline(self, data: pd.DataFrame, columns: list[str] | None = None) -> None:
        """设置基线数据统计。

        Args:
            data: 基线数据
            columns: 要统计的列名列表
        """
        self._baseline_timestamp = datetime.now(timezone.utc)
        self._baseline_stats = {}

        if columns is None:
            columns = data.select_dtypes(include=[np.number]).columns.tolist()

        for col in columns:
            if col in data.columns:
                col_data = data[col].dropna()
                if len(col_data) > 0:
                    self._baseline_stats[col] = {
                        "mean": col_data.mean(),
                        "std": col_data.std(),
                        "min": col_data.min(),
                        "max": col_data.max(),
                        "q25": col_data.quantile(0.25),
                        "q50": col_data.quantile(0.50),
                        "q75": col_data.quantile(0.75),
                        "count": len(col_data),
                    }

    @property
    def baseline_stats(self) -> dict[str, Any] | None:
        """获取基线统计信息。"""
        return self._baseline_stats

    @property
    def baseline_timestamp(self) -> datetime | None:
        """获取基线时间戳。"""
        return self._baseline_timestamp

    def detect_drift(
        self,
        data: pd.DataFrame,
        columns: list[str] | None = None,
    ) -> DriftResult:
        """检测数据漂移。

        Args:
            data: 当前数据
            columns: 要检测的列名列表

        Returns:
            漂移检测结果
        """
        if not self._baseline_stats:
            raise RuntimeError("No baseline data set. Call set_baseline() first.")

        current_timestamp = datetime.now(timezone.utc)
        drift_metrics = []

        if columns is None:
            columns = list(self._baseline_stats.keys())

        for col in columns:
            if col not in data.columns or col not in self._baseline_stats:
                continue

            baseline_stats = self._baseline_stats[col]
            current_data = data[col].dropna()

            if len(current_data) == 0:
                continue

            # 执行KS检验
            ks_statistic, p_value = stats.ks_2samp(
                # 重新采样基线数据（简化处理）
                np.random.normal(
                    baseline_stats["mean"],
                    baseline_stats["std"],
                    min(len(current_data), 1000),
                ),
                current_data.sample(min(len(current_data), 1000), replace=False),
            )

            # 计算漂移分数
            drift_score = 1 - p_value

            has_drift = p_value < self.threshold

            drift_metrics.append(
                DriftMetric(
                    field=col,
                    drift_score=drift_score,
                    p_value=p_value,
                    threshold=self.threshold,
                    drifted=has_drift,
                )
            )

        # 计算总体漂移分数
        overall_drift_score = (
            sum(m.drift_score for m in drift_metrics) / len(drift_metrics)
            if drift_metrics
            else 0.0
        )

        has_drift = any(m.drifted for m in drift_metrics)

        return DriftResult(
            has_drift=has_drift,
            drift_metrics=drift_metrics,
            overall_drift_score=overall_drift_score,
            baseline_timestamp=self._baseline_timestamp,
            current_timestamp=current_timestamp,
        )

    def get_baseline_info(self) -> dict[str, Any]:
        """获取基线信息。

        Returns:
            基线信息字典
        """
        return {
            "timestamp": self._baseline_timestamp.isoformat()
            if self._baseline_timestamp
            else None,
            "columns": list(self._baseline_stats.keys()),
            "statistics": self._baseline_stats,
        }

    def reset_baseline(self) -> None:
        """重置基线数据。"""
        self._baseline_stats = {}
        self._baseline_timestamp = None
