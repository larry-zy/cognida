"""Internal lifecycle HTTP adapters with bounded requests and cancellation."""

from __future__ import annotations

import asyncio

from config import get_settings
from fastapi import APIRouter, HTTPException, Request
from starlette.responses import JSONResponse

from services.quality.dimension_names import STRUCTURED_DIMENSIONS, UNSTRUCTURED_DIMENSIONS

from . import compute, enrichment, images
from .enrichment import CapabilityUnavailableError
from .models import (
    CleanRequest,
    EmbedRequest,
    EnrichRequest,
    ImageIndexRequest,
    ImageSearchRequest,
    InferenceRequest,
    MergeRequest,
    ProfileRequest,
    RecommendRequest,
    ResizeRequest,
)

router = APIRouter(prefix="/api/v1/lifecycle", tags=["lifecycle-compute"])
MAX_BODY_BYTES = 20 * 1024 * 1024


class LifecycleBodyLimitMiddleware:
    """Bound streamed bodies before JSON parsing, including requests without Content-Length."""

    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http" or not scope["path"].startswith("/api/v1/lifecycle"):
            return await self.app(scope, receive, send)
        buffered = bytearray()
        while True:
            message = await receive()
            if message["type"] == "http.disconnect":
                return
            buffered.extend(message.get("body", b""))
            if len(buffered) > MAX_BODY_BYTES:
                return await JSONResponse(
                    {"error": {"code": "body_too_large", "retryable": False}}, status_code=413
                )(scope, receive, send)
            if not message.get("more_body", False):
                break
        delivered = False
        disconnected = asyncio.Event()
        scope["lifecycle_disconnected"] = disconnected

        async def watch_disconnect():
            while True:
                event = await receive()
                if event["type"] == "http.disconnect":
                    disconnected.set()
                    return

        watcher = asyncio.create_task(watch_disconnect())

        async def replay():
            nonlocal delivered
            if not delivered:
                delivered = True
                return {"type": "http.request", "body": bytes(buffered), "more_body": False}
            await disconnected.wait()
            return {"type": "http.disconnect"}

        try:
            await self.app(scope, replay, send)
        finally:
            watcher.cancel()
            await asyncio.gather(watcher, return_exceptions=True)


_cpu_call = compute.limited_cpu


async def _disconnected(request):
    event = getattr(request, "scope", {}).get("lifecycle_disconnected")
    if event is not None:
        await event.wait()
        return
    while not await request.is_disconnected():
        await asyncio.sleep(0.1)


async def execute(request: Request, operation):
    task = asyncio.create_task(operation)
    disconnect = asyncio.create_task(_disconnected(request))
    try:
        done, _ = await asyncio.wait(
            [task, disconnect],
            timeout=get_settings().lifecycle_timeout_seconds,
            return_when=asyncio.FIRST_COMPLETED,
        )
        if task in done:
            return await task
        if disconnect in done:
            raise HTTPException(499, detail={"code": "client_disconnected", "retryable": True})
        raise HTTPException(504, detail={"code": "compute_timeout", "retryable": True})
    except CapabilityUnavailableError as exc:
        raise HTTPException(
            503, detail={"code": "capability_unavailable", "message": str(exc), "retryable": True}
        ) from exc
    except ValueError as exc:
        raise HTTPException(
            422, detail={"code": "invalid_input", "message": str(exc), "retryable": False}
        ) from exc
    except HTTPException:
        raise
    except asyncio.CancelledError:
        raise
    except Exception as exc:
        raise HTTPException(503, detail={"code": "compute_failed", "retryable": True}) from exc
    finally:
        for pending in (task, disconnect):
            if not pending.done():
                pending.cancel()
        await asyncio.gather(task, disconnect, return_exceptions=True)


def tenant(request: Request) -> int:
    try:
        value = int(request.headers.get("X-Tenant-ID", ""))
        if not 0 < value < 2**63:
            raise ValueError
        return value
    except ValueError as exc:
        raise HTTPException(422, detail={"code": "tenant_required", "retryable": False}) from exc


@router.get("/capabilities")
async def capabilities():
    settings = get_settings()
    return {
        "schema_version": "1",
        "input_kinds": ["text", "table"],
        "structured_dimensions": list(STRUCTURED_DIMENSIONS),
        "text_dimensions": list(UNSTRUCTURED_DIMENSIONS),
        "clean_operations": [
            "trim",
            "normalize_ws",
            "drop_empty",
            "dedup",
            "text_cleaner",
            "pii_masker",
        ],
        "enrichment_methods": ["rules", "llm"],
        "operations": [
            "profile",
            "clean",
            "enrich",
            "merge",
            "recommend",
            "batch-inference",
            "images/resize",
            "images/embed",
            "images/index",
            "images/search",
        ],
        "configuration": {
            "llm_configured": bool(settings.llm_api_key),
            "image_model_configured": bool(settings.image_model_id),
            "image_store_configured": bool(settings.image_milvus_uri),
        },
        "limits": {
            "request_bytes": MAX_BODY_BYTES,
            "table_rows": 10000,
            "table_cells": 100000,
            "text_characters": 200000,
            "timeout_seconds": settings.lifecycle_timeout_seconds,
        },
        "ownership": "Go resolves authorized asset references and persists versions; Python computes bounded payloads.",
    }


@router.post("/profile")
async def profile(payload: ProfileRequest, request: Request):
    return await execute(request, asyncio.to_thread(_cpu_call, compute.profile, payload))


@router.post("/clean")
async def clean(payload: CleanRequest, request: Request):
    return await execute(request, asyncio.to_thread(_cpu_call, compute.clean, payload))


@router.post("/enrich")
async def enrich(payload: EnrichRequest, request: Request):
    return await execute(request, enrichment.enrich(payload))


@router.post("/merge")
async def merge(payload: MergeRequest, request: Request):
    return await execute(request, asyncio.to_thread(_cpu_call, compute.merge, payload))


@router.post("/recommend")
async def recommend(payload: RecommendRequest, request: Request):
    return await execute(request, asyncio.to_thread(_cpu_call, compute.recommend, payload))


@router.post("/batch-inference")
async def inference(payload: InferenceRequest, request: Request):
    return await execute(request, enrichment.infer_batch(payload))


@router.post("/images/resize")
async def resize(payload: ResizeRequest, request: Request):
    return await execute(request, asyncio.to_thread(_cpu_call, images.resize, payload))


@router.post("/images/embed")
async def embed(payload: EmbedRequest, request: Request):
    return await execute(request, images.embed(payload))


@router.post("/images/index")
async def index(payload: ImageIndexRequest, request: Request):
    tenant_id = tenant(request)
    return await execute(request, images.index_images(payload, tenant_id))


@router.post("/images/search")
async def search(payload: ImageSearchRequest, request: Request):
    tenant_id = tenant(request)
    return await execute(request, images.search_images(payload, tenant_id))
