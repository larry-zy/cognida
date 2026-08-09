"""健康检查测试。"""

from fastapi.testclient import TestClient


def test_health_check(client: TestClient) -> None:
    """测试健康检查端点。

    Args:
        client: 测试客户端
    """
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "healthy"}
