"""Download pinned data bytes only; never execute Hugging Face dataset scripts."""

from __future__ import annotations

import argparse
import hashlib
import json
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import requests

SOURCES = {
    "banking77": {
        "dataset_id": "mteb/banking77",
        "revision": "18072d2685ea682290f7b8924d94c62acc19c0b2",
        "license": "cc-by-4.0 (original PolyAI data); mirror card says mit",
        "original_source": "https://huggingface.co/datasets/PolyAI/banking77",
        "paths": ["README.md", "train.jsonl", "test.jsonl"],
    },
    "massive_zh": {
        "dataset_id": "mteb/amazon_massive_intent",
        "revision": "940fd47a81eaa7f2cc7b129674d945d618ac38c2",
        "license": "cc-by-4.0 (original Amazon MASSIVE); mirror card says apache-2.0",
        "original_source": "https://huggingface.co/datasets/AmazonScience/massive",
        "paths": ["README.md", "train/zh-CN.json.gz", "test/zh-CN.json.gz"],
    },
    "few_nerd": {
        "dataset_id": "DFKI-SLT/few-nerd",
        "revision": "205f3e9c9f3577ea2561d43f2f62dc249ab92d5b",
        "license": "cc-by-sa-4.0",
        "paths": [
            "README.md",
            "supervised/train-00000-of-00001.parquet",
            "supervised/test-00000-of-00001.parquet",
        ],
    },
    "wine_quality": {
        "dataset_id": "codesignal/wine-quality",
        "revision": "a90690dd91e947252ac664b495fb043ca545da23",
        "license": "cc-by-4.0",
        "paths": [
            "README.md",
            "winequality.names",
            "winequality-red.csv",
            "winequality-white.csv",
        ],
    },
}


def digest(path: Path) -> str:
    checksum = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            checksum.update(chunk)
    return checksum.hexdigest()


def download(root: Path, key: str, source: dict, relative: str) -> dict:
    target = root / key / "raw" / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    url = f"https://huggingface.co/datasets/{source['dataset_id']}/resolve/{source['revision']}/{relative}"
    if not target.exists():
        temporary = target.with_suffix(target.suffix + ".partial")
        try:
            size = 0
            with requests.get(url, stream=True, timeout=(15, 90)) as response:
                response.raise_for_status()
                with temporary.open("xb") as output:
                    for chunk in response.iter_content(1 << 20):
                        size += len(chunk)
                        if size > 64 << 20:
                            raise ValueError(
                                "dataset file exceeds 64 MiB download limit"
                            )
                        output.write(chunk)
            temporary.replace(target)
        finally:
            temporary.unlink(missing_ok=True)
    return {
        "dataset": key,
        "source_path": relative,
        "path": str(target.resolve()),
        "url": url,
        "bytes": target.stat().st_size,
        "sha256": digest(target),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--lock", type=Path, default=Path(__file__).with_name("datasets.lock.json")
    )
    parser.add_argument(
        "--create-lock",
        action="store_true",
        help="Explicitly trust first download when creating a new lock; never replaces an existing lock",
    )
    args = parser.parse_args()
    if not args.lock.exists() and not args.create_lock:
        parser.error(
            "dataset lock is missing; restore the committed lock or explicitly use --create-lock"
        )
    args.output.mkdir(parents=True, exist_ok=True)
    inputs = [
        (key, source, relative)
        for key, source in SOURCES.items()
        for relative in source["paths"]
    ]
    with ThreadPoolExecutor(max_workers=3) as pool:
        rows = list(pool.map(lambda item: download(args.output, *item), inputs))
    lock_rows = [{k: v for k, v in row.items() if k != "path"} for row in rows]
    actual = {"schema_version": 1, "datasets": SOURCES, "files": lock_rows}
    if args.lock.exists():
        if json.loads(args.lock.read_text()) != actual:
            raise ValueError(
                "pinned dataset bytes or source definitions differ from lock"
            )
    else:
        args.lock.write_text(json.dumps(actual, indent=2, ensure_ascii=False) + "\n")
    (args.output / "downloads.json").write_text(
        json.dumps(rows, indent=2, ensure_ascii=False) + "\n"
    )
    print(
        json.dumps(
            {
                "files": len(rows),
                "bytes": sum(row["bytes"] for row in rows),
                "lock": str(args.lock),
            }
        )
    )


if __name__ == "__main__":
    main()
