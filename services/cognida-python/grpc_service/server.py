"""gRPC 服务器启动模块。"""

import asyncio
import os

from core import setup_logging
from config import get_settings
from grpc_service.servicer import serve_grpc

# 设置环境变量来增加 gRPC 消息大小限制 (用于 Windows 兼容性)
# 注意：这些环境变量在某些 grpcio 版本中可能不起作用
os.environ['GRPC_SERVER_MAX_RECV_MESSAGE_LENGTH'] = '104857600'  # 100MB
os.environ['GRPC_SERVER_MAX_SEND_MESSAGE_LENGTH'] = '104857600'  # 100MB


async def main() -> None:
    """主函数。"""
    settings = get_settings()

    # 设置日志
    setup_logging()

    from core import get_logger

    logger = get_logger()
    logger.info(
        "启动 Python 工具服务",
        app_name=settings.app_name,
        env=settings.app_env,
    )

    # 启动 gRPC 服务器
    grpc_port = getattr(settings, "grpc_port", 50051)
    await serve_grpc(grpc_port)


if __name__ == "__main__":
    asyncio.run(main())
