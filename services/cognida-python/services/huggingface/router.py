"""Authenticated internal bytes-only decoder; never downloads data."""
import asyncio
import threading

from fastapi import APIRouter, HTTPException, Request
from starlette.responses import JSONResponse
from pydantic import ValidationError
from core.http_auth import authorized_internal_request
from .decode import DecodeRequest, decode

router = APIRouter(prefix="/api/v1/connectors/huggingface", tags=["huggingface-decode"])
_capacity = threading.BoundedSemaphore(2)


@router.post("/decode")
async def decode_file(request: Request):
    if not authorized_internal_request(request.headers.get("authorization")):
        raise HTTPException(401, "unauthorized")
    if not _capacity.acquire(blocking=False):
        raise HTTPException(503, "decode capacity unavailable")
    try:
        buffer = bytearray()
        async with asyncio.timeout(30):
            async for chunk in request.stream():
                buffer.extend(chunk)
                if len(buffer) > 12 * 1024 * 1024:
                    raise HTTPException(413, "decode body limit")
        payload = DecodeRequest.model_validate_json(buffer)
        # Keep the permit until the real worker exits, even if HTTP cancellation
        # stops waiting. asyncio cancellation cannot kill a native arrow call.
        def work():
            try:
                return decode(payload)
            finally:
                _capacity.release()
        task = asyncio.create_task(asyncio.to_thread(work))
        task.add_done_callback(lambda done: done.exception() if not done.cancelled() else None)
        buffer = None
        result = await asyncio.shield(task)
        return JSONResponse(result)
    except (ValueError, ValidationError, UnicodeError, RecursionError, OSError):
        raise HTTPException(422, "invalid static dataset file or mapping") from None
    except TimeoutError:
        raise HTTPException(408, "decode body timeout") from None
    finally:
        if buffer is not None:
            _capacity.release()
