#!/usr/bin/env python3
"""sbdev — SimpleBase 工作区重放与静态站点发布工具（planv6.4 P0a / planv6.5）。

把工作区（simplebase.json + 迁移 + 云函数 + 定时任务）幂等地重放到任意一套
SimpleBase 实例，并支持将前端编译产物上传到对象存储、开启子域名站点访问。
只用 Python 3 标准库；所有远端 CLI 操作都通过 `simplebase`，凭据经子进程
环境变量或内存请求头传入，不出现在 argv、日志或 lock 中。

子命令：init / doctor / validate / plan / apply / status / site。退出码：
0 成功；1 校验或用法错误；2 远端错误；3 需要确认（远端写操作未带 --yes）。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import secrets
import shutil
import string
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

VERSION = "0.1.0"
MANIFEST = "simplebase.json"
STATE_DIR = ".simplebase"
LOCK_NAME = "lock.json"
MIGRATIONS_TABLE = "_migrations"
MAX_BATCH = 100  # limits.max_batch_statements 默认值
MAX_SOURCE = 256 * 1024
MAX_FUNCTIONS = 100
MAX_CRON_INPUT = 8 * 1024

EXIT_OK, EXIT_INVALID, EXIT_REMOTE, EXIT_CONFIRM = 0, 1, 2, 3

LOCAL_HOSTS = {"127.0.0.1", "localhost", "::1", "0.0.0.0"}
FUNC_NAME = re.compile(r"^[A-Za-z][A-Za-z0-9_]{0,62}$")
CRON_NAME = re.compile(r"^[A-Za-z][A-Za-z0-9_-]{0,62}$")
PROJECT_ID = re.compile(r"^[A-Za-z0-9-]{8}$")
SITE_PROJECT_ID = re.compile(r"^[a-z0-9][a-z0-9-]{6}[a-z0-9]$")
EXPORT_FUNC = re.compile(r"^func\s+([A-Z][A-Za-z0-9_]*)\s*\(", re.M)
ALLOWED_IMPORTS = {
    "fmt", "strings", "math", "time", "regexp", "strconv", "errors", "bytes", "io",
    "io/ioutil", "sync/atomic", "net/http", "encoding/json", "encoding/base64",
    "encoding/binary", "github.com/json-iterator/go",
}
DENIED_SQL_KEYWORDS = {
    "ATTACH", "DETACH", "INSTALL", "LOAD", "LOAD_EXTENSION", "COPY", "EXPORT", "IMPORT",
    "SET", "PRAGMA", "CHECKPOINT", "CALL", "USE", "VACUUM", "PREPARE", "EXECUTE", "DEALLOCATE",
}
RESERVED_IDENT = re.compile(r"\b(__sb_|__ducklake_metadata_|sqlite_)\w*", re.I)
DUCKLAKE_UNSUPPORTED = re.compile(r"\b(PRIMARY\s+KEY|UNIQUE|CREATE\s+(UNIQUE\s+)?INDEX|CREATE\s+SEQUENCE)\b", re.I)
SECRET_PATTERNS = [
    ("私钥", re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
    ("OpenAI 风格 key", re.compile(r"\bsk-[A-Za-z0-9_-]{20,}")),
    ("腾讯云 SecretId", re.compile(r"\bAKID[A-Za-z0-9]{13,}")),
    ("AWS AccessKey", re.compile(r"\bAKIA[0-9A-Z]{16}\b")),
    ("GitHub token", re.compile(r"\bgh[pousr]_[A-Za-z0-9]{30,}")),
    ("SimpleBase API Key", re.compile(r"\bsb_live_[0-9a-f]{32}\b")),
]
IGNORED_KEYS = {
    "kv": "kv 属于数据，P1 再纳入 --seed",
    "objects": "对象属于数据，P1 再纳入 --seed",
    "api_keys": "签发 Key 会产出密钥，请在控制台操作",
    "agents": "助手不在重放范围",
    "requires": "仅 examples 使用",
    "function_triggers": "事件触发器只存在于系统库，由 examples 初始化脚本幂等同步",
}

MAX_SITE_FILE_BYTES = 50 * 1024 * 1024
SITE_TEXT_EXTS = {
    ".html", ".htm", ".js", ".mjs", ".cjs", ".css", ".json",
    ".svg", ".txt", ".md", ".xml", ".map", ".webmanifest",
}


class Fail(Exception):
    def __init__(self, message: str, code: int = EXIT_INVALID):
        super().__init__(message)
        self.code = code


class CLIError(Exception):
    def __init__(self, argv, err_code: str, message: str):
        super().__init__(f"{err_code}: {message}")
        self.argv = argv
        self.err_code = err_code
        self.message = message


# ── 工具函数 ─────────────────────────────────────────────


def sha256(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def seed_doc_marker(collection: str, doc_id: str) -> str:
    """集合种子文档在 _migrations 里的记账名（与 dev.go 的 devSeedDocMarker 一致）。"""
    return f"seed-doc:{collection}/{doc_id}"


def say(msg: str = "") -> None:
    print(msg, flush=True)


def warn(msg: str) -> None:
    print(f"warning: {msg}", file=sys.stderr, flush=True)


def parse_env_file(path: Path) -> dict:
    out = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue
        key, val = line.split("=", 1)
        key, val = key.strip(), val.strip()
        if len(val) >= 2 and val[0] == val[-1] and val[0] in "\"'":
            val = val[1:-1]
        out[key] = val
    return out


DOLLAR_TAG = re.compile(r"^\$([A-Za-z_][A-Za-z0-9_]*)?\$")


def _sql_statements(text: str, mask: bool) -> list:
    """按 `;` 拆语句，识别 '..'/".."/$..$ 与 -- / /* */ 注释（注释丢弃）。"""
    out, cur, n, i = [], [], len(text), 0

    def flush():
        s = "".join(cur).strip()
        if s:
            out.append(s)
        cur.clear()

    while i < n:
        c = text[i]
        if c in "'\"":
            cur.append(c)
            i += 1
            start = i
            while i < n:
                if text[i] == c:
                    if i + 1 < n and text[i + 1] == c:  # '' / "" 转义
                        i += 2
                        continue
                    break
                i += 1
            cur.append(" " * (i - start) if mask else text[start:i])
            if i < n:
                cur.append(c)
                i += 1
        elif c == "-" and text[i:i + 2] == "--":
            while i < n and text[i] != "\n":
                i += 1
            cur.append(" ")
        elif c == "/" and text[i:i + 2] == "/*":
            i += 2
            while i + 1 < n and text[i:i + 2] != "*/":
                i += 1
            if i + 1 < n:
                i += 2
            cur.append(" ")
        elif c == "$" and DOLLAR_TAG.match(text[i:]):
            tag = DOLLAR_TAG.match(text[i:]).group(0)
            cur.append(tag)
            i += len(tag)
            start = i
            while i < n and not text.startswith(tag, i):
                i += 1
            cur.append(" " * (i - start) if mask else text[start:i])
            if text.startswith(tag, i):
                cur.append(tag)
                i += len(tag)
        elif c == ";":
            flush()
            i += 1
        else:
            cur.append(c)
            i += 1
    flush()
    return out


def split_sql(text: str) -> list:
    """按 `;` 拆分语句，引号与注释不会拆错。"""
    return _sql_statements(text, False)


def mask_sql(text: str) -> list:
    """与 split_sql 一一对应，但把字面量内容换成空格，用于关键字与保留前缀检查。"""
    return _sql_statements(text, True)


def first_keyword(stmt: str) -> str:
    m = re.match(r"\s*\(?\s*([A-Za-z_]+)", stmt)
    return m.group(1).upper() if m else ""


def safe_path(ws: Path, rel, what: str, errors: list):
    if not isinstance(rel, str) or not rel.strip():
        errors.append(f"{what}: 路径为空")
        return None
    p = Path(rel)
    if p.is_absolute():
        errors.append(f"{what}: 必须是工作区内的相对路径（{rel}）")
        return None
    full = (ws / p).resolve()
    try:
        full.relative_to(ws.resolve())
    except ValueError:
        errors.append(f"{what}: 路径越出工作区（{rel}）")
        return None
    if any(part.startswith(".env") for part in p.parts):
        errors.append(f"{what}: 不允许引用 .env 文件（{rel}）")
        return None
    return full


def quote(value) -> str:
    """按 Go 的 %q 输出双引号字符串，保证两端提示文本逐字一致。"""
    out = ['"']
    for ch in str(value):
        if ch == '"':
            out.append('\\"')
        elif ch == "\\":
            out.append("\\\\")
        elif ch == "\n":
            out.append("\\n")
        elif ch == "\t":
            out.append("\\t")
        elif ord(ch) < 0x20 or ord(ch) == 0x7F:
            out.append("\\x%02x" % ord(ch))
        else:
            out.append(ch)
    out.append('"')
    return "".join(out)


def scan_secrets(label: str, text: str, errors: list) -> None:
    for kind, pat in SECRET_PATTERNS:
        if pat.search(text):
            errors.append(f"{label}: 疑似包含{kind}，密钥不能进入工作区产物")


# ── 清单校验 ─────────────────────────────────────────────


class Workspace:
    """校验后的工作区：所有路径已解析，文件内容已读入。"""

    def __init__(self, root: Path):
        self.root = root.resolve()
        self.errors: list = []
        self.warnings: list = []
        self.project_id = ""
        self.databases: list = []
        self.collections: list = []
        self.functions: list = []
        self.cron_jobs: list = []

    def check(self) -> "Workspace":
        mpath = self.root / MANIFEST
        if not mpath.is_file():
            raise Fail(f"{mpath} 不存在：先运行 init")
        text = mpath.read_text(encoding="utf-8")
        try:
            m = json.loads(text)
        except json.JSONDecodeError as e:
            raise Fail(f"{MANIFEST} 不是合法 JSON：{e}")
        if not isinstance(m, dict):
            raise Fail(f"{MANIFEST} 顶层必须是对象")
        scan_secrets(MANIFEST, text, self.errors)
        # 按 key 排序，与 Go 端一致（两端提示的顺序也要可比）。
        for key in sorted(IGNORED_KEYS):
            if key in m:
                self.warnings.append(f"忽略 `{key}`：{IGNORED_KEYS[key]}")
        project = m.get("project") or {}
        self.project_id = str(project.get("id") or "")
        if self.project_id and not PROJECT_ID.match(self.project_id):
            self.errors.append(f"project.id 必须是 8 位字母、数字或 -（当前 {quote(self.project_id)}）")
        self._databases(m.get("databases") or [],
                        {c.get("database") for c in (m.get("collections") or []) if isinstance(c, dict)})
        self._collections(m.get("collections") or [])
        self._functions(m.get("functions") or [])
        self._cron(m.get("cron_jobs") or [])
        return self

    def _sql_file(self, rel: str, what: str, is_migration: bool):
        path = safe_path(self.root, rel, what, self.errors)
        if path is None:
            return None
        if not path.is_file():
            self.errors.append(f"{what}: 文件不存在（{rel}）")
            return None
        text = path.read_text(encoding="utf-8")
        scan_secrets(rel, text, self.errors)
        stmts = split_sql(text)
        if not stmts:
            self.errors.append(f"{rel}: 没有可执行语句")
        if len(stmts) + 1 > MAX_BATCH:
            self.errors.append(f"{rel}: {len(stmts)} 条语句超过单批上限 {MAX_BATCH - 1}，请拆成多个文件")
        # 关键字与保留前缀用打了码的版本检查，字符串字面量里的同名文本不算。
        masks = mask_sql(text)
        for i, s in enumerate(stmts, 1):
            probe = masks[i - 1] if i - 1 < len(masks) else s
            kw = first_keyword(probe)
            if kw in DENIED_SQL_KEYWORDS:
                self.errors.append(f"{rel} 第 {i} 条: {kw} 被 sqlguard 拒绝")
            if RESERVED_IDENT.search(probe):
                self.errors.append(f"{rel} 第 {i} 条: 引用了保留前缀（__sb_ / __ducklake_metadata_ / sqlite_）")
            if re.search(rf"\b{MIGRATIONS_TABLE}\b", probe, re.I):
                self.errors.append(f"{rel} 第 {i} 条: {MIGRATIONS_TABLE} 是迁移记账表，不要在迁移里操作")
            if is_migration and DUCKLAKE_UNSUPPORTED.search(probe):
                self.warnings.append(f"{rel} 第 {i} 条: PRIMARY KEY/UNIQUE/INDEX/SEQUENCE 在 DuckLake 上不受支持，examples 约定不用")
            if is_migration and kw in {"DROP", "DELETE", "TRUNCATE"}:
                self.warnings.append(f"{rel} 第 {i} 条: {kw} 是破坏性语句，云端重放前请确认")
        return {"marker": path.name if is_migration else f"seed:{rel}", "path": path,
                "stmts": stmts, "sha": sha256(text)}

    def _databases(self, items: list, with_collections: set = frozenset()) -> None:
        seen = set()
        for i, d in enumerate(items):
            what = f"databases[{i}]"
            name = str(d.get("name") or "").strip()
            if not name or len(name) > 63:
                self.errors.append(f"{what}.name 必填且不超过 63 字符")
                continue
            if name in seen:
                self.errors.append(f"{what}: 库名 {name} 重复")
            seen.add(name)
            # 缺省是 collection（与建库接口一致）；只有声明了迁移/种子、且没有集合引用它时才推断为 sql。
            model = d.get("data_model") or (
                "sql" if (d.get("migrations") or d.get("seed")) and name not in with_collections else "collection")
            if model not in ("sql", "collection"):
                self.errors.append(f"{what}.data_model 只能是 sql 或 collection")
                continue
            # 声明了 migrations/seed 且未写 data_model 时按 sql；显式写 collection 则允许同一个库
            # 既跑迁移 SQL 又存集合文档（examples 的 community / ops-assistant 就是这样）。
            migrations = []
            if d.get("migrations"):
                mdir = safe_path(self.root, d["migrations"], f"{what}.migrations", self.errors)
                if mdir is not None and not mdir.is_dir():
                    self.errors.append(f"{what}.migrations: 目录不存在（{d['migrations']}）")
                elif mdir is not None:
                    for f in sorted(mdir.glob("*.sql")):
                        rel = f.relative_to(self.root).as_posix()
                        item = self._sql_file(rel, f"{what}.migrations", True)
                        if item:
                            migrations.append(item)
                    if not migrations:
                        self.warnings.append(f"{what}.migrations: 目录里没有 .sql 文件")
            seed = self._sql_file(d["seed"], f"{what}.seed", False) if d.get("seed") else None
            self.databases.append({"name": name, "data_model": model, "migrations": migrations, "seed": seed})
        if len(self.databases) > 1:
            self.warnings.append(f"databases: 声明了 {len(self.databases)} 个库，但每个项目只能有一个用户数据库，"
                                 "多出的库建库时会返回 database_already_exists")

    def _collections(self, items: list) -> None:
        models = {d["name"]: d["data_model"] for d in self.databases}
        for i, c in enumerate(items):
            what = f"collections[{i}]"
            db, name = c.get("database"), str(c.get("name") or "").strip()
            if db not in models:
                self.errors.append(f"{what}.database 必须引用 databases 里的库（{quote(db)}）")
                continue
            if models[db] != "collection":
                self.errors.append(f"{what}: {db} 是 sql 库，不能建集合")
            if not name:
                self.errors.append(f"{what}.name 必填")
                continue
            seed = c.get("seed") or []
            for j, doc in enumerate(seed):
                if not isinstance(doc, dict) or not isinstance(doc.get("id"), str) or not doc["id"]:
                    self.errors.append(f"{what}.seed[{j}] 必须是带字符串 id 的对象（用于去重）")
            scan_secrets(f"{what}.seed", json.dumps(seed, ensure_ascii=False), self.errors)
            self.collections.append({"database": db, "name": name, "seed": seed})

    def _functions(self, items: list) -> None:
        if len(items) > MAX_FUNCTIONS:
            self.errors.append(f"functions 超过每项目 {MAX_FUNCTIONS} 个上限")
        seen = set()
        for i, f in enumerate(items):
            what = f"functions[{i}]"
            name = str(f.get("name") or "")
            if not FUNC_NAME.match(name):
                self.errors.append(f"{what}.name 须字母开头、仅字母数字下划线、最长 63（{quote(name)}）")
                continue
            if name in seen:
                self.errors.append(f"{what}: 函数名 {name} 重复")
            seen.add(name)
            path = safe_path(self.root, f.get("file"), f"{what}.file", self.errors)
            if path is None:
                continue
            if not path.is_file():
                self.errors.append(f"{what}.file 不存在（{f.get('file')}）")
                continue
            src = path.read_text(encoding="utf-8")
            if len(src.encode("utf-8")) > MAX_SOURCE:
                self.errors.append(f"{what}: 源码超过 256KiB")
            if not re.search(r"^\s*package\s+main\b", src, re.M):
                self.errors.append(f"{what}: 源码必须是 package main")
            exports = EXPORT_FUNC.findall(src)
            if not exports:
                self.errors.append(f"{what}: 没有大写开头的顶层导出函数")
            for imp in self._imports(src):
                if imp not in ALLOWED_IMPORTS:
                    self.warnings.append(f"{what}: import {quote(imp)} 不在已注册包清单里，编译可能失败")
            scan_secrets(str(f.get("file")), src, self.errors)
            test = f.get("test")
            if test is None:
                self.warnings.append(f"{what}: 没有配置 test，apply 会不经试跑直接生效")
            else:
                if not isinstance(test, dict) or not test.get("function_name"):
                    self.errors.append(f"{what}.test 需要 function_name")
                    test = None
                elif test["function_name"] not in exports:
                    self.errors.append(f"{what}.test.function_name={test['function_name']} 不是导出函数")
            self.functions.append({"name": name, "path": path, "source": src, "sha": sha256(src),
                                   "description": f.get("description"), "test": test, "exports": exports})

    @staticmethod
    def _imports(src: str) -> list:
        out = re.findall(r'^\s*import\s+(?:\w+\s+)?"([^"]+)"', src, re.M)
        for block in re.findall(r"^\s*import\s*\((.*?)\)", src, re.M | re.S):
            out += re.findall(r'"([^"]+)"', block)
        return out

    def _cron(self, items: list) -> None:
        funcs = {f["name"]: f for f in self.functions}
        seen = set()
        for i, j in enumerate(items):
            what = f"cron_jobs[{i}]"
            name = str(j.get("name") or "")
            if not CRON_NAME.match(name):
                self.errors.append(f"{what}.name 须字母开头、字母数字 _-、最长 63（{quote(name)}）")
                continue
            if name in seen:
                self.errors.append(f"{what}: 任务名 {name} 重复")
            seen.add(name)
            if "trigger_once" in j:
                self.warnings.append(f"{what}: 忽略 trigger_once（触发有副作用，不进入自动重放）")
            kind = j.get("schedule_kind")
            # description 为 None 表示清单没写（保留远端值）；空串表示显式清空。
            want = {"name": name, "schedule_kind": kind, "func_file": j.get("func_file"),
                    "func_export": j.get("func_export"), "input_json": j.get("input_json") or "{}",
                    "enabled": j.get("enabled", True), "description": j.get("description")}
            if kind == "cron":
                if len(str(j.get("cron_expr") or "").split()) != 5:
                    self.errors.append(f"{what}.cron_expr 需要 5 段（分 时 日 月 周）")
                want["cron_expr"] = j.get("cron_expr")
            elif kind == "interval":
                iv = j.get("interval_seconds")
                if not isinstance(iv, int) or not 60 <= iv <= 2592000:
                    self.errors.append(f"{what}.interval_seconds 必须是 60–2592000 的整数")
                want["interval_seconds"] = iv
            elif kind == "once":
                try:
                    datetime.fromisoformat(str(j.get("run_at")).replace("Z", "+00:00"))
                except ValueError:
                    self.errors.append(f"{what}.run_at 必须是 RFC3339 时间")
                want["run_at"] = j.get("run_at")
            else:
                self.errors.append(f"{what}.schedule_kind 只能是 cron / interval / once")
            if not want["func_file"] or not want["func_export"]:
                self.errors.append(f"{what}: func_file 与 func_export 必填")
            elif want["func_file"] not in funcs:
                self.warnings.append(f"{what}: func_file={want['func_file']} 不在清单里，需远端已存在")
            elif want["func_export"] not in funcs[want["func_file"]]["exports"]:
                self.errors.append(f"{what}: {want['func_export']} 不是 {want['func_file']} 的导出函数")
            if not isinstance(want["enabled"], bool):
                self.errors.append(f"{what}.enabled 必须是布尔值")
            try:
                json.loads(want["input_json"])
            except (TypeError, json.JSONDecodeError):
                self.errors.append(f"{what}.input_json 必须是 JSON 字符串")
            if len(str(want["input_json"]).encode("utf-8")) > MAX_CRON_INPUT:
                self.errors.append(f"{what}.input_json 超过 8KB")
            self.cron_jobs.append(want)


# ── 目标与 CLI ───────────────────────────────────────────


class Target:
    def __init__(self, args, ws: Workspace):
        # 来源优先级：--env-file > 进程环境 > 工作区 .env。
        sources = [dict(os.environ)]
        if args.env_file:
            p = Path(args.env_file)
            if not p.is_file():
                raise Fail(f"--env-file {p} 不存在")
            sources.insert(0, parse_env_file(p))
        elif (ws.root / ".env").is_file():
            sources.append(parse_env_file(ws.root / ".env"))
        env = {}
        for src in reversed(sources):
            env.update({k: v for k, v in src.items() if v})
        self.url = (args.url or env.get("SIMPLEBASE_URL") or "").rstrip("/")
        # Key 取自优先级最高、且设置了 Key 的那一个来源，避免 --env-file 的云端 URL 配上进程环境里残留的本地 Key；
        # 同一来源内与 CLI 一致，SIMPLEBASE_TOKEN 优先于 SIMPLEBASE_API_KEY。
        self.token = next((s.get("SIMPLEBASE_TOKEN") or s.get("SIMPLEBASE_API_KEY") for s in sources
                           if s.get("SIMPLEBASE_TOKEN") or s.get("SIMPLEBASE_API_KEY")), "")
        self.project = args.project or env.get("SIMPLEBASE_PROJECT_ID") or ws.project_id
        if ws.project_id and self.project and self.project != ws.project_id:
            warn(f"目标 project {self.project} 与清单 project.id {ws.project_id} 不同：业务侧将不能只换 URL")
        self.bin = args.bin or env.get("SIMPLEBASE_BIN") or shutil.which("simplebase") or ""
        u = urllib.parse.urlparse(self.url)
        self.host = u.hostname or ""
        self.is_local = self.host in LOCAL_HOSTS
        self.label = "local" if self.is_local else (re.sub(r"[^a-z0-9]+", "-", self.host.lower()).strip("-") or "remote")
        # 本地分区键不带端口：每人本地端口不同，写进 lock 会在提交后散成多份分区。
        # 库 ID 命中失败会回退到按名称匹配，共用一份本地分区是安全的。
        self.key = f"local#{self.project}" if self.is_local else f"{self.url}#{self.project}"

    def require_http(self) -> None:
        missing = [n for n, v in (("SIMPLEBASE_URL", self.url), ("SIMPLEBASE_API_KEY", self.token),
                                  ("SIMPLEBASE_PROJECT_ID", self.project)) if not v]
        if missing:
            raise Fail("缺少 " + " / ".join(missing) + "：写进工作区 .env 或用 --env-file 指定")

    def require(self) -> None:
        self.require_http()
        if not self.bin or not os.access(self.bin, os.X_OK):
            raise Fail("找不到 simplebase 二进制：放进 PATH，或用 --bin / SIMPLEBASE_BIN 指定")


class CLI:
    def __init__(self, t: Target):
        self.bin = t.bin
        self.env = {k: v for k, v in os.environ.items() if not k.startswith("SIMPLEBASE_")}
        self.env.update(SIMPLEBASE_URL=t.url, SIMPLEBASE_TOKEN=t.token, SIMPLEBASE_PROJECT_ID=t.project)

    def __call__(self, *argv: str, stdin: str = None):
        argv = [str(a) for a in argv]
        try:
            p = subprocess.run([self.bin, *argv], input=stdin, capture_output=True, text=True,
                               env=self.env, timeout=180)
        except subprocess.TimeoutExpired:
            raise CLIError(argv, "cli_timeout", "simplebase 命令 180 秒未返回")
        if p.returncode == 0:
            lines = [ln for ln in p.stdout.splitlines() if ln.strip()]
            if not lines:
                return None
            try:
                return json.loads(lines[-1]).get("data")
            except json.JSONDecodeError:
                return lines[-1]
        err_lines = [ln for ln in p.stderr.splitlines() if ln.strip()]
        code, msg = "cli_failed", (p.stderr.strip() or p.stdout.strip())[:300]
        if err_lines:
            try:
                e = json.loads(err_lines[-1]).get("error") or {}
                code, msg = e.get("code") or code, e.get("message") or msg
            except json.JSONDecodeError:
                pass
        raise CLIError(argv, code, msg)


# ── lock ─────────────────────────────────────────────────


def load_lock(ws: Workspace) -> dict:
    p = ws.root / STATE_DIR / LOCK_NAME
    if p.is_file():
        try:
            data = json.loads(p.read_text(encoding="utf-8"))
            if isinstance(data, dict) and isinstance(data.get("targets"), dict):
                return data
        except json.JSONDecodeError:
            raise Fail(f"{p} 损坏：修复或删除后重跑（删除会丢失迁移 checksum）")
    return {"version": 1, "targets": {}}


def save_lock(ws: Workspace, lock: dict) -> None:
    d = ws.root / STATE_DIR
    d.mkdir(exist_ok=True)
    (d / LOCK_NAME).write_text(json.dumps(lock, indent=2, ensure_ascii=False, sort_keys=True) + "\n", encoding="utf-8")


# ── 重放 ─────────────────────────────────────────────────


class Replayer:
    def __init__(self, ws: Workspace, target: Target, cli: CLI, lock: dict, apply: bool, seed: bool):
        self.ws, self.t, self.cli, self.apply, self.seed = ws, target, cli, apply, seed
        self.entry = lock["targets"].setdefault(target.key, {}) if apply else \
            json.loads(json.dumps(lock["targets"].get(target.key, {})))
        self.db_ids: dict = {}
        self.changes = 0
        self.skips = 0

    def act(self, verb: str, kind: str, name: str, detail: str = "") -> None:
        if verb == "skip":
            self.skips += 1
        else:
            self.changes += 1
        say(f"  {verb:<7} {kind:<10} {name}" + (f"  ({detail})" if detail else ""))

    def check_engine(self, lock: dict) -> None:
        """记录目标项目的数据库引擎，并与其他环境的记录比对。"""
        try:
            data = self.cli("project", "list") or {}
        except CLIError as e:
            # 不阻断重放，但要说清楚「没校验成」，别让人以为已经校验过。
            if e.err_code in ("forbidden", "unauthenticated", "invalid_api_key", "api_key_revoked"):
                warn(f"无法读取项目引擎（{e.err_code}）：跳过本地与云端的引擎一致性检查，请自行确认两端引擎相同")
            elif "not_found" in e.err_code or e.err_code == "usage":
                warn(f"服务端没有 /v1/projects 接口（{e.err_code}）：跳过引擎一致性检查")
            else:
                warn(f"读取项目引擎失败（{e.err_code}）：跳过引擎一致性检查")
            return
        if not isinstance(data, dict) or not isinstance(data.get("projects"), list):
            warn("项目列表格式无法解析：跳过引擎一致性检查")
            return
        engine = next((p.get("engine") for p in data["projects"] if p.get("id") == self.t.project), "")
        if not engine:
            warn(f"项目 {self.t.project} 不在 project list 返回里：跳过引擎一致性检查")
            return
        if self.apply:
            self.entry["engine"] = engine
        for key, e in lock["targets"].items():
            if key == self.t.key or e.get("engine") in (None, "", engine):
                continue
            warn(f"{key} 的数据库引擎是 {e['engine']}，{self.t.key} 是 {engine}："
                 "迁移只按一种方言写，两端要保持一致")

    def run(self) -> None:
        self.databases()
        for d in self.ws.databases:
            self.migrations(d)
        self.collections()
        self.functions()
        self.cron()
        if self.apply:
            self.entry["applied_at"] = now_iso()
            self.entry["sbdev"] = VERSION

    # 数据库
    def databases(self) -> None:
        if not self.ws.databases:
            return
        remote, cursor = [], ""
        for _ in range(50):
            args = ["database", "list", "--limit", "200"] + (["--cursor", cursor] if cursor else [])
            data = self.cli(*args) or {}
            remote += data.get("databases") or []
            cursor = data.get("next_cursor") or ""
            if not cursor:
                break
        locked = self.entry.setdefault("databases", {})
        for d in self.ws.databases:
            name, model = d["name"], d["data_model"]
            hit = None
            lid = (locked.get(name) or {}).get("id")
            if lid:
                hit = next((x for x in remote if x.get("id") == lid), None)
                if hit is None:
                    warn(f"lock 中 {name} 的库 ID {lid} 在远端不存在，改按名称匹配")
            if hit is None:
                same = [x for x in remote if x.get("name") == name]
                if len(same) > 1:
                    raise Fail(f"远端有 {len(same)} 个名为 {name} 的库，请在 lock 的 databases.{name}.id 指定一个", EXIT_REMOTE)
                hit = same[0] if same else None
            if hit is not None:
                rmodel = hit.get("data_model") or "collection"
                if rmodel != model:
                    raise Fail(f"远端库 {name} 是 {rmodel}，清单要求 {model}；换库名或修正 data_model", EXIT_REMOTE)
                self.db_ids[name] = hit["id"]
                self.act("skip", "database", name, hit["id"])
            elif self.apply:
                created = self.cli("database", "create", "--name", name, "--data-model", model)
                self.db_ids[name] = created["id"]
                self.act("create", "database", name, f"{model} → {created['id']}")
            else:
                self.db_ids[name] = None
                self.act("create", "database", name, model)
            if self.db_ids[name]:
                locked[name] = {"id": self.db_ids[name]}

    # 迁移与种子
    def _applied(self, db_id: str) -> set:
        if self.apply:
            self.cli("sql", "exec", "--database", db_id, "--statement",
                     f"CREATE TABLE IF NOT EXISTS {MIGRATIONS_TABLE} (name VARCHAR, applied_at VARCHAR)")
        try:
            data = self.cli("sql", "query", "--database", db_id, "--statement", f"SELECT name FROM {MIGRATIONS_TABLE}") or {}
        except CLIError as e:
            # plan 不建表：记账表还不存在时查询会失败，按「一个都没应用」处理。
            # 不用 schema show 判断：它只对 sql 库可用，而 collection 库同样可以跑迁移 SQL。
            if self.apply or e.err_code in ("unauthenticated", "forbidden", "invalid_api_key", "api_key_revoked"):
                raise
            return set()
        return {row[0] for row in data.get("rows") or [] if row}

    def migrations(self, d: dict) -> None:
        items = list(d["migrations"])
        if self.seed and d["seed"]:
            items.append(d["seed"])
        if not items:
            return
        name, db_id = d["name"], self.db_ids.get(d["name"])
        applied = self._applied(db_id) if db_id else set()
        sums = self.entry.setdefault("migrations", {}).setdefault(name, {})
        for it in items:
            label = f"{name}/{it['marker']}"
            if it["marker"] in applied:
                old = sums.get(it["marker"])
                if old and old != it["sha"] and not it["marker"].startswith("seed:"):
                    raise Fail(f"migration_modified: {label} 已应用但内容变了；不要改旧迁移，新增一个文件", EXIT_INVALID)
                if not old:
                    sums[it["marker"]] = it["sha"]
                self.act("skip", "migration", label)
                continue
            self.act("apply", "migration", label, f"{len(it['stmts'])} 条")
            if not self.apply:
                continue
            batch = [{"sql": s, "args": []} for s in it["stmts"]]
            batch.append({"sql": f"INSERT INTO {MIGRATIONS_TABLE} (name, applied_at) VALUES (?, ?)",
                          "args": [it["marker"], now_iso()]})
            res = self.cli("sql", "batch", "--database", db_id, "--file", "-", "--transactional",
                           stdin=json.dumps(batch, ensure_ascii=False)) or {}
            bad = [r for r in res.get("results") or [] if r.get("error_code")]
            if res.get("error") or bad:
                detail = res.get("error") or bad[0]
                raise Fail(f"迁移 {label} 失败（已整体回滚）：{json.dumps(detail, ensure_ascii=False)}", EXIT_REMOTE)
            sums[it["marker"]] = it["sha"]

    # 集合
    def collections(self) -> None:
        for c in self.ws.collections:
            db_id, cname = self.db_ids.get(c["database"]), c["name"]
            label = f"{c['database']}/{cname}"
            existing = set()
            if db_id:
                data = self.cli("collection", "list", "--database", db_id) or {}
                existing = set(data.get("collections") or []) | {x.get("name") for x in data.get("items") or []}
            present = cname in existing
            if present:
                self.act("skip", "collection", label)
            else:
                self.act("create", "collection", label)
                if self.apply:
                    self.cli("collection", "create", "--database", db_id, "--name", cname)
            if not self.seed or not c["seed"]:
                continue
            # 服务端建文档时总是生成 UUID、忽略传入的 id，按 id 查重永远落空。
            # 改为与迁移同一张记账表：每篇种子文档写一行 seed-doc:<集合>/<id>，已记账的跳过。
            applied = self._applied(db_id) if db_id else set()
            for doc in c["seed"]:
                marker = seed_doc_marker(cname, doc["id"])
                if marker in applied:
                    self.act("skip", "document", f"{label}/{doc['id']}")
                    continue
                self.act("create", "document", f"{label}/{doc['id']}")
                if self.apply:
                    self.cli("document", "insert", "--database", db_id, "--collection", cname,
                             "--json", json.dumps(doc, ensure_ascii=False))
                    self.cli("sql", "exec", "--database", db_id, "--statement",
                             f"INSERT INTO {MIGRATIONS_TABLE} (name, applied_at) VALUES (?, ?)",
                             "--param", json.dumps([marker, now_iso()], ensure_ascii=False))

    # 云函数
    def _test(self, f: dict, version: int) -> None:
        t = f["test"]
        if not t:
            return
        data = self.cli("function", "test", "--name", f["name"], "--version", version,
                        "--export", t["function_name"], "--input", json.dumps(t.get("body") or {}, ensure_ascii=False)) or {}
        if not data.get("ok"):
            raise Fail(f"函数 {f['name']} v{version} 试跑失败：{data.get('error')}", EXIT_REMOTE)

    def _latest(self, name: str, created) -> int:
        ver = (created or {}).get("latest_version") if isinstance(created, dict) else None
        if ver:
            return int(ver)
        listed = self.cli("function", "versions", "list", "--name", name) or {}
        return max(int(v["version"]) for v in listed.get("versions") or [{"version": 0}])

    def functions(self) -> None:
        locked = self.entry.setdefault("functions", {})
        for f in self.ws.functions:
            name = f["name"]
            try:
                remote = self.cli("function", "get", "--name", name)
            except CLIError as e:
                # function get 经 mapNotFound 返回 not_found；其余路径是 gofunction_not_found
                if e.err_code not in ("not_found", "gofunction_not_found"):
                    raise
                remote = None
            if remote is None:
                self.act("create", "function", name)
                if self.apply:
                    # --activate false：先建未生效版本，试跑通过后再 activate
                    args = ["function", "create", "--name", name, "--file", f["path"], "--activate", "false"]
                    if f["description"]:
                        args += ["--description", f["description"]]
                    ver = self._latest(name, self.cli(*args))
                    self._test(f, ver)
                    self.cli("function", "activate", "--name", name, "--version", ver)
                    locked[name] = {"sha": f["sha"], "version": ver}
                continue
            active = int(remote.get("active_version") or 0)
            latest = int(remote.get("latest_version") or 0)
            if active and sha256(remote.get("source") or "") == f["sha"]:
                self.act("skip", "function", name, f"v{active}")
                if self.apply:
                    locked[name] = {"sha": f["sha"], "version": active}
            else:
                reuse = None
                if latest and latest != active:
                    v = self.cli("function", "versions", "get", "--name", name, "--version", latest) or {}
                    if sha256(v.get("source") or "") == f["sha"]:
                        reuse = latest
                self.act("update", "function", name, f"v{active} → " + (f"复用 v{reuse}" if reuse else "新版本"))
                if self.apply:
                    ver = reuse
                    if ver is None:
                        created = self.cli("function", "versions", "create", "--name", name, "--file", f["path"],
                                           "--note", f"sbdev {f['sha'][:12]}")
                        ver = self._latest(name, created)
                    self._test(f, ver)
                    self.cli("function", "activate", "--name", name, "--version", ver)
                    locked[name] = {"sha": f["sha"], "version": ver}
            if f["description"] is not None and (remote.get("description") or "") != f["description"]:
                self.act("update", "function", name, "description")
                if self.apply:
                    self.cli("function", "update", "--name", name, "--description", f["description"])

    # 定时任务
    @staticmethod
    def _cron_flags(w: dict) -> list:
        out = ["--schedule-kind", w["schedule_kind"]]
        if w["schedule_kind"] == "cron":
            out += ["--cron", w["cron_expr"]]
        elif w["schedule_kind"] == "interval":
            out += ["--interval-seconds", str(w["interval_seconds"])]
        else:
            out += ["--run-at", w["run_at"]]
        out += ["--func-file", w["func_file"], "--func-export", w["func_export"],
                "--input", w["input_json"], "--enabled", "true" if w["enabled"] else "false"]
        if w["description"] is not None:
            out += ["--description", w["description"]]
        return out

    @staticmethod
    def _cron_diff(old: dict, w: dict) -> list:
        def ts(v):
            try:
                return datetime.fromisoformat(str(v).replace("Z", "+00:00"))
            except ValueError:
                return v

        def js(v):
            try:
                return json.loads(v or "{}")
            except (TypeError, json.JSONDecodeError):
                return v

        diff = []
        if old.get("schedule_kind") != w["schedule_kind"]:
            diff.append("schedule_kind")
        elif w["schedule_kind"] == "cron" and old.get("cron_expr") != w["cron_expr"]:
            diff.append("cron_expr")
        elif w["schedule_kind"] == "interval" and int(old.get("interval_seconds") or 0) != w["interval_seconds"]:
            diff.append("interval_seconds")
        elif w["schedule_kind"] == "once" and ts(old.get("run_at")) != ts(w["run_at"]):
            diff.append("run_at")
        for k in ("func_file", "func_export"):
            if old.get(k) != w[k]:
                diff.append(k)
        if js(old.get("input_json")) != js(w["input_json"]):
            diff.append("input_json")
        if bool(old.get("enabled")) != w["enabled"]:
            diff.append("enabled")
        # None 表示清单没写 description：保留远端值。写了（含空串）才比较，这样能把描述清空。
        if w["description"] is not None and (old.get("description") or "") != w["description"]:
            diff.append("description")
        return diff

    def cron(self) -> None:
        if not self.ws.cron_jobs:
            return
        jobs = (self.cli("cron", "list") or {}).get("jobs") or []
        locked = self.entry.setdefault("cron", {})
        for w in self.ws.cron_jobs:
            name = w["name"]
            old = next((j for j in jobs if j.get("name") == name), None)
            if w["schedule_kind"] == "once" and not old:
                at = datetime.fromisoformat(str(w["run_at"]).replace("Z", "+00:00"))
                if at.tzinfo is None:
                    at = at.replace(tzinfo=timezone.utc)
                if at <= datetime.now(timezone.utc):
                    warn(f"cron {name}: run_at 已过期，跳过创建")
                    self.act("skip", "cron", name, "run_at 已过期")
                    continue
            if old is None:
                self.act("create", "cron", name, w["schedule_kind"])
                if self.apply:
                    created = self.cli("cron", "create", "--name", name, *self._cron_flags(w)) or {}
                    locked[name] = {"id": created.get("id")}
                continue
            if old.get("target_kind") not in (None, "", "function"):
                raise Fail(f"远端 cron {name} 的目标不是云函数（{old.get('target_kind')}），换个任务名", EXIT_REMOTE)
            diff = self._cron_diff(old, w)
            if diff:
                self.act("update", "cron", name, ",".join(diff))
                if self.apply:
                    # update 是全量替换：清单没写 description 时用远端现值，避免被清空。
                    if w["description"] is None:
                        w["description"] = old.get("description") or ""
                    self.cli("cron", "update", "--id", old["id"], *self._cron_flags(w))
            else:
                self.act("skip", "cron", name)
            if self.apply:
                locked[name] = {"id": old.get("id")}


def write_env_hint(ws: Workspace, t: Target, db_ids: dict) -> Path:
    """生成不含密钥的环境提示文件，业务侧复制需要的行。"""
    lines = ["# sbdev 生成，不含密钥。SIMPLEBASE_API_KEY 请在该环境的控制台签发后自行填写。",
             f"SIMPLEBASE_URL={t.url}", f"SIMPLEBASE_PROJECT_ID={t.project}"]
    ids = [(n, i) for n, i in db_ids.items() if i]
    # 不带后缀的 SIMPLEBASE_DATABASE_ID 只在单库时写出；多库时依赖清单顺序容易用错。
    if len(ids) == 1:
        lines.append(f"SIMPLEBASE_DATABASE_ID={ids[0][1]}")
    for n, i in ids:
        lines.append(f"SIMPLEBASE_DATABASE_ID_{re.sub(r'[^A-Za-z0-9]', '_', n).upper()}={i}")
    d = ws.root / STATE_DIR
    d.mkdir(exist_ok=True)
    p = d / f"env.{t.label}"
    p.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return p


# ── 子命令 ───────────────────────────────────────────────


def print_findings(ws: Workspace) -> None:
    for w in ws.warnings:
        warn(w)
    for e in ws.errors:
        print(f"error: {e}", file=sys.stderr)


def checked(root: Path) -> Workspace:
    ws = Workspace(root).check()
    print_findings(ws)
    if ws.errors:
        raise Fail(f"工作区校验失败（{len(ws.errors)} 个错误）")
    return ws


def cmd_validate(args) -> int:
    ws = checked(Path(args.dir))
    n_mig = sum(len(d["migrations"]) for d in ws.databases)
    say(f"ok: {len(ws.databases)} 个库 / {n_mig} 个迁移 / {len(ws.collections)} 个集合 / "
        f"{len(ws.functions)} 个函数 / {len(ws.cron_jobs)} 个定时任务")
    return EXIT_OK


def run_replay(args, apply: bool) -> int:
    ws = checked(Path(args.dir))
    t = Target(args, ws)
    t.require()
    cli = CLI(t)
    lock = load_lock(ws)
    if apply and not t.is_local and not args.yes:
        say(f"目标是远端 {t.label}（{t.key}），先看计划：")
        rp = Replayer(ws, t, cli, lock, apply=False, seed=args.seed)
        rp.run()
        if rp.changes == 0:
            say("无变更。")
            return EXIT_OK
        say(f"共 {rp.changes} 项变更。确认后加 --yes 重跑 apply。")
        return EXIT_CONFIRM
    say(("执行" if apply else "计划") + f" → {t.label}（{t.key}）" + ("，含种子" if args.seed else ""))
    rp = Replayer(ws, t, cli, lock, apply=apply, seed=args.seed)
    rp.check_engine(lock)
    try:
        rp.run()
    except CLIError as e:
        raise Fail(f"`simplebase {' '.join(e.argv[:2])}` 失败：{e.err_code}: {e.message}", EXIT_REMOTE)
    finally:
        if apply:
            save_lock(ws, lock)
    if apply:
        hint = write_env_hint(ws, t, rp.db_ids)
        say(f"完成：{rp.changes} 项变更，{rp.skips} 项无变化。环境提示：{hint.relative_to(ws.root)}")
    else:
        say(f"计划：{rp.changes} 项变更，{rp.skips} 项无变化。")
    return EXIT_OK


def cmd_plan(args) -> int:
    return run_replay(args, apply=False)


def cmd_apply(args) -> int:
    return run_replay(args, apply=True)


def cmd_status(args) -> int:
    ws = Workspace(Path(args.dir))
    lock = load_lock(ws)
    if not lock["targets"]:
        say("还没有 apply 过任何环境。")
        return EXIT_OK
    for key, e in sorted(lock["targets"].items()):
        say(f"{key}")
        say(f"  applied_at: {e.get('applied_at', '-')}")
        for n, d in (e.get("databases") or {}).items():
            migs = sorted((e.get("migrations") or {}).get(n, {}))
            say(f"  database {n}: {d.get('id')}  迁移 {len(migs)} 个" + (f"（最新 {migs[-1]}）" if migs else ""))
        for n, f in (e.get("functions") or {}).items():
            say(f"  function {n}: v{f.get('version')} sha {str(f.get('sha'))[:12]}")
        for n, c in (e.get("cron") or {}).items():
            say(f"  cron {n}: {c.get('id')}")
    return EXIT_OK


def cmd_doctor(args) -> int:
    ok = True

    def line(good: bool, msg: str) -> None:
        nonlocal ok
        ok = ok and good
        say(f"  [{'ok' if good else 'xx'}] {msg}")

    root = Path(args.dir).resolve()
    ws = Workspace(root)
    try:
        ws.check()
        line(not ws.errors, f"工作区 {root}：{len(ws.errors)} 个错误，{len(ws.warnings)} 个警告（详情跑 validate）")
    except Fail as e:
        line(False, str(e))
    t = Target(args, ws)
    line(bool(t.bin) and os.access(t.bin, os.X_OK), f"simplebase 二进制：{t.bin or '未找到'}")
    line(bool(t.url), f"SIMPLEBASE_URL：{t.url or '未设置'}")
    line(bool(t.token), "SIMPLEBASE_API_KEY：" + ("已设置" if t.token else "未设置（控制台「设置 → 连接」签发）"))
    line(bool(PROJECT_ID.match(t.project or "")), f"SIMPLEBASE_PROJECT_ID：{t.project or '未设置'}")
    if t.url:
        try:
            with urllib.request.urlopen(t.url + "/health/ready", timeout=5) as r:
                line(r.status == 200, f"{t.url}/health/ready → {r.status}")
        except urllib.error.HTTPError as e:
            line(False, f"{t.url}/health/ready → {e.code}（实例未就绪或只读）")
        except (urllib.error.URLError, OSError) as e:
            line(False, f"{t.url} 连不上：{getattr(e, 'reason', e)}（本地可用 ./build.sh dev 启动）")
    if ok:
        try:
            CLI(t)("database", "list", "--limit", "1")
            line(True, "Key 与项目可用（database list 成功）")
        except CLIError as e:
            line(False, f"database list 失败：{e.err_code}: {e.message}")
    say("doctor: " + ("全部通过" if ok else "有问题，按 [xx] 行修复"))
    return EXIT_OK if ok else EXIT_INVALID


HELLO_GO = '''//go:build ignore

package main

import "fmt"

type HelloInput struct {
	Name string `json:"name"`
}

type HelloOutput struct {
	Message string `json:"message"`
}

// Hello 是可调用导出：大写开头，恰好一个入参、一个返回值。
func Hello(in HelloInput) HelloOutput {
	name := in.Name
	if name == "" {
		name = "world"
	}
	return HelloOutput{Message: fmt.Sprintf("hello, %s", name)}
}
'''

INIT_SQL = '''-- 迁移只追加不修改：要改表结构就新增 002_xxx.sql。
CREATE TABLE IF NOT EXISTS notes (id VARCHAR, title VARCHAR, created_at VARCHAR);
'''

ENV_EXAMPLE = '''# 复制为 .env（本地）或 .env.cloud（云端）后填写；两个文件都不要提交。
SIMPLEBASE_URL=http://127.0.0.1:8080
SIMPLEBASE_API_KEY=
SIMPLEBASE_PROJECT_ID={project}
'''

GITIGNORE_LINES = [".env", ".env.*", "!.env.example", ".simplebase/env.*"]


def cmd_init(args) -> int:
    root = Path(args.dir).resolve()
    root.mkdir(parents=True, exist_ok=True)
    pid = args.project_id or ""
    if pid and not PROJECT_ID.match(pid):
        raise Fail("--project-id 必须是 8 位字母、数字或 -")
    if pid and (not SITE_PROJECT_ID.match(pid) or pid == "sb-admin"):
        warn(f"project.id {quote(pid)} 含大写字母、首尾为 - 或为保留名，后续无法发布为子域名站点（需 8 位 [a-z0-9-] 且首尾不为 -）")
    if not pid:
        pid = "".join(secrets.choice(string.ascii_lowercase + string.digits) for _ in range(8))
        warn(f"未指定 --project-id，生成了 {pid}；云端与本地请用同一个 ID 建项目")
    name = args.name or root.name
    manifest = {
        "project": {"id": pid, "name": name},
        "databases": [{"name": "main", "data_model": "sql", "migrations": "migrations"}],
        "functions": [{"name": "hello", "file": "functions/hello.go", "description": "示例函数",
                       "test": {"function_name": "Hello", "body": {"name": "sb"}}}],
        "cron_jobs": [],
    }
    files = {
        MANIFEST: json.dumps(manifest, indent=2, ensure_ascii=False) + "\n",
        "migrations/001_init.sql": INIT_SQL,
        "functions/hello.go": HELLO_GO,
        ".env.example": ENV_EXAMPLE.format(project=pid),
    }
    for rel, content in files.items():
        p = root / rel
        if p.exists():
            say(f"  keep    {rel}（已存在）")
            continue
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")
        say(f"  create  {rel}")
    gi = root / ".gitignore"
    have = gi.read_text(encoding="utf-8").splitlines() if gi.exists() else []
    add = [ln for ln in GITIGNORE_LINES if ln not in have]
    if add:
        with gi.open("a", encoding="utf-8") as fh:
            if have and have[-1].strip():
                fh.write("\n")
            fh.write("# simplebase 工作区\n" + "\n".join(add) + "\n")
        say("  update  .gitignore")
    say("下一步：cp .env.example .env 并填写 Key，然后 doctor → apply。")
    return EXIT_OK


# ── 对象存储静态站点发布（planv6.5）──────────────────────


def _workspace_for_target(dir_arg: str) -> Workspace:
    ws = Workspace(Path(dir_arg))
    mpath = ws.root / MANIFEST
    if mpath.is_file():
        try:
            m = json.loads(mpath.read_text(encoding="utf-8"))
            if isinstance(m, dict) and isinstance(m.get("project"), dict):
                ws.project_id = str(m["project"].get("id") or "")
        except (OSError, json.JSONDecodeError):
            pass
    return ws


def site_api(t: Target, method: str, suffix: str, body: dict = None) -> dict:
    url = f"{t.url}/v1/projects/{urllib.parse.quote(t.project, safe='')}{suffix}"
    headers = {"Authorization": f"Bearer {t.token}", "Accept": "application/json"}
    payload = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        payload = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=payload, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read().decode("utf-8", errors="replace")
            return json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace") if e.fp else ""
        code, msg = f"http_{e.code}", raw.strip()[:240]
        try:
            err = (json.loads(raw) or {}).get("error") or {}
            code = err.get("code") or code
            msg = err.get("message") or msg
        except json.JSONDecodeError:
            pass
        raise Fail(f"{method} {suffix} 失败：{code}: {msg}", EXIT_REMOTE)
    except (urllib.error.URLError, OSError) as e:
        raise Fail(f"{method} {suffix} 连不上 {t.url}：{getattr(e, 'reason', e)}", EXIT_REMOTE)


def _site_reason_hint(project_id: str, reason: str) -> str:
    if reason == "sites_disabled":
        return ("目标实例未开启站点托管（需配置 sites.enabled=true 与 sites.base_domain；"
                "本地可用 SIMPLEBASE_SITES_ENABLED=true SIMPLEBASE_SITES_BASE_DOMAIN=localhost "
                "SIMPLEBASE_SITES_URL_SCHEME=http SIMPLEBASE_SITES_URL_PORT=8080）。仅上传文件可加 --no-publish")
    if reason == "project_id_not_dns_safe":
        return f"项目 ID {quote(project_id)} 不能作为子域名标签（需 8 位 [a-z0-9-] 小写字符且首尾不为 -）"
    if reason == "system_project":
        return "系统项目 sb-admin 不允许发布网站"
    return reason or "未知原因"


def normalize_site_prefix(raw: str) -> str:
    p = (raw or "").strip().lstrip("/")
    if not p:
        return ""
    if not p.endswith("/"):
        p += "/"
    if "\\" in p or "\x00" in p:
        raise Fail("--prefix 不能包含反斜杠或空字节")
    for seg in p[:-1].split("/"):
        if not seg or seg in (".", ".."):
            raise Fail(f"--prefix 含非法路径段：{quote(raw)}")
    return p


def collect_site_files(root: Path, dist_arg: str, prefix_arg: str):
    p = Path(dist_arg)
    dist = (p if p.is_absolute() else (root / p)).resolve()
    if not dist.is_dir():
        raise Fail(f"--dist 目录不存在：{dist_arg}（请先编译前端/JS 产物）")
    if (dist / MANIFEST).is_file() or (dist / ".env").is_file():
        raise Fail(f"--dist 不能直接指向含 {MANIFEST} 或 .env 的工作区目录，请指定构建输出目录（如 dist 或 web/dist）")
    prefix = normalize_site_prefix(prefix_arg)
    files = []
    errs: list = []
    for cur, dirnames, filenames in os.walk(dist):
        cur_p = Path(cur)
        dirnames[:] = sorted(d for d in dirnames if not d.startswith(".") and d != "node_modules")
        for fn in sorted(filenames):
            if fn in (".DS_Store", "Thumbs.db"):
                continue
            if fn == ".env" or fn.startswith(".env."):
                errs.append(f"{(cur_p / fn).relative_to(dist).as_posix()}: 构建产物目录中不能包含 .env 文件")
                continue
            if fn.startswith("."):
                continue
            fp = cur_p / fn
            resolved = fp.resolve()
            try:
                resolved.relative_to(dist)
            except ValueError:
                errs.append(f"{fp.relative_to(dist).as_posix()}: 符号链接越出构建目录")
                continue
            if not resolved.is_file():
                continue
            rel = fp.relative_to(dist).as_posix()
            key = prefix + rel
            if len(key.encode("utf-8")) > 1024:
                errs.append(f"{rel}: 对象 key 超过 1024 字节上限")
                continue
            size = resolved.stat().st_size
            if size > MAX_SITE_FILE_BYTES:
                errs.append(f"{rel}: 文件大小 {size} 超过单文件 50MiB 上限")
                continue
            ext = fp.suffix.lower()
            if ext in SITE_TEXT_EXTS:
                text = resolved.read_text(encoding="utf-8", errors="replace")
                scan_secrets(rel, text, errs)
                if ext == ".svg" and text.lstrip().lower().startswith("<?xml"):
                    warn(f"{rel}: 以 <?xml 开头会被服务端嗅探为 text/xml 并报 415 mime_mismatch，建议直接以 <svg 开头")
            files.append((key, resolved))
    if errs:
        for e in errs:
            print(f"error: {e}", file=sys.stderr)
        raise Fail(f"构建产物检查未通过（{len(errs)} 个错误），已中止上传")
    if not files:
        raise Fail(f"--dist 目录没有可上传的静态文件：{dist}")
    if not (dist / "index.html").is_file():
        warn("构建目录根下没有 index.html，访问子域名根路径或目录路径时可能返回 404")
    return files, prefix


def probe_site_url(t: Target, visit_url: str):
    if not visit_url:
        return None
    u = urllib.parse.urlparse(visit_url)
    host = u.hostname or ""
    # 本机 *.localhost 在部分系统 libc 中不走 DNS 解析，改为连 t.url 并显式传 Host 头验证站点分流。
    if t.is_local and host.endswith(".localhost"):
        target_url = f"{t.url}{u.path or '/'}"
        req = urllib.request.Request(target_url, headers={"Host": u.netloc}, method="GET")
    else:
        req = urllib.request.Request(visit_url, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.status
    except urllib.error.HTTPError as e:
        return e.code
    except (urllib.error.URLError, OSError):
        return None


def cmd_site_status(args) -> int:
    ws = _workspace_for_target(args.dir)
    t = Target(args, ws)
    t.require_http()
    pub = site_api(t, "GET", "/s3/public")
    info = pub.get("site") or site_api(t, "GET", "/s3/site")
    paths = pub.get("paths") or []
    say(f"project: {t.project}")
    say(f"  publishable: {bool(info.get('publishable'))}" +
        (f"（{_site_reason_hint(t.project, info.get('reason', ''))}）" if not info.get("publishable") else ""))
    say(f"  enabled:     {bool(info.get('enabled'))}")
    if info.get("baseUrl"):
        say(f"  base_url:    {info['baseUrl']}")
    if not paths:
        say("  public_paths: (无)")
    else:
        for p in paths:
            url_part = f" -> {p['publicUrl']}" if p.get("publicUrl") else ""
            say(f"  public {p.get('kind', 'file'):6s} {p.get('path') or '/'}{url_part}")
    return EXIT_OK


def cmd_site_deploy(args) -> int:
    ws = _workspace_for_target(args.dir)
    files, prefix = collect_site_files(ws.root, args.dist, args.prefix)
    t = Target(args, ws)
    t.require_http()
    if not t.is_local and not args.yes:
        say(f"目标是远端 {t.label}（{t.key}），先看站点发布计划：")
        for key, _ in files[:20]:
            say(f"  upload  object     {key}")
        if len(files) > 20:
            say(f"  ... 其余 {len(files) - 20} 个文件（共 {len(files)} 个）")
        if not args.no_publish:
            say("  enable  site       开启项目子域名访问")
            say(f"  public  folder     {prefix or '/'}（对外匿名可读）")
        say("确认将构建产物上传并发布后，加 --yes 重跑 site deploy。")
        return EXIT_CONFIRM
    t.require()
    if not args.no_publish:
        info = site_api(t, "GET", "/s3/site")
        if not info.get("publishable"):
            reason = info.get("reason") or "not_publishable"
            raise Fail(f"当前项目不可发布站点（{reason}）：{_site_reason_hint(t.project, reason)}", EXIT_REMOTE)
    cli = CLI(t)
    say(f"上传静态产物 → {t.label}（{t.key}），共 {len(files)} 个文件")
    try:
        for key, fp in files:
            cli("object", "upload", "--key", key, "--file", str(fp))
            say(f"  upload  object     {key}")
    except CLIError as e:
        raise Fail(f"`simplebase {' '.join(e.argv[:2])}` 失败：{e.err_code}: {e.message}", EXIT_REMOTE)
    if args.no_publish:
        say(f"完成：已上传 {len(files)} 个文件（--no-publish，未改动站点与公开路径开关）。")
        return EXIT_OK
    site_info = site_api(t, "PUT", "/s3/site", {"enabled": True})
    pub_info = site_api(t, "PUT", "/s3/public", {"key": prefix, "kind": "folder", "public": True})
    base_url = (site_info.get("baseUrl") or "").rstrip("/")
    visit_url = pub_info.get("publicUrl") or (f"{base_url}/{prefix}" if base_url else "")
    subdomain = urllib.parse.urlparse(base_url).hostname or ""
    say(f"  enable  site       {base_url or t.project}")
    say(f"  public  folder     {prefix or '/'} -> {visit_url}")
    status = probe_site_url(t, visit_url)
    if status == 200:
        say(f"站点已就绪：{visit_url}（HTTP 200）")
    elif status is not None:
        warn(f"站点已开启（{visit_url}），探活返回 HTTP {status}（请检查入口文件如 index.html 是否在 {prefix or '/'} 下）")
    else:
        u = urllib.parse.urlparse(visit_url)
        hint = f"curl -i -H 'Host: {u.netloc}' {t.url}{u.path or '/'}" if u.netloc else visit_url
        warn(f"站点已开启（{visit_url}），本机未能直连子域名，可用 `{hint}` 或浏览器验证")
    say(f"subdomain: {subdomain}")
    say(f"url:       {visit_url}")
    if args.json:
        say(json.dumps({"project": t.project, "subdomain": subdomain, "base_url": base_url, "url": visit_url,
                        "prefix": prefix, "files": len(files), "http_status": status}, ensure_ascii=False))
    return EXIT_OK


def cmd_site_unpublish(args) -> int:
    ws = _workspace_for_target(args.dir)
    t = Target(args, ws)
    t.require_http()
    if not t.is_local and not args.yes:
        say(f"目标是远端 {t.label}（{t.key}）：将下线项目子域名站点（关闭 /s3/site，保留路径规则）。确认后加 --yes 重跑。")
        return EXIT_CONFIRM
    site_api(t, "PUT", "/s3/site", {"enabled": False})
    say(f"已下线站点：{t.project}（子域名所有路径现返回 404，路径级公开配置保留）")
    return EXIT_OK


def main(argv=None) -> int:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("-C", "--dir", default=".", help="工作区目录（默认当前目录）")
    remote = argparse.ArgumentParser(add_help=False)
    remote.add_argument("--env-file", help="读取该文件里的 SIMPLEBASE_*（优先于进程环境）；缺省补读工作区 .env")
    remote.add_argument("--url", help="覆盖 SIMPLEBASE_URL")
    remote.add_argument("--project", help="覆盖 SIMPLEBASE_PROJECT_ID")
    remote.add_argument("--bin", help="simplebase 二进制路径（缺省 SIMPLEBASE_BIN 或 PATH）")

    ap = argparse.ArgumentParser(prog="sbdev", description="SimpleBase 工作区：本地开发、云端重放")
    ap.add_argument("--version", action="version", version=f"sbdev {VERSION}")
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("init", parents=[common], help="生成工作区骨架")
    p.add_argument("--project-id", help="8 位项目 ID（本地与云端用同一个）")
    p.add_argument("--name", help="项目显示名")
    p.set_defaults(fn=cmd_init)
    sub.add_parser("validate", parents=[common], help="离线校验清单、迁移、函数").set_defaults(fn=cmd_validate)
    sub.add_parser("doctor", parents=[common, remote], help="检查二进制、凭据、连通性").set_defaults(fn=cmd_doctor)
    for name, fn, hlp in (("plan", cmd_plan, "只读：列出将要执行的变更"), ("apply", cmd_apply, "幂等重放到目标环境")):
        p = sub.add_parser(name, parents=[common, remote], help=hlp)
        p.add_argument("--seed", action="store_true", help="同时执行种子（seed 文件与集合文档）")
        if name == "apply":
            p.add_argument("--yes", action="store_true", help="远端目标的确认开关")
        else:
            p.set_defaults(yes=False)
        p.set_defaults(fn=fn)
    sub.add_parser("status", parents=[common], help="查看 lock 中各环境的状态").set_defaults(fn=cmd_status)

    site_p = sub.add_parser("site", help="上传编译后的前端站点到对象存储并通过子域名发布")
    site_sub = site_p.add_subparsers(dest="site_cmd", required=True)
    site_sub.add_parser("status", parents=[common, remote], help="查看站点开关、子域名与已公开路径").set_defaults(fn=cmd_site_status)
    sd = site_sub.add_parser("deploy", parents=[common, remote], help="扫描密钥、上传构建目录并开启子域名访问")
    sd.add_argument("--dist", required=True, help="前端编译输出目录（如 dist 或 web/dist）")
    sd.add_argument("--prefix", default="", help="对象存储目标目录前缀（缺省发布到站点根 /，例 app/）")
    sd.add_argument("--no-publish", action="store_true", help="只上传静态文件，不开启站点与外部访问开关")
    sd.add_argument("--yes", action="store_true", help="远端目标的确认开关")
    sd.add_argument("--json", action="store_true", help="最后一行额外输出 JSON：subdomain / base_url / url")
    sd.set_defaults(fn=cmd_site_deploy)
    su = site_sub.add_parser("unpublish", parents=[common, remote], help="下线项目站点（关闭总开关，保留路径规则）")
    su.add_argument("--yes", action="store_true", help="远端目标的确认开关")
    su.set_defaults(fn=cmd_site_unpublish)

    args = ap.parse_args(argv)
    try:
        return args.fn(args)
    except Fail as e:
        print(f"sbdev: {e}", file=sys.stderr)
        return e.code
    except KeyboardInterrupt:
        print("sbdev: 已中断；重跑同一命令即可从断点继续", file=sys.stderr)
        return EXIT_REMOTE


if __name__ == "__main__":
    sys.exit(main())
