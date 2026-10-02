"""Bounded Pillow processing and configured CLIP -> tenant-filtered Milvus search."""

from __future__ import annotations

import asyncio
import base64
import hashlib
import io
import json
import threading
from functools import lru_cache

from config import get_settings

from .compute import envelope
from .enrichment import CapabilityUnavailableError
from .models import EmbedRequest, ImageIndexRequest, ImageSearchRequest, ResizeRequest

MAX_PIXELS = 16_000_000
MAX_IMAGE_BYTES = 10 * 1024 * 1024
_MODEL_LOCK = threading.Lock()


def decode_image(encoded: str):
    from PIL import Image, ImageOps, UnidentifiedImageError

    try:
        raw = base64.b64decode(encoded, validate=True)
        if len(raw) > MAX_IMAGE_BYTES:
            raise ValueError("image exceeds 10 MiB")
        with Image.open(io.BytesIO(raw)) as source:
            if source.width * source.height > MAX_PIXELS:
                raise ValueError("image exceeds 16 million pixels")
            if source.format not in {"JPEG", "PNG", "WEBP"}:
                raise ValueError("only JPEG, PNG and WebP images are supported")
            return ImageOps.exif_transpose(source).convert("RGB")
    except (UnidentifiedImageError, OSError, Image.DecompressionBombError) as exc:
        raise ValueError("invalid or oversized image") from exc


def resize(request: ResizeRequest) -> dict:
    from PIL import Image, ImageOps

    if len({item.id for item in request.images}) != len(request.images):
        raise ValueError("image IDs must be unique")
    outputs = []
    total_output = 0
    for item in request.images:
        with decode_image(item.image_base64) as source:
            size = (request.width, request.height)
            if request.mode == "fit":
                result = ImageOps.fit(source, size, method=Image.Resampling.LANCZOS)
            elif request.mode == "contain":
                result = ImageOps.contain(source, size, method=Image.Resampling.LANCZOS)
            else:
                result = source.resize(size, Image.Resampling.LANCZOS)
            try:
                output = io.BytesIO()
                result.save(output, format=request.format.upper(), quality=90)
                raw = output.getvalue()
                total_output += len(raw)
                if total_output > 20 * 1024 * 1024:
                    raise ValueError("resized output exceeds 20 MiB")
                outputs.append(
                    {
                        "id": item.id,
                        "width": result.width,
                        "height": result.height,
                        "format": request.format,
                        "image_base64": base64.b64encode(raw).decode(),
                        "sha256": hashlib.sha256(raw).hexdigest(),
                    }
                )
            finally:
                result.close()
    return envelope(request, "pillow-lanczos-v1", {"images": outputs})


@lru_cache(maxsize=2)
def _load_model(model_id: str, revision: str, allow_download: bool):
    if not model_id:
        raise CapabilityUnavailableError("IMAGE_MODEL_ID is not configured")
    try:
        from transformers import CLIPImageProcessor, CLIPModel

        processor = CLIPImageProcessor.from_pretrained(
            model_id,
            revision=revision,
            local_files_only=not allow_download,
            trust_remote_code=False,
        )
        model = (
            CLIPModel.from_pretrained(
                model_id, revision=revision, local_files_only=not allow_download
            )
            .eval()
            .to("cpu")
        )
        return processor, model
    except Exception as exc:
        raise CapabilityUnavailableError(
            "Configured CLIP model/dependencies are unavailable; provision the model first"
        ) from exc


def _embed(images):
    import torch

    settings = get_settings()
    # Serialize model load/inference; request limits bound memory and the event loop stays responsive.
    if not _MODEL_LOCK.acquire(blocking=False):
        raise CapabilityUnavailableError("Image model is busy; retry later")
    try:
        processor, model = _load_model(
            settings.image_model_id,
            settings.image_model_revision,
            settings.image_allow_model_download,
        )
        decoded = []
        try:
            if len({image.id for image in images}) != len(images):
                raise ValueError("image IDs must be unique")
            for image in images:
                decoded.append(decode_image(image.image_base64))
            inputs = processor(images=decoded, return_tensors="pt")
            with torch.inference_mode():
                # Stable CLIP components across transformers versions (get_image_features return type changed).
                pooled = model.vision_model(pixel_values=inputs["pixel_values"]).pooler_output
                vectors = model.visual_projection(pooled)
                vectors = torch.nn.functional.normalize(vectors, p=2, dim=-1)
            if not torch.isfinite(vectors).all():
                raise CapabilityUnavailableError("Model produced non-finite embeddings")
            return vectors.cpu().tolist()
        finally:
            for image in decoded:
                image.close()

    finally:
        _MODEL_LOCK.release()


def model_identity() -> str:
    settings = get_settings()
    if not settings.image_model_id:
        raise CapabilityUnavailableError("IMAGE_MODEL_ID is not configured")
    return settings.image_model_id + "@" + settings.image_model_revision


async def embed(request: EmbedRequest) -> dict:
    identity = model_identity()
    vectors = await asyncio.to_thread(_embed, request.images)
    return envelope(
        request,
        "clip-image-embedding-v1",
        {
            "model": identity,
            "dimension": len(vectors[0]),
            "embeddings": [
                {"id": image.id, "vector": vector} for image, vector in zip(request.images, vectors)
            ],
        },
    )


def collection_name(identity: str, dimension: int) -> str:
    # Image spaces never reuse the text/knowledge collection; model + dimension define identity.
    suffix = hashlib.sha256(f"{identity}:{dimension}".encode()).hexdigest()[:16]
    return f"{get_settings().image_collection_prefix}_{suffix}"


def _milvus_client():
    settings = get_settings()
    if not settings.image_milvus_uri:
        raise CapabilityUnavailableError("IMAGE_MILVUS_URI is not configured")
    try:
        from pymilvus import AsyncMilvusClient

        return AsyncMilvusClient(
            uri=settings.image_milvus_uri, token=settings.image_milvus_token, timeout=10
        )
    except Exception as exc:
        raise CapabilityUnavailableError("Image Milvus connection is unavailable") from exc


async def ensure_collection(client, name, dimension):
    from pymilvus import DataType, MilvusClient

    if not await client.has_collection(name, timeout=10):
        schema = MilvusClient.create_schema(auto_id=False, enable_dynamic_field=False)
        schema.add_field(field_name="id", datatype=DataType.VARCHAR, is_primary=True, max_length=64)
        schema.add_field(field_name="tenant_id", datatype=DataType.INT64)
        schema.add_field(field_name="asset_id", datatype=DataType.VARCHAR, max_length=512)
        schema.add_field(field_name="asset_version", datatype=DataType.INT64)
        schema.add_field(field_name="image_id", datatype=DataType.VARCHAR, max_length=512)
        schema.add_field(field_name="vector", datatype=DataType.FLOAT_VECTOR, dim=dimension)
        indexes = MilvusClient.prepare_index_params()
        indexes.add_index(field_name="vector", index_type="AUTOINDEX", metric_type="COSINE")
        try:
            await client.create_collection(
                collection_name=name,
                schema=schema,
                index_params=indexes,
                consistency_level="Strong",
                timeout=10,
            )
        except Exception:
            # Concurrent first writers may create the same deterministic space.
            if not await client.has_collection(name, timeout=10):
                raise
    description = await client.describe_collection(name, timeout=10)
    fields = {field["name"]: field for field in description["fields"]}
    if (
        set(fields) != {"id", "tenant_id", "asset_id", "asset_version", "image_id", "vector"}
        or int(fields["vector"].get("params", {}).get("dim", 0)) != dimension
    ):
        raise CapabilityUnavailableError("Existing image collection has an incompatible schema")


async def index_images(request: ImageIndexRequest, tenant_id: int) -> dict:
    identity = model_identity()
    client = _milvus_client()
    try:
        vectors = await asyncio.to_thread(_embed, request.images)
        name = collection_name(identity, len(vectors[0]))
        await ensure_collection(client, name, len(vectors[0]))
        rows = [
            {
                "id": hashlib.sha256(
                    json.dumps(
                        [tenant_id, item.asset_id, item.asset_version, item.id], ensure_ascii=False
                    ).encode()
                ).hexdigest(),
                "tenant_id": tenant_id,
                "asset_id": item.asset_id,
                "asset_version": item.asset_version,
                "image_id": item.id,
                "vector": vector,
            }
            for item, vector in zip(request.images, vectors)
        ]
        await client.upsert(collection_name=name, data=rows, timeout=10)
        return envelope(
            request,
            "clip-milvus-upsert-v1",
            {
                "model": identity,
                "indexed": len(rows),
                "ids": [row["id"] for row in rows],
                "collection": name,
                "tenant_id": tenant_id,
            },
        )
    finally:
        await client.close()


async def search_images(request: ImageSearchRequest, tenant_id: int) -> dict:
    from .models import ImageItem

    identity = model_identity()
    client = _milvus_client()
    try:
        vectors = await asyncio.to_thread(
            _embed, [ImageItem(id="query", image_base64=request.image_base64)]
        )
        name = collection_name(identity, len(vectors[0]))
        if not await client.has_collection(name, timeout=10):
            return envelope(
                request,
                "clip-milvus-search-v1",
                {"matches": [], "model": identity, "collection": name},
            )
        # No caller-provided expressions: tenant is an integer, IDs are JSON string literals.
        predicate = f"tenant_id == {tenant_id} and asset_id in {json.dumps(request.candidate_asset_ids, ensure_ascii=False)}"
        matches = await client.search(
            collection_name=name,
            data=vectors,
            filter=predicate,
            limit=request.limit,
            output_fields=["asset_id", "asset_version", "image_id"],
            consistency_level="Strong",
            timeout=10,
        )
        return envelope(
            request,
            "clip-milvus-search-v1",
            {
                "model": identity,
                "collection": name,
                "matches": [
                    {"score": match["distance"], **match["entity"]} for match in matches[0]
                ],
            },
        )
    finally:
        await client.close()
