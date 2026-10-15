import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "milvus_local_paths.py"
spec = importlib.util.spec_from_file_location("milvus_local_paths", SCRIPT)
ops = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ops)


class PathRepairTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name).resolve()
        self.volume = base / "live"
        self.root = self.volume / "data"
        self.backup = base / "backup"
        for name in ("data", "etcd", "rdb_data", "rdb_data_meta_kv"):
            (self.volume / name).mkdir(parents=True)
            (self.volume / name / "marker").write_bytes(b"keep exact data")
        target = self.root / ops.ALIAS_TARGET
        target.mkdir(parents=True)
        (target / "bloom").write_bytes(b"original bloom bytes")
        shutil.copytree(self.volume, self.backup)
        self.info = {"State": {"Running": False}, "Config": {"Env": ["COMMON_STORAGETYPE=local"]},
                     "Mounts": [{"Type": "bind", "Source": str(self.volume), "Destination": "/var/lib/milvus"}]}

    def test_inventory_fails_closed_on_unreadable_directory(self):
        def walk_error(*args, **kwargs):
            kwargs["onerror"](PermissionError("cannot read folder"))
        with patch.object(ops.os, "walk", side_effect=walk_error):
            with self.assertRaises(PermissionError):
                ops.file_inventory(self.volume)

    def test_other_running_writer_is_rejected(self):
        mounts = [{"Mounts": [{"Type": "bind", "Source": str(self.volume), "RW": True}]}]
        with patch.object(ops, "docker", side_effect=["other-id", json.dumps(mounts)]):
            with self.assertRaisesRegex(ValueError, "another running"):
                ops.ensure_volume_stopped(self.root)

    def test_dry_run_never_changes_files(self):
        before = ops.file_inventory(self.volume)
        out = ops.host_alias(self.root, self.info, None)
        self.assertTrue(out["dry_run"])
        self.assertFalse((self.root / "insert_log").exists())
        self.assertEqual(before, ops.file_inventory(self.volume))
        self.assertFalse((self.backup / ops.RECEIPT).exists())

    def test_apply_is_idempotent_and_rollback_preserves_new_data(self):
        out = ops.host_alias(self.root, self.info, self.backup, apply=True)
        self.assertTrue(out["changed"])
        self.assertEqual((self.root / "insert_log/bloom").read_bytes(), b"original bloom bytes")
        (self.root / "insert_log/new").write_bytes(b"later write")
        self.assertFalse(ops.host_alias(self.root, self.info, self.backup, apply=True)["changed"])
        self.assertTrue(ops.host_alias(self.root, self.info, self.backup, apply=True, rollback=True)["changed"])
        self.assertFalse((self.root / "insert_log").exists())
        self.assertEqual((self.root / ops.ALIAS_TARGET / "new").read_bytes(), b"later write")
        self.assertFalse(ops.host_alias(self.root, self.info, self.backup, apply=True, rollback=True)["changed"])

    def test_empty_directory_is_restored(self):
        (self.root / "insert_log").mkdir()
        ops.host_alias(self.root, self.info, self.backup, apply=True)
        ops.host_alias(self.root, self.info, self.backup, apply=True, rollback=True)
        self.assertTrue((self.root / "insert_log").is_dir())
        self.assertFalse((self.root / "insert_log").is_symlink())
        self.assertEqual(list((self.root / "insert_log").iterdir()), [])

    def test_refuses_running_container_and_missing_backup(self):
        self.info["State"]["Running"] = True
        with self.assertRaisesRegex(ValueError, "stop"):
            ops.host_alias(self.root, self.info, self.backup, apply=True)
        self.info["State"]["Running"] = False
        with self.assertRaisesRegex(ValueError, "backup-root"):
            ops.host_alias(self.root, self.info, None, apply=True)

    def test_refuses_nonempty_directory_and_unrelated_symlink(self):
        link = self.root / "insert_log"
        link.mkdir()
        (link / "valuable").write_text("keep")
        with self.assertRaisesRegex(ValueError, "already has data"):
            ops.host_alias(self.root, self.info, self.backup, apply=True)
        self.assertEqual((link / "valuable").read_text(), "keep")
        shutil.rmtree(link)
        link.symlink_to("../etcd")
        with self.assertRaisesRegex(ValueError, "unrelated"):
            ops.host_alias(self.root, self.info, self.backup, apply=True)

    def test_refuses_changed_backup_and_changed_live_volume(self):
        (self.backup / "etcd/marker").write_text("not a complete copy")
        with self.assertRaisesRegex(ValueError, "differ"):
            ops.host_alias(self.root, self.info, self.backup, apply=True)
        self.assertFalse((self.root / "insert_log").exists())
        (self.backup / "etcd/marker").write_bytes(b"keep exact data")
        ops.host_alias(self.root, self.info, self.backup, apply=True)
        (self.backup / "etcd/marker").write_text("changed after repair")
        with self.assertRaisesRegex(ValueError, "backup has changed"):
            ops.host_alias(self.root, self.info, self.backup, apply=True, rollback=True)
        self.assertTrue((self.root / "insert_log").is_symlink())

    def test_root_must_match_inspect_and_cannot_escape(self):
        self.assertEqual(ops.inspect_storage(self.info, str(self.root)), self.root)
        with self.assertRaisesRegex(ValueError, "does not match"):
            ops.inspect_storage(self.info, str(self.backup / "data"))
        with self.assertRaisesRegex(ValueError, "canonical"):
            ops.inspect_storage(self.info, str(self.root / ".." / "data"))
        self.info["Config"]["Env"] = ["COMMON_STORAGETYPE=minio"]
        with self.assertRaisesRegex(ValueError, "local"):
            ops.inspect_storage(self.info, str(self.root))

    def test_missing_key_diagnosis_checks_both_layouts_without_contents(self):
        relative = "files/insert_log/1/_stats/bloom_filter.100/2"
        path = self.root / relative
        path.parent.mkdir(parents=True)
        path.write_text("PRIVATE BLOOM")
        absolute = "/var/lib/milvus/data/insert_log/bloom"
        logs = f"error[key={relative}] error[key={absolute}] error[key=files/../../etcd/marker]"
        rows = ops.missing_paths(logs, self.root)
        self.assertEqual(len(rows), 2)
        self.assertTrue(all(row["existing_host_paths"] for row in rows))
        self.assertNotIn("PRIVATE", json.dumps(rows))


class APISafetyTests(unittest.TestCase):
    def test_loopback_only_no_credentials_no_writes(self):
        for url in ("http://example.com", "http://user:password@localhost", "http://localhost/path", "https://localhost", "http://localhost?token=secret"):
            with self.assertRaises(ValueError):
                ops.API(url)
        api = ops.API("http://127.0.0.1:19530")
        with self.assertRaisesRegex(ValueError, "read operations"):
            api.call("collections/drop", {"collectionName": "valuable"})

    def test_probe_reports_counts_and_search_without_private_values(self):
        for vector, functions, mode in (("FloatVector", [], "dense_passed"), ("SparseFloatVector", [
                {"type": 1, "inputFieldNames": ["text"], "outputFieldNames": ["vector"]}], "bm25_passed")):
            class FakeAPI:
                wrong_primary_key = False

                def call(self, operation, body):
                    if operation == "collections/list":
                        return ["test"]
                    if operation == "collections/describe":
                        return {"collectionID": 1, "fields": [{"name": "pk", "primaryKey": True}, {"name": "vector", "type": vector}], "functions": functions}
                    if operation == "collections/get_load_state":
                        return {"loadState": "LoadStateLoaded", "loadProgress": 100}
                    if operation == "collections/get_stats":
                        return {"rowCount": 1}
                    if operation == "entities/query":
                        return [{"count(*)": 1}] if body["outputFields"] == ["count(*)"] else [{"pk": "PRIVATE ID", "vector": [1.0], "text": "PRIVATE TEXT"}]
                    if operation == "entities/search":
                        assert body["filter"] == 'pk == "PRIVATE ID"'
                        return [{"pk": "OTHER ID" if self.wrong_primary_key else "PRIVATE ID"}]
                    raise AssertionError("unexpected operation")
            report = ops.collections(FakeAPI(), verify=True)
            self.assertEqual(report[0]["search"], mode)
            self.assertNotIn("PRIVATE", json.dumps(report))
            mismatched = FakeAPI()
            mismatched.wrong_primary_key = True
            with self.assertRaisesRegex(ValueError, "selected primary key"):
                ops.collections(mismatched, verify=True)

    def test_docker_logs_include_stderr_but_inspect_does_not(self):
        class Result:
            returncode, stdout, stderr = 0, "stdout", "stderr"
        with patch.object(ops.subprocess, "run", return_value=Result()):
            self.assertEqual(ops.docker("logs", "container"), "stdoutstderr")
            self.assertEqual(ops.docker("inspect", "container"), "stdout")


if __name__ == "__main__":
    unittest.main()
