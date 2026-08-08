"""gRPC 代码生成脚本。

运行此脚本生成 protobuf Python 代码：
    python scripts/generate_grpc.py
"""

import subprocess
from pathlib import Path


def generate_proto(proto_file: str, output_dir: str) -> None:
    """生成单个 proto 文件的 Python 代码。

    Args:
        proto_file: proto 文件路径
        output_dir: 输出目录
    """
    proto_path = Path(proto_file)
    proto_dir = proto_path.parent.parent

    cmd = [
        "python",
        "-m",
        "grpc_tools.protoc",
        f"--proto_path={proto_dir}",
        f"--python_out={output_dir}",
        f"--grpc_python_out={output_dir}",
        str(proto_path),
    ]

    print(f"Generating {proto_path.name}...")
    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        print(f"Error: {result.stderr}")
        raise RuntimeError(f"Failed to generate {proto_path.name}")

    print(f"  [OK] {proto_path.name}")


def main() -> None:
    """Main function."""
    project_root = Path(__file__).parent.parent
    proto_dir = project_root / "proto"
    output_dir = project_root

    print("Generating gRPC Python code...")

    # Generate all proto files
    for proto_file in proto_dir.glob("*.proto"):
        generate_proto(str(proto_file), str(output_dir))

    print("\n[OK] Code generation complete!")
    print(f"Output directory: {output_dir}")


if __name__ == "__main__":
    main()
