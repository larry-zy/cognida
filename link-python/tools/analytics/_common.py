"""Analytics MCP 工具公共辅助。

提供 JSON 行集 ↔ DataFrame 转换、JSON 序列化、统一错误封装。
所有 analytics 工具共享这些工具函数，保证数据契约一致。
"""

from __future__ import annotations

import dataclasses
import math
from abc import abstractmethod
from datetime import date, datetime
from enum import Enum
from typing import Any

import numpy as np
import pandas as pd

from core import get_logger
from services.analytics import DEFAULT_MAX_ROWS
from tools.base import BaseTool

# 行集最大行数上限（超出则截断并告警）
MAX_ROWS = DEFAULT_MAX_ROWS

_logger = get_logger(__name__)


class AnalyticsTool(BaseTool):
    """Analytics MCP 工具基类。

    统一把工具输出封装为 ``{success, data, error}`` 信封，与 Go 侧
    ``SkillInvokeResult`` / ``MCPClient.Invoke`` 的解析约定对齐：
    - 成功：``{"success": True, "data": <分析结果>}``
    - 失败：``{"success": False, "error": ..., "error_type": ...}``

    子类只需实现 :meth:`_run` 返回裸结果 dict；若 ``_run`` 返回带
    ``error_type`` 的结构化错误（见 :func:`error_result`），基类自动归类为失败。
    异常一律捕获为 ``internal_error`` 失败信封，绝不抛给 MCP handler。
    """

    async def execute(self, **kwargs: Any) -> dict[str, Any]:
        try:
            result = await self._run(**kwargs)
        except Exception as e:  # noqa: BLE001 — 工具内异常统一降级为结构化错误
            _logger.error("analytics tool failed", tool=self.name, error=str(e))
            return {
                "success": False,
                "error": str(e),
                "error_type": "internal_error",
            }

        # _run 自身返回的结构化错误（含 error_type）归类为失败
        if isinstance(result, dict) and result.get("error_type"):
            return {"success": False, **result}

        return {"success": True, "data": result}

    @abstractmethod
    async def _run(self, **kwargs: Any) -> Any:
        """执行实际分析，返回裸结果 dict（成功）或结构化错误 dict。"""
        raise NotImplementedError


def records_to_df(data: Any) -> pd.DataFrame:
    """将 JSON 行集转换为 DataFrame。

    支持两种输入格式：
    - ``{"columns": [...], "rows": [[...], ...]}``（与 Go sql_execute 输出对齐）
    - ``[{col: val, ...}, ...]``（records 数组）

    Args:
        data: 行集输入

    Returns:
        pd.DataFrame: 转换后的数据框

    Raises:
        ValueError: 输入缺失、格式无效或为空
    """
    if data is None:
        raise ValueError("缺少 data 参数")

    if isinstance(data, pd.DataFrame):
        df = data.copy()
    elif isinstance(data, dict) and "rows" in data:
        columns = data.get("columns")
        rows = data.get("rows") or []
        df = pd.DataFrame(rows, columns=columns)
    elif isinstance(data, list):
        df = pd.DataFrame(data)
    else:
        raise ValueError("data 格式无效，应为 {columns, rows} 或 records 数组")

    if df.empty:
        raise ValueError("行集为空")

    return _coerce_numeric(df)


def _coerce_numeric(df: pd.DataFrame) -> pd.DataFrame:
    """尝试把 object 列转为数值列（仅当不引入新的 NA 时才替换）。"""
    for col in df.columns:
        if df[col].dtype != object:
            continue
        converted = pd.to_numeric(df[col], errors="coerce")
        # 仅当原本的非空值都能转成数值时才替换，避免误伤文本列
        if converted.notna().sum() >= df[col].notna().sum() and converted.notna().any():
            df[col] = converted
    return df


def truncate(df: pd.DataFrame, max_rows: int = MAX_ROWS) -> tuple[pd.DataFrame, bool]:
    """按上限截断行集。

    Returns:
        (截断后的数据框, 是否发生截断)
    """
    if len(df) > max_rows:
        return df.head(max_rows), True
    return df, False


def numeric_columns(df: pd.DataFrame, requested: list[str] | None = None) -> list[str]:
    """返回参与分析的数值列。

    Args:
        df: 数据框
        requested: 调用方指定的列；为空则取全部数值列
    """
    numeric = df.select_dtypes(include=[np.number]).columns.tolist()
    if requested:
        return [c for c in requested if c in numeric]
    return numeric


def to_jsonable(obj: Any) -> Any:
    """递归地把分析结果转换为 JSON 可序列化结构。

    处理 Enum、dataclass、numpy/pandas 标量、Timestamp、NaN/Inf 等。
    """
    if obj is None or isinstance(obj, (bool, int, str)):
        return obj
    if isinstance(obj, float):
        return obj if math.isfinite(obj) else None
    if isinstance(obj, Enum):
        return obj.value
    if isinstance(obj, np.integer):
        return int(obj)
    if isinstance(obj, np.floating):
        v = float(obj)
        return v if math.isfinite(v) else None
    if isinstance(obj, np.bool_):
        return bool(obj)
    if isinstance(obj, (pd.Timestamp, datetime, date)):
        return obj.isoformat()
    if dataclasses.is_dataclass(obj) and not isinstance(obj, type):
        return {k: to_jsonable(v) for k, v in dataclasses.asdict(obj).items()}
    if isinstance(obj, dict):
        return {str(k): to_jsonable(v) for k, v in obj.items()}
    if isinstance(obj, pd.Series):
        return [to_jsonable(v) for v in obj.tolist()]
    if isinstance(obj, np.ndarray):
        return [to_jsonable(v) for v in obj.tolist()]
    if isinstance(obj, (list, tuple, set)):
        return [to_jsonable(v) for v in obj]
    return str(obj)


def error_result(message: str, error_type: str = "invalid_input", **extra: Any) -> dict[str, Any]:
    """构造结构化错误结果（不抛异常，交给 Agent 推理）。"""
    result: dict[str, Any] = {"error": message, "error_type": error_type}
    result.update(extra)
    return result
