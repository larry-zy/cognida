"""就绪检查（readiness）——对本部署已配置的后端存储做有界探活（架构评审 INF-5）。

设计原则：
- 只检查「本部署显式配置」的存储：以对应环境变量是否设置为准（REDIS_URL /
  DATABASE_URL / MILVUS_HOST / NEO4J_URI），未配置则跳过，不因缺省的可选依赖而误判未就绪。
- 复用既有连接方式，不引入新驱动：
    * Redis —— 进程内已安装 redis.asyncio，做真 PING。
    * MySQL / Milvus / Neo4j —— 该 Python 进程内无对应客户端（连接由 Go 侧持有），
      退化为有界 TCP 可达性探测（最小安全检查），并在明细里标注探测方式，避免臆造驱动。
- 全部探测均设超时上限，且用 asyncio 非阻塞方式执行，保证 /ready 不会挂起事件循环。
"""

from __future__ import annotations

import asyncio
import os
from dataclasses import dataclass
from urllib.parse import urlparse

from config import get_settings
from core import get_logger

# 单个依赖探测的超时上限（秒）。/ready 作为容器就绪探针需快速返回。
_PROBE_TIMEOUT_S = 2.0

# Python 进程内无原生驱动、退化为 TCP 探测的说明（写入 detail 便于运维辨识）。
_TCP_NOTE = "tcp-probe (no python driver in this process; owned by go service)"

logger = get_logger(__name__)


@dataclass
class DependencyStatus:
    """单个依赖的就绪状态。

    status 取值：
      - "ok"      已配置且可达
      - "error"   已配置但不可达（阻断就绪 → 503）
      - "skipped" 本部署未配置（不阻断就绪）
    """

    name: str
    status: str
    detail: str | None = None

    def is_blocking(self) -> bool:
        """是否阻断就绪（仅「已配置但不可达」阻断）。"""
        return self.status == "error"


async def _tcp_probe(name: str, host: str, port: int) -> DependencyStatus:
    """有界 TCP 可达性探测：能在超时内建立连接即视为可达。"""
    if not host:
        return DependencyStatus(name, "error", f"无法解析主机地址（{_TCP_NOTE}）")
    try:
        reader, writer = await asyncio.wait_for(
            asyncio.open_connection(host, port), timeout=_PROBE_TIMEOUT_S
        )
        writer.close()
        try:
            await writer.wait_closed()
        except Exception:  # noqa: BLE001 - 关闭阶段异常不影响可达判定
            pass
        return DependencyStatus(name, "ok", f"{host}:{port} 可达（{_TCP_NOTE}）")
    except Exception as exc:  # noqa: BLE001 - 探测失败归一化为未就绪
        return DependencyStatus(
            name, "error", f"{host}:{port} 不可达: {exc}（{_TCP_NOTE}）"
        )


async def _check_redis(url: str) -> DependencyStatus:
    """Redis 真 PING（复用已安装的 redis.asyncio 客户端）。"""
    try:
        import redis.asyncio as aioredis
    except Exception as exc:  # noqa: BLE001 - 未安装则视为无法检查
        return DependencyStatus("redis", "error", f"redis 客户端不可用: {exc}")

    client = aioredis.from_url(
        url,
        socket_connect_timeout=_PROBE_TIMEOUT_S,
        socket_timeout=_PROBE_TIMEOUT_S,
    )
    try:
        await asyncio.wait_for(client.ping(), timeout=_PROBE_TIMEOUT_S)
        return DependencyStatus("redis", "ok", "PING ok")
    except Exception as exc:  # noqa: BLE001
        return DependencyStatus("redis", "error", f"PING 失败: {exc}")
    finally:
        try:
            await client.aclose()
        except Exception:  # noqa: BLE001 - 关闭异常不影响探测结论
            pass


def _milvus_host_port(settings) -> tuple[str, int]:
    """从 milvus 配置解析 host/port（兼容裸主机名与 URI 形态）。"""
    raw = (settings.milvus_host or "").strip()
    if "://" in raw:
        parsed = urlparse(raw)
        host = parsed.hostname or ""
        default_port = 443 if parsed.scheme in ("https", "grpcs") else settings.milvus_port
        return host, (parsed.port or default_port)
    return raw, settings.milvus_port


def _neo4j_host_port(settings) -> tuple[str, int]:
    """从 neo4j_uri 解析 host/port（bolt/neo4j[+s] 默认 7687）。"""
    parsed = urlparse(settings.neo4j_uri or "")
    return (parsed.hostname or ""), (parsed.port or 7687)


def _mysql_host_port(settings) -> tuple[str, int]:
    """从 database_url 解析 host/port（默认 3306）。"""
    parsed = urlparse(settings.database_url or "")
    return (parsed.hostname or "localhost"), (parsed.port or 3306)


async def check_readiness() -> tuple[bool, list[DependencyStatus]]:
    """并发探测本部署已配置的后端存储，返回 (是否就绪, 各依赖状态)。

    各探测内部均已 time-bound 且吞掉自身异常，asyncio.gather 不会抛出。
    「未配置」的依赖直接跳过、不纳入探测，也不阻断就绪。
    """
    settings = get_settings()
    checks: list = []

    # Redis：仅在显式配置（REDIS_URL）时检查——真 PING。
    if os.getenv("REDIS_URL") and settings.redis_url:
        checks.append(_check_redis(settings.redis_url))

    # MySQL：仅在显式配置（DATABASE_URL）时检查——TCP 探测（进程内无 SQL 驱动）。
    if os.getenv("DATABASE_URL") and settings.database_url:
        host, port = _mysql_host_port(settings)
        checks.append(_tcp_probe("mysql", host, port))

    # Milvus：仅在显式配置（MILVUS_HOST）时检查——TCP 探测。
    if os.getenv("MILVUS_HOST"):
        host, port = _milvus_host_port(settings)
        checks.append(_tcp_probe("milvus", host, port))

    # Neo4j：仅在显式配置（NEO4J_URI）时检查——TCP 探测。
    if os.getenv("NEO4J_URI"):
        host, port = _neo4j_host_port(settings)
        checks.append(_tcp_probe("neo4j", host, port))

    if not checks:
        # 本部署未显式配置任何后端存储：就绪（liveness 之外无强依赖）。
        return True, []

    results: list[DependencyStatus] = await asyncio.gather(*checks)
    ready = all(not r.is_blocking() for r in results)
    if not ready:
        logger.warning(
            "就绪检查未通过",
            failed=[r.name for r in results if r.is_blocking()],
        )
    return ready, results
