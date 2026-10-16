"""Decode supplied bytes only: no URLs, paths, pickle, scripts or dataset loaders."""
from __future__ import annotations

import base64
import csv
import gzip
import io
import json
import math
from decimal import Decimal
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator

MAX_FILE = 8 * 1024 * 1024
MAX_EXPANDED = 32 * 1024 * 1024
MAX_OUTPUT = 4 * 1024 * 1024


class ColumnMapping(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)
    source: str = Field(min_length=1, max_length=128)
    target: str = Field(min_length=1, max_length=128)
    encoding: Literal["", "json"] = ""


class Mapping(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)
    text_field: str = ""
    id_field: str = ""
    columns: list[ColumnMapping] = Field(default_factory=list, max_length=32)

    @model_validator(mode="after")
    def check(self):
        if self.columns:
            if self.text_field or self.id_field or len({c.target for c in self.columns}) != len(self.columns):
                raise ValueError("invalid mapping")
        elif not self.text_field or len(self.text_field) > 128 or len(self.id_field) > 128:
            raise ValueError("text field required")
        return self


class DecodeRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)
    format: Literal["json", "jsonl", "csv", "json.gz", "jsonl.gz", "csv.gz", "parquet"]
    content_base64: str = Field(max_length=(MAX_FILE * 4 // 3) + 8)
    source_file: str = Field(min_length=1, max_length=512)
    max_rows: int = Field(ge=1, le=1000)
    field_mapping: Mapping


def exact_json(value):
    """Emit original decimal/int tokens without a binary float intermediate."""
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (Decimal, float)):
        if not math.isfinite(value):
            raise ValueError("non-finite numeric value")
        return str(value)
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False)
    if isinstance(value, list):
        return "[" + ",".join(exact_json(x) for x in value) + "]"
    if isinstance(value, dict) and all(isinstance(k, str) for k in value):
        return "{" + ",".join(json.dumps(k, ensure_ascii=False) + ":" + exact_json(v) for k, v in value.items()) + "}"
    raise ValueError("unsupported scalar type")


def _pairs(pairs):
    out = {}
    for key, value in pairs:
        if key in out:
            raise ValueError("duplicate JSON key")
        out[key] = value
    return out


def _json(raw):
    return json.loads(raw, parse_float=Decimal, parse_constant=lambda _: (_ for _ in ()).throw(ValueError("non-finite")), object_pairs_hook=_pairs)


def _type(value):
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "BOOLEAN"
    if isinstance(value, int):
        return "BIGINT"
    if isinstance(value, Decimal):
        return "DECIMAL"
    if isinstance(value, float):
        return "DOUBLE"
    if isinstance(value, str):
        return "VARCHAR"
    if isinstance(value, (list, dict)):
        return "JSON"
    return "UNSUPPORTED"


def decode(request: DecodeRequest):
    raw = base64.b64decode(request.content_base64, validate=True)
    if not raw or len(raw) > MAX_FILE:
        raise ValueError("file size limit")
    fmt = request.format
    if fmt.endswith(".gz"):
        with gzip.GzipFile(fileobj=io.BytesIO(raw)) as zipped:
            raw = zipped.read(MAX_EXPANDED + 1)
        if len(raw) > MAX_EXPANDED:
            raise ValueError("expanded size limit")
        fmt = fmt[:-3]
    total = None
    if fmt == "parquet":
        import pyarrow as pa
        import pyarrow.parquet as pq
        with pq.ParquetFile(pa.BufferReader(raw)) as file:
            meta = file.metadata
            if meta.num_columns > 200 or meta.num_row_groups > 1024 or sum(meta.row_group(i).total_byte_size for i in range(meta.num_row_groups)) > MAX_EXPANDED:
                raise ValueError("parquet expansion limit")
            total = meta.num_rows
            records = []
            for batch in file.iter_batches(batch_size=min(request.max_rows + 1, 1024), use_threads=False):
                records.extend(batch.to_pylist())
                if len(records) > request.max_rows:
                    break
    else:
        text = raw.decode("utf-8-sig", errors="strict")
        if fmt == "json" and text.lstrip().startswith("["):
            records = _json(text)
            if not isinstance(records, list):
                raise ValueError("JSON array required; use jsonl for line-delimited records")
            total = len(records)
        elif fmt in ("json", "jsonl"):
            lines = text.splitlines()
            if any(not line.strip() for line in lines):
                raise ValueError("empty JSONL record")
            total = len(lines)
            records = [_json(line) for line in lines[:request.max_rows + 1]]
        else:
            reader = csv.DictReader(io.StringIO(text, newline=""), strict=True)
            names = reader.fieldnames
            if not names or len(names) != len(set(names)) or len(names) > 200:
                raise ValueError("invalid CSV header")
            records = []
            for record in reader:
                if None in record or any(v is None for v in record.values()):
                    raise ValueError("invalid CSV width")
                records.append(record)
                if len(records) > request.max_rows:
                    break
            if len(records) <= request.max_rows:
                total = len(records)
    if not records:
        raise ValueError("no records")
    truncated = (total is not None and total > request.max_rows) or len(records) > request.max_rows
    records = records[:request.max_rows]
    mapping = request.field_mapping
    columns = [c.target for c in mapping.columns] if mapping.columns else ["source_id", "text"]
    source_types = {}
    target_types = {}
    rows = []
    for index, record in enumerate(records):
        if not isinstance(record, dict) or len(record) > 200:
            raise ValueError("object record required")
        for key, value in record.items():
            if not key or len(key) > 128:
                raise ValueError("invalid column")
            typ = _type(value)
            prior = source_types.get(key, "NULL")
            source_types[key] = typ if prior == "NULL" else prior if typ in (prior, "NULL") else "MIXED"
        row = []
        if mapping.columns:
            for column in mapping.columns:
                if column.source not in record:
                    raise ValueError("mapped column absent")
                value = record[column.source]
                if column.encoding == "json":
                    value = exact_json(value)
                elif isinstance(value, (dict, list)):
                    raise ValueError("nested column requires explicit json encoding")
                row.append(value)
        else:
            value = record.get(mapping.text_field)
            if not isinstance(value, str) or not value.strip():
                raise ValueError("mapped text must be a nonempty string")
            identifier = record.get(mapping.id_field) if mapping.id_field else f"{request.source_file}:{index}"
            if identifier is None or isinstance(identifier, (list, dict, bool, float, Decimal)):
                raise ValueError("mapped ID must be a string or integer")
            row = [identifier, value]
        for col, val in zip(columns, row):
            typ = _type(val)
            prior = target_types.get(col, "NULL")
            target_types[col] = typ if prior == "NULL" else prior if typ in (prior, "NULL") else "MIXED"
            if len(exact_json(val).encode()) > 200_000:
                raise ValueError("cell limit")
        rows.append(row)
    encoded = exact_json(rows)
    if len(encoded.encode()) > MAX_OUTPUT:
        raise ValueError("output limit")
    return {"columns": columns, "column_types": target_types, "source_columns": [{"name": k, "type": v} for k, v in sorted(source_types.items())], "rows_json": encoded, "total_rows": total, "truncated": truncated}
