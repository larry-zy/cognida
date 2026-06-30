"""数理统计模块。

提供描述统计、分布分析、相关性分析、假设检验等统计计算能力。
"""

from dataclasses import dataclass
from enum import Enum
from typing import Any

import numpy as np
import pandas as pd
from scipy import stats


class HttpMethod(Enum):
    """假设检验方法"""
    PEARSON = "pearson"
    SPEARMAN = "spearman"
    KENDALL = "kendall"


@dataclass
class DescribeResult:
    """描述统计结果"""
    count: int
    mean: float
    median: float
    std: float
    min: float
    max: float
    q25: float
    q75: float
    iqr: float
    skewness: float | None = None
    kurtosis: float | None = None


@dataclass
class DistributionResult:
    """分布分析结果"""
    is_normal: bool
    test_statistic: float
    p_value: float
    test_type: str


@dataclass
class CorrelationResult:
    """相关性分析结果"""
    matrix: dict[str, dict[str, float]]
    significant_pairs: list[tuple[str, str, float]]
    method: str


@dataclass
class TTestResult:
    """t 检验结果"""
    statistic: float
    p_value: float
    significant: bool
    equal_var: bool
    levene_p: float


@dataclass
class ChiSquareResult:
    """卡方检验结果"""
    chi2: float
    p_value: float
    dof: int
    significant: bool


class DescriptiveStats:
    """描述统计"""

    @staticmethod
    def describe(series: pd.Series) -> DescribeResult:
        """计算描述统计量

        Args:
            series: 输入数据序列

        Returns:
            DescribeResult: 描述统计结果

        Raises:
            ValueError: 当序列为空或全为 NA 时
        """
        series = series.dropna()

        if len(series) == 0:
            raise ValueError("序列为空或全为 NA 值")

        q25 = float(series.quantile(0.25))
        q75 = float(series.quantile(0.75))

        return DescribeResult(
            count=int(series.count()),
            mean=float(series.mean()),
            median=float(series.median()),
            std=float(series.std()),
            min=float(series.min()),
            max=float(series.max()),
            q25=q25,
            q75=q75,
            iqr=float(q75 - q25),
            skewness=float(series.skew()) if len(series) > 3 else None,
            kurtosis=float(series.kurtosis()) if len(series) > 4 else None,
        )

    @staticmethod
    def describe_df(df: pd.DataFrame) -> dict[str, DescribeResult]:
        """批量计算描述统计

        Args:
            df: 输入数据框

        Returns:
            dict[str, DescribeResult]: 每列的描述统计结果
        """
        results = {}
        for col in df.select_dtypes(include=[np.number]).columns:
            try:
                results[col] = DescriptiveStats.describe(df[col])
            except ValueError:
                # 跳过空列
                continue
        return results


class DistributionAnalysis:
    """分布分析"""

    @staticmethod
    def test_normality(
        series: pd.Series,
        method: str = "auto"
    ) -> DistributionResult:
        """正态性检验

        Args:
            series: 输入数据序列
            method: 检验方法 (auto/shapiro/ks/normaltest)

        Returns:
            DistributionResult: 检验结果
        """
        series = series.dropna()
        n = len(series)

        if n < 3:
            return DistributionResult(
                is_normal=False,
                test_statistic=0.0,
                p_value=1.0,
                test_type="insufficient_data"
            )

        if method == "auto":
            if n < 5000:
                method = "shapiro"
            else:
                method = "ks"

        try:
            if method == "shapiro":
                stat, p_value = stats.shapiro(series)
            elif method == "ks":
                stat, p_value = stats.kstest(series, "norm")
            elif method == "normaltest":
                stat, p_value = stats.normaltest(series)
            else:
                raise ValueError(f"未知的检验方法: {method}")
        except Exception as e:
            return DistributionResult(
                is_normal=False,
                test_statistic=0.0,
                p_value=1.0,
                test_type=f"error: {e}"
            )

        return DistributionResult(
            is_normal=bool(p_value > 0.05),
            test_statistic=float(stat),
            p_value=float(p_value),
            test_type=method
        )

    @staticmethod
    def percentile(
        series: pd.Series,
        q: list[float] | None = None
    ) -> dict[str, float]:
        """计算分位数

        Args:
            series: 输入数据序列
            q: 分位数列表，默认 [1, 5, 10, 25, 50, 75, 90, 95, 99]

        Returns:
            dict[str, float]: 分位数结果
        """
        if q is None:
            q = [0.01, 0.05, 0.10, 0.25, 0.50, 0.75, 0.90, 0.95, 0.99]

        series = series.dropna()
        return {f"p{int(v * 100)}": float(series.quantile(v)) for v in q}


class CorrelationAnalysis:
    """相关性分析"""

    @staticmethod
    def compute_correlation(
        df: pd.DataFrame,
        method: str = "pearson",
        threshold: float = 0.7
    ) -> CorrelationResult:
        """计算相关性矩阵

        Args:
            df: 输入数据框（只包含数值列）
            method: 相关性方法 (pearson/spearman/kendall)
            threshold: 显著相关阈值

        Returns:
            CorrelationResult: 相关性分析结果
        """
        # 只选择数值列
        numeric_df = df.select_dtypes(include=[np.number])

        if numeric_df.shape[1] < 2:
            return CorrelationResult(
                matrix={},
                significant_pairs=[],
                method=method
            )

        # 计算相关性矩阵
        corr_matrix = numeric_df.corr(method=method)

        # 转换为字典格式
        matrix_dict = {}
        for col1 in corr_matrix.columns:
            matrix_dict[col1] = {}
            for col2 in corr_matrix.columns:
                matrix_dict[col1][col2] = float(corr_matrix.loc[col1, col2])

        # 找出显著相关对（排除自相关）
        significant = []
        for i, col1 in enumerate(corr_matrix.columns):
            for j, col2 in enumerate(corr_matrix.columns):
                if i >= j:  # 避免重复和自相关
                    continue
                corr_val = corr_matrix.loc[col1, col2]
                if not pd.isna(corr_val) and abs(corr_val) >= threshold:
                    significant.append((col1, col2, float(corr_val)))

        # 按相关性绝对值排序
        significant.sort(key=lambda x: abs(x[2]), reverse=True)

        return CorrelationResult(
            matrix=matrix_dict,
            significant_pairs=significant,
            method=method
        )


class HypothesisTest:
    """假设检验"""

    @staticmethod
    def t_test_ind(
        sample1: pd.Series,
        sample2: pd.Series
    ) -> TTestResult:
        """独立样本 t 检验

        Args:
            sample1: 第一个样本
            sample2: 第二个样本

        Returns:
            TTestResult: t 检验结果
        """
        sample1 = sample1.dropna()
        sample2 = sample2.dropna()

        if len(sample1) < 2 or len(sample2) < 2:
            return TTestResult(
                statistic=0.0,
                p_value=1.0,
                significant=False,
                equal_var=True,
                levene_p=1.0
            )

        # 方差齐性检验
        try:
            _, p_levene = stats.levene(sample1, sample2)
            equal_var = bool(p_levene > 0.05)
        except Exception:
            p_levene = 1.0
            equal_var = True

        # t 检验
        try:
            stat, p_value = stats.ttest_ind(sample1, sample2, equal_var=equal_var)
        except Exception:
            return TTestResult(
                statistic=0.0,
                p_value=1.0,
                significant=False,
                equal_var=equal_var,
                levene_p=float(p_levene)
            )

        return TTestResult(
            statistic=float(stat),
            p_value=float(p_value),
            significant=bool(p_value < 0.05),
            equal_var=equal_var,
            levene_p=float(p_levene)
        )

    @staticmethod
    def chi_square(observed: pd.DataFrame) -> ChiSquareResult:
        """卡方检验

        Args:
            observed: 列联表

        Returns:
            ChiSquareResult: 卡方检验结果
        """
        try:
            chi2, p_value, dof, _ = stats.chi2_contingency(observed)
        except Exception as e:
            return ChiSquareResult(
                chi2=0.0,
                p_value=1.0,
                dof=0,
                significant=False
            )

        return ChiSquareResult(
            chi2=float(chi2),
            p_value=float(p_value),
            dof=int(dof),
            significant=bool(p_value < 0.05)
        )
