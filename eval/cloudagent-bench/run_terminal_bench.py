#!/usr/bin/env python3
"""用 Harbor 0.24.0 跑 Terminal-Bench v4.0.0 的固定 8 题。

oracle 只验证题目和 Docker。cloudagent 模式才调用模型。
密钥只放进服务进程和 CLOUDAGENT_TOKEN，不打印、不写入报告。
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime
from pathlib import Path

import run_smoke as smoke

IDS = Path(__file__).resolve().parent / "tasks" / "core" / "terminal_bench_ids.txt"
HARBOR = Path("/home/ubuntu/cloudagent-bench/venv/bin/harbor")
JOBS = smoke.RUN_ROOT / "jobs"
MARKER = "## Terminal-Bench 8"
DATASET = "terminal-bench/terminal-bench@4.0.0"


def task_ids() -> list[str]:
    rows = []
    for line in IDS.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        rows.append(line)
    if len(rows) != 8:
        raise SystemExit(f"want 8 terminal-bench ids, got {len(rows)}")
    return rows


def harbor_cmd(agent: str, job_name: str) -> list[str]:
    cmd = [
        str(HARBOR),
        "run",
        "-d",
        DATASET,
        "-a",
        agent,
        "--env",
        "docker",
        "-n",
        "2",
        "-k",
        "1",
        "-y",
        "-o",
        str(JOBS),
        "--job-name",
        job_name,
        "-q",
    ]
    if agent != "oracle":
        model, _ = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", smoke.DEFAULT_MODEL)
        cmd.extend(["-m", model])
    for task_id in task_ids():
        # Harbor 数据集里的名字带 registry 前缀，过滤时必须带上。
        cmd.extend(["-i", f"terminal-bench/{task_id}"])
    return cmd


def run_harbor(agent: str, job_name: str, extra_env: dict[str, str] | None = None) -> int:
    env = os.environ.copy()
    env["PYTHONPATH"] = str(Path(__file__).resolve().parent) + os.pathsep + env.get("PYTHONPATH", "")
    if extra_env:
        env.update(extra_env)
    print("harbor", agent, job_name, flush=True)
    proc = subprocess.run(harbor_cmd(agent, job_name), env=env, check=False)
    return proc.returncode


def phase_ms(timing: object) -> int:
    if not isinstance(timing, dict):
        return 0
    start = timing.get("started_at")
    end = timing.get("finished_at")
    if not start or not end:
        return 0
    try:
        begin = datetime.fromisoformat(str(start).replace("Z", "+00:00"))
        finish = datetime.fromisoformat(str(end).replace("Z", "+00:00"))
    except ValueError:
        return 0
    return max(0, int((finish - begin).total_seconds() * 1000))


def trial_results(job_name: str) -> list[dict]:
    root = JOBS / job_name
    rows = []
    if not root.exists():
        return rows
    for path in sorted(root.rglob("result.json")):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            continue
        if "task_name" not in data:
            continue
        reward = None
        verifier = data.get("verifier_result") or {}
        rewards = verifier.get("rewards") if isinstance(verifier, dict) else None
        if isinstance(rewards, dict) and rewards:
            reward = next(iter(rewards.values()))
        exc = data.get("exception_info") or {}
        duration_ms = phase_ms(data.get("agent_execution")) or phase_ms(
            {"started_at": data.get("started_at"), "finished_at": data.get("finished_at")}
        )
        meta = {}
        agent_result = data.get("agent_result") or {}
        if isinstance(agent_result, dict) and isinstance(agent_result.get("metadata"), dict):
            meta = agent_result["metadata"]
        reason = ""
        if isinstance(exc, dict) and exc.get("exception_type"):
            reason = str(exc.get("exception_type"))
            message = str(exc.get("exception_message") or "")
            if "unsupported_libc" in message:
                reason = "unsupported_libc"
            elif message:
                reason = f"{reason}: {message.splitlines()[-1][:160]}"
        elif reward == 0 or reward == 0.0:
            reason = "verifier 未通过"
        raw_name = str(data.get("task_name") or "")
        rows.append(
            {
                "task_id": raw_name.split("/")[-1],
                "ok": reward == 1 or reward == 1.0,
                "reward": reward,
                "duration_ms": duration_ms,
                "steps": meta.get("steps", ""),
                "reason": smoke.redact(reason)[:180],
            }
        )
    # 同题多次时保留最后一次
    latest: dict[str, dict] = {}
    for row in rows:
        if row["task_id"]:
            latest[row["task_id"]] = row
    return [latest[task_id] for task_id in task_ids() if task_id in latest]


def classify_tb(row: dict) -> str:
    if row["ok"]:
        return "pass"
    reason = row.get("reason") or ""
    if "unsupported_libc" in reason:
        return "unsupported_libc"
    if "Timeout" in reason or "timeout" in reason.lower():
        return "timeout"
    if reason.startswith("verifier"):
        return "verifier_fail"
    if reason:
        return "agent_or_env_error"
    return "verifier_fail"


def append_report(title_rows: list[dict], note: str, meta: dict) -> None:
    passed = sum(1 for row in title_rows if row["ok"])
    counted = [row for row in title_rows if classify_tb(row) != "unsupported_libc"]
    counted_pass = sum(1 for row in counted if row["ok"])
    denom = len(counted)
    lines = [
        MARKER,
        "",
        f"> 日期：2026-10-15。git `{smoke.git_sha()}`。数据集 `{DATASET}`。Harbor 0.24.0，`--env docker`，并发 2，每题 1 次。",
        f"> 模型 `{meta['model']}`，网关 host `{meta['base_host']}`。agent 装进任务容器，经宿主机网关回连 simplebase。非官方榜提交。",
        "",
        f"计入分母 **{counted_pass}/{denom}** 通过。表内共 {passed}/{len(title_rows)} 题奖励为 1。`unsupported_libc` 不计入分母。",
        "",
        note,
        "",
        "公开基线：Z.ai 公布 GLM-5.3-Flash 的 Terminal-Bench 2.1 为 84.3%（Claude Code 2.1.207，5 次平均）。那是另一套题目、另一个 agent 和多次试验，不能和这 8 题的 Harbor 结果横比。",
        "",
        "| 题 | 结果 | 耗时 | 步数 | 分类 | 原因 |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    labels = {
        "pass": "通过",
        "unsupported_libc": "libc 不支持",
        "timeout": "超时",
        "verifier_fail": "verifier 未通过",
        "agent_or_env_error": "agent 或环境错误",
    }
    for row in title_rows:
        category = classify_tb(row)
        reason = "" if row["ok"] else (row.get("reason") or "").replace("|", "/")
        lines.append(
            f"| {row['task_id']} | {'通过' if row['ok'] else '失败'} | {row['duration_ms']} ms | {row['steps']} | {labels.get(category, category)} | {reason} |"
        )
    lines.append("")
    text = "\n".join(lines)
    for secret in smoke._secret_values:
        if secret and secret in text:
            raise SystemExit("report contained a secret; not written")
    existing = smoke.REPORT.read_text(encoding="utf-8")
    if MARKER in existing:
        existing = existing.split(MARKER)[0].rstrip() + "\n"
    smoke.REPORT.write_text(existing.rstrip() + "\n\n" + text, encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--oracle", action="store_true")
    args = parser.parse_args()
    if not HARBOR.is_file():
        raise SystemExit("harbor is not installed")
    subprocess.run(["sudo", "chmod", "666", "/var/run/docker.sock"], check=False)
    if args.oracle:
        code = run_harbor("oracle", "tb8-oracle")
        print(f"oracle_exit {code}", flush=True)
        for row in trial_results("tb8-oracle"):
            print(f"oracle {row['task_id']} ok={row['ok']} reward={row['reward']} reason={row['reason']}", flush=True)
        sys.exit(code)
    secrets = smoke.require_secrets()
    model, _source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", smoke.DEFAULT_MODEL)
    base, _base_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_BASE_URL", smoke.DEFAULT_BASE)
    host = base.split("//", 1)[-1].split("/", 1)[0]
    print("compile", flush=True)
    built = smoke.compile_bins(smoke.RUN_ROOT / "bin")
    if not built["ok"]:
        raise SystemExit(built["reason"])
    binary = smoke.RUN_ROOT / "bin" / "cloudagent"
    print("server", flush=True)
    server = smoke.start_server(binary.parent / "simplebase", secrets, model, base, bind_host="0.0.0.0")
    try:
        _, token = smoke.bootstrap()
        probe_row = smoke.probe(binary, token, model)
        if not probe_row["ok"]:
            raise SystemExit("probe failed")
        extra = {
            "CLOUDAGENT_BIN": str(binary),
            "CLOUDAGENT_TOKEN": token,
            "CLOUDAGENT_PROJECT_ID": smoke.PROJECT_ID,
            "CLOUDAGENT_MODEL": model,
            "CLOUDAGENT_PORT": str(smoke.PORT),
            "CLOUDAGENT_MAX_ITERATIONS": smoke.MAX_ITER,
        }
        started = time.monotonic()
        code = run_harbor("harbor_agent:CloudAgentInstalled", "tb8-cloudagent", extra)
        print(f"cloudagent_exit {code}", flush=True)
        rows = trial_results("tb8-cloudagent")
        note = (
            f"Harbor 退出码 {code}。墙钟 {int(time.monotonic() - started)} 秒。"
            "只跑了计划中的固定 8 题；66 题要等这 8 题的耗时可以接受再扩。"
        )
        append_report(rows, note, {"model": model, "base_host": host})
        print(f"report {smoke.REPORT}", flush=True)
    finally:
        smoke.stop_server(server)


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
