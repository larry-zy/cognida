"""Task-owned loopback fixture: real lifecycle router, body limits and HTTP auth.

Run only with a fresh INTERNAL_HTTP_AUTH_TOKEN, APP_ENV=test and offline model
flags. Does not create databases, workers, model clients or third-party calls.
"""

import os

from fastapi import FastAPI

if not os.environ.get("INTERNAL_HTTP_AUTH_TOKEN") or os.environ.get("APP_ENV") != "test":
    raise RuntimeError("explicit test environment and internal token required")

from core.http_auth import InternalHTTPAuthMiddleware  # noqa: E402
from core.request_context import RequestIDMiddleware  # noqa: E402
from services.lifecycle.router import LifecycleBodyLimitMiddleware, router  # noqa: E402

app = FastAPI()
app.include_router(router)
app.add_middleware(LifecycleBodyLimitMiddleware)
app.add_middleware(InternalHTTPAuthMiddleware)
app.add_middleware(RequestIDMiddleware)


@app.get("/health")
def health():
    return {"status": "healthy", "fixture": "cleaning-benchmark"}
