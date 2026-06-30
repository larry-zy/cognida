"""测试配置和 fixtures。"""

from typing import AsyncGenerator, Generator

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from httpx import ASGITransport, AsyncClient

from core.app import create_app


@pytest.fixture(scope="session")
def app() -> FastAPI:
    """创建测试应用实例。

    Returns:
        FastAPI 应用实例
    """
    return create_app()


@pytest.fixture
def client(app: FastAPI) -> Generator[TestClient, None, None]:
    """创建测试客户端。

    Args:
        app: FastAPI 应用实例

    Yields:
        测试客户端
    """
    with TestClient(app) as test_client:
        yield test_client


@pytest.fixture
async def async_client(app: FastAPI) -> AsyncGenerator[AsyncClient, None]:
    """创建异步测试客户端。

    Args:
        app: FastAPI 应用实例

    Yields:
        异步测试客户端
    """
    async with AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as ac:
        yield ac
