#!/usr/bin/env python3
"""核心层里本次要跑的 HumanEval 164。Terminal-Bench 在 run_terminal_bench.py。

只用标准库。密钥只检查非空，不打印、不写入报告。
"""

from __future__ import annotations

import argparse
import gzip
import json
import sys
import time
from pathlib import Path

import run_smoke as smoke

FIXTURE = Path(__file__).resolve().parent / "tasks" / "core" / "humaneval.jsonl"
JSONL = smoke.RUN_ROOT / "humaneval164-headerfix.jsonl"
PREV_JSONL = smoke.RUN_ROOT / "humaneval164-rerun.jsonl"
REGRESS_JSONL = smoke.RUN_ROOT / "humaneval164-f1e7c1e.jsonl"
MARKER = "## HumanEval 164 首字节修复后"
# 修复基于的 origin/main。分支上后续的报告提交不改变这个基线。
MAIN_COMMIT = "f1e7c1e02bb894dfefa7fb79057a2c2074201fed"
PREV_LABEL = "main e6994c6，158/164"


def load_rows() -> list[dict]:
    rows = []
    for line in FIXTURE.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rows.append(json.loads(line))
    if len(rows) != 164:
        raise SystemExit(f"humaneval fixture has {len(rows)} rows, want 164")
    ids = [row["task_id"] for row in rows]
    if ids != [f"HumanEval/{i}" for i in range(164)]:
        raise SystemExit("humaneval fixture is not HumanEval/0..163 in order")
    for row in rows:
        if "canonical_solution" in row:
            raise SystemExit("fixture must not contain canonical_solution")
    return rows


def classify(reason: str, run: dict) -> str:
    text = reason or ""
    low = text.lower()
    if run.get("timed_out") or low == "timeout" or "测试超时" in text:
        return "timeout"
    if "哨兵" in text or "隔离" in text:
        return "isolation"
    if "SyntaxError" in text or "IndentationError" in text or "TabError" in text:
        return "syntax_error"
    if "AssertionError" in text:
        return "test_fail"
    if "llm_upstream" in low or "http 502" in low or "http 429" in low or "run_failed" in low:
        return "agent_error"
    if any(name in text for name in ("Error", "Exception", "Traceback")):
        return "runtime_error"
    if text.startswith("exit_") or "exec_missing" in text or "api_error" in low or "error:" in low:
        return "agent_error"
    if "没有" in text:
        return "no_output"
    return "test_fail"


def one_task(binary: Path, token: str, model: str, item: dict) -> dict:
    task_id = item["task_id"]
    work, home = smoke.setup_task(task_id)
    (work / "solution.py").write_text(item["prompt"], encoding="utf-8")
    prompt = (
        "solution.py 里是一个未完成的 Python 函数。补全它，使函数行为正确。"
        f"函数名必须保持 {item['entry_point']}。不要增加测试，不要改函数名。完成后停止。"
    )
    print(f"task {task_id}", flush=True)
    run = smoke.run_jailed(
        smoke.gocloudagent_cmd(binary, work, prompt),
        smoke.agent_env(home, token, smoke.PROJECT_ID, model),
        work,
        home,
        smoke.HUMANEVAL_TIMEOUT,
    )
    if not run["sentinel_ok"] or run["code"] in (90, 91, 92, 93, 94, 95):
        reason = "哨兵或隔离失败"
        row = smoke.record(task_id, "humaneval164", False, run, reason)
        row["category"] = "isolation"
        return attach_usage(row, run)
    if not run["events"] and run["code"] not in (0, 124):
        reason = run["reason"] or f"exit_{run['code']}"
        row = smoke.record(task_id, "humaneval164", False, run, reason)
        row["category"] = classify(reason, run)
        return attach_usage(row, run)
    grade_ok, grade_reason = smoke.grade_humaneval(work, item["entry_point"], item["test"])
    reason = "" if grade_ok else (grade_reason or run["reason"])
    if run["timed_out"] and not grade_ok:
        reason = "timeout"
    row = smoke.record(task_id, "humaneval164", grade_ok, run, reason)
    row["category"] = "pass" if grade_ok else classify(reason, run)
    return attach_usage(row, run)


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


def category_of(row: dict) -> str:
    if row.get("ok"):
        return "pass"
    return classify(row.get("fail_reason") or row.get("reason") or "", row)


def load_jsonl(path: Path) -> dict[str, dict]:
    rows: dict[str, dict] = {}
    if not path.exists():
        return rows
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        rows[row["task_id"]] = row
    return rows


def attach_usage(row: dict, run: dict) -> dict:
    prompt = 0
    completion = 0
    for event in run.get("events") or []:
        if not isinstance(event, dict) or event.get("type") != "end":
            continue
        usage = event.get("usage")
        if not isinstance(usage, dict):
            continue
        prompt += int(usage.get("prompt_tokens") or 0)
        completion += int(usage.get("completion_tokens") or 0)
    row["prompt_tokens"] = prompt
    row["completion_tokens"] = completion
    return row


def safe(text: str) -> str:
    raw = text or ""
    low = raw.lower()
    if "http 502" in low or "模型服务暂时不可用" in raw:
        raw = "模型服务暂时不可用 (HTTP 502)"
    elif "http 429" in low:
        raw = "模型服务限流 (HTTP 429)"
    return raw.replace("|", "/").replace("\n", " ")[:180]


CATEGORY_LABEL = {
    "pass": "通过",
    "timeout": "超时",
    "test_fail": "断言失败",
    "syntax_error": "语法错误",
    "runtime_error": "运行错误",
    "agent_error": "agent 错误",
    "isolation": "隔离失败",
    "no_output": "无输出",
}


def comparison_lines(rows: list[dict]) -> list[str]:
    prev = load_jsonl(PREV_JSONL)
    if len(prev) != 164:
        return ["上次 JSONL 不完整，没有逐题对比。", ""]
    gained = []
    lost = []
    changed = []
    for index in range(164):
        task_id = f"HumanEval/{index}"
        old = prev[task_id]
        new = next(row for row in rows if row["task_id"] == task_id)
        old_cat = category_of(old)
        new_cat = category_of(new)
        if (not old["ok"]) and new["ok"]:
            gained.append(f"{task_id}（上次 {CATEGORY_LABEL.get(old_cat, old_cat)}）")
        elif old["ok"] and not new["ok"]:
            lost.append(f"{task_id}（这次 {CATEGORY_LABEL.get(new_cat, new_cat)}：{safe(new.get('fail_reason') or '')}）")
        elif old_cat != new_cat:
            changed.append(
                f"{task_id}：{CATEGORY_LABEL.get(old_cat, old_cat)} → {CATEGORY_LABEL.get(new_cat, new_cat)}"
            )
    old_pass = sum(1 for row in prev.values() if row["ok"])
    lines = [
        f"与上次（{PREV_LABEL}；JSONL 实计 {old_pass}/164）对比，同一套题、同一模型：",
        "",
        f"- 新增通过 {len(gained)} 题：" + ("、".join(gained) if gained else "没有。"),
        f"- 新增失败 {len(lost)} 题：" + ("、".join(lost) if lost else "没有。"),
        f"- 仍失败但分类变化 {len(changed)} 题：" + ("、".join(changed) if changed else "没有。"),
        "",
    ]
    return lines


def write_section(meta: dict, rows: list[dict]) -> None:
    passed = sum(1 for row in rows if row["ok"])
    counts: dict[str, int] = {}
    prompt_tokens = 0
    completion_tokens = 0
    for row in rows:
        prompt_tokens += int(row.get("prompt_tokens") or 0)
        completion_tokens += int(row.get("completion_tokens") or 0)
        if row["ok"]:
            continue
        key = category_of(row)
        counts[key] = counts.get(key, 0) + 1
    if prompt_tokens or completion_tokens:
        yuan = prompt_tokens / 1_000_000 * 0.80 + completion_tokens / 1_000_000 * 2.80
        cost = (
            f"token 合计 prompt {prompt_tokens}、completion {completion_tokens}。"
            f"估算费用 ¥{yuan:.2f}（输入 ¥0.80/百万 token，输出 ¥2.80/百万 token，按计划 2026-10-15 硅基流动公开价，不是账单）。"
        )
    else:
        cost = "费用仍未知：`end` 事件没有 token。"
    regress = load_jsonl(REGRESS_JSONL)
    regress_line = ""
    if regress:
        regress_pass = sum(1 for row in regress.values() if row.get("ok"))
        regress_line = (
            f"同一 main 在修复前是 {regress_pass}/{len(regress)}。"
            "根因是 CLI 把响应头超时设成 30 秒，而服务端要等上游首字节才写响应头；"
            "推理模型经常超过 30 秒，agent 只读完文件就以 exit 2 退出，原文件被当成断言失败。"
            "修复后首字节等待与 5 分钟整体超时对齐，并在等上游之前解除服务端写超时。网关重试保留。"
        )
    lines = [
        MARKER,
        "",
        f"> 日期：2026-10-16。main `{MAIN_COMMIT}`。本次分支 git `{smoke.git_sha()}`。模型 `{meta['model']}`，网关 host `{meta['base_host']}`。",
        "> 修复首字节超时后重编 gocloudagent，HumanEval 164 全部新跑。pass@1，每题 1 次。不是单次补全，不能直接和官方 HumanEval 榜横比。",
        "",
        f"通过 **{passed}/{len(rows)}**（{passed / len(rows) * 100:.1f}%）。耗时合计 {meta['wall_s']} 秒。{cost}",
        "",
    ]
    if regress_line:
        lines.extend([regress_line, ""])
    if len(rows) == 164:
        lines.extend(comparison_lines(rows))
    lines.extend(["失败分类：", ""])
    if not counts:
        lines.append("- 没有失败。")
    else:
        for key, count in sorted(counts.items(), key=lambda item: (-item[1], item[0])):
            lines.append(f"- {CATEGORY_LABEL.get(key, key)}：{count}")
    lines.extend(
        [
            "",
            "公开基线：Z.ai 的 GLM-5.3-Flash 模型卡没有公布 HumanEval。它公布的 Terminal-Bench 2.1 为 84.3%，用的是 Claude Code 2.1.207，不是本 adapter，也不是 Terminal-Bench 4.0。第三方量化权重上的 greedy HumanEval 不是这条链路，不拿来当基线。",
            "",
            "| 题 | 结果 | 耗时 | 步数 | 分类 | 原因 |",
            "| --- | --- | --- | --- | --- | --- |",
        ]
    )
    for row in rows:
        label = "通过" if row["ok"] else "失败"
        category_key = "pass" if row["ok"] else classify(row.get("fail_reason") or row.get("reason") or "", row)
        category = CATEGORY_LABEL.get(category_key, category_key)
        reason = "" if row["ok"] else safe(row.get("fail_reason") or row.get("reason") or "")
        lines.append(
            f"| {row['task_id']} | {label} | {row['duration_ms']} ms | {row['steps']} | {category} | {reason} |"
        )
    lines.append("")
    text = "\n".join(lines)
    for secret in smoke._secret_values:
        if secret and secret in text:
            raise SystemExit("report contained a secret; not written")
    existing = smoke.REPORT.read_text(encoding="utf-8")
    smoke.REPORT.write_text(smoke.replace_marked_section(existing, MARKER, text), encoding="utf-8")


def self_check() -> None:
    rows = load_rows()
    gz = Path("/tmp/HumanEval.jsonl.gz")
    if not gz.exists():
        print("self-check ok (no canonical gz, skipped grade)")
        return
    official = []
    with gzip.open(gz, "rt", encoding="utf-8") as handle:
        for line in handle:
            if line.strip():
                official.append(json.loads(line))
    item = official[0]
    work, _home = smoke.setup_task("humaneval-canonical-0")
    solution = item["prompt"] + item["canonical_solution"]
    (work / "solution.py").write_text(solution, encoding="utf-8")
    ok, reason = smoke.grade_humaneval(work, item["entry_point"], item["test"])
    if not ok:
        raise SystemExit(f"canonical grade failed: {reason}")
    assert rows[0]["task_id"] == "HumanEval/0"
    print("self-check ok")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-check", action="store_true")
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return
    secrets = smoke.require_secrets()
    model, model_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", smoke.DEFAULT_MODEL)
    base, base_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_BASE_URL", smoke.DEFAULT_BASE)
    host = base.split("//", 1)[-1].split("/", 1)[0]
    rows_spec = load_rows()
    done = load_done()
    print("compile", flush=True)
    built = smoke.compile_bins(smoke.RUN_ROOT / "bin")
    if not built["ok"]:
        raise SystemExit(built["reason"])
    binary = smoke.RUN_ROOT / "bin" / "gocloudagent"
    print("preflight", flush=True)
    smoke.preflight_jail()
    print("server", flush=True)
    server = smoke.start_server(binary.parent / "simplebase", secrets, model, base)
    started = time.monotonic()
    try:
        _, token = smoke.bootstrap()
        print("probe", flush=True)
        probe_row = smoke.probe(binary, token, model)
        if not probe_row["ok"]:
            raise SystemExit("probe failed " + (probe_row["reason"] or ""))
        for item in rows_spec:
            if item["task_id"] in done:
                print(f"skip {item['task_id']}", flush=True)
                continue
            row = one_task(binary, token, model, item)
            append_jsonl(row)
            done[item["task_id"]] = row
            if row.get("category") == "isolation":
                print("isolation blocked, stop", flush=True)
                break
    finally:
        smoke.stop_server(server)
        ordered = [done[item["task_id"]] for item in rows_spec if item["task_id"] in done]
        write_section(
            {
                "model": model,
                "base_host": host,
                "model_source": model_source,
                "base_source": base_source,
                "wall_s": int(time.monotonic() - started),
            },
            ordered,
        )
        print(f"report {smoke.REPORT}", flush=True)


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
