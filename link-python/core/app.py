"""FastAPI 应用配置。"""

from contextlib import asynccontextmanager
from typing import AsyncGenerator

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from config import get_settings
from core import setup_logging
from core.exceptions import setup_exception_handlers


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    """应用生命周期管理。

    Args:
        app: FastAPI 应用实例

    Yields:
        None
    """
    settings = get_settings()
    setup_logging()

    from core import get_logger

    logger = get_logger(__name__)
    logger.info(
        "应用启动",
        app_name=settings.app_name,
        env=settings.app_env,
        log_level=settings.log_level,
    )

    yield

    logger.info("应用关闭")


def create_app() -> FastAPI:
    """创建 FastAPI 应用实例。

    Returns:
        配置好的 FastAPI 应用
    """
    settings = get_settings()

    app = FastAPI(
        title=settings.app_name,
        version="0.1.0",
        description="Python 基础服务",
        lifespan=lifespan,
        docs_url="/docs",
        redoc_url="/redoc",
    )

    # 配置 CORS
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"] if settings.is_dev else [],  # 生产环境需要明确指定
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # 设置异常处理器
    setup_exception_handlers(app)

    # 注册路由
    from api.routes import health, example

    app.include_router(health.router, tags=["health"])
    app.include_router(example.router, prefix="/api/v1", tags=["api"])

    return app
