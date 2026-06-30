"""质量服务测试配置。"""

import sys
from pathlib import Path

# 添加项目根目录到Python路径
project_root = Path(__file__).parent.parent.parent
sys.path.insert(0, str(project_root))

import pytest


def pytest_configure(config):
    """Pytest配置。"""
    config.addinivalue_line(
        "markers",
        "slow: marks tests as slow (deselect with '-m \"not slow\"')"
    )
    config.addinivalue_line(
        "markers",
        "integration: marks tests as integration tests"
    )
    config.addinivalue_line(
        "markers",
        "unit: marks tests as unit tests"
    )


@pytest.fixture(scope="session")
def test_data_dir():
    """获取测试数据目录。"""
    return Path(__file__).parent / "fixtures"
