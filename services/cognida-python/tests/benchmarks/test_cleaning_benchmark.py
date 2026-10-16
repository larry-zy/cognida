"""Small deterministic regression of the benchmark itself; no HF download."""

import copy
import hashlib
import json

import pytest
from cleaning_benchmark import (
    COLUMNS,
    benchmark_records,
    bind_raw_records,
    compute_client,
    digest,
    gold_rows,
    load_source,
    oracle,
    perturb,
    score,
    table,
)


@pytest.fixture
def client():
    with compute_client() as client:
        yield client


def test_real_http_preserves_original_duplicates_unicode_and_edge_whitespace(client):
    records = [
        {"text": "退款迟迟未到账，编号００１。", "label": 3},
        {"text": "退款迟迟未到账，编号００１。", "label": 3},
        {"text": "  legitimate original whitespace\t", "label": 4},
        {"text": "", "label": 4},
        {"text": "e\u0301 ≠ é 👩🏽‍💻", "label": 7},
        {"text": "line1\r\nline2\twith  spaces", "label": 8},
    ]
    original = copy.deepcopy(records)
    result = benchmark_records(client, records, batch_size=4)
    assert result["passed"] is True
    assert result["original_rows"] == 6
    assert sum(b["original_edge_whitespace_rows"] for b in result["batches"]) == 1
    assert len(result["batches"]) == 2
    assert records == original
    assert not any(result["error_totals"].values())


@pytest.mark.parametrize("keep", ["first", "last"])
def test_oracle_only_drops_injected_rows_and_honors_keep(keep):
    rows = gold_rows(
        [{"text": "same", "label": 1}, {"text": "same", "label": 1}, {"text": "", "label": 2}],
        "text",
        0,
    )
    dirty, injections = perturb(rows)
    expected, indices = oracle(dirty, keep)
    assert expected == rows  # naturally repeated / empty text remain distinct source records
    assert len(indices) == 3
    assert (0 if keep == "first" else 1) in indices
    assert not set(injections["artificial_empty_indices"]) & set(indices)
    assert len(dirty) - len(indices) == 2


def test_scorer_detects_balanced_false_delete_missed_delete_and_precision_loss():
    rows = gold_rows([{"text": "alpha"}, {"text": "beta"}], "text", 0)
    expected = table(rows)
    actual = copy.deepcopy(expected)
    actual["rows"] = [actual["rows"][1], copy.deepcopy(actual["rows"][1])]
    actual["rows"][0][6] = 9007199254740992.0
    result = score(expected, [0, 2], {"data": actual, "source_row_indices": [2, 3]})
    assert result["false_deletions"] == 1
    assert result["missed_deletions"] == 1
    assert result["exact_value_or_type_errors"] == 1
    assert result["output_mismatch"] == 1


def test_local_intake_requires_fixed_revision_and_exact_file_hash(tmp_path):
    path = tmp_path / "corpus.jsonl"
    records = [{"text": "原始中文\t", "label": 1, "nested": ["00123", True, None]}]
    raw = (json.dumps(records[0], ensure_ascii=False) + "\n").encode()
    path.write_bytes(raw)
    source = {
        "dataset_id": "fixture/not-hugging-face",
        "revision": "a" * 40,
        "split": "test",
        "path": path.name,
        "sha256": hashlib.sha256(raw).hexdigest(),
        "format": "jsonl",
        "text_field": "text",
    }
    actual, metadata = load_source(source, tmp_path)
    assert actual == records
    assert metadata["original_records_sha256"] == digest(records)
    assert metadata["file_sha256"] == source["sha256"]
    with pytest.raises(ValueError, match="revision"):
        load_source({**source, "revision": "main"}, tmp_path)
    with pytest.raises(ValueError, match="SHA256 mismatch"):
        load_source({**source, "sha256": "0" * 64}, tmp_path)
    with pytest.raises(ValueError, match="row count"):
        load_source({**source, "rows": 2}, tmp_path)


def test_parquet_intake_preserves_native_fields(tmp_path):
    import pyarrow as pa
    import pyarrow.parquet as pq

    path = tmp_path / "corpus.parquet"
    records = [{"text": "literal", "label": 7, "items": ["a", "b"]}]
    pq.write_table(pa.Table.from_pylist(records), path)
    source = {
        "dataset_id": "fixture/not-hugging-face",
        "revision": "b" * 40,
        "split": "test",
        "path": path.name,
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        "format": "parquet",
    }
    actual, _ = load_source(source, tmp_path)
    assert actual == records


def test_raw_hf_text_and_all_fields_are_verified_and_protected(tmp_path):
    raw_record = {"text": "原始 空格\t", "label": 4, "lang": "zh-CN", "nested": [True, None]}
    path = tmp_path / "train.jsonl"
    path.write_text(json.dumps(raw_record, ensure_ascii=False) + "\n")
    raw_file = {
        "path": path.name,
        "source_path": "train.jsonl",
        "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
    }
    normalized = [{"id": "train-0", "split": "train", "text": raw_record["text"], "label": "four"}]
    bound, proof = bind_raw_records(normalized, {"split": "train"}, [raw_file], tmp_path)
    assert bound[0]["_hf_original"] == raw_record
    assert proof["exact_original_text_match"] is True
    assert "_hf_original" not in normalized[0]
    with pytest.raises(ValueError, match="exactly match"):
        bind_raw_records(
            [{**normalized[0], "text": raw_record["text"].strip()}],
            {"split": "train"},
            [raw_file],
            tmp_path,
        )
    with pytest.raises(ValueError, match="SHA256"):
        bind_raw_records(
            normalized, {"split": "train"}, [{**raw_file, "sha256": "0" * 64}], tmp_path
        )


def test_http_bounds_and_empty_working_field_are_not_false_delete(client):
    rows = gold_rows([{"text": "source text", "label": 0}], "text", 0)
    rows[0][3] = ""
    response = client.post(
        "/api/v1/lifecycle/clean",
        json={"input": table(rows), "options": {"operations": ["drop_empty"]}},
    )
    assert response.status_code == 200
    assert response.json()["result"]["data"]["rows"] == rows
    response = client.post(
        "/api/v1/lifecycle/clean",
        json={"input": table(rows), "options": {"operations": ["trim"], "columns": ["missing"]}},
    )
    assert response.status_code == 422
    response = client.post(
        "/api/v1/lifecycle/clean", json={"input": table([[None] * len(COLUMNS)] * 10001)}
    )
    assert response.status_code == 422


def test_live_client_cannot_send_token_to_remote_address():
    with pytest.raises(ValueError, match="local"):
        with compute_client("https://example.com"):
            pass


@pytest.mark.parametrize("keep", ["first", "last"])
@pytest.mark.parametrize("selected", [False, True])
def test_real_http_exact_dedup_preserves_json_scalar_types(client, keep, selected):
    values = [
        True,
        1,
        1.0,
        "1",
        False,
        0,
        0.0,
        None,
        "",
        "00123",
        9007199254740993,
        "9007199254740993.123456789012345678",
    ]
    rows = [[v, "first"] for v in values] + [[v, "last" if selected else "first"] for v in values]
    options = {"operations": ["dedup"], "keep": keep}
    if selected:
        options["columns"] = ["value"]
    source = {"kind": "table", "columns": ["value", "other"], "rows": rows}
    response = client.post("/api/v1/lifecycle/clean", json={"input": source, "options": options})
    assert response.status_code == 200
    result = response.json()["result"]
    start = 0 if keep == "first" else len(values)
    assert digest(result["data"]["rows"]) == digest(rows[start : start + len(values)])
    assert result["source_row_indices"] == list(range(start, start + len(values)))
    assert result["removed_count"] == len(values)
    assert result["operations"][0]["count"] == len(values)
    assert result["operations"][0]["field"] == (["value"] if selected else None)
    assert result["diagnostics"]["after"]["duplicate_rows"] == 0
