"""健康检查路由。"""

from fastapi import APIRouter

from core import get_logger

router = APIRouter()
logger = get_logger(__name__)


@router.get("/health")
async def health_check() -> dict[str, str]:
    """健康检查端点。

    Returns:
        健康状态
    """
    logger.debug("健康检查请求")
    return {"status": "healthy"}
