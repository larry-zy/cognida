"""应用入口点。"""

import uvicorn

from config import get_settings
from core.app import create_app


def main() -> None:
    """启动应用。"""
    settings = get_settings()
    app = create_app()

    uvicorn.run(
        app,
        host=settings.api_host,
        port=settings.api_port,
        log_level=settings.log_level.lower(),
        access_log=True,
    )


if __name__ == "__main__":
    main()
