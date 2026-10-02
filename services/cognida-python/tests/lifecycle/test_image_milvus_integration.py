"""Actual local CLIP math and Milvus; random tiny weights test plumbing, not semantic quality."""

import base64
import io
import os
import uuid

import pytest
from config import get_settings
from PIL import Image
from services.lifecycle import images
from services.lifecycle.models import ImageIndexRequest, ImageSearchRequest


@pytest.mark.integration
@pytest.mark.skipif(
    os.getenv("RUN_IMAGE_MILVUS_INTEGRATION") != "1", reason="explicit local Milvus opt-in"
)
async def test_real_clip_milvus_tenant_candidates_and_retry(tmp_path, monkeypatch):
    import torch
    from pymilvus import AsyncMilvusClient
    from transformers import CLIPConfig, CLIPImageProcessor, CLIPModel

    torch.manual_seed(7)
    config = CLIPConfig(
        text_config={
            "vocab_size": 20,
            "bos_token_id": 0,
            "eos_token_id": 1,
            "hidden_size": 16,
            "intermediate_size": 32,
            "num_hidden_layers": 1,
            "num_attention_heads": 2,
            "max_position_embeddings": 16,
        },
        vision_config={
            "hidden_size": 16,
            "intermediate_size": 32,
            "num_hidden_layers": 1,
            "num_attention_heads": 2,
            "image_size": 16,
            "patch_size": 8,
        },
        projection_dim=8,
    )
    CLIPModel(config).save_pretrained(tmp_path)
    CLIPImageProcessor(
        size={"shortest_edge": 16}, crop_size={"height": 16, "width": 16}
    ).save_pretrained(tmp_path)
    settings = get_settings()
    monkeypatch.setattr(settings, "image_model_id", str(tmp_path))
    monkeypatch.setattr(settings, "image_model_revision", "test")
    monkeypatch.setattr(settings, "image_milvus_uri", "http://localhost:19530")
    monkeypatch.setattr(settings, "image_milvus_token", "")
    monkeypatch.setattr(settings, "image_collection_prefix", "it_image_" + uuid.uuid4().hex)
    client = AsyncMilvusClient(uri="http://localhost:19530", timeout=10)
    name = images.collection_name(images.model_identity(), 8)

    def encoded(color):
        out = io.BytesIO()
        Image.new("RGB", (20, 10), color).save(out, format="PNG")
        return base64.b64encode(out.getvalue()).decode()

    red = encoded("red")
    try:
        request = ImageIndexRequest(
            images=[{"id": "first", "asset_id": "allowed", "asset_version": 1, "image_base64": red}]
        )
        one = await images.index_images(request, 101)
        retry = await images.index_images(request, 101)
        assert one["result"]["ids"] == retry["result"]["ids"]
        await images.index_images(request, 202)
        await images.index_images(
            ImageIndexRequest(
                images=[
                    {"id": "second", "asset_id": "hidden", "asset_version": 1, "image_base64": red}
                ]
            ),
            101,
        )
        search = ImageSearchRequest(image_base64=red, candidate_asset_ids=["allowed"])
        result = await images.search_images(search, 101)
        assert len(result["result"]["matches"]) == 1
        assert result["result"]["matches"][0]["asset_id"] == "allowed"
        assert result["result"]["matches"][0]["score"] > 0.99
        assert (await images.search_images(search, 303))["result"]["matches"] == []
        # A crafted candidate literal cannot expand the filter.
        injected = ImageSearchRequest(
            image_base64=red,
            candidate_asset_ids=['allowed" or tenant_id == 202 or asset_id == "hidden'],
        )
        assert (await images.search_images(injected, 101))["result"]["matches"] == []
    finally:
        if await client.has_collection(name, timeout=10):
            await client.drop_collection(name, timeout=10)
        await client.close()
        images._load_model.cache_clear()
