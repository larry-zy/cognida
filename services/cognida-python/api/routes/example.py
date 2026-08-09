"""示例 API 路由。"""

from typing import Any

from fastapi import APIRouter, Query

from core import get_logger
from core.exceptions import NotFoundError

router = APIRouter()
logger = get_logger(__name__)


@router.get("/hello")
async def hello(name: str = Query(default="World", description="名称")) -> dict[str, Any]:
    """示例 Hello 端点。

    Args:
        name: 要问候的名字

    Returns:
        问候响应
    """
    logger.info("hello 请求", name=name)
    return {"message": f"Hello, {name}!"}


@router.get("/items/{item_id}")
async def get_item(item_id: int) -> dict[str, Any]:
    """获取商品示例。

    Args:
        item_id: 商品 ID

    Returns:
        商品信息

    Raises:
        NotFoundError: 商品不存在
    """
    logger.info("获取商品", item_id=item_id)

    if item_id <= 0:
        raise NotFoundError(f"商品 {item_id} 不存在")

    return {"item_id": item_id, "name": f"Item {item_id}"}
