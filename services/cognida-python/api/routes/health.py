"""健康检查 / 就绪检查 / 指标路由（架构评审 INF-5 / INF-6）。"""

from fastapi import APIRouter, Response

from core import get_logger

router = APIRouter()
logger = get_logger(__name__)


@router.get("/health")
async def health_check() -> dict[str, str]:
    """存活探针（liveness）。

    进程存活即返回 200，不触达任何外部依赖，保证廉价且不会挂起。

    Returns:
        健康状态
    """
    logger.debug("健康检查请求")
    return {"status": "healthy"}


@router.get("/ready")
async def readiness_check(response: Response) -> dict:
    """就绪探针（readiness）。

    对本部署已配置的后端存储（MySQL/Redis/Milvus/Neo4j）做有界探活：
    全部可达 → 200 ready；任一已配置依赖不可达 → 503 not_ready，并返回逐依赖状态。
    未配置的依赖直接跳过，不阻断就绪。

    Returns:
        含逐依赖状态的就绪信息（未就绪时 HTTP 状态码置为 503）
    """
    from core.readiness import check_readiness

    ready, deps = await check_readiness()
    if not ready:
        response.status_code = 503
    return {
        "status": "ready" if ready else "not_ready",
        "dependencies": {
            d.name: {"status": d.status, "detail": d.detail} for d in deps
        },
    }


@router.get("/metrics")
async def metrics() -> Response:
    """Prometheus 指标端点（INF-6）。

    暴露默认 registry（含进程/平台采集器与代码中已注册的计数器）。
    prometheus_client 未安装时降级返回 501，而非让服务崩溃。
    """
    try:
        from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
    except ImportError:
        return Response(
            content="prometheus_client 未安装，/metrics 不可用\n",
            media_type="text/plain",
            status_code=501,
        )

    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)
