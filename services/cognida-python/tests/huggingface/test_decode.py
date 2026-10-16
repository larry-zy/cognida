import base64
import gzip
import io
import json
from decimal import Decimal

import pyarrow as pa
import pyarrow.parquet as pq
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from services.huggingface.decode import DecodeRequest, decode
from services.huggingface import router as api


def request(raw, fmt="jsonl", mapping=None, rows=10):
    return DecodeRequest.model_validate({"format": fmt, "content_base64": base64.b64encode(raw).decode(), "source_file": "test.jsonl", "max_rows": rows, "field_mapping": mapping or {"text_field": "text", "id_field": "id"}})


def test_text_projection_does_not_import_gold_and_preserves_id():
    result = decode(request('{"id":"00123","text":"订明天的火车票","label":"transport"}\n'.encode()))
    assert json.loads(result["rows_json"]) == [["00123", "订明天的火车票"]]
    assert "transport" not in json.dumps(result, ensure_ascii=False)
    assert result["total_rows"] == 1 and not result["truncated"]
    assert {c["name"] for c in result["source_columns"]} == {"id", "text", "label"}


@pytest.mark.parametrize("fmt", ["json", "jsonl", "json.gz", "jsonl.gz"])
def test_json_lines_and_explicit_precision_mapping(fmt):
    raw = b'{"id":9007199254740993,"amount":9007199254740993.123456789012345678,"code":"00123","messages":[{"role":"user","content":"hello"}]}\n'
    if fmt.endswith(".gz"):
        raw = gzip.compress(raw)
    mappings = [{"source": k, "target": k, **({"encoding": "json"} if k == "messages" else {})} for k in ("id", "amount", "code", "messages")]
    result = decode(request(raw, fmt, {"columns": mappings}))
    row = json.loads(result["rows_json"], parse_float=Decimal)[0]
    assert row[:3] == [9007199254740993, Decimal("9007199254740993.123456789012345678"), "00123"]
    assert json.loads(row[3]) == [{"role": "user", "content": "hello"}]
    assert result["column_types"]["amount"] == "DECIMAL"


def test_parquet_exact_decimal_nested_and_sampling():
    table = pa.table({"id": pa.array([9007199254740993, 2], type=pa.int64()), "amount": pa.array([Decimal("9007199254740993.123456789012345678"), Decimal("2.0")], type=pa.decimal128(38, 18)), "messages": [[{"role": "user", "content": "你好"}], []]})
    buf = io.BytesIO()
    pq.write_table(table, buf)
    mapping = {"columns": [{"source": "id", "target": "id"}, {"source": "amount", "target": "value"}, {"source": "messages", "target": "messages", "encoding": "json"}]}
    result = decode(request(buf.getvalue(), "parquet", mapping, rows=1))
    assert result["truncated"] and result["total_rows"] == 2
    row = json.loads(result["rows_json"], parse_float=Decimal)[0]
    assert row[:2] == [9007199254740993, Decimal("9007199254740993.123456789012345678")]


def test_csv_preserves_numeric_looking_identifiers_and_unknown_total():
    result = decode(request(b'id,text,amount\n00123,hello,12.345\n00234,world,1\n', "csv", rows=1))
    assert json.loads(result["rows_json"]) == [["00123", "hello"]]
    assert result["truncated"] and result["total_rows"] is None


@pytest.mark.parametrize("raw,fmt", [
    (b'{"id":1,"text":"ok","text":"override"}', "jsonl"),
    (b'{"id":1,"text":"ok","n":NaN}', "jsonl"),
    (b'{"id":1,"text":null}', "jsonl"),
    (b'{"id":1,"text":""}', "jsonl"),
    (b'{"id":1,"text":"ok"}\n\n', "jsonl"),
    (b'id,text\n1,ok,extra', "csv"),
    (b'id,id,text\n1,2,ok', "csv"),
    (b'not parquet', "parquet"),
])
def test_invalid_static_data_fails_without_skipping_rows(raw, fmt):
    with pytest.raises((ValueError, OSError)):
        decode(request(raw, fmt))


def test_nested_requires_explicit_encoding_and_missing_column_fails():
    raw = b'{"messages":[{"role":"user","content":"hi"}]}'
    for source in ("messages", "absent"):
        with pytest.raises(ValueError):
            decode(request(raw, mapping={"columns": [{"source": source, "target": "messages"}]}))


def test_gzip_expansion_is_bounded():
    raw = gzip.compress(b" " * (32 * 1024 * 1024 + 1))
    with pytest.raises(ValueError, match="expanded"):
        decode(request(raw, "json.gz"))


@pytest.mark.parametrize("patch", [{"url": "https://example.com"}, {"path": "/etc/passwd"}, {"format": "python"}, {"max_rows": 0}, {"max_rows": 1001}, {"field_mapping": {"text_field": "text", "columns": [{"source": "text", "target": "text"}]}}])
def test_decoder_has_no_network_or_path_contract(patch):
    body = request(b'{"id":1,"text":"ok"}').model_dump()
    body.update(patch)
    with pytest.raises(ValueError):
        DecodeRequest.model_validate(body)


def test_internal_http_auth_mapping_and_errors(monkeypatch):
    app = FastAPI()
    app.include_router(api.router)
    monkeypatch.setattr(api, "authorized_internal_request", lambda value: value == "Bearer synthetic-internal-token")
    with TestClient(app) as client:
        body = request(b'{"id":1,"text":"hello","label":"gold"}').model_dump()
        assert client.post("/api/v1/connectors/huggingface/decode", json=body).status_code == 401
        headers = {"Authorization": "Bearer synthetic-internal-token"}
        result = client.post("/api/v1/connectors/huggingface/decode", json=body, headers=headers)
        assert result.status_code == 200 and json.loads(result.json()["rows_json"]) == [[1, "hello"]]
        body["field_mapping"]["text_field"] = "missing"
        failed = client.post("/api/v1/connectors/huggingface/decode", json=body, headers=headers)
        assert failed.status_code == 422 and "hello" not in failed.text
