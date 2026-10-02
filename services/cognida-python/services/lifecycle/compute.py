"""Reusable quality/cleaning engines plus bounded dataset transformations."""

from __future__ import annotations

import hashlib
import json
import math
import re
import threading
from collections import Counter
from dataclasses import asdict
from enum import Enum

import pandas as pd

from services.quality import cleaning as _cleaning  # noqa: F401
from services.quality.cleaning.pii_masker import PIIMasker
from services.quality.dimension_names import STRUCTURED_DIMENSIONS, UNSTRUCTURED_DIMENSIONS
from services.quality.evaluator import DataQualityEvaluator
from services.quality.registry import CleanerRegistry

from .models import CleanRequest, MergeRequest, ProfileRequest, RecommendRequest


def json_safe(value):
    if isinstance(value, dict):
        return {str(k): json_safe(v) for k, v in value.items()}
    if isinstance(value, (tuple, list)):
        return [json_safe(v) for v in value]
    if isinstance(value, Enum):
        return value.value
    if hasattr(value, "item"):
        return json_safe(value.item())
    if isinstance(value, float) and not math.isfinite(value):
        return None
    if hasattr(value, "isoformat"):
        return value.isoformat()
    return value


def envelope(payload, method: str, result: dict, warnings=None) -> dict:
    source = getattr(payload, "input", payload)
    raw = source.model_dump(mode="json") if hasattr(source, "model_dump") else source
    encoded = json.dumps(
        raw, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False
    )
    return {
        "schema_version": "1",
        "input_digest": hashlib.sha256(encoded.encode()).hexdigest(),
        "method": method,
        "result": json_safe(result),
        "warnings": warnings or [],
    }


def dataframe(table) -> pd.DataFrame:
    # object dtype preserves identifiers, booleans and explicit nulls without inference.
    return pd.DataFrame(table.rows, columns=table.columns, dtype=object)


def table_output(frame: pd.DataFrame) -> dict:
    rows = json_safe(frame.values.tolist())
    return {"kind": "table", "columns": list(frame.columns), "rows": rows}


def tokens(text: str) -> list[str]:
    from services.evaluation.metrics.tokenizer import tokenize_mixed

    return [word.lower() for word in tokenize_mixed(text) if re.search(r"[\w\u4e00-\u9fff]", word)]


def sensitivity(text: str) -> dict:
    findings = []
    for kind, pattern in [
        ("phone", PIIMasker.PHONE_PATTERN),
        ("email", PIIMasker.EMAIL_PATTERN),
        ("id_card", PIIMasker.ID_CARD_PATTERN),
    ]:
        for match in pattern.finditer(text):
            findings.append({"type": kind, "start": match.start(), "end": match.end()})
            if len(findings) >= 50:
                break
    return {
        "level": "sensitive" if findings else "none_detected",
        "method": "builtin-pii-patterns",
        "confidence": 0.85 if findings else 0.5,
        "evidence": findings[:50],
        "recommendation": "mask_or_review" if findings else "review_before_publication",
    }


def profile(request: ProfileRequest) -> dict:
    structured = request.input.kind == "table"
    allowed = set(STRUCTURED_DIMENSIONS if structured else UNSTRUCTURED_DIMENSIONS)
    dimensions = request.options.dimensions
    if dimensions is not None and (
        not dimensions or any(name not in allowed for name in dimensions)
    ):
        raise ValueError("unsupported or empty dimension selection for input kind")
    evaluator = DataQualityEvaluator()
    if structured:
        report = evaluator.evaluate_structured(
            dataframe(request.input).infer_objects(), dimensions=dimensions
        )
        text = "\n".join(
            str(value) for row in request.input.rows for value in row if value is not None
        )
    else:
        text = request.input.text
        if dimensions and "relevance" in dimensions and not request.options.reference_text:
            raise ValueError("relevance requires reference_text")
        config = (
            {"expected_keywords": tokens(request.options.reference_text)}
            if request.options.reference_text
            else {}
        )
        report = evaluator.evaluate_unstructured(text, dimensions=dimensions, config=config)
    result = asdict(report)
    # Reports may become agent-visible evidence; mask any raw sample/snippet values.
    masker = PIIMasker()
    result["issues"] = result.get("issues", [])[:50]
    for issue in result["issues"]:
        if issue.get("sample"):
            issue["sample"] = [masker.mask_text(str(value))[:160] for value in issue["sample"][:5]]
        if issue.get("snippet"):
            issue["snippet"] = masker.mask_text(issue["snippet"])[:160]
    for dimension in result.get("dimensions", []):
        dimension["issues"] = [
            issue
            for issue in result["issues"]
            if issue.get("dimension", issue.get("type")) == dimension["name"]
        ][:10]
    result["sensitivity_recommendation"] = sensitivity(text)
    return envelope(request, "quality-engine-v1", result)


def clean(request: CleanRequest) -> dict:
    is_text = request.input.kind == "text"
    frame = (
        pd.DataFrame({"text": request.input.text.splitlines() or [request.input.text]})
        if is_text
        else dataframe(request.input)
    )
    original = frame.copy(deep=True)
    selected = request.options.columns
    if selected is not None and (
        not selected or any(column not in frame.columns for column in selected)
    ):
        raise ValueError("cleaning columns must belong to the input")
    operations = []
    for name in request.options.operations:
        cls = CleanerRegistry.get(name)
        if cls is None:
            raise ValueError(f"cleaner is not registered: {name}")
        config = {"columns": selected, "subset": selected, "text_column": None}
        cleaner = cls()
        if name == "dedup":
            # Reversing twice lets the existing keep-first engine preserve the last occurrence.
            data = frame.iloc[::-1] if request.options.keep == "last" else frame
            result = cleaner.clean(data, config)
            frame = result.cleaned_data
            if request.options.keep == "last":
                frame = frame.iloc[::-1]
        elif selected and name in {"trim", "normalize_ws", "text_cleaner"}:
            result = cleaner.clean(frame[selected], config)
            frame = frame.copy()
            frame[selected] = result.cleaned_data
        else:
            if name == "pii_masker":
                frame = frame.copy()
                for column in selected or frame.columns:
                    frame[column] = frame[column].map(
                        lambda value: (
                            str(value)
                            if value is not None and sensitivity(str(value))["evidence"]
                            else value
                        )
                    )
            result = cleaner.clean(frame, config)
            frame = result.cleaned_data
        if frame is None:
            raise RuntimeError("cleaner did not produce transformed data")
        operations.extend(asdict(operation) for operation in result.operations)
    output = (
        {"kind": "text", "text": "\n".join(frame["text"].astype(str))}
        if is_text
        else table_output(frame)
    )
    changed = output != request.input.model_dump(mode="json")
    return envelope(
        request,
        "cleaner-registry-v1",
        {
            "data": output,
            "operations": operations,
            "original_count": len(original),
            "cleaned_count": len(frame),
            "removed_count": len(original) - len(frame),
            "changed": changed,
            "source_row_indices": [int(index) for index in frame.index],
        },
    )


def merge(request: MergeRequest) -> dict:
    columns = list(request.inputs[0].columns)
    for table in request.inputs[1:]:
        if request.options.schema_policy == "strict" and table.columns != columns:
            raise ValueError("strict merge requires identical ordered schemas")
        for column in table.columns:
            if column not in columns:
                columns.append(column)
    if len(columns) > 200:
        raise ValueError("merged schema exceeds 200 columns")
    if request.options.keys and any(key not in columns for key in request.options.keys):
        raise ValueError("merge keys must belong to the merged schema")
    frame = pd.concat(
        [dataframe(table).reindex(columns=columns) for table in request.inputs], ignore_index=True
    )
    if frame.size > 100_000:
        raise ValueError("merged dataset exceeds 100000 cells")
    provenance = [
        {"input_index": index, "row_index": row}
        for index, table in enumerate(request.inputs)
        for row in range(len(table.rows))
    ]
    original_count = len(frame)
    if request.options.deduplicate:
        frame = frame.drop_duplicates(subset=request.options.keys, keep="first")
    return envelope(
        request,
        "schema-aligned-concat-v1",
        {
            "data": table_output(frame),
            "original_count": original_count,
            "cleaned_count": len(frame),
            "removed_count": original_count - len(frame),
            "row_provenance": [provenance[index] for index in frame.index],
            "schema_fills": [
                {
                    "input_index": index,
                    "missing_columns": [c for c in columns if c not in table.columns],
                }
                for index, table in enumerate(request.inputs)
            ],
        },
    )


def recommend(request: RecommendRequest) -> dict:
    query = Counter(tokens(request.query))
    scores = []
    for candidate in request.candidates:
        words = Counter(tokens(" ".join([candidate.title, candidate.description, *candidate.tags])))
        common = sorted(query.keys() & words.keys())
        denominator = math.sqrt(
            sum(v * v for v in query.values()) * sum(v * v for v in words.values())
        )
        score = (
            sum(query[word] * words[word] for word in common) / denominator if denominator else 0.0
        )
        if score:
            scores.append(
                {
                    "asset_id": candidate.asset_id,
                    "score": score,
                    "evidence": {"matched_terms": common[:20]},
                }
            )
    scores.sort(key=lambda item: (-item["score"], item["asset_id"]))
    return envelope(
        request,
        "metadata-token-cosine-v1",
        {
            "recommendations": scores[: request.limit],
            "candidate_count": len(request.candidates),
            "scope": "caller-authorized-candidates-only",
        },
    )


_CPU_CAPACITY = threading.BoundedSemaphore(4)


def limited_cpu(function, payload):
    from .enrichment import CapabilityUnavailableError

    if not _CPU_CAPACITY.acquire(blocking=False):
        raise CapabilityUnavailableError("Compute capacity is busy; retry later")
    try:
        return function(payload)
    finally:
        _CPU_CAPACITY.release()
