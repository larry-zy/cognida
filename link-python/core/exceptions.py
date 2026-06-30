"""异常处理模块。"""

from typing import Any

from fastapi import FastAPI, Request, status
from fastapi.responses import JSONResponse


class APIError(Exception):
    """API 错误基类。"""

    def __init__(
        self,
        message: str,
        code: str = "API_ERROR",
        status_code: int = status.HTTP_500_INTERNAL_SERVER_ERROR,
        details: dict[str, Any] | None = None,
    ) -> None:
        self.message = message
        self.code = code
        self.status_code = status_code
        self.details = details or {}
        super().__init__(message)


class ValidationError(APIError):
    """验证错误。"""

    def __init__(self, message: str, details: dict[str, Any] | None = None) -> None:
        super().__init__(
            message=message,
            code="VALIDATION_ERROR",
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            details=details,
        )


class NotFoundError(APIError):
    """资源未找到错误。"""

    def __init__(self, message: str = "资源不存在", details: dict[str, Any] | None = None) -> None:
        super().__init__(
            message=message,
            code="NOT_FOUND",
            status_code=status.HTTP_404_NOT_FOUND,
            details=details,
        )


class BusinessError(APIError):
    """业务逻辑错误。"""

    def __init__(self, message: str, details: dict[str, Any] | None = None) -> None:
        super().__init__(
            message=message,
            code="BUSINESS_ERROR",
            status_code=status.HTTP_400_BAD_REQUEST,
            details=details,
        )


class AnalyticsError(APIError):
    """数据分析错误。"""

    def __init__(
        self,
        message: str,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(
            message=message,
            code="ANALYTICS_ERROR",
            status_code=status.HTTP_400_BAD_REQUEST,
            details=details,
        )


class DataValidationError(AnalyticsError):
    """数据验证错误。"""

    def __init__(
        self,
        message: str,
        field: str | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        if field:
            message = f"{field}: {message}"
        if details is None:
            details = {}
        if field:
            details["field"] = field
        super().__init__(message, details=details)


class InsufficientDataError(AnalyticsError):
    """数据不足错误。"""

    def __init__(
        self,
        message: str = "数据量不足，无法进行分析",
        min_required: int | None = None,
        actual: int | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        if details is None:
            details = {}
        if min_required is not None:
            details["min_required"] = min_required
        if actual is not None:
            details["actual"] = actual
        super().__init__(message, details=details)


def error_response(
    code: str, message: str, status_code: int, details: dict[str, Any] | None = None
) -> JSONResponse:
    """创建错误响应。

    Args:
        code: 错误代码
        message: 错误消息
        status_code: HTTP 状态码
        details: 错误详情

    Returns:
        JSON 错误响应
    """
    content = {
        "code": code,
        "message": message,
    }
    if details:
        content["details"] = details

    return JSONResponse(status_code=status_code, content=content)


def setup_exception_handlers(app: FastAPI) -> None:
    """设置全局异常处理器。

    Args:
        app: FastAPI 应用实例
    """

    @app.exception_handler(APIError)
    async def api_error_handler(request: Request, exc: APIError) -> JSONResponse:
        """处理 API 错误。"""
        return error_response(
            code=exc.code,
            message=exc.message,
            status_code=exc.status_code,
            details=exc.details,
        )

    @app.exception_handler(Exception)
    async def generic_exception_handler(request: Request, exc: Exception) -> JSONResponse:
        """处理未捕获的异常。"""
        from core import get_logger

        logger = get_logger(__name__)
        logger.exception("未处理的异常", exc_info=exc, path=request.url.path)

        return error_response(
            code="INTERNAL_ERROR",
            message="服务器内部错误",
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        )
