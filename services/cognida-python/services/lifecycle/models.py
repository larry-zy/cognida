"""Version-one internal compute contracts. Reject unknown options explicitly."""

from __future__ import annotations

import json
import math
from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

Scalar = str | int | float | bool | None


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class TextInput(StrictModel):
    kind: Literal["text"]
    text: str = Field(min_length=1, max_length=200_000)


class TableInput(StrictModel):
    kind: Literal["table"]
    columns: list[str] = Field(min_length=1, max_length=200)
    rows: list[list[Scalar]] = Field(min_length=1, max_length=10_000)

    @model_validator(mode="after")
    def rectangular(self):
        if len(set(self.columns)) != len(self.columns) or any(
            not c or len(c) > 200 for c in self.columns
        ):
            raise ValueError("columns must be unique nonempty names of at most 200 characters")
        if any(len(row) != len(self.columns) for row in self.rows):
            raise ValueError("every row must match the column count")
        if sum(len(row) for row in self.rows) > 100_000:
            raise ValueError("table exceeds 100000 cells")
        for row in self.rows:
            for value in row:
                if isinstance(value, float) and not math.isfinite(value):
                    raise ValueError("non-finite numbers are not supported")
                if isinstance(value, str) and len(value) > 20_000:
                    raise ValueError("cell exceeds 20000 characters")
        if len(json.dumps(self.model_dump(), ensure_ascii=False).encode()) > 4 * 1024 * 1024:
            raise ValueError("table exceeds 4 MiB")
        return self


DataInput = Annotated[TextInput | TableInput, Field(discriminator="kind")]


class ProfileOptions(StrictModel):
    dimensions: list[str] | None = Field(default=None, max_length=12)
    reference_text: str | None = Field(default=None, max_length=20_000)


class ProfileRequest(StrictModel):
    input: DataInput
    options: ProfileOptions = Field(default_factory=ProfileOptions)


Operation = Literal["trim", "normalize_ws", "drop_empty", "dedup", "text_cleaner", "pii_masker"]


class CleanOptions(StrictModel):
    operations: list[Operation] = Field(
        default_factory=lambda: ["trim", "dedup"], min_length=1, max_length=12
    )
    columns: list[str] | None = Field(default=None, max_length=200)
    keep: Literal["first", "last"] = "first"


class CleanRequest(StrictModel):
    input: DataInput
    options: CleanOptions = Field(default_factory=CleanOptions)


class Label(StrictModel):
    name: str = Field(min_length=1, max_length=80)
    keywords: list[str] = Field(min_length=1, max_length=30)

    @model_validator(mode="after")
    def bound_keywords(self):
        if any(not word.strip() or len(word) > 100 for word in self.keywords):
            raise ValueError("label keywords must contain 1-100 characters")
        return self


Feature = Literal["tags", "entities", "keywords", "summary", "category", "sentiment", "sensitivity"]


class EnrichOptions(StrictModel):
    method: Literal["rules", "llm"] = "rules"
    labels: list[Label] = Field(default_factory=list, max_length=30)
    max_keywords: int = Field(default=10, ge=1, le=30)
    max_summary_chars: int = Field(default=300, ge=20, le=2000)
    features: list[Feature] = Field(
        default_factory=lambda: [
            "tags",
            "entities",
            "keywords",
            "summary",
            "category",
            "sentiment",
            "sensitivity",
        ],
        min_length=1,
        max_length=7,
    )


class EnrichRequest(StrictModel):
    input: DataInput
    options: EnrichOptions = Field(default_factory=EnrichOptions)


class AnalyzeOptions(StrictModel):
    prompt: str = Field(min_length=1, max_length=4000)
    source_ref: str = Field(min_length=1, max_length=128)
    statistics: dict = Field(default_factory=dict)

    @model_validator(mode="after")
    def bounded_statistics(self):
        if not self.prompt.strip():
            raise ValueError("analysis goal must not be blank")
        if len(json.dumps(self.statistics, ensure_ascii=False, allow_nan=False)) > 100_000:
            raise ValueError("statistics exceed analysis context limit")
        return self


class AnalyzeRequest(StrictModel):
    input: DataInput
    options: AnalyzeOptions


class MergeOptions(StrictModel):
    schema_policy: Literal["strict", "union"] = "strict"
    deduplicate: bool = False
    keys: list[str] | None = Field(default=None, max_length=200)


class MergeRequest(StrictModel):
    inputs: list[TableInput] = Field(min_length=2, max_length=10)
    options: MergeOptions = Field(default_factory=MergeOptions)

    @model_validator(mode="after")
    def total_limit(self):
        if sum(len(table.rows) for table in self.inputs) > 10_000:
            raise ValueError("merged dataset exceeds 10000 rows")
        return self


class Candidate(StrictModel):
    asset_id: str = Field(min_length=1, max_length=128)
    title: str = Field(min_length=1, max_length=300)
    description: str = Field(default="", max_length=2000)
    tags: list[str] = Field(default_factory=list, max_length=50)

    @model_validator(mode="after")
    def tag_limit(self):
        if any(len(tag) > 100 for tag in self.tags):
            raise ValueError("tag exceeds 100 characters")
        return self


class RecommendRequest(StrictModel):
    query: str = Field(min_length=1, max_length=2000)
    candidates: list[Candidate] = Field(min_length=1, max_length=200)
    limit: int = Field(default=10, ge=1, le=50)

    @model_validator(mode="after")
    def unique_candidates(self):
        if len({c.asset_id for c in self.candidates}) != len(self.candidates):
            raise ValueError("candidate asset IDs must be unique")
        return self


class InferenceItem(StrictModel):
    id: str = Field(min_length=1, max_length=128)
    prompt: str = Field(min_length=1, max_length=20_000)


class InferenceRequest(StrictModel):
    items: list[InferenceItem] = Field(min_length=1, max_length=20)
    concurrency: int = Field(default=2, ge=1, le=4)
    max_tokens: int = Field(default=512, ge=1, le=2048)
    temperature: float = Field(default=0.2, ge=0, le=2)

    @model_validator(mode="after")
    def unique_items(self):
        if len({item.id for item in self.items}) != len(self.items):
            raise ValueError("inference item IDs must be unique")
        return self


class ImageItem(StrictModel):
    id: str = Field(min_length=1, max_length=128)
    image_base64: str = Field(min_length=1, max_length=14_000_000)


class ResizeRequest(StrictModel):
    images: list[ImageItem] = Field(min_length=1, max_length=8)
    width: int = Field(ge=1, le=4096)
    height: int = Field(ge=1, le=4096)
    mode: Literal["fit", "contain", "stretch"] = "contain"
    format: Literal["png", "jpeg", "webp"] = "png"


class EmbedRequest(StrictModel):
    images: list[ImageItem] = Field(min_length=1, max_length=8)


class IndexImage(ImageItem):
    asset_id: str = Field(min_length=1, max_length=128)
    asset_version: int = Field(ge=1)


class ImageIndexRequest(StrictModel):
    images: list[IndexImage] = Field(min_length=1, max_length=8)


class ImageSearchRequest(StrictModel):
    image_base64: str = Field(min_length=1, max_length=14_000_000)
    # A mandatory bounded allowlist is resolved/authorized by Go, never an arbitrary Milvus filter.
    candidate_asset_ids: list[str] = Field(min_length=1, max_length=200)
    limit: int = Field(default=10, ge=1, le=50)

    @model_validator(mode="after")
    def bound_ids(self):
        if any(not value or len(value) > 128 for value in self.candidate_asset_ids):
            raise ValueError("candidate IDs must contain 1-128 characters")
        return self
