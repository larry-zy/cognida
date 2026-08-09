#!/usr/bin/env python
"""运行质量服务测试。"""

import sys
import subprocess
from pathlib import Path


def run_tests(args: list[str] | None = None) -> int:
    """运行测试。

    Args:
        args: 额外的pytest参数

    Returns:
        退出码
    """
    project_root = Path(__file__).parent.parent
    tests_dir = project_root / "tests" / "quality"

    cmd = [
        sys.executable, "-m", "pytest",
        str(tests_dir),
        "-v",
        "--tb=short",
        "--strict-markers",
    ]

    if args:
        cmd.extend(args)

    print(f"运行测试: {' '.join(cmd)}")
    return subprocess.run(cmd, cwd=project_root).returncode


def main() -> int:
    """主函数。"""
    import argparse

    parser = argparse.ArgumentParser(description="运行质量服务测试")
    parser.add_argument(
        "--unit",
        action="store_true",
        help="只运行单元测试",
    )
    parser.add_argument(
        "--integration",
        action="store_true",
        help="只运行集成测试",
    )
    parser.add_argument(
        "--fast",
        action="store_true",
        help="跳过慢速测试",
    )
    parser.add_argument(
        "--coverage",
        action="store_true",
        help="生成覆盖率报告",
    )
    parser.add_argument(
        "extra_args",
        nargs="*",
        help="额外的pytest参数",
    )

    args = parser.parse_args()

    pytest_args = []

    if args.unit:
        pytest_args.extend(["-m", "unit"])
    elif args.integration:
        pytest_args.extend(["-m", "integration"])

    if args.fast:
        pytest_args.extend(["-m", "not slow"])

    if args.coverage:
        pytest_args.extend([
            "--cov=services.quality",
            "--cov-report=html",
            "--cov-report=term-missing",
        ])

    pytest_args.extend(args.extra_args)

    return run_tests(pytest_args)


if __name__ == "__main__":
    sys.exit(main())
