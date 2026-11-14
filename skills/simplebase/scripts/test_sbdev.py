#!/usr/bin/env python3
"""sbdev 验收测试（planv6.4 P0a）。

用一个带状态文件的假 `simplebase` 可执行脚本模拟远端，不依赖真实服务。
运行：python3 -m unittest discover -s skills/simplebase/scripts -p 'test_sbdev.py'
"""

from __future__ import annotations

import argparse
import contextlib
import http.server
import io
import json
import os
import sys
import tempfile
import textwrap
import threading
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import sbdev  # noqa: E402

# 假 CLI：状态保存在 $FAKE_SB_STATE（JSON），输出信封与真实 CLI 一致。
FAKE_CLI = r'''
import json, os, sys

STATE = os.environ["FAKE_SB_STATE"]


def load():
    if os.path.exists(STATE):
        with open(STATE, encoding="utf-8") as fh:
            return json.load(fh)
    return {"dbs": {}, "funcs": {}, "cron": {}, "collections": {}, "batches": [], "calls": [], "n": 0}


def save(st):
    with open(STATE, "w", encoding="utf-8") as fh:
        json.dump(st, fh)


def ok(data):
    print(json.dumps({"ok": True, "data": data}))
    sys.exit(0)


def fail(code, message="x"):
    print(json.dumps({"ok": False, "error": {"code": code, "message": message}}), file=sys.stderr)
    sys.exit(2)


def flags(argv):
    out, i = {}, 0
    while i < len(argv):
        a = argv[i]
        if not a.startswith("--"):
            i += 1
            continue
        key = a[2:]
        if key == "transactional":
            out[key] = True
            i += 1
        else:
            out[key] = argv[i + 1]
            i += 2
    return out


st = load()
argv = sys.argv[1:]
res, act = argv[0], argv[1]
sub = ""
rest = argv[2:]
if res == "function" and act == "versions":
    sub, rest = argv[2], argv[3:]
f = flags(rest)
st["calls"].append(" ".join([res, act, sub]).strip())
tokens = (os.environ.get("SIMPLEBASE_TOKEN"), os.environ.get("SIMPLEBASE_URL"), os.environ.get("SIMPLEBASE_PROJECT_ID"))
if not all(tokens):
    fail("config_missing")
if any(a == tokens[0] for a in argv):
    fail("token_in_argv")


def src_of(name, version):
    return st["funcs"][name]["versions"][int(version) - 1]["source"]


def func_dto(name):
    fn = st["funcs"][name]
    active = fn["active"]
    pick = active or len(fn["versions"])
    return {"name": name, "description": fn["description"], "active_version": active,
            "latest_version": len(fn["versions"]), "source": src_of(name, pick)}


key = (res, act, sub)
if key == ("database", "list", ""):
    ok({"databases": list(st["dbs"].values()), "next_cursor": ""})
elif key == ("database", "create", ""):
    st["n"] += 1
    d = {"id": "db-%d" % st["n"], "name": f["name"], "data_model": f.get("data-model", "collection"), "migs": [], "has_mig": False}
    st["dbs"][d["id"]] = d
    save(st)
    ok({"id": d["id"], "name": d["name"], "data_model": d["data_model"]})
elif key == ("sql", "exec", ""):
    if "CREATE TABLE IF NOT EXISTS _migrations" in f["statement"]:
        st["dbs"][f["database"]]["has_mig"] = True
        save(st)
    if f["statement"].startswith("INSERT INTO _migrations"):
        st["dbs"][f["database"]]["migs"].append(json.loads(f["param"])[0])
        save(st)
    ok({"rows_affected": 0})
elif key == ("document", "insert", ""):
    # 与真实服务端一致：忽略传入的 id，总是生成新 id。
    st["n"] += 1
    st.setdefault("docs", []).append("doc-%d" % st["n"])
    save(st)
    ok({"id": "doc-%d" % st["n"]})
elif key == ("document", "get", ""):
    fail("not_found")
elif key == ("sql", "query", ""):
    d = st["dbs"][f["database"]]
    if not d["has_mig"]:
        fail("table_not_found", "_migrations does not exist")
    ok({"columns": ["name"], "rows": [[m] for m in d["migs"]], "row_count": len(d["migs"])})
elif key == ("schema", "show", ""):
    d = st["dbs"][f["database"]]
    ok({"tables": [{"name": "_migrations"}] if d["has_mig"] else []})
elif key == ("sql", "batch", ""):
    stmts = json.loads(sys.stdin.read())
    d = st["dbs"][f["database"]]
    if any("FAIL_SQL" in s["sql"] for s in stmts):
        ok({"results": [], "error": {"failed_index": 0, "code": "sql_failed", "message": "boom"}})
    marker = stmts[-1]["args"][0]
    d["migs"].append(marker)
    st["batches"].append(marker)
    save(st)
    ok({"results": [{"index": i, "rows_affected": 0} for i in range(len(stmts))]})
elif key == ("collection", "list", ""):
    ok({"collections": st["collections"].get(f["database"], []), "items": []})
elif key == ("collection", "create", ""):
    st["collections"].setdefault(f["database"], []).append(f["name"])
    save(st)
    ok({"ok": True, "name": f["name"]})
elif key == ("function", "get", ""):
    if f["name"] not in st["funcs"]:
        fail("not_found")
    ok(func_dto(f["name"]))
elif key == ("function", "create", ""):
    source = open(f["file"], encoding="utf-8").read()
    active = 1 if f.get("activate", "true") == "true" else 0
    st["funcs"][f["name"]] = {"description": f.get("description", ""), "active": active, "versions": [{"source": source}]}
    save(st)
    ok({"name": f["name"], "latest_version": 1, "active_version": active})
elif key == ("function", "update", ""):
    st["funcs"][f["name"]]["description"] = f["description"]
    save(st)
    ok({"name": f["name"]})
elif key == ("function", "versions", "create"):
    source = open(f["file"], encoding="utf-8").read()
    st["funcs"][f["name"]]["versions"].append({"source": source})
    save(st)
    ok({"latest_version": len(st["funcs"][f["name"]]["versions"])})
elif key == ("function", "versions", "get"):
    ok({"version": int(f["version"]), "source": src_of(f["name"], f["version"])})
elif key == ("function", "versions", "list"):
    ok({"versions": [{"version": i + 1} for i in range(len(st["funcs"][f["name"]]["versions"]))]})
elif key == ("function", "test", ""):
    good = "FAIL_TEST" not in src_of(f["name"], f["version"])
    ok({"ok": good, "error": None if good else "试跑失败"})
elif key == ("function", "activate", ""):
    st["funcs"][f["name"]]["active"] = int(f["version"])
    save(st)
    ok({"active_version": int(f["version"])})
elif key == ("function", "invoke", ""):
    fn = st["funcs"].get(f["name"])
    if not fn or not fn["active"]:
        fail("function_not_active")
    ok({"message": "hello, " + json.loads(f.get("input") or "{}").get("name", "world")})
elif key == ("project", "list", ""):
    if os.environ.get("FAKE_SB_NO_PROJECT_LIST") == "1":
        fail("forbidden", "missing permission")
    ok({"projects": [{"id": "myapp001", "engine": st.get("engine", "duckdb")}]})
elif key == ("object", "upload", ""):
    # 与真实 CLI 一致：读取 --file 内容并按 key 记账（站点上传测试用）。
    data = open(f["file"], "rb").read()
    st.setdefault("objects", {})[f["key"]] = len(data)
    save(st)
    ok({"key": f["key"], "size": len(data)})
elif key == ("object", "list", ""):
    ok([{"key": k, "size": v} for k, v in sorted(st.setdefault("objects", {}).items())])
elif key == ("cron", "list", ""):
    ok({"jobs": list(st["cron"].values())})
elif key in (("cron", "create", ""), ("cron", "update", "")):
    if act == "create":
        st["n"] += 1
        job = {"id": "job-%d" % st["n"], "name": f["name"], "target_kind": "function"}
    else:
        job = next(j for j in st["cron"].values() if j["id"] == f["id"])
        job["description"] = ""  # update 是全量替换（与服务端一致）
    mapping = {"description": "description", "schedule-kind": "schedule_kind", "cron": "cron_expr",
               "run-at": "run_at", "func-file": "func_file", "func-export": "func_export", "input": "input_json"}
    for k, v in mapping.items():
        if k in f:
            job[v] = f[k]
    if "interval-seconds" in f:
        job["interval_seconds"] = int(f["interval-seconds"])
    if "enabled" in f:
        job["enabled"] = f["enabled"] == "true"
    st["cron"][job["id"]] = job
    save(st)
    ok(job)
else:
    fail("usage", "unknown command " + " ".join(key))
'''


class SbdevCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.ws = base / "ws"
        self.state = base / "state.json"
        self.bin = base / "simplebase"
        # shebang 不支持带空格的解释器路径（如 macOS 的 "Application Support"），用 sh 包装再 exec。
        fake_py = base / "fake_simplebase.py"
        fake_py.write_text(textwrap.dedent(FAKE_CLI), encoding="utf-8")
        self.bin.write_text(f'#!/bin/sh\nexec "{sys.executable}" "{fake_py}" "$@"\n', encoding="utf-8")
        self.bin.chmod(0o755)
        env = {k: v for k, v in os.environ.items() if not k.startswith("SIMPLEBASE_")}
        env["FAKE_SB_STATE"] = str(self.state)
        patcher = mock.patch.dict(os.environ, env, clear=True)
        patcher.start()
        self.addCleanup(patcher.stop)
        self.run_cmd("init", "--project-id", "myapp001")
        (self.ws / ".env").write_text(
            "SIMPLEBASE_URL=http://127.0.0.1:8080\nSIMPLEBASE_API_KEY=local-secret-key\nSIMPLEBASE_PROJECT_ID=myapp001\n",
            encoding="utf-8")
        (self.ws / ".env.cloud").write_text(
            "SIMPLEBASE_URL=https://cloud.example.com\nSIMPLEBASE_API_KEY=cloud-secret-key\nSIMPLEBASE_PROJECT_ID=myapp001\n",
            encoding="utf-8")

    # 辅助
    def run_cmd(self, *args):
        argv = list(args)
        if argv[0] in ("plan", "apply", "doctor", "site"):
            argv += ["--bin", str(self.bin)]
        argv += ["-C", str(self.ws)]
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = sbdev.main(argv)
        return code, out.getvalue(), err.getvalue()

    def fake(self, *args, stdin=None):
        import subprocess
        env = dict(os.environ, SIMPLEBASE_URL="http://127.0.0.1:8080", SIMPLEBASE_TOKEN="t", SIMPLEBASE_PROJECT_ID="myapp001")
        p = subprocess.run([str(self.bin), *args], input=stdin, capture_output=True, text=True, env=env)
        return p.returncode, p.stdout, p.stderr

    def st(self) -> dict:
        return json.loads(self.state.read_text(encoding="utf-8"))

    @staticmethod
    def changed(out: str) -> list:
        return [ln for ln in out.splitlines() if ln.split()[:1] and ln.split()[0] in ("create", "update", "apply")]

    def write(self, rel: str, content: str) -> None:
        p = self.ws / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")

    def edit_manifest(self, fn) -> None:
        p = self.ws / "simplebase.json"
        m = json.loads(p.read_text(encoding="utf-8"))
        fn(m)
        p.write_text(json.dumps(m, indent=2), encoding="utf-8")


class AcceptanceTests(SbdevCase):
    # 验收 1：init → 本地 apply → function invoke
    def test_init_apply_then_invoke(self):
        code, out, err = self.run_cmd("validate")
        self.assertEqual(code, 0, err)
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        s = self.st()
        self.assertEqual(s["funcs"]["hello"]["active"], 1)
        self.assertEqual(s["batches"], ["001_init.sql"])
        code, out, _ = self.fake("function", "invoke", "--name", "hello", "--export", "Hello", "--input", '{"name":"sb"}')
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(out)["data"]["message"], "hello, sb")

    # 验收 2：新增 002 后只执行 002
    def test_new_migration_runs_only_new_file(self):
        self.run_cmd("apply")
        self.write("migrations/002_more.sql", "CREATE TABLE IF NOT EXISTS tags (id VARCHAR);\n")
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(self.st()["batches"], ["001_init.sql", "002_more.sql"])
        self.assertEqual(len(self.changed(out)), 1, out)

    # 验收 3：连续 apply 两次，第二次全部 skip
    def test_second_apply_is_all_skip(self):
        self.edit_manifest(lambda m: m.update(
            collections=[{"database": "docs", "name": "posts"}],
            cron_jobs=[{"name": "daily", "schedule_kind": "cron", "cron_expr": "0 3 * * *",
                        "func_file": "hello", "func_export": "Hello", "input_json": "{}", "enabled": True}]))
        self.edit_manifest(lambda m: m["databases"].append({"name": "docs", "data_model": "collection"}))
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertGreaterEqual(len(self.changed(out)), 5, out)
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(self.changed(out), [], out)
        self.assertIn("0 项变更", out)
        code, out, _ = self.run_cmd("plan")
        self.assertEqual(code, 0)
        self.assertIn("计划：0 项变更", out)

    # 验收 4：改已应用的迁移 → migration_modified
    def test_modified_migration_is_rejected(self):
        self.run_cmd("apply")
        self.write("migrations/001_init.sql", "CREATE TABLE IF NOT EXISTS notes (id VARCHAR, extra VARCHAR);\n")
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertIn("migration_modified", err)
        self.assertEqual(self.st()["batches"], ["001_init.sql"])

    # 验收 5：函数源码不变不新增版本；变化时新版本、试跑后才生效
    def test_function_versions(self):
        self.run_cmd("apply")
        self.run_cmd("apply")
        self.assertEqual(len(self.st()["funcs"]["hello"]["versions"]), 1)
        src = (self.ws / "functions/hello.go").read_text(encoding="utf-8")
        self.write("functions/hello.go", src + "\n// v2\n")
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        fn = self.st()["funcs"]["hello"]
        self.assertEqual((len(fn["versions"]), fn["active"]), (2, 2))
        self.run_cmd("apply")
        self.assertEqual(len(self.st()["funcs"]["hello"]["versions"]), 2)

    def test_function_test_failure_keeps_old_version_active(self):
        self.run_cmd("apply")
        src = (self.ws / "functions/hello.go").read_text(encoding="utf-8")
        self.write("functions/hello.go", src + "\n// FAIL_TEST\n")
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, sbdev.EXIT_REMOTE)
        self.assertIn("试跑失败", err)
        self.assertEqual(self.st()["funcs"]["hello"]["active"], 1)
        # 失败后重跑复用未生效的同源码版本，不继续耗版本
        self.run_cmd("apply")
        self.assertEqual(len(self.st()["funcs"]["hello"]["versions"]), 2)

    def test_new_function_is_tested_before_activation(self):
        self.write("functions/hello.go",
                   (self.ws / "functions/hello.go").read_text(encoding="utf-8") + "\n// FAIL_TEST\n")
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, sbdev.EXIT_REMOTE, err)
        self.assertEqual(self.st()["funcs"]["hello"]["active"], 0)

    # 验收 6（脚本侧）：远端未带 --yes 退出码 3 且不写入；带 --yes 后写入，环境提示不含密钥
    def test_remote_requires_yes(self):
        code, out, _ = self.run_cmd("apply", "--env-file", str(self.ws / ".env.cloud"))
        self.assertEqual(code, sbdev.EXIT_CONFIRM)
        self.assertIn("--yes", out)
        self.assertEqual(self.st()["dbs"] if self.state.exists() else {}, {})
        code, out, err = self.run_cmd("apply", "--env-file", str(self.ws / ".env.cloud"), "--yes")
        self.assertEqual(code, 0, err)
        hint = (self.ws / ".simplebase/env.cloud-example-com").read_text(encoding="utf-8")
        self.assertIn("SIMPLEBASE_URL=https://cloud.example.com", hint)
        self.assertIn("SIMPLEBASE_PROJECT_ID=myapp001", hint)
        self.assertIn("SIMPLEBASE_DATABASE_ID_MAIN=db-1", hint)
        self.assertNotIn("secret-key", hint)

    # 验收 7：疑似密钥 → validate 失败
    def test_secret_in_function_fails_validate(self):
        src = (self.ws / "functions/hello.go").read_text(encoding="utf-8")
        self.write("functions/hello.go", src + '\nvar leaked = "sk-' + "a" * 24 + '"\n')
        code, _, err = self.run_cmd("validate")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertIn("疑似包含", err)


class BehaviorTests(SbdevCase):
    def test_lock_and_env_never_contain_key(self):
        self.run_cmd("apply")
        for p in (self.ws / ".simplebase").iterdir():
            self.assertNotIn("local-secret-key", p.read_text(encoding="utf-8"), p.name)
        lock = json.loads((self.ws / ".simplebase/lock.json").read_text(encoding="utf-8"))
        self.assertIn("local#myapp001", lock["targets"])  # 本地分区键不含端口

    def test_token_not_passed_in_argv(self):
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertNotIn("token_in_argv", self.state.read_text(encoding="utf-8"))

    def test_failed_migration_rolls_back_and_resumes(self):
        self.write("migrations/002_bad.sql", "CREATE TABLE IF NOT EXISTS t (id VARCHAR);\n-- x\nSELECT 'FAIL_SQL';\n")
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, sbdev.EXIT_REMOTE)
        self.assertIn("002_bad.sql", err)
        self.assertEqual(self.st()["batches"], ["001_init.sql"])
        self.write("migrations/002_bad.sql", "CREATE TABLE IF NOT EXISTS t (id VARCHAR);\n")
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(self.st()["batches"], ["001_init.sql", "002_bad.sql"])

    def test_cron_create_skip_update(self):
        job = {"name": "daily", "schedule_kind": "cron", "cron_expr": "0 3 * * *",
               "func_file": "hello", "func_export": "Hello", "input_json": "{}", "enabled": True}
        self.edit_manifest(lambda m: m.update(cron_jobs=[job]))
        self.run_cmd("apply")
        self.assertEqual(len(self.st()["cron"]), 1)
        _, out, _ = self.run_cmd("apply")
        self.assertEqual(self.changed(out), [])
        job["cron_expr"] = "0 4 * * *"
        self.edit_manifest(lambda m: m.update(cron_jobs=[job]))
        _, out, _ = self.run_cmd("apply")
        self.assertIn("cron_expr", out)
        self.assertEqual(list(self.st()["cron"].values())[0]["cron_expr"], "0 4 * * *")

    def test_expired_once_cron_is_skipped(self):
        job = {"name": "old", "schedule_kind": "once", "run_at": "2020-01-01T00:00:00Z",
               "func_file": "hello", "func_export": "Hello"}
        self.edit_manifest(lambda m: m.update(cron_jobs=[job]))
        code, out, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(self.st()["cron"], {})
        self.assertIn("已过期", out + err)

    def test_seed_only_with_flag(self):
        self.write("seed/dev.sql", "INSERT INTO notes (id) VALUES ('a');\n")
        self.edit_manifest(lambda m: m["databases"][0].update(seed="seed/dev.sql"))
        self.run_cmd("apply")
        self.assertEqual(self.st()["batches"], ["001_init.sql"])
        self.run_cmd("apply", "--seed")
        self.assertEqual(self.st()["batches"], ["001_init.sql", "seed:seed/dev.sql"])
        _, out, _ = self.run_cmd("apply", "--seed")
        self.assertEqual(self.changed(out), [])

    def test_seed_documents_are_idempotent(self):
        # 服务端忽略文档 id（假 CLI 同样），去重只能靠 _migrations 记账。
        self.edit_manifest(lambda m: (
            m["databases"].append({"name": "docs", "data_model": "collection"}),
            m.update(collections=[{"database": "docs", "name": "posts", "seed": [{"id": "demo", "title": "hi"}]}])))
        self.run_cmd("apply", "--seed")
        self.assertEqual(len(self.st().get("docs", [])), 1)
        _, out, _ = self.run_cmd("apply", "--seed")
        self.assertEqual(self.changed(out), [])
        self.assertEqual(len(self.st().get("docs", [])), 1)

    def test_validate_rejects_denied_sql_and_env_reference(self):
        self.write("migrations/002_bad.sql", "ATTACH 'x.db' AS other;\n")
        code, _, err = self.run_cmd("validate")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertIn("ATTACH", err)
        self.write("migrations/002_bad.sql", "CREATE TABLE IF NOT EXISTS t (id VARCHAR);\n")
        self.edit_manifest(lambda m: m["functions"][0].update(file=".env"))
        code, _, err = self.run_cmd("validate")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertIn(".env", err)

    def test_collection_database_may_run_migrations(self):
        self.edit_manifest(lambda m: m["databases"].append(
            {"name": "docs", "data_model": "collection", "migrations": "migrations"}))
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, 0, err)
        self.assertEqual(self.st()["batches"], ["001_init.sql", "001_init.sql"])

    def test_env_file_key_beats_process_env(self):
        # --env-file 指向云端时，不能用进程环境里残留的本地 Key（哪怕变量名不同）
        with mock.patch.dict(os.environ, {"SIMPLEBASE_TOKEN": "stale-local-token"}):
            args = argparse.Namespace(
                env_file=str(self.ws / ".env.cloud"), url=None, project=None, bin=str(self.bin))
            t = sbdev.Target(args, sbdev.Workspace(self.ws).check())
        self.assertEqual((t.url, t.token), ("https://cloud.example.com", "cloud-secret-key"))
        with mock.patch.dict(os.environ, {"SIMPLEBASE_API_KEY": "from-process"}):
            args.env_file = None
            t = sbdev.Target(args, sbdev.Workspace(self.ws).check())
        self.assertEqual(t.token, "from-process")

    def test_missing_credentials(self):
        (self.ws / ".env").unlink()
        code, _, err = self.run_cmd("apply")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertIn("SIMPLEBASE_URL", err)

    def test_plan_does_not_write(self):
        code, out, err = self.run_cmd("plan")
        self.assertEqual(code, 0, err)
        self.assertFalse(self.state.exists() and self.st()["dbs"])
        self.assertIn("create", out)
        self.assertFalse((self.ws / ".simplebase/lock.json").exists())


FIXTURES = Path(__file__).resolve().parents[3] / "internal" / "sbcli" / "testdata" / "dev"


class SiteDeployTests(SbdevCase):
    """site deploy：上传编译产物到对象存储 → 开站点 → 公开前缀 → 返回子域名与 URL。"""

    def serve(self):
        calls = []

        class H(http.server.BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def reply(self, code, body, ctype="application/json"):
                data = body.encode("utf-8")
                self.send_response(code)
                self.send_header("Content-Type", ctype)
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                calls.append(("GET", self.path, self.headers.get("Host")))
                if self.path.endswith("/s3/site"):
                    return self.reply(200, json.dumps({"enabled": False, "publishable": True}))
                if self.headers.get("Host", "").startswith("myapp001.localhost"):
                    return self.reply(200, "<html></html>", "text/html")
                self.reply(404, "{}")

            def do_PUT(self):
                n = int(self.headers.get("Content-Length") or 0)
                body = json.loads(self.rfile.read(n) or b"{}")
                calls.append(("PUT", self.path, body))
                if self.path.endswith("/s3/site"):
                    return self.reply(200, json.dumps({"enabled": True, "publishable": True,
                                                       "baseUrl": f"http://myapp001.localhost:{port}"}))
                self.reply(200, json.dumps({"public": True, "publicUrl": f"http://myapp001.localhost:{port}/{body['key']}"}))

        srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), H)
        port = srv.server_address[1]
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        self.addCleanup(srv.server_close)
        self.addCleanup(srv.shutdown)
        (self.ws / ".env").write_text(
            f"SIMPLEBASE_URL=http://127.0.0.1:{port}\nSIMPLEBASE_API_KEY=local-secret-key\nSIMPLEBASE_PROJECT_ID=myapp001\n",
            encoding="utf-8")
        return port, calls

    def test_deploy_uploads_build_and_returns_subdomain_url(self):
        port, calls = self.serve()
        self.write("dist/index.html", "<!doctype html><script src=\"./assets/app.js\"></script>")
        self.write("dist/assets/app.js", "console.log('hi')")
        code, out, err = self.run_cmd("site", "deploy", "--dist", "dist", "--json")
        self.assertEqual(code, 0, err)
        self.assertEqual(sorted(self.st()["objects"]), ["assets/app.js", "index.html"])
        self.assertIn(("PUT", "/v1/projects/myapp001/s3/public", {"key": "", "kind": "folder", "public": True}), calls)
        self.assertIn("subdomain: myapp001.localhost", out)
        self.assertIn(f"url:       http://myapp001.localhost:{port}/", out)
        result = json.loads(out.strip().splitlines()[-1])
        self.assertEqual(result["subdomain"], "myapp001.localhost")
        self.assertEqual(result["url"], f"http://myapp001.localhost:{port}/")
        self.assertEqual(result["http_status"], 200)
        self.assertNotIn("local-secret-key", out + err)

    def test_deploy_rejects_secret_in_bundle(self):
        self.serve()
        self.write("dist/index.html", "<html></html>")
        self.write("dist/app.js", "const k = 'sb_live_0123456789abcdef0123456789abcdef'")
        code, _, err = self.run_cmd("site", "deploy", "--dist", "dist")
        self.assertEqual(code, sbdev.EXIT_INVALID)
        self.assertFalse(self.state.exists() and self.st().get("objects"))


@unittest.skipUnless(FIXTURES.is_dir(), "共享 fixture 不在（仅 skill 被单独拷贝时跳过）")
class FixtureParityTests(SbdevCase):
    """与 Go 版 `simplebase dev`（internal/sbcli）共用 testdata/dev：校验结论与 plan 动作序列必须一致。"""

    def test_validation_matches_expect(self):
        for name in ("basic", "invalid"):
            exp = json.loads((FIXTURES / name / "expect.json").read_text(encoding="utf-8"))
            ws = sbdev.Workspace(FIXTURES / name).check()
            # 严格逐条比对：错误与警告的文本、顺序、条数都要和 Go 端一致，
            # 避免两端新增/改动提示时静默漂移。
            self.assertEqual(ws.errors, exp.get("errors", []), f"{name}: 错误不一致")
            self.assertEqual(ws.warnings, exp.get("warnings", []), f"{name}: 警告不一致")
            counts = exp.get("counts")
            if counts:
                got = {"databases": len(ws.databases), "migrations": sum(len(d["migrations"]) for d in ws.databases),
                       "collections": len(ws.collections), "functions": len(ws.functions), "cron_jobs": len(ws.cron_jobs)}
                for k, v in counts.items():
                    self.assertEqual(got[k], v, f"{name}.{k}")

    def test_plan_matches_expect(self):
        import shutil
        shutil.rmtree(self.ws)
        shutil.copytree(FIXTURES / "basic", self.ws)
        (self.ws / ".env").write_text(
            "SIMPLEBASE_URL=http://127.0.0.1:8080\nSIMPLEBASE_API_KEY=local-secret-key\nSIMPLEBASE_PROJECT_ID=fixtur01\n",
            encoding="utf-8")
        code, out, err = self.run_cmd("plan")
        self.assertEqual(code, 0, err)
        got = [" ".join(ln.split()[:3]) for ln in self.changed(out)]
        exp = json.loads((FIXTURES / "basic" / "expect.json").read_text(encoding="utf-8"))["plan"]
        self.assertEqual(got, exp)


if __name__ == "__main__":
    unittest.main()


class EngineCheckTests(SbdevCase):
    """引擎一致性检查：能读到就比对，读不到要明确提示「没校验」。"""

    def test_mismatch_warns(self):
        self.run_cmd("apply")  # 第一个环境记录 engine=duckdb
        self.state.write_text(json.dumps({**self.st(), "engine": "sqlite"}), encoding="utf-8")
        code, out, err = self.run_cmd("plan", "--env-file", str(self.ws / ".env.cloud"))
        self.assertEqual(code, 0, err)
        self.assertIn("引擎", out + err)

    def test_no_permission_warns_instead_of_silent(self):
        with mock.patch.dict(os.environ, {"FAKE_SB_NO_PROJECT_LIST": "1"}):
            code, out, err = self.run_cmd("plan")
        self.assertEqual(code, 0, err)
        self.assertIn("跳过", out + err)
        self.assertIn("forbidden", out + err)


class CronDescriptionTests(SbdevCase):
    """清单写了 description（含空串）才同步：显式清空生效，没写则保留远端值。"""

    def job(self, desc=None):
        j = {"name": "daily", "schedule_kind": "cron", "cron_expr": "0 3 * * *",
             "func_file": "hello", "func_export": "Hello", "input_json": "{}", "enabled": True}
        if desc is not None:
            j["description"] = desc
        return j

    def test_explicit_empty_clears_and_missing_field_keeps(self):
        self.edit_manifest(lambda m: m.update(cron_jobs=[self.job("夜间汇总")]))
        self.assertEqual(self.run_cmd("apply")[0], 0)
        self.assertEqual(list(self.st()["cron"].values())[0]["description"], "夜间汇总")
        self.edit_manifest(lambda m: m.update(cron_jobs=[self.job("")]))
        self.assertEqual(self.run_cmd("apply")[0], 0)
        self.assertEqual(list(self.st()["cron"].values())[0]["description"], "")
        self.edit_manifest(lambda m: m.update(cron_jobs=[self.job("夜间汇总")]))
        self.assertEqual(self.run_cmd("apply")[0], 0)
        self.edit_manifest(lambda m: m.update(cron_jobs=[self.job()]))
        self.assertEqual(self.run_cmd("apply")[0], 0)
        self.assertEqual(list(self.st()["cron"].values())[0]["description"], "夜间汇总")


class SQLSplitterTests(unittest.TestCase):
    """与 Go 端 devSplitSQL / devMaskSQL 同一套行为。"""

    def test_quotes_and_comments(self):
        got = sbdev.split_sql(
            "INSERT INTO t (id, note) VALUES ('a;b', \"c;d\");\n-- 注释;\nUPDATE t SET n = $$x;y$$;\n")
        self.assertEqual(len(got), 2, got)
        self.assertIn("$$x;y$$", got[1])

    def test_literals_are_masked(self):
        masked = sbdev.mask_sql("INSERT INTO t (v) VALUES ('PRIMARY KEY; ATTACH');")
        self.assertEqual(len(masked), 1)
        self.assertIsNone(sbdev.DUCKLAKE_UNSUPPORTED.search(masked[0]))
        self.assertNotIn("ATTACH", masked[0])

    def test_workspace_uses_masked_statements(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "migrations").mkdir()
            (root / "migrations/001_init.sql").write_text(
                "INSERT INTO notes (id, title) VALUES ('a', 'ATTACH; PRIMARY KEY');\n", encoding="utf-8")
            (root / "simplebase.json").write_text(json.dumps({
                "project": {"id": "myapp001", "name": "x"},
                "databases": [{"name": "main", "data_model": "sql", "migrations": "migrations"}],
            }), encoding="utf-8")
            ws = sbdev.Workspace(root).check()
            self.assertEqual(ws.errors, [])
            self.assertEqual(ws.warnings, [])
