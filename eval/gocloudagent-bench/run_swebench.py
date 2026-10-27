#!/usr/bin/env python3
"""SWE-bench Verified 的固定 50 题。宿主机检出后 gocloudagent 改代码，swebench 5.0.2 在 Docker 里评分。

不把 gocloudagent 装进评测镜像。补丁和数据集缓存都在仓库外。密钥不打印、不写入报告。
"""

from __future__ import annotations

import argparse
import fcntl
import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

import run_core
import run_smoke as smoke

IDS_FILE = Path(__file__).resolve().parent / "tasks" / "core" / "swebench_verified_50.txt"
CACHE = smoke.RUN_ROOT / "swebench" / "verified.jsonl"
JSONL = smoke.RUN_ROOT / "swebench50.jsonl"
PREDS = smoke.RUN_ROOT / "swe-preds.jsonl"
EVAL_ROOT = smoke.RUN_ROOT / "swe-eval"
SWEBENCH = smoke.RUN_ROOT / "venv-swe" / "bin" / "swebench"
MARKER_PREFIX = "## SWE-bench Verified"
DATASET = "SWE-bench/SWE-bench_Verified"
SPLIT_LEN = 500
AGENT_TIMEOUT = 900
EVAL_TIMEOUT = 1800
EVAL_WORKERS = 2
EVAL_BATCH = 2
MODEL_NAME = "gocloudagent-glm-5.3-flash"
PORT = 18081
INPUT_YUAN_PER_MILLION = 0.80
OUTPUT_YUAN_PER_MILLION = 2.80
GOLD_INSTANCE = "astropy__astropy-13033"

CATEGORY_LABEL = {
    **run_core.CATEGORY_LABEL,
    "no_patch": "没有补丁",
    "clone_error": "检出失败",
    "unresolved": "测试未通过",
    "infra": "评测环境失败",
    "patch_fail": "补丁未应用",
}


def load_ids() -> list[str]:
    ids = []
    for line in IDS_FILE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        ids.append(line)
    if len(ids) != 50 or len(set(ids)) != 50:
        raise SystemExit(f"swebench id file has {len(ids)} unique {len(set(ids))}, want 50")
    return ids


def load_cache(ids: list[str]) -> dict[str, dict]:
    if not CACHE.exists():
        raise SystemExit("missing dataset cache outside the repo")
    want = set(ids)
    found: dict[str, dict] = {}
    for line in CACHE.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        if row.get("instance_id") in want:
            found[row["instance_id"]] = row
    missing = [item for item in ids if item not in found]
    if missing:
        raise SystemExit(f"cache missing {len(missing)} instances")
    for row in found.values():
        for key in ("repo", "base_commit", "problem_statement", "patch"):
            if not row.get(key):
                raise SystemExit(f"{row.get('instance_id')} cache missing {key}")
    return found


def git(args: list[str], cwd: Path | None = None, timeout: int = 300) -> subprocess.CompletedProcess:
    env = os.environ.copy()
    env["GIT_TERMINAL_PROMPT"] = "0"
    return subprocess.run(
        ["git", *args],
        cwd=None if cwd is None else str(cwd),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=timeout,
        check=False,
    )


def checkout(instance: dict, dest: Path) -> None:
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    repo = instance["repo"]
    commit = instance["base_commit"]
    if git(["init"], cwd=dest, timeout=60).returncode != 0:
        raise RuntimeError("git init failed")
    if git(["remote", "add", "origin", f"https://github.com/{repo}.git"], cwd=dest, timeout=30).returncode != 0:
        raise RuntimeError("git remote failed")
    fetched = git(["fetch", "--depth", "1", "origin", commit], cwd=dest, timeout=600)
    if fetched.returncode != 0:
        raise RuntimeError("git fetch failed")
    if git(["checkout", "--detach", "FETCH_HEAD"], cwd=dest, timeout=120).returncode != 0:
        raise RuntimeError("git checkout failed")


def capture_patch(dest: Path, base_commit: str) -> str:
    added = git(["add", "-A"], cwd=dest, timeout=120)
    if added.returncode != 0:
        return ""
    diff = git(["diff", "--cached", base_commit], cwd=dest, timeout=120)
    if diff.returncode != 0:
        return ""
    return diff.stdout.decode("utf-8", "replace")


def one_task(binary: Path, token: str, model: str, instance: dict) -> dict:
    task_id = instance["instance_id"]
    root = smoke.RUN_ROOT / "swe-work" / task_id.replace("/", "_")
    if root.exists():
        shutil.rmtree(root)
    work = root / "repo"
    home = root / "home"
    home.mkdir(parents=True)
    print(f"task {task_id}", flush=True)
    try:
        checkout(instance, work)
    except (RuntimeError, subprocess.TimeoutExpired, OSError):
        row = {
            "task_id": task_id,
            "benchmark": "swebench",
            "ok": False,
            "duration_ms": 0,
            "steps": 0,
            "tools": [],
            "reason": "检出失败",
            "fail_reason": "检出失败",
            "category": "clone_error",
            "patch": "",
            "timed_out": False,
            "prompt_tokens": 0,
            "completion_tokens": 0,
        }
        shutil.rmtree(root, ignore_errors=True)
        return row
    prompt = (
        "这是一个需要修复的 GitHub issue。请在当前仓库里直接改代码，只改解决问题所必需的部分。"
        "不要运行需要下载依赖的完整测试；最终是否通过由外部 swebench 评测决定。"
        "可以查看相关文件。改完后停止，不要执行 git commit。\n\nIssue:\n"
        + instance["problem_statement"]
    )
    run = smoke.run_jailed(
        smoke.gocloudagent_cmd(binary, work, prompt),
        smoke.agent_env(home, token, smoke.PROJECT_ID, model),
        work,
        home,
        AGENT_TIMEOUT,
    )
    patch = ""
    try:
        patch = capture_patch(work, instance["base_commit"])
    except (subprocess.TimeoutExpired, OSError):
        patch = ""
    shutil.rmtree(root, ignore_errors=True)
    if not run["sentinel_ok"] or run["code"] in (90, 91, 92, 93, 94, 95):
        row = smoke.record(task_id, "swebench", False, run, "哨兵或隔离失败")
        row["category"] = "isolation"
        row["patch"] = patch
        row["timed_out"] = bool(run["timed_out"])
        return run_core.attach_usage(row, run)
    if not patch.strip():
        reason = "timeout" if run["timed_out"] else (run["reason"] or "没有补丁")
        row = smoke.record(task_id, "swebench", False, run, reason)
        row["category"] = "timeout" if run["timed_out"] else "no_patch"
        row["patch"] = ""
        row["timed_out"] = bool(run["timed_out"])
        return run_core.attach_usage(row, run)
    row = smoke.record(task_id, "swebench", False, run, "待评测")
    row["category"] = "pending"
    row["patch"] = patch
    row["timed_out"] = bool(run["timed_out"])
    row["fail_reason"] = ""
    return run_core.attach_usage(row, run)


def load_done() -> dict[str, dict]:
    done = {}
    if not JSONL.exists():
        return done
    for line in JSONL.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        done[row["task_id"]] = row
    return done


def append_jsonl(row: dict) -> None:
    JSONL.parent.mkdir(parents=True, exist_ok=True)
    with JSONL.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(row, ensure_ascii=False) + "\n")


def write_preds(rows: list[dict]) -> None:
    PREDS.parent.mkdir(parents=True, exist_ok=True)
    with PREDS.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(
                json.dumps(
                    {
                        "instance_id": row["task_id"],
                        "model_name_or_path": MODEL_NAME,
                        "model_patch": row.get("patch") or "",
                    },
                    ensure_ascii=False,
                )
                + "\n"
            )


def ensure_docker() -> None:
    info = subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    if info.returncode != 0:
        raise SystemExit("docker unavailable")
    subprocess.run(["sudo", "-n", "iptables-legacy", "-P", "FORWARD", "ACCEPT"], check=False)


def free_gb() -> int:
    return int(shutil.disk_usage("/").free / (1024**3))


def remove_instance_images(instance_ids: list[str]) -> None:
    proc = subprocess.run(
        ["docker", "images", "--format", "{{.Repository}}:{{.Tag}}"],
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        check=False,
        text=True,
    )
    tails = [item.split("__", 1)[-1].lower() for item in instance_ids if item]
    remove = []
    for image in proc.stdout.splitlines():
        low = image.lower()
        if "sweb.eval" not in low:
            continue
        if any(tail and tail in low for tail in tails):
            remove.append(image)
    if remove:
        subprocess.run(["docker", "image", "rm", "-f", *remove], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
    subprocess.run(["docker", "image", "prune", "-f"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


def eval_predictions(ids: list[str]) -> tuple[int, str]:
    ensure_docker()
    started = time.monotonic()
    have_patch = []
    for line in PREDS.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        if row.get("instance_id") in ids and (row.get("model_patch") or "").strip():
            have_patch.append(row["instance_id"])
    run_id = f"gocloudagent-{smoke.git_sha()}-verified50"
    code = 0
    for offset in range(0, len(have_patch), EVAL_BATCH):
        batch = have_patch[offset : offset + EVAL_BATCH]
        print(f"eval batch {offset // EVAL_BATCH + 1} n={len(batch)} free_gb={free_gb()}", flush=True)
        if free_gb() < 30:
            remove_instance_images(have_patch)
        cmd = [
            DATASET,
            "-p",
            str(PREDS),
            "--run-id",
            run_id,
            "-j",
            str(EVAL_WORKERS),
            "--timeout",
            str(EVAL_TIMEOUT),
            "--report-dir",
            str(EVAL_ROOT / "reports"),
        ]
        for instance_id in batch:
            cmd.extend(["-i", instance_id])
        code = run_swebench(cmd, f"verified50-{offset}.log")
        print(f"batch exit {code}", flush=True)
        remove_instance_images(batch)
    return int(time.monotonic() - started), run_id


def run_swebench(args: list[str], log_name: str) -> int:
    EVAL_ROOT.mkdir(parents=True, exist_ok=True)
    log_path = EVAL_ROOT / log_name
    with log_path.open("w", encoding="utf-8") as handle:
        proc = subprocess.run(
            [str(SWEBENCH), "eval", *args],
            cwd=str(EVAL_ROOT),
            stdout=handle,
            stderr=subprocess.STDOUT,
            check=False,
        )
    return proc.returncode


def gold_one() -> None:
    ensure_docker()
    if not SWEBENCH.exists():
        raise SystemExit("swebench binary missing")
    code = run_swebench(
        [
            "verified",
            "--gold",
            "-i",
            GOLD_INSTANCE,
            "--run-id",
            "gold-verified-one",
            "-j",
            "1",
            "--timeout",
            str(EVAL_TIMEOUT),
            "--report-dir",
            str(EVAL_ROOT / "reports"),
        ],
        "gold-one.log",
    )
    report = EVAL_ROOT / "logs" / "run_evaluation" / "gold-verified-one" / "gold" / GOLD_INSTANCE / "report.json"
    if not report.exists():
        raise SystemExit(f"gold report missing, exit {code}")
    body = json.loads(report.read_text(encoding="utf-8"))
    resolved = bool(body.get(GOLD_INSTANCE, {}).get("resolved"))
    print(f"gold {GOLD_INSTANCE} resolved={resolved} exit={code}", flush=True)
    if not resolved:
        raise SystemExit("gold instance was not resolved")


def instance_report(run_id: str, instance_id: str) -> dict:
    path = (
        EVAL_ROOT
        / "logs"
        / "run_evaluation"
        / run_id
        / MODEL_NAME.replace("/", "__")
        / instance_id
        / "report.json"
    )
    if not path.exists():
        return {}
    try:
        body = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}
    item = body.get(instance_id)
    return item if isinstance(item, dict) else {}


def apply_eval(rows: list[dict], run_id: str) -> None:
    summary_path = EVAL_ROOT / "reports" / f"{MODEL_NAME}.{run_id}.json"
    summary = {}
    if summary_path.exists():
        try:
            summary = json.loads(summary_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            summary = {}
    infra_ids = set(summary.get("infra_failure_ids") or [])
    for row in rows:
        if row.get("category") in ("clone_error", "isolation"):
            continue
        patch = (row.get("patch") or "").strip()
        if not patch:
            row["ok"] = False
            reason = str(row.get("fail_reason") or row.get("reason") or "")
            low = reason.lower()
            if row.get("timed_out") or low == "timeout":
                row["category"] = "timeout"
                row["fail_reason"] = "timeout"
            elif any(token in low for token in ("run_failed", "client.timeout", "deadline exceeded", "llm_upstream", "http 502", "http 429")):
                row["category"] = "agent_error"
                if "429" in low:
                    row["fail_reason"] = "模型服务限流 (HTTP 429)"
                elif "502" in low:
                    row["fail_reason"] = "模型服务暂时不可用 (HTTP 502)"
                elif "deadline" in low or "timeout" in low:
                    row["fail_reason"] = "调用网关超时"
                else:
                    row["fail_reason"] = "agent 调用失败"
            else:
                row["category"] = "no_patch"
                row["fail_reason"] = "没有补丁"
            continue
        report = instance_report(run_id, row["task_id"])
        if report.get("resolved"):
            row["ok"] = True
            row["category"] = "pass"
            row["fail_reason"] = ""
            row["reason"] = "resolved"
            continue
        row["ok"] = False
        if row.get("timed_out"):
            row["category"] = "timeout"
            row["fail_reason"] = "timeout"
        elif not report:
            row["category"] = "infra" if row["task_id"] in infra_ids else "infra"
            row["fail_reason"] = "评测没有写出 report.json"
        elif report.get("infra_failure") or row["task_id"] in infra_ids:
            row["category"] = "infra"
            row["fail_reason"] = str(report.get("infra_failure_reason") or "评测环境失败")[:180]
        elif not report.get("patch_successfully_applied"):
            row["category"] = "patch_fail"
            row["fail_reason"] = "补丁未应用"
        else:
            row["category"] = "unresolved"
            row["fail_reason"] = "测试未通过"


def estimate_yuan(prompt_tokens: int, completion_tokens: int) -> tuple[float | None, str]:
    if not prompt_tokens and not completion_tokens:
        return None, "费用未知：`end` 事件没有 token。"
    yuan = prompt_tokens / 1_000_000 * INPUT_YUAN_PER_MILLION + completion_tokens / 1_000_000 * OUTPUT_YUAN_PER_MILLION
    text = (
        f"token 合计 prompt {prompt_tokens}、completion {completion_tokens}。"
        f"估算费用 ¥{yuan:.2f}（输入 ¥0.80/百万 token，输出 ¥2.80/百万 token，"
        "按计划 2026-10-15 硅基流动公开价，不是账单）。"
    )
    return yuan, text


def extrapolate(rows: list[dict], prompt_tokens: int, completion_tokens: int, eval_s: int) -> str:
    n = len(rows)
    if not n:
        return "没有题目，不能外推。"
    agent_ms = sum(int(row.get("duration_ms") or 0) for row in rows)
    full_agent_hours = (agent_ms / n) * SPLIT_LEN / 3_600_000
    counted = [row for row in rows if int(row.get("prompt_tokens") or 0) or int(row.get("completion_tokens") or 0)]
    if not counted or not (prompt_tokens or completion_tokens):
        cost = "费用无法外推：这些题的 `end` 事件没有 token。"
    else:
        scale = SPLIT_LEN / len(counted)
        yuan = (
            prompt_tokens * scale / 1_000_000 * INPUT_YUAN_PER_MILLION
            + completion_tokens * scale / 1_000_000 * OUTPUT_YUAN_PER_MILLION
        )
        note = "" if len(counted) == n else f"，其中 {len(counted)}/{n} 题有 token"
        cost = f"推理估算费用约 ¥{yuan:.0f}{note}"
    patched = sum(1 for row in rows if (row.get("patch") or "").strip())
    if eval_s and patched:
        full_eval_hours = (eval_s / patched) * (SPLIT_LEN * patched / n) / 3600
        eval_text = f"Docker 评测按同样的空补丁比例外推约 {full_eval_hours:.1f} 小时"
    else:
        eval_text = "这次没有评测墙钟，评测时间不外推"
    return (
        f"按本次每题平均 agent 耗时外推到 Verified 全量 {SPLIT_LEN} 题：约 {full_agent_hours:.1f} 小时，{cost}。{eval_text}。"
        f"外推的推理时间不含 git 检出；费用按公开单价，不是账单。本次停在这 {n} 题，不跑 {SPLIT_LEN}。"
    )


def write_section(meta: dict, rows: list[dict]) -> None:
    passed = sum(1 for row in rows if row["ok"])
    counts: dict[str, int] = {}
    prompt_tokens = 0
    completion_tokens = 0
    agent_ms = 0
    for row in rows:
        prompt_tokens += int(row.get("prompt_tokens") or 0)
        completion_tokens += int(row.get("completion_tokens") or 0)
        agent_ms += int(row.get("duration_ms") or 0)
        if row["ok"]:
            continue
        key = row.get("category") or "pending"
        counts[key] = counts.get(key, 0) + 1
    _yuan, cost = estimate_yuan(prompt_tokens, completion_tokens)
    rate = (passed / len(rows) * 100) if rows else 0.0
    marker = f"{MARKER_PREFIX} {len(rows)}"
    lines = [
        marker,
        "",
        f"> 日期：2026-10-16。main `{run_core.MAIN_COMMIT}`。本次分支 git `{smoke.git_sha()}`。模型 `{meta['model']}`，网关 host `{meta['base_host']}`。",
        f"> **初步结果。** 数据集 `{DATASET}` test split，`len(split)={SPLIT_LEN}`。抽样：Python 3.12 `random.Random(20261016).sample(sorted(instance_id), 50)`，再取清单 `eval/gocloudagent-bench/tasks/core/swebench_verified_50.txt` 里按 instance_id 排序后的前 {len(rows)} 题。不是 Verified 全量。",
        "> 推理在宿主机检出的 base commit 上，`gocloudagent` 不进评测镜像。评分是 `swebench==5.0.2` 的 `swebench eval verified`，Docker，每批 2 题、`-j 2`，单题评测超时 1800 秒。单题镜像约数 GB，批结束后删除该题镜像。agent 超时 900 秒。pass@1。不是官方榜提交。",
        "",
        f"通过 **{passed}/{len(rows)}**（{rate:.1f}%），通过指 swebench `resolved`。题目推理耗时合计 {agent_ms / 1000:.0f} 秒（各题 agent 耗时相加）。评测墙钟 {meta.get('eval_s', 0)} 秒。{cost}",
        "",
        extrapolate(rows, prompt_tokens, completion_tokens, int(meta.get("eval_s") or 0)),
        "",
        "失败分类：",
        "",
    ]
    if not counts:
        lines.append("- 没有失败。")
    else:
        for key, count in sorted(counts.items(), key=lambda item: (-item[1], item[0])):
            lines.append(f"- {CATEGORY_LABEL.get(key, key)}：{count}")
    lines.extend(
        [
            "",
            "公开基线：Z.ai 的 GLM-5.3-Flash 模型卡公布 DeepSWE v1.1 为 63.4%（mini-swe-agent，temperature 0.95，timeout 6 小时，40 万上下文）。那不是 SWE-bench Verified，也不是这 10 题。模型卡没有公布 SWE-bench Verified。不引用第三方聚合站上的数字。",
            "",
            "| 题 | 结果 | 耗时 | 步数 | 分类 | 原因 |",
            "| --- | --- | --- | --- | --- | --- |",
        ]
    )
    for row in rows:
        label = "通过" if row["ok"] else "失败"
        category_key = "pass" if row["ok"] else (row.get("category") or "pending")
        category = CATEGORY_LABEL.get(category_key, category_key)
        reason = "" if row["ok"] else run_core.safe(row.get("fail_reason") or row.get("reason") or "")
        lines.append(
            f"| {row['task_id']} | {label} | {row['duration_ms']} ms | {row['steps']} | {category} | {reason} |"
        )
    lines.append("")
    text = "\n".join(lines)
    for secret in smoke._secret_values:
        if secret and secret in text:
            raise SystemExit("report contained a secret; not written")
    marker = f"{MARKER_PREFIX} {len(rows)}"
    lock_path = smoke.RUN_ROOT / "report.lock"
    with lock_path.open("a", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        existing = smoke.REPORT.read_text(encoding="utf-8")
        smoke.REPORT.write_text(smoke.replace_marked_section(existing, marker, text), encoding="utf-8")
        fcntl.flock(lock, fcntl.LOCK_UN)


def self_check() -> None:
    ids = load_ids()
    if GOLD_INSTANCE not in ids:
        raise SystemExit("gold instance not in the 50")
    if not CACHE.exists():
        print("self-check ok (no dataset cache, skipped lookup)")
        return
    found = load_cache(ids)
    if len(found) != 50:
        raise SystemExit("cache lookup failed")
    print("self-check ok")


def infer_and_eval(no_compile: bool, eval_only: bool, limit: int) -> None:
    secrets = smoke.require_secrets()
    model, _model_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", smoke.DEFAULT_MODEL)
    base, _base_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_BASE_URL", smoke.DEFAULT_BASE)
    host = base.split("//", 1)[-1].split("/", 1)[0]
    ids = load_ids()
    if limit:
        if limit < 1:
            raise SystemExit("--limit must be positive")
        ids = ids[:limit]
    cache = load_cache(ids)
    done = load_done()
    smoke.PORT = PORT
    smoke.BASE_URL = f"http://127.0.0.1:{PORT}"
    started = time.monotonic()
    eval_s = 0
    if not eval_only:
        if not no_compile:
            print("compile", flush=True)
            built = smoke.compile_bins(smoke.RUN_ROOT / "bin")
            if not built["ok"]:
                raise SystemExit(built["reason"])
        binary = smoke.RUN_ROOT / "bin" / "gocloudagent"
        print("preflight", flush=True)
        smoke.preflight_jail()
        print("server", flush=True)
        server = smoke.start_server(
            binary.parent / "simplebase",
            secrets,
            model,
            base,
            run_dir=smoke.RUN_ROOT / "server-swe",
        )
        try:
            _, token = smoke.bootstrap()
            print("probe", flush=True)
            probe_row = smoke.probe(binary, token, model)
            if not probe_row["ok"]:
                raise SystemExit("probe failed " + (probe_row["reason"] or ""))
            for instance_id in ids:
                previous = done.get(instance_id)
                if previous and previous.get("category") == "isolation":
                    print("isolation already blocked, stop", flush=True)
                    break
                retry = previous is None or previous.get("category") == "clone_error" or (
                    previous.get("category") == "pending" and not (previous.get("patch") or "").strip()
                )
                if previous and not retry:
                    print(f"skip {instance_id}", flush=True)
                    continue
                row = one_task(binary, token, model, cache[instance_id])
                append_jsonl(row)
                done[instance_id] = row
                if row.get("category") == "isolation":
                    print("isolation blocked, stop", flush=True)
                    break
        finally:
            smoke.stop_server(server)
    ordered = [done[item] for item in ids if item in done]
    write_preds(ordered)
    run_id = ""
    if ordered and any((row.get("patch") or "").strip() for row in ordered):
        eval_s, run_id = eval_predictions(ids)
        print(f"swebench run {run_id}", flush=True)
        apply_eval(ordered, run_id)
    write_section(
        {
            "model": model,
            "base_host": host,
            "wall_s": int(time.monotonic() - started),
            "eval_s": eval_s,
        },
        ordered,
    )
    print(f"report {smoke.REPORT}", flush=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-check", action="store_true")
    parser.add_argument("--gold-one", action="store_true")
    parser.add_argument("--no-compile", action="store_true")
    parser.add_argument("--eval-only", action="store_true")
    parser.add_argument("--limit", type=int, default=0)
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return
    if args.gold_one:
        gold_one()
        return
    infer_and_eval(args.no_compile, args.eval_only, args.limit)


if __name__ == "__main__":
    try:
        main()
    except SystemExit as exc:
        if exc.code not in (0, None):
            print(smoke.redact(str(exc)), file=sys.stderr)
        raise
    except Exception as exc:
        print(smoke.redact(f"{type(exc).__name__}: {exc}"), file=sys.stderr)
        sys.exit(1)
