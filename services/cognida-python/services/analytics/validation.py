"""数据分析输入验证模块。

提供数据验证、数据量检查、时序数据检测等功能。
"""

from typing import Any

import numpy as np
import pandas as pd

from core.exceptions import DataValidationError, InsufficientDataError


# 默认配置
DEFAULT_MAX_ROWS = 100_000  # 最大行数限制
DEFAULT_MIN_ROWS = 3        # 最小行数要求


class DataValidator:
    """数据验证器"""

    @staticmethod
    def validate_dataframe(
        df: pd.DataFrame,
        min_rows: int = DEFAULT_MIN_ROWS,
        max_rows: int = DEFAULT_MAX_ROWS,
        require_numeric: bool = False
    ) -> None:
        """验证数据框

        Args:
            df: 输入数据框
            min_rows: 最小行数要求
            max_rows: 最大行数限制
            require_numeric: 是否要求数值列

        Raises:
            DataValidationError: 数据验证失败
            InsufficientDataError: 数据量不足
        """
        if df is None or df.empty:
            raise DataValidationError("输入数据为空")

        if len(df) < min_rows:
            raise InsufficientDataError(
                message=f"数据量不足，至少需要 {min_rows} 行数据",
                min_required=min_rows,
                actual=len(df)
            )

        if len(df) > max_rows:
            raise DataValidationError(
                f"数据量过大，最多支持 {max_rows} 行数据",
                details={"max_rows": max_rows, "actual_rows": len(df)}
            )

        if require_numeric:
            numeric_cols = df.select_dtypes(include=[np.number]).columns
            if len(numeric_cols) == 0:
                raise DataValidationError("未找到数值列，无法进行分析")

    @staticmethod
    def validate_column(
        df: pd.DataFrame,
        column: str,
        allow_na: bool = True
    ) -> pd.Series:
        """验证列存在并返回序列

        Args:
            df: 输入数据框
            column: 列名
            allow_na: 是否允许 NA 值

        Returns:
            pd.Series: 列数据

        Raises:
            DataValidationError: 列不存在或数据无效
        """
        if column not in df.columns:
            raise DataValidationError(
                f"列不存在: {column}",
                field=column,
                details={"available_columns": list(df.columns)}
            )

        series = df[column]

        if not allow_na and series.isna().all():
            raise DataValidationError(
                f"列 {column} 全为 NA 值",
                field=column
            )

        return series

    @staticmethod
    def validate_numeric_column(
        df: pd.DataFrame,
        column: str
    ) -> pd.Series:
        """验证数值列

        Args:
            df: 输入数据框
            column: 列名

        Returns:
            pd.Series: 数值序列

        Raises:
            DataValidationError: 列不是数值类型
        """
        if column not in df.columns:
            raise DataValidationError(
                f"列不存在: {column}",
                field=column
            )

        series = df[column]

        # 尝试转换为数值类型
        try:
            series = pd.to_numeric(series, errors="coerce")
        except Exception as e:
            raise DataValidationError(
                f"列 {column} 无法转换为数值类型",
                field=column,
                details={"error": str(e)}
            )

        if series.isna().all():
            raise DataValidationError(
                f"列 {column} 转换后全为 NA 值",
                field=column
            )

        return series


class TimeSeriesDetector:
    """时序数据检测器"""

    @staticmethod
    def detect_time_column(
        df: pd.DataFrame,
        candidate_columns: list[str] | None = None
    ) -> str | None:
        """检测时间列

        Args:
            df: 输入数据框
            candidate_columns: 候选列名列表

        Returns:
            str | None: 检测到的时间列名
        """
        if candidate_columns is None:
            # 优先检测名称中包含 time/date 的列
            candidate_columns = [
                col for col in df.columns
                if any(keyword in col.lower() for keyword in ["time", "date", "timestamp", "datetime"])
            ]

        for col in candidate_columns:
            if col not in df.columns:
                continue

            try:
                # 尝试解析为时间
                pd.to_datetime(df[col].head(100), errors="coerce")
                # 如果成功率超过 50%，认为是时间列
                parsed = pd.to_datetime(df[col], errors="coerce")
                if parsed.notna().sum() / len(df) > 0.5:
                    return col
            except Exception:
                continue

        # 尝试检测索引
        if df.index.dtype == "datetime64[ns]" or isinstance(
            df.index, (pd.DatetimeIndex, pd.PeriodIndex)
        ):
            return "index"

        return None

    @staticmethod
    def validate_time_series(
        df: pd.DataFrame,
        time_column: str | None = None
    ) -> tuple[pd.DataFrame, str | None]:
        """验证并准备时序数据

        Args:
            df: 输入数据框
            time_column: 时间列名

        Returns:
            tuple[pd.DataFrame, str | None]: (处理后的数据框, 时间列名)
        """
        # 检测时间列
        if time_column is None:
            time_column = TimeSeriesDetector.detect_time_column(df)

        # 如果检测到时间列，尝试解析
        if time_column and time_column != "index":
            try:
                df = df.copy()
                df[time_column] = pd.to_datetime(df[time_column], errors="coerce")
                # 按时间排序
                df = df.sort_values(time_column)
            except Exception:
                time_column = None

        return df, time_column

    @staticmethod
    def is_regular_interval(series: pd.Series) -> bool:
        """检测时间序列是否为规则间隔

        Args:
            series: 时间序列

        Returns:
            bool: 是否为规则间隔
        """
        if len(series) < 2:
            return True

        try:
            # 转换为 datetime
            dt_series = pd.to_datetime(series)
            # 计算时间差
            diffs = dt_series.diff().dropna()
            # 检查是否所有时间差相同（允许小误差）
            if len(diffs) == 0:
                return True
            std_diff = diffs.std()
            mean_diff = diffs.mean()
            # 如果标准差相对于均值很小，认为是规则间隔
            return (std_diff / mean_diff) < 0.01 if mean_diff > 0 else True
        except Exception:
            return False


class DataChecker:
    """数据检查器 - 综合检查工具"""

    @staticmethod
    def check_data_quality(
        df: pd.DataFrame,
        verbose: bool = False
    ) -> dict[str, Any]:
        """检查数据质量

        Args:
            df: 输入数据框
            verbose: 是否返回详细信息

        Returns:
            dict[str, Any]: 数据质量报告
        """
        report = {
            "row_count": len(df),
            "col_count": len(df.columns),
            "numeric_cols": len(df.select_dtypes(include=[np.number]).columns),
            "missing_values": {},
            "data_types": {},
        }

        # 缺失值统计
        for col in df.columns:
            na_count = df[col].isna().sum()
            if na_count > 0:
                report["missing_values"][col] = {
                    "count": int(na_count),
                    "ratio": float(na_count / len(df))
                }

        # 数据类型
        if verbose:
            for col in df.columns:
                report["data_types"][col] = str(df[col].dtype)

        # 检测时间列
        time_col = TimeSeriesDetector.detect_time_column(df)
        if time_col:
            report["time_column"] = time_col
            report["is_regular_interval"] = TimeSeriesDetector.is_regular_interval(
                df[time_col] if time_col != "index" else df.index
            )

        return report

    @staticmethod
    def sanitize_dataframe(
        df: pd.DataFrame,
        drop_na: bool = False,
        drop_duplicates: bool = False,
        max_rows: int | None = None
    ) -> pd.DataFrame:
        """清洗数据框

        Args:
            df: 输入数据框
            drop_na: 是否删除 NA 行
            drop_duplicates: 是否删除重复行
            max_rows: 最大行数限制

        Returns:
            pd.DataFrame: 清洗后的数据框
        """
        df = df.copy()

        if drop_na:
            df = df.dropna()

        if drop_duplicates:
            df = df.drop_duplicates()

        if max_rows and len(df) > max_rows:
            df = df.head(max_rows)

        return df
