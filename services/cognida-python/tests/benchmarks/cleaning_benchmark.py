"""Offline, pinned-corpus benchmark of the real lifecycle cleaning HTTP contract.

Never downloads data. Original HF records are immutable gold; injected duplicate,
padding and empty rows are synthetic test defects, not labels of natural dirt.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import sys
import time
from collections import Counter
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import urlparse

if __name__ == "__main__":
    sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

PREFIX = "/api/v1/lifecycle/clean"
COLUMNS = [
    "source_id",
    "hf_record_json",
    "hf_text",
    "working_text",
    "decimal_guard",
    "identifier_guard",
    "integer_guard",
    "bool_guard",
    "null_guard",
]
TYPES = {"decimal_guard": "DECIMAL", "identifier_guard": "VARCHAR", "integer_guard": "BIGINT"}
LIMITATIONS = [
    "HF original rows are not ground truth for naturally dirty data.",
    "Duplicate/padding/empty-row defects and precision guards are artificial.",
    "source_id protects distinct original records, including naturally repeated text.",
    "Intake checks pinned local file integrity; this does not test Go collection, storage or approval.",
    "Each HTTP request is bounded; no cross-batch deduplication is claimed.",
]


class CleaningHTTPError(RuntimeError):
    def __init__(self, status: int):
        self.status = status
        super().__init__(f"clean HTTP status {status}; response body omitted")


def encoded(value) -> bytes:
    return json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False
    ).encode("utf-8")


def digest(value) -> str:
    return hashlib.sha256(encoded(value)).hexdigest()


def load_source(source: dict, manifest_dir: Path) -> tuple[list[dict], dict]:
    """Verify bytes before parsing; a revision is required, never inferred as main."""
    if not isinstance(source.get("dataset_id"), str) or not source["dataset_id"]:
        raise ValueError("dataset_id is required")
    if not re.fullmatch(r"[0-9a-f]{40}", source.get("revision", "")):
        raise ValueError("an exact 40-character dataset revision is required")
    if not re.fullmatch(r"[0-9a-f]{64}", source.get("sha256", "")):
        raise ValueError("an exact source SHA256 is required")
    if not isinstance(source.get("split"), str) or not source["split"]:
        raise ValueError("split is required")
    path = manifest_dir / source["path"]
    if path.stat().st_size > 100 * 1024 * 1024:
        raise ValueError("source exceeds the 100 MiB local benchmark bound")
    raw = path.read_bytes()
    actual_hash = hashlib.sha256(raw).hexdigest()
    if actual_hash != source["sha256"]:
        raise ValueError("source SHA256 mismatch; benchmark not run")
    if source.get("format") == "parquet":
        import pyarrow.parquet as pq

        records = pq.read_table(path).to_pylist()
    elif source.get("format") == "jsonl":
        records = [json.loads(line) for line in raw.decode("utf-8").splitlines() if line.strip()]
    else:
        raise ValueError("source format must be parquet or jsonl")
    field = source.get("text_field", "text")
    if not records or len(records) > 100_000:
        raise ValueError("source must contain 1..100000 complete records")
    if "rows" in source and (type(source["rows"]) is not int or source["rows"] != len(records)):
        raise ValueError("declared source row count does not match complete records")
    if any(not isinstance(row, dict) or not isinstance(row.get(field), str) for row in records):
        raise ValueError("every original record must contain the declared string text field")
    # Nested source fields are preserved as canonical JSON, never flattened or inferred.
    digest(records)
    provenance = {key: source[key] for key in ("dataset_id", "revision", "split", "format")}
    provenance["license"] = source.get("license", "unspecified in local manifest")
    provenance.update(
        file_sha256=actual_hash,
        original_records_sha256=digest(records),
        original_rows=len(records),
        text_field=field,
    )
    return records, provenance


def bind_raw_records(records: list[dict], source: dict, raw_files: list[dict], manifest_dir: Path):
    """Check the downloaded bytes and exact text, retaining raw fields as gold.

    These two approved corpora publish JSONL: banking77 uses split.jsonl and
    MASSIVE uses split/zh-CN.json.gz. This is explicit, not guessed from content.
    """
    names = {source["split"] + ".jsonl", source["split"] + "/zh-CN.json.gz"}
    matches = [item for item in raw_files if item.get("source_path") in names]
    if len(matches) != 1:
        raise ValueError("exactly one declared raw JSONL source is required per split")
    declared = matches[0]
    path = manifest_dir / declared["path"]
    if path.stat().st_size > 100 * 1024 * 1024:
        raise ValueError("raw source exceeds benchmark size bound")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != declared["sha256"]:
        raise ValueError("raw source SHA256 mismatch")
    if path.suffix == ".gz":
        with gzip.open(path, "rb") as stream:
            raw = stream.read(100 * 1024 * 1024 + 1)
        if len(raw) > 100 * 1024 * 1024:
            raise ValueError("expanded raw source exceeds benchmark size bound")
    original = [json.loads(line) for line in raw.decode("utf-8").splitlines() if line.strip()]
    if len(original) != len(records) or any(
        not isinstance(row, dict)
        or row.get(source.get("text_field", "text")) != record[source.get("text_field", "text")]
        for row, record in zip(original, records, strict=False)
    ):
        raise ValueError("normalized text does not exactly match downloaded HF original")
    # Reserved benchmark-only field protects original labels/lang/IDs as well as
    # text, without discarding normalized identifiers or claiming they were raw.
    if any("_hf_original" in row for row in records):
        raise ValueError("reserved benchmark field already present")
    bound = [
        {**record, "_hf_original": raw_record}
        for record, raw_record in zip(records, original, strict=True)
    ]
    return bound, {
        "downloaded_file_sha256": declared["sha256"],
        "downloaded_records_sha256": digest(original),
        "exact_original_text_match": True,
        "original_fields_protected_in": "hf_record_json._hf_original",
    }


def table(rows: list[list]) -> dict:
    return {"kind": "table", "columns": COLUMNS, "rows": rows, "column_types": TYPES}


def gold_rows(records: list[dict], text_field: str, offset: int) -> list[list]:
    return [
        [
            str(offset + i),
            encoded(record).decode(),
            record[text_field],
            record[text_field],
            "9007199254740993.123456789012345678",
            "00123",
            9007199254740993,
            bool((offset + i) % 2),
            None,
        ]
        for i, record in enumerate(records)
    ]


def perturb(rows: list[list]) -> tuple[list[list], dict]:
    """Only edge-clean strings are padded, so their exact original is the oracle."""
    dirty, duplicates, blanks, padded = [], [], [], []
    for i, original in enumerate(rows):
        row = list(original)
        if i % 3 == 0:
            row[3] = " \t\u2003" + row[3] + "\u00a0 \n"
            padded.append(len(dirty))
        dirty.append(row)
        if i % 5 == 0:
            duplicates.append(len(dirty))
            dirty.append(list(row))
            if i % 3 == 0:
                padded.append(len(dirty) - 1)
        if i % 13 == 0:
            blanks.append(len(dirty))
            dirty.append([None] * len(COLUMNS))
    return dirty, {
        "artificial_duplicate_indices": duplicates,
        "artificial_empty_indices": blanks,
        "artificial_padding_indices": padded,
    }


def oracle(dirty: list[list], keep: str) -> tuple[list[list], list[int]]:
    # Deliberately independent of Pandas and the production CleanerRegistry.
    normalized = [list(row) for row in dirty]
    for row in normalized:
        if isinstance(row[3], str):
            row[3] = row[3].strip()
    chosen = {}
    for index, row in enumerate(normalized):
        if all(value is None for value in row):
            continue
        key = encoded(row)
        if keep == "last" or key not in chosen:
            chosen[key] = index
    indices = sorted(chosen.values())
    return [normalized[index] for index in indices], indices


def score(expected: dict, expected_indices: list[int], result: dict) -> dict:
    actual = result.get("data", {})
    actual_rows = actual.get("rows", [])
    indices = result.get("source_row_indices", [])
    valid_indices = all(type(value) is int for value in indices)
    expected_count, actual_count = (
        Counter(expected_indices),
        Counter(indices) if valid_indices else Counter(),
    )
    false_deletions = sum((expected_count - actual_count).values())
    missed_deletions = sum((actual_count - expected_count).values())
    gold = {row[0]: row for row in expected["rows"]}
    field_errors = guard_errors = unknown_rows = 0
    for row in actual_rows:
        if (
            not isinstance(row, list)
            or not row
            or not isinstance(row[0], str)
            or row[0] not in gold
        ):
            unknown_rows += 1
            continue
        original = gold[row[0]]
        field_errors += int(encoded(row[:4]) != encoded(original[:4]))
        guard_errors += int(encoded(row[4:]) != encoded(original[4:]))
    return {
        "false_deletions": false_deletions,
        "missed_deletions": missed_deletions,
        "field_preservation_errors": field_errors,
        "exact_value_or_type_errors": guard_errors,
        "unexpected_rows": unknown_rows,
        "lineage_errors": int(not valid_indices or indices != expected_indices),
        "schema_errors": int(
            actual.get("columns") != expected["columns"]
            or actual.get("column_types") != expected["column_types"]
        ),
        "output_mismatch": int(encoded(actual) != encoded(expected)),
    }


def post_clean(client, payload: dict) -> tuple[dict, float]:
    started = time.perf_counter()
    response = client.post(PREFIX, json=payload)
    elapsed = time.perf_counter() - started
    if response.status_code != 200:
        # Validation errors can echo the complete input: never copy bodies into artifacts.
        raise CleaningHTTPError(response.status_code)
    body = response.json()
    if body.get("schema_version") != "1" or body.get("method") != "cleaner-registry-v1":
        raise RuntimeError("unexpected cleaning response contract")
    if body.get("input_digest") != digest(payload["input"]):
        raise RuntimeError("HTTP input digest mismatch")
    return body["result"], elapsed


def benchmark_records(client, records: list[dict], text_field="text", batch_size=2000) -> dict:
    if not records:
        raise ValueError("empty corpus cannot establish a cleaning benchmark")
    if not 1 <= batch_size <= 2000:
        raise ValueError("batch_size must be 1..2000")
    batches = []
    original_hash = digest(records)
    for offset in range(0, len(records), batch_size):
        gold = gold_rows(records[offset : offset + batch_size], text_field, offset)
        baseline = table(gold)
        actual, duration = post_clean(
            client, {"input": baseline, "options": {"operations": ["dedup"]}}
        )
        baseline_score = score(baseline, list(range(len(gold))), actual)
        # Original leading/trailing whitespace is not assumed to be an injected defect.
        eligible = [row for row in gold if row[3] == row[3].strip()]
        cases = []
        if eligible:
            dirty, changes = perturb(eligible)
            for keep in ("first", "last"):
                trimmed, trim_time = post_clean(
                    client,
                    {
                        "input": table(dirty),
                        "options": {"operations": ["trim"], "columns": ["working_text"]},
                    },
                )
                expected_rows, indices = oracle(dirty, keep)
                expected = table(expected_rows)
                options = {"operations": ["drop_empty", "dedup"], "keep": keep}
                cleaned, clean_time = post_clean(
                    client, {"input": trimmed["data"], "options": options}
                )
                repeated, repeat_time = post_clean(
                    client, {"input": cleaned["data"], "options": options}
                )
                metrics = score(expected, indices, cleaned)
                metrics["trim_lineage_errors"] = int(
                    trimmed["source_row_indices"] != list(range(len(dirty)))
                )
                metrics["idempotency_errors"] = int(
                    encoded(repeated["data"]) != encoded(cleaned["data"]) or repeated["changed"]
                )
                cases.append(
                    {
                        "keep": keep,
                        "input_rows": len(dirty),
                        "expected_rows": len(expected_rows),
                        "actual_rows": len(cleaned["data"]["rows"]),
                        "artificial_duplicate_rows": len(changes["artificial_duplicate_indices"]),
                        "artificial_empty_rows": len(changes["artificial_empty_indices"]),
                        "artificial_padded_rows": len(changes["artificial_padding_indices"]),
                        "oracle_sha256": digest(
                            {"expected": expected, "indices": indices, "injections": changes}
                        ),
                        "input_sha256": digest(table(dirty)),
                        "expected_sha256": digest(expected),
                        "actual_sha256": digest(cleaned["data"]),
                        "metrics": metrics,
                        "seconds": {"trim": trim_time, "clean": clean_time, "repeat": repeat_time},
                    }
                )
        batches.append(
            {
                "source_offset": offset,
                "original_rows": len(gold),
                "baseline_metrics": baseline_score,
                "baseline_seconds": duration,
                "baseline_sha256": digest(actual["data"]),
                "perturbation_eligible_rows": len(eligible),
                "original_edge_whitespace_rows": len(gold) - len(eligible),
                "cases": cases,
            }
        )
    if digest(records) != original_hash:
        raise RuntimeError("benchmark mutated original records")
    totals = Counter()
    for batch in batches:
        totals.update(batch["baseline_metrics"])
        for case in batch["cases"]:
            totals.update(case["metrics"])
    seconds = sum(
        batch["baseline_seconds"] + sum(sum(case["seconds"].values()) for case in batch["cases"])
        for batch in batches
    )
    return {
        "original_rows": len(records),
        "original_records_sha256": original_hash,
        "natural_repeated_text_rows": len(records) - len({row[text_field] for row in records}),
        "original_blank_text_rows": sum(not row[text_field].strip() for row in records),
        "original_non_ascii_text_rows": sum(not row[text_field].isascii() for row in records),
        "http_requests": sum(1 + 3 * len(batch["cases"]) for batch in batches),
        "measured_http_seconds": seconds,
        "batch_size": batch_size,
        "batches": batches,
        "error_totals": dict(totals),
        "passed": not any(totals.values()),
    }


@contextmanager
def compute_client(base_url: str | None = None):
    if base_url:
        import httpx

        parsed = urlparse(base_url)
        if parsed.hostname not in {"127.0.0.1", "localhost", "::1"} or parsed.scheme not in {
            "http",
            "https",
        }:
            raise ValueError("live smoke requires an explicitly local compute endpoint")
        if parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ValueError("credentials and query strings must not be placed in the endpoint")
        if parsed.path not in {"", "/"}:
            raise ValueError("the live endpoint must not include a path")
        token = os.getenv("COGNIDA_CLEANING_BENCH_TOKEN", "")
        headers = {"Authorization": f"Bearer {token}"} if token else {}
        with httpx.Client(
            base_url=base_url, headers=headers, timeout=60, trust_env=False
        ) as client:
            yield client
    else:
        from fastapi import FastAPI
        from fastapi.testclient import TestClient
        from services.lifecycle.router import LifecycleBodyLimitMiddleware, router

        app = FastAPI()
        app.include_router(router)
        app.add_middleware(LifecycleBodyLimitMiddleware)
        with TestClient(app) as client:
            yield client


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, action="append", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--batch-size", type=int, default=2000)
    parser.add_argument(
        "--base-url", help="local live HTTP endpoint; token comes only from environment"
    )
    args = parser.parse_args()
    reports = []
    with compute_client(args.base_url) as client:
        for manifest_path in args.manifest:
            manifest = json.loads(manifest_path.read_text())
            files = manifest.get("files")
            if not isinstance(files, list) or not files:
                raise ValueError("expected a dataset-level manifest with nonempty files")
            for file in files:
                source = {
                    **{key: manifest.get(key) for key in ("dataset_id", "revision", "license")},
                    **file,
                }
                stage = "intake"
                try:
                    records, provenance = load_source(source, manifest_path.parent)
                    if manifest.get("raw_files"):
                        records, raw_provenance = bind_raw_records(
                            records, source, manifest["raw_files"], manifest_path.parent
                        )
                        provenance.update(raw_provenance)
                    stage = "clean_http"
                    reports.append(
                        {
                            "source": provenance,
                            "measurement": benchmark_records(
                                client, records, provenance["text_field"], args.batch_size
                            ),
                        }
                    )
                except Exception as exc:
                    failure = {"stage": stage, "error_type": type(exc).__name__}
                    if isinstance(exc, CleaningHTTPError):
                        failure["http_status"] = exc.status
                    reports.append(
                        {
                            "source_index": len(reports),
                            "failure": failure,
                            "measurement": {"passed": False, "original_rows": 0},
                        }
                    )
    report = {
        "schema_version": 1,
        "benchmark": "hf-cleaning-fidelity-v1",
        "transport": "live-local-http" if args.base_url else "production-router-testclient",
        "limitations": LIMITATIONS,
        "sources": reports,
        "passed": all(row["measurement"]["passed"] for row in reports),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(
        json.dumps(
            {
                "passed": report["passed"],
                "sources": len(reports),
                "original_rows": sum(row["measurement"]["original_rows"] for row in reports),
            }
        )
    )
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
