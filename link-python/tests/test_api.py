"""API 测试。"""

import pytest
from fastapi.testclient import TestClient
from httpx import AsyncClient


class TestHelloAPI:
    """Hello API 测试。"""

    def test_hello_default(self, client: TestClient) -> None:
        """测试默认 hello。

        Args:
            client: 测试客户端
        """
        response = client.get("/api/v1/hello")
        assert response.status_code == 200
        assert response.json() == {"message": "Hello, World!"}

    def test_hello_with_name(self, client: TestClient) -> None:
        """测试带名字的 hello。

        Args:
            client: 测试客户端
        """
        response = client.get("/api/v1/hello?name=Claude")
        assert response.status_code == 200
        assert response.json() == {"message": "Hello, Claude!"}

    @pytest.mark.asyncio
    async def test_hello_async(self, async_client: AsyncClient) -> None:
        """测试异步 hello。

        Args:
            async_client: 异步测试客户端
        """
        response = await async_client.get("/api/v1/hello")
        assert response.status_code == 200
        assert response.json() == {"message": "Hello, World!"}


class TestItemAPI:
    """Item API 测试。"""

    def test_get_item_success(self, client: TestClient) -> None:
        """测试获取成功商品。

        Args:
            client: 测试客户端
        """
        response = client.get("/api/v1/items/1")
        assert response.status_code == 200
        data = response.json()
        assert data["item_id"] == 1
        assert data["name"] == "Item 1"

    def test_get_item_not_found(self, client: TestClient) -> None:
        """测试获取不存在商品。

        Args:
            client: 测试客户端
        """
        response = client.get("/api/v1/items/-1")
        assert response.status_code == 404
        data = response.json()
        assert data["code"] == "NOT_FOUND"
