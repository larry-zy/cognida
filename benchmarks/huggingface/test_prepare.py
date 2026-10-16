"""Boundary tests for pinned input provenance and gold annotation conversion."""

import gzip
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import prepare
from fetch import digest


class PrepareTests(unittest.TestCase):
    def test_unicode_spans_use_codepoints_and_io_runs(self):
        text, spans = prepare.entity_spans(
            ["Hi", "北京", "😀", "Lee", "."],
            [0, 1, 0, 2, 0],
            ["O", "location", "person"],
        )
        self.assertEqual(text, "Hi 北京 😀 Lee .")
        self.assertEqual(
            spans,
            [
                {"label": "location", "start": 3, "end": 5, "text": "北京"},
                {"label": "person", "start": 8, "end": 11, "text": "Lee"},
            ],
        )
        self.assertEqual(
            prepare.entity_spans(["New", "York"], [1, 1], ["O", "location"])[1][0][
                "text"
            ],
            "New York",
        )
        for tokens, tags in [(["x"], []), (["x"], [2]), (["x"], [-1])]:
            with self.assertRaises(ValueError):
                prepare.entity_spans(tokens, tags, ["O", "location"])

    def test_classification_preserves_text_and_split(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "input.json.gz"
            with gzip.open(path, "wt", encoding="utf-8") as stream:
                stream.write(
                    json.dumps(
                        {
                            "text": "  北京\t😀  ",
                            "label_text": "intent",
                            "id": "original",
                        },
                        ensure_ascii=False,
                    )
                    + "\n"
                )
            self.assertEqual(
                list(prepare.classification_rows(path, "test")),
                [
                    {
                        "id": "test:0",
                        "split": "test",
                        "text": "  北京\t😀  ",
                        "label": "intent",
                        "source_id": "original",
                    }
                ],
            )

    def test_pin_rejects_inventory_path_and_content_tampering(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = {
                "toy": {
                    "dataset_id": "org/toy",
                    "revision": "a" * 40,
                    "paths": ["train.jsonl"],
                }
            }
            raw = root / "toy" / "raw" / "train.jsonl"
            raw.parent.mkdir(parents=True)
            raw.write_bytes(b"original\n")
            record = {
                "dataset": "toy",
                "source_path": "train.jsonl",
                "url": "https://example.invalid/fixed",
                "bytes": raw.stat().st_size,
                "sha256": digest(raw),
            }
            download = {**record, "path": str(raw)}
            lock = root / "lock.json"
            lock.write_text(
                json.dumps({"schema_version": 1, "datasets": source, "files": [record]})
            )
            manifest = root / "downloads.json"
            with patch.object(prepare, "SOURCES", source):
                manifest.write_text(json.dumps([download]))
                self.assertEqual(prepare.verify_downloads(root, lock), [download])
                for invalid in [
                    [],
                    [download, download],
                    [{**download, "path": str(root / "elsewhere")}],
                ]:
                    manifest.write_text(json.dumps(invalid))
                    with self.assertRaises(ValueError):
                        prepare.verify_downloads(root, lock)
                raw.write_bytes(b"tampered\n")
                # Updating the untrusted manifest's own hash cannot bless changed bytes.
                manifest.write_text(json.dumps([{**download, "sha256": digest(raw)}]))
                with self.assertRaises(ValueError):
                    prepare.verify_downloads(root, lock)
                manifest.write_text(json.dumps([download]))
                with self.assertRaises(ValueError):
                    prepare.verify_downloads(root, lock)

    def test_label_config_uses_explicit_split_hashes(self):
        manifest = {
            "dataset_id": "org/data",
            "revision": "r",
            "language": "zh-CN",
            "task": "classification",
            "label_names": ["a", "b"],
            "files": [
                {"split": "test", "path": "test.jsonl", "sha256": "test-digest"},
                {"split": "train", "path": "train.jsonl", "sha256": "train-digest"},
            ],
        }
        config = prepare.label_config(manifest)
        self.assertEqual(config["train_sha256"], "train-digest")
        self.assertEqual(config["test_file"], "test.jsonl")
        self.assertEqual(
            config["labels"], [{"id": "a", "name": "a"}, {"id": "b", "name": "b"}]
        )


if __name__ == "__main__":
    unittest.main()
