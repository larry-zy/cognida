"""Filesystem boundary for paths supplied by remote callers."""

from pathlib import Path

from config import get_settings


def validated_input_path(value: str) -> str:
    """Resolve symlinks and require a regular file inside a configured root."""
    path = Path(value).expanduser().resolve()
    roots = [Path(root).expanduser().resolve() for root in get_settings().allowed_paths]
    if not any(path.is_relative_to(root) for root in roots):
        raise ValueError("File path is outside the configured allowed paths")
    if not path.is_file():
        raise ValueError("Input file does not exist or is not a regular file")
    return str(path)
