"""统一单进程多协议入口（容器内真实拓扑）。

在同一个 asyncio 事件循环内并发启动多协议服务，取代此前
`scripts/dev-all.sh` 的四进程碎片化拓扑（见架构评审 PY-1 / PY-2）：

  - 主 FastAPI HTTP 服务   —— `settings.api_port`（默认 8000）
      提供 /health（容器健康检查）等基础路由；并挂载评测路由，使 8000
      不再是空壳。
  - 评测 FastAPI HTTP 服务 —— `EVALUATION_HTTP_PORT`（18888）
      Go worker 经 PYTHON_EVALUATION_ENDPOINT（默认 http://localhost:18888）
      调用，端口契约保持不变。
  - gRPC 服务             —— `settings.grpc_port`（默认 50051）
      docreader / quality（数据分析 dead server 不在此启动，见 X-2）。

三者通过 `asyncio.gather` 跑在同一事件循环，统一处理 SIGINT/SIGTERM
优雅退出。任一子服务因缺少可选依赖而无法加载时，记录日志并降级，
不再让整个进程崩溃（此前 `uvicorn src.main:app` 直接指向不存在目录）。

入口：`python -m core.main`。
"""

from __future__ import annotations

import asyncio
import signal
from typing import Any

import uvicorn

from config import get_settings
from core import get_logger, setup_logging

# 评测 HTTP 服务端口（与 Go 侧 PYTHON_EVALUATION_ENDPOINT 默认值对齐）。
EVALUATION_HTTP_PORT = 18888


def _load_evaluation_app() -> Any | None:
    """加载评测 FastAPI 应用。

    评测应用依赖可选 extras（jieba / sklearn / sentence-transformers 等）。
    若这些依赖在当前镜像中未安装，则降级为不提供评测 HTTP 面，而不是让
    整个进程启动失败。

    Returns:
        评测 FastAPI 应用实例；加载失败时返回 None。
    """
    logger = get_logger(__name__)
    try:
        from services.evaluation.fastapi_app import app as evaluation_app

        return evaluation_app
    except Exception as exc:  # noqa: BLE001 - 缺少可选依赖时降级而非崩溃
        logger.warning("评测 HTTP 服务未加载（缺少可选依赖，已降级）", error=str(exc))
        return None


def _build_http_server(app: Any, host: str, port: int, log_level: str) -> uvicorn.Server:
    """构造一个由统一入口托管信号的 uvicorn Server。"""
    config = uvicorn.Config(
        app,
        host=host,
        port=port,
        log_level=log_level,
        access_log=True,
    )
    server = uvicorn.Server(config)
    # 单进程内多个 Server：统一入口负责信号处理，禁用各 Server 自带处理器，
    # 避免相互抢占导致关闭行为不确定。
    server.install_signal_handlers = lambda: None  # type: ignore[method-assign]
    return server


async def _serve_grpc(port: int, stop_event: asyncio.Event) -> None:
    """启动 gRPC 服务器并在收到停止信号时优雅关闭。

    使用同步 grpc.server（线程池）实现；start() 非阻塞，本协程通过
    stop_event 等待关闭信号后调用 stop()。
    """
    logger = get_logger(__name__)
    try:
        from grpc_service.servicer import create_grpc_server

        server = create_grpc_server(port)
        server.start()
        logger.info("gRPC 服务器已启动", port=port)
    except Exception as exc:  # noqa: BLE001 - 记录并降级，保持进程存活
        logger.error("gRPC 服务器启动失败（已降级）", error=str(exc))
        return

    await stop_event.wait()
    logger.info("正在关闭 gRPC 服务器...")
    server.stop(grace=5)


async def _run() -> None:
    """统一入口主协程。"""
    settings = get_settings()
    setup_logging()
    logger = get_logger(__name__)

    from core.app import create_app

    main_app = create_app()
    log_level = settings.log_level.lower()

    # 将评测路由挂载到主应用，使 8000 端口对外提供真实评测能力（消除空壳）。
    evaluation_app = _load_evaluation_app()
    if evaluation_app is not None:
        # 主应用自身已注册的路由（/health、/docs、/api/v1/...）优先匹配；
        # 未命中的评测路径回退到该挂载点。
        main_app.mount("/", evaluation_app)

    grpc_port = getattr(settings, "grpc_port", 50051)

    main_server = _build_http_server(
        main_app, settings.api_host, settings.api_port, log_level
    )

    servers: list[uvicorn.Server] = [main_server]
    coros: list[Any] = [main_server.serve()]

    # 评测应用同时独立监听 18888，保持 Go worker 端口契约不变。
    if evaluation_app is not None:
        eval_server = _build_http_server(
            evaluation_app, settings.api_host, EVALUATION_HTTP_PORT, log_level
        )
        servers.append(eval_server)
        coros.append(eval_server.serve())

    stop_event = asyncio.Event()
    coros.append(_serve_grpc(grpc_port, stop_event))

    def _request_shutdown() -> None:
        logger.info("收到停止信号，正在优雅关闭全部子服务...")
        for srv in servers:
            srv.should_exit = True
        stop_event.set()

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, _request_shutdown)
        except NotImplementedError:
            # Windows 等平台不支持 add_signal_handler，交由默认行为处理。
            pass

    logger.info(
        "统一入口启动",
        app_name=settings.app_name,
        env=settings.app_env,
        http_port=settings.api_port,
        evaluation_port=(EVALUATION_HTTP_PORT if evaluation_app is not None else None),
        grpc_port=grpc_port,
    )

    await asyncio.gather(*coros)


def main() -> None:
    """同步入口。"""
    asyncio.run(_run())


if __name__ == "__main__":
    main()
