#!/usr/bin/env python3
"""Exercise the real scanner against permitted evidence and synthetic canaries.

Run from any directory with gitleaks on PATH, or set GITLEAKS_BIN. This creates
only temporary files, never real credentials, and always redacts scanner output.
"""

import hashlib
import json
import os
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CONFIG = ROOT / ".gitleaks.toml"
BINARY = os.environ.get("GITLEAKS_BIN", "gitleaks")
EVIDENCE = Path("apps/cognida-web/e2e/evidence")
MIGRATIONS = EVIDENCE / "2026-10-16-migrations-16-35.json"
RUNTIME = EVIDENCE / "2026-10-15-task-runtime-restart.json"
OBJECT_TEST = Path(
    "services/cognida-go/internal/repository/mysql/collection_repo_integration_test.go"
)
# Generated locally, never a credential issued by a provider.
CANARY = hashlib.sha256(b"cognida-gitleaks-negative-control-not-a-secret").hexdigest()


def scan(name: str, relative_path: Path, content: str, should_detect: bool) -> None:
    with tempfile.TemporaryDirectory(prefix="cognida-gitleaks-") as folder:
        source = Path(folder) / "source"
        fixture = source / relative_path
        fixture.parent.mkdir(parents=True)
        fixture.write_text(content + "\n", encoding="utf-8")
        report = Path(folder) / "redacted.json"
        result = subprocess.run(
            [
                BINARY,
                "detect",
                "--no-git",
                "--source",
                str(source),
                "--config",
                str(CONFIG),
                "--redact",
                "--no-banner",
                "--exit-code",
                "1",
                "--report-format",
                "json",
                "--report-path",
                str(report),
            ],
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        if result.returncode not in (0, 1) or not report.exists():
            raise RuntimeError(f"{name}: scanner failed: {result.stderr}")
        findings = json.loads(report.read_text(encoding="utf-8")) or []
        if should_detect:
            passed = result.returncode == 1 and any(
                row["RuleID"] == "generic-api-key" for row in findings
            )
        else:
            passed = result.returncode == 0 and not findings
        if not passed:
            rules = [row["RuleID"] for row in findings]
            raise AssertionError(f"{name}: exit={result.returncode}, rules={rules}")
        print(f"PASS {name}")


def main() -> None:
    migration_data = json.loads((ROOT / MIGRATIONS).read_text(encoding="utf-8"))
    runtime_data = json.loads((ROOT / RUNTIME).read_text(encoding="utf-8"))
    verified = []
    for suffix in ("up", "down"):
        filename = f"000026_api_ingestion.{suffix}.sql"
        digest = migration_data["files"][filename]
        actual = hashlib.sha256(
            (ROOT / "services/cognida-go/migrations" / filename).read_bytes()
        ).hexdigest()
        if digest != actual:
            raise AssertionError(f"migration evidence mismatch: {filename}")
        verified.append((f"migration-{suffix}", MIGRATIONS, filename, digest))
    keys = sorted(
        {
            row["operation_key"]
            for phase in ("before", "after")
            for row in runtime_data[phase]["outbox"]
        }
    )
    for index, digest in enumerate(keys):
        verified.append((f"operation-{index}", RUNTIME, "operation_key", digest))

    for name, path, field, digest in verified:
        line = f'  "{field}": "{digest}",'
        scan(f"{name}: verified value", path, line, False)
        scan(f"{name}: different file", path.with_name("unrelated.json"), line, True)
        scan(f"{name}: changed value", path, line.replace(digest, CANARY), True)
        scan(f"{name}: different field", path, f'"api_token": "{digest}"', True)
        scan(
            f"{name}: additional secret",
            path,
            line + f'\n  "api_token": "{CANARY}"',
            True,
        )
        scan(
            f"{name}: same-line secret", path, line + f' "api_token": "{CANARY}"', True
        )

    # Full historical evidence must remain usable without exempting its whole path.
    for path in (MIGRATIONS, RUNTIME):
        scan(
            f"complete evidence: {path.name}",
            path,
            (ROOT / path).read_text(encoding="utf-8"),
            False,
        )
    for field, value in (
        ("RawObjectKey", "raw"),
        ("NormalizedObjectKey", "normalized"),
    ):
        line = f'item.{field} = "collection/7001/fixture/{value}"'
        scan(f"object path: {field}", OBJECT_TEST, line, False)
        # These paths trigger only newer scanner versions; the canary must always fail.
        scan(
            f"object path: {field} with secret",
            OBJECT_TEST,
            line + f'\napi_token = "{CANARY}"',
            True,
        )
    print("All gitleaks allowlist boundary checks passed.")


if __name__ == "__main__":
    main()
