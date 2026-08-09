"""日志配置模块。"""

import logging
import sys
from typing import Any

import structlog
from config import get_settings


def setup_logging() -> None:
    """配置结构化日志。"""
    settings = get_settings()

    # 配置标准库 logging
    logging.basicConfig(
        format="%(message)s",
        stream=sys.stdout,
        level=getattr(logging, settings.log_level),
    )

    # 配置 structlog
    if settings.is_dev:
        # 开发环境：可读格式
        structlog.configure(
            processors=[
                structlog.contextvars.merge_contextvars,
                # structlog.stdlib.add_logger_name,  # PrintLogger 没有 name 属性
                structlog.stdlib.add_log_level,
                structlog.stdlib.PositionalArgumentsFormatter(),
                structlog.processors.TimeStamper(fmt="%Y-%m-%d %H:%M:%S"),
                structlog.processors.StackInfoRenderer(),
                structlog.dev.set_exc_info,
                structlog.processors.format_exc_info,
                structlog.dev.ConsoleRenderer(colors=True),
            ],
            wrapper_class=structlog.make_filtering_bound_logger(
                getattr(logging, settings.log_level)
            ),
            context_class=dict,
            logger_factory=structlog.PrintLoggerFactory(),
            cache_logger_on_first_use=True,
        )
    else:
        # 生产环境：JSON 格式
        structlog.configure(
            processors=[
                structlog.contextvars.merge_contextvars,
                # structlog.stdlib.add_logger_name,  # PrintLogger 没有 name 属性
                structlog.stdlib.add_log_level,
                structlog.stdlib.PositionalArgumentsFormatter(),
                structlog.processors.TimeStamper(fmt="iso"),
                structlog.processors.StackInfoRenderer(),
                structlog.processors.format_exc_info,
                structlog.processors.UnicodeDecoder(),
                structlog.processors.JSONRenderer(),
            ],
            wrapper_class=structlog.make_filtering_bound_logger(
                getattr(logging, settings.log_level)
            ),
            context_class=dict,
            logger_factory=structlog.PrintLoggerFactory(),
            cache_logger_on_first_use=True,
        )


def get_logger(name: str | None = None) -> structlog.stdlib.BoundLogger:
    """获取日志记录器。

    Args:
        name: 日志记录器名称

    Returns:
        配置好的日志记录器
    """
    return structlog.get_logger(name)


class LoggerMixin:
    """日志记录器混入类。"""

    @property
    def logger(self) -> structlog.stdlib.BoundLogger:
        """获取当前类的日志记录器。"""
        return get_logger(self.__class__.__name__)

    def _log_dict(self, level: str, msg: str, **kwargs: Any) -> None:
        """记录字典格式的日志。

        Args:
            level: 日志级别
            msg: 日志消息
            **kwargs: 额外的上下文信息
        """
        log_func = getattr(self.logger, level)
        log_func(msg, **kwargs)
