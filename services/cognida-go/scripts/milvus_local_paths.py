#!/usr/bin/env python3
"""Read-only Milvus diagnosis and guarded, reversible local Storage V3 alias repair."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

CONTAINER_DATA = "/var/lib/milvus/data"
ALIAS_TARGET = "var/lib/milvus/data/insert_log"
RECEIPT = "cognida-milvus-path-repair.json"


class OpsError(ValueError):
    """Safe operator-facing validation message, with no server response body."""


def docker(*args):
    result = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise OpsError("Docker operation failed; inspect Docker locally (output suppressed)")
    return result.stdout + result.stderr if args[0] == "logs" else result.stdout


def inspect_storage(info, requested):
    env = dict(v.split("=", 1) for v in info["Config"].get("Env", []) if "=" in v)
    if env.get("COMMON_STORAGETYPE") != "local":
        raise OpsError("requires COMMON_STORAGETYPE=local")
    mounts = [m for m in info["Mounts"] if m["Destination"] == "/var/lib/milvus"]
    if len(mounts) != 1 or mounts[0]["Type"] != "bind":
        raise OpsError("requires exactly one /var/lib/milvus host bind mount")
    root = Path(requested)
    if not root.is_absolute() or root != root.resolve() or root == Path("/"):
        raise OpsError("data root must be an absolute, canonical host path")
    if root != Path(mounts[0]["Source"]).resolve() / "data":
        raise OpsError("data root does not match the inspected container bind mount")
    for name in ("data", "etcd", "rdb_data", "rdb_data_meta_kv"):
        p = root.parent / name
        if not p.is_dir() or p.is_symlink():
            raise OpsError("expected local storage directory is absent or a symlink")
    return root


def ensure_volume_stopped(root):
    identifiers = docker("ps", "-q").split()
    if not identifiers:
        return
    for container in json.loads(docker("inspect", *identifiers)):
        for mount in container.get("Mounts", []):
            if mount.get("Type") != "bind" or not mount.get("RW", False):
                continue
            source = Path(mount["Source"]).resolve()
            if source.is_relative_to(root.parent) or root.parent.is_relative_to(source):
                raise OpsError("another running container can write this volume; stop it first")


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def file_inventory(root):
    files = {}
    def fail_walk(error):
        raise error
    for folder, dirs, names in os.walk(root, followlinks=False, onerror=fail_walk):
        for name in dirs + names:
            path = Path(folder) / name
            relative = path.relative_to(root).as_posix()
            if path.is_symlink():
                if relative == "data/insert_log" and os.readlink(path) == ALIAS_TARGET:
                    continue
                raise OpsError("unexpected symlink in storage snapshot")
            if path.is_file():
                files[relative] = digest(path)
            elif not path.is_dir():
                raise OpsError("unsupported special file in storage snapshot")
    return files


def check_backup(root, backup, expected_digest=None):
    backup = Path(backup)
    if not backup.is_absolute() or backup != backup.resolve():
        raise OpsError("backup root must be an absolute, canonical directory")
    if backup == root.parent or backup.is_relative_to(root.parent) or root.parent.is_relative_to(backup):
        raise OpsError("backup must be outside the live volume")
    for name in ("data", "etcd", "rdb_data", "rdb_data_meta_kv"):
        if not (backup / name).is_dir() or (backup / name).is_symlink():
            raise OpsError("backup must contain data, etcd, rdb_data and rdb_data_meta_kv")
    saved = file_inventory(backup)
    saved.pop(RECEIPT, None)
    fingerprint = hashlib.sha256(json.dumps(saved, sort_keys=True).encode()).hexdigest()
    if expected_digest:
        if fingerprint != expected_digest:
            raise OpsError("backup has changed since repair; refusing rollback")
    elif not saved or file_inventory(root.parent) != saved:
        raise OpsError("backup files differ from the stopped live volume; no repair performed")
    return backup, fingerprint


def host_alias(root, info, backup, apply=False, rollback=False):
    if apply and info["State"]["Running"]:
        raise OpsError("stop the inspected container gracefully before repair or rollback")
    link = root / "insert_log"
    target = root / ALIAS_TARGET
    if not target.is_dir() or target.resolve() != target:
        raise OpsError("double-root insert_log must be a real directory inside this data root")
    if link.is_symlink():
        if os.readlink(link) != ALIAS_TARGET:
            raise OpsError("insert_log is an unrelated symlink; refusing to replace it")
        original = "alias"
    elif link.exists():
        if not link.is_dir() or any(link.iterdir()):
            raise OpsError("insert_log already has data; refusing to replace it")
        original = "empty_directory"
    else:
        original = "absent"
    plan = {"dry_run": not apply, "action": "rollback" if rollback else "repair-host",
            "path": str(link), "target": ALIAS_TARGET, "current": original}
    if not apply:
        return plan
    if not backup:
        raise OpsError("--apply requires --backup-root with a complete stopped-volume copy")
    if original == "alias" and not rollback:
        plan["changed"] = False
        return plan
    receipt_path = Path(backup) / RECEIPT
    if receipt_path.is_symlink():
        raise OpsError("repair receipt cannot be a symlink")
    receipt = json.loads(receipt_path.read_text()) if receipt_path.exists() else None
    if receipt and (receipt.get("root") != str(root) or receipt.get("target") != ALIAS_TARGET):
        raise OpsError("backup contains an unrelated repair receipt")
    if rollback:
        if not receipt:
            raise OpsError("rollback requires this tool's repair receipt")
        check_backup(root, backup, receipt["backup_sha256"])
        if original == "alias":
            link.unlink()
            if receipt["original"] == "empty_directory":
                link.mkdir()
        plan["changed"] = original == "alias"
        return plan
    _, fingerprint = check_backup(root, backup)
    if not receipt:
        with receipt_path.open("x") as stream:
            json.dump({"root": str(root), "target": ALIAS_TARGET, "original": original, "backup_sha256": fingerprint}, stream)
    if original == "empty_directory":
        link.rmdir()  # Only an empty directory is replaceable; never recursive deletion.
    link.symlink_to(ALIAS_TARGET, target_is_directory=True)
    plan["changed"] = True
    return plan


class API:
    def __init__(self, endpoint):
        parsed = urllib.parse.urlsplit(endpoint)
        if parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost", "::1"} or parsed.username or parsed.password or parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
            raise OpsError("endpoint must be a credential-free loopback HTTP origin")
        self.endpoint = endpoint.rstrip("/")

    def call(self, operation, body):
        if operation not in {"collections/list", "collections/describe", "collections/get_load_state",
                             "collections/get_stats", "entities/query", "entities/search"}:
            raise OpsError("only read operations are permitted")
        headers = {"Content-Type": "application/json"}
        if os.environ.get("MILVUS_TOKEN"):
            headers["Authorization"] = "Bearer " + os.environ["MILVUS_TOKEN"]
        request = urllib.request.Request(self.endpoint + "/v2/vectordb/" + operation,
                                         json.dumps(body).encode(), headers)
        # Refuse redirects so an auth header or probe vector cannot leave loopback.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, *args, **kwargs):
                return None
        with urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect).open(request, timeout=10) as response:
            value = json.load(response)
        if value.get("code") != 0:
            raise OpsError("Milvus API failed for " + operation + " (details suppressed)")
        return value["data"]


def collections(api, verify=False):
    report = []
    for name in api.call("collections/list", {}):
        body = {"collectionName": name}
        description = api.call("collections/describe", body)
        state = api.call("collections/get_load_state", body)
        stats = api.call("collections/get_stats", body)
        row = {"name": name, "id": description.get("collectionID"), **state, **stats}
        if verify:
            if state.get("loadState") != "LoadStateLoaded":
                raise OpsError("collection is not loaded: " + name)
            count = api.call("entities/query", {**body, "filter": "", "outputFields": ["count(*)"]})
            row["read_count"] = count[0]["count(*)"]
            fields = description.get("fields", [])
            primary = next((f["name"] for f in fields if f.get("primaryKey")), None)
            vector = next((f["name"] for f in fields if f.get("type") == "FloatVector"), None)
            input_field, search_field, mode = vector, vector, "dense"
            if not vector:
                bm25 = next((f for f in description.get("functions", []) if f.get("type") == 1), None)
                if bm25:
                    input_field, search_field, mode = bm25["inputFieldNames"][0], bm25["outputFieldNames"][0], "bm25"
            if primary and input_field and row["read_count"]:
                sample = api.call("entities/query", {**body, "filter": "", "outputFields": [primary, input_field], "limit": 1})
                hits = api.call("entities/search", {**body, "data": [sample[0][input_field]], "annsField": search_field,
                                                    "filter": primary + " == " + json.dumps(sample[0][primary]),
                                                    "outputFields": [primary], "limit": 1})
                if not hits or not any(isinstance(hit, dict) and hit.get(primary, hit.get("id")) == sample[0][primary] for hit in hits):
                    raise OpsError("search probe did not return the selected primary key: " + name)
                row["search"] = mode + "_passed"
            else:
                row["search"] = "not_tested_empty_or_unsupported_vector"
        report.append(row)
    return report


def missing_paths(logs, root):
    keys = sorted(set(re.findall(r"key=((?:files/|/var/lib/milvus/data/)[^\]\\\s]+)", logs)))
    result = []
    for key in keys:
        relative = key.removeprefix(CONTAINER_DATA + "/") if key.startswith("/") else key
        if ".." in Path(relative).parts:
            continue
        candidates = [root / relative]
        if key.startswith(CONTAINER_DATA + "/"):
            candidates.append(root / key.lstrip("/"))
        matches = [str(p.relative_to(root)) for p in candidates if p.is_file() and p.resolve().is_relative_to(root)]
        result.append({"key": key, "existing_host_paths": matches})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["diagnose", "verify", "repair-host", "rollback"])
    parser.add_argument("--container", default="milvus-standalone")
    parser.add_argument("--data-root", required=True, help="exact host bind source plus /data")
    parser.add_argument("--endpoint", default="http://127.0.0.1:19530")
    parser.add_argument("--backup-root", help="stopped-volume copy containing data/etcd/rdb_data/rdb_data_meta_kv")
    parser.add_argument("--apply", action="store_true", help="apply the one fixed host symlink operation")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", args.container):
        parser.error("invalid container name")
    if args.apply and args.action not in {"repair-host", "rollback"}:
        parser.error("--apply is only supported for repair-host and rollback")
    try:
        info = json.loads(docker("inspect", args.container))[0]
        root = inspect_storage(info, args.data_root)
        if args.action in {"repair-host", "rollback"}:
            if args.apply:
                ensure_volume_stopped(root)
            output = host_alias(root, info, args.backup_root, args.apply, args.action == "rollback")
        else:
            output = {"collections": collections(API(args.endpoint), args.action == "verify")}
            logs = docker("logs", "--since", "2m", "--tail", "12000", args.container)
            # Docker writes Milvus logs to stderr too; never display unfiltered logs.
            output["missing_keys"] = missing_paths(logs, root)
            output["persistent_files_mount"] = any(m["Destination"] == "/milvus/files" and
                m.get("Type") == "bind" and m.get("RW", False) and
                Path(m["Source"]).resolve() == root / "files" for m in info["Mounts"])
            alias = root / "insert_log"
            output["host_insert_log_alias"] = alias.is_symlink() and os.readlink(alias) == ALIAS_TARGET
        print(json.dumps(output, indent=2, ensure_ascii=False))
        if args.action == "verify" and (not output["persistent_files_mount"] or
                ((root / ALIAS_TARGET).exists() and not output["host_insert_log_alias"])):
            return 1
        return 0
    except (ValueError, KeyError, IndexError, TypeError, OSError, subprocess.SubprocessError, urllib.error.URLError) as error:
        # Exceptions may contain URLs or response bodies; only our own validation messages are shown.
        print("error: " + (str(error) if isinstance(error, OpsError) else type(error).__name__), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
