"""Build deterministic local benchmark views without mixing train/test labels."""

from __future__ import annotations

import argparse
import csv
import gzip
import hashlib
import json
from pathlib import Path

from fetch import SOURCES, digest


def entity_spans(
    tokens: list[str], tags: list[int], names: list[str]
) -> tuple[str, list[dict]]:
    if len(tokens) != len(tags) or any(t < 0 or t >= len(names) for t in tags):
        raise ValueError("invalid token/tag alignment")
    text = " ".join(tokens)
    offsets, cursor = [], 0
    for token in tokens:
        offsets.append(cursor)
        cursor += len(token) + 1
    entities, index = [], 0
    while index < len(tokens):
        tag = tags[index]
        end = index + 1
        if tag == 0:
            index = end
            continue
        while end < len(tokens) and tags[end] == tag:
            end += 1
        start_char, end_char = offsets[index], offsets[end - 1] + len(tokens[end - 1])
        entities.append(
            {
                "label": names[tag],
                "start": start_char,
                "end": end_char,
                "text": text[start_char:end_char],
            }
        )
        index = end
    return text, entities


def write_rows(path: Path, rows) -> tuple[int, set[str], set[str]]:
    count, labels, hashes = 0, set(), set()
    with path.open("w", encoding="utf-8", newline="\n") as output:
        for row in rows:
            output.write(
                json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n"
            )
            count += 1
            if "label" in row:
                labels.add(row["label"])
            else:
                labels.update(entity["label"] for entity in row["entities"])
            hashes.add(hashlib.sha256(row["text"].encode()).hexdigest())
    return count, labels, hashes


def classification_rows(path: Path, split: str):
    opener = gzip.open if path.suffix == ".gz" else open
    with opener(path, "rt", encoding="utf-8") as source:
        for index, line in enumerate(source):
            row = json.loads(line)
            text, label = row["text"], row["label_text"]
            if (
                not isinstance(text, str)
                or not isinstance(label, str)
                or not text
                or not label
            ):
                raise ValueError("classification schema mismatch")
            yield {
                "id": f"{split}:{index}",
                "split": split,
                "text": text,
                "label": label,
                "source_id": row.get("id", str(index)),
            }


def ner_rows(table, split: str, names: list[str]):
    for index, row in enumerate(table.to_pylist()):
        text, entities = entity_spans(row["tokens"], row["ner_tags"], names)
        yield {
            "id": f"{split}:{index}",
            "split": split,
            "text": text,
            "entities": entities,
            "tokens": row["tokens"],
            "ner_tags": row["ner_tags"],
            "source_id": row["id"],
        }


def label_config(manifest: dict) -> dict:
    files = {row["split"]: row for row in manifest["files"]}
    return {
        "dataset": manifest["dataset_id"],
        "revision": manifest["revision"],
        "language": manifest["language"],
        "kind": manifest["task"],
        "train_file": files["train"]["path"],
        "test_file": files["test"]["path"],
        "train_sha256": files["train"]["sha256"],
        "test_sha256": files["test"]["sha256"],
        "labels": [{"id": name, "name": name} for name in manifest["label_names"]],
    }


def verify_downloads(root: Path, lock_path: Path) -> list[dict]:
    """Verify consumed paths against the committed lock, not a self-reported hash."""
    lock = json.loads(lock_path.read_text())
    if lock.get("schema_version") != 1 or lock.get("datasets") != SOURCES:
        raise ValueError("dataset lock source definitions mismatch")
    expected_keys = {
        (key, path) for key, source in SOURCES.items() for path in source["paths"]
    }
    locked = {(row["dataset"], row["source_path"]): row for row in lock["files"]}
    downloads = json.loads((root / "downloads.json").read_text())
    supplied = {(row["dataset"], row["source_path"]): row for row in downloads}
    if set(locked) != expected_keys or len(locked) != len(lock["files"]):
        raise ValueError("dataset lock file inventory mismatch")
    if set(supplied) != expected_keys or len(supplied) != len(downloads):
        raise ValueError("download file inventory mismatch")
    verified = []
    for key, source in SOURCES.items():
        for relative in source["paths"]:
            path = (root / key / "raw" / relative).resolve()
            expected = locked[(key, relative)]
            actual = supplied[(key, relative)]
            if {k: v for k, v in actual.items() if k != "path"} != expected or Path(
                actual["path"]
            ).resolve() != path:
                raise ValueError("download manifest differs from pinned lock/path")
            if (
                path.stat().st_size != expected["bytes"]
                or digest(path) != expected["sha256"]
            ):
                raise ValueError("pinned source bytes mismatch")
            verified.append(actual)
    return verified


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path, required=True)
    parser.add_argument(
        "--lock", type=Path, default=Path(__file__).with_name("datasets.lock.json")
    )
    args = parser.parse_args()
    root = args.data.resolve()
    downloads = verify_downloads(root, args.lock)
    summaries = []
    for key, source in SOURCES.items():
        folder = root / key
        manifest = {k: v for k, v in source.items() if k != "paths"}
        manifest.update(
            {
                "schema_version": 1,
                "files": [],
                "raw_files": [row for row in downloads if row["dataset"] == key],
            }
        )
        split_hashes, all_labels = {}, set()
        if key == "wine_quality":
            manifest["language"] = "tabular"
            for split in ("red", "white"):
                path = folder / "raw" / f"winequality-{split}.csv"
                with path.open(newline="") as stream:
                    raw_rows = list(csv.reader(stream, delimiter=";"))
                rows = [row for row in raw_rows if row]
                assert len(rows[0]) == 12 and all(len(row) == 12 for row in rows)
                manifest["files"].append(
                    {
                        "split": split,
                        "path": str(path),
                        "sha256": digest(path),
                        "format": "csv",
                        "delimiter": ";",
                        "rows": len(rows) - 1,
                        "blank_lines": len(raw_rows) - len(rows),
                    }
                )
        else:
            manifest["language"] = "zh-CN" if key == "massive_zh" else "en"
            manifest["task"] = "named_entity" if key == "few_nerd" else "classification"
            for split in ("train", "test"):
                if key == "few_nerd":
                    import pyarrow.parquet as pq

                    table = pq.read_table(
                        folder
                        / "raw"
                        / "supervised"
                        / f"{split}-00000-of-00001.parquet"
                    )
                    names = json.loads(table.schema.metadata[b"huggingface"])["info"][
                        "features"
                    ]["ner_tags"]["feature"]["names"]
                    rows = ner_rows(table, split, names)
                    manifest["config"] = "supervised"
                    manifest["text_reconstruction"] = (
                        "Join tokens with one ASCII space. Unicode codepoint offsets. Each contiguous identical non-O coarse IO tag run is one span; original sentence spacing and adjacent same-type boundaries are not recoverable."
                    )
                else:
                    relative = (
                        f"{split}/zh-CN.json.gz"
                        if key == "massive_zh"
                        else f"{split}.jsonl"
                    )
                    rows = classification_rows(folder / "raw" / relative, split)
                path = folder / f"{split}.jsonl"
                count, labels, split_hashes[split] = write_rows(path, rows)
                all_labels.update(labels)
                manifest["files"].append(
                    {
                        "split": split,
                        "path": str(path),
                        "sha256": digest(path),
                        "format": "jsonl",
                        "rows": count,
                        "text_field": "text",
                        "label_field": "label" if key != "few_nerd" else "entities",
                    }
                )
            manifest["label_names"] = sorted(all_labels)
            manifest["cross_split_exact_text_overlap"] = len(
                split_hashes["train"] & split_hashes["test"]
            )
            manifest["overlap_policy"] = (
                "Report overlap; keep official test denominator unchanged. No rules/prompts are selected using test labels."
            )
            (folder / "label-config.json").write_text(
                json.dumps(label_config(manifest), ensure_ascii=False, indent=2) + "\n"
            )
        path = folder / "manifest.json"
        path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        summaries.append(
            {
                "dataset": key,
                "manifest": str(path),
                "files": [
                    {"split": row["split"], "rows": row["rows"]}
                    for row in manifest["files"]
                ],
                "overlap": manifest.get("cross_split_exact_text_overlap"),
            }
        )
    (root / "prepared.json").write_text(json.dumps(summaries, indent=2) + "\n")
    print(json.dumps(summaries, ensure_ascii=False))


if __name__ == "__main__":
    main()
