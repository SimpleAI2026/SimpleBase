#!/usr/bin/env python3
"""MBPP sanitized 全量。agent 补全 solution.py，再在隔离里跑官方 test_list。

fixture 不含标准答案。密钥只检查非空，不打印、不写入报告。
"""

from __future__ import annotations

import argparse
import fcntl
import json
import sys
import time
from pathlib import Path

import run_core
import run_smoke as smoke

FIXTURE = Path(__file__).resolve().parent / "tasks" / "core" / "mbpp_sanitized.jsonl"
JSONL = smoke.RUN_ROOT / "mbpp-headerfix.jsonl"
PREV_JSONL = smoke.RUN_ROOT / "mbpp.jsonl"
REGRESS_JSONL = smoke.RUN_ROOT / "mbpp-f1e7c1e.jsonl"
MARKER = "## MBPP sanitized 首字节修复后"
PREV_LABEL = "main e6994c6，352/427"
FULL_N = 427
CANONICAL = Path("/tmp/sanitized-mbpp.json")
SELF_CHECK_IDS = (2, 6, 82)
INPUT_YUAN_PER_MILLION = 0.80
OUTPUT_YUAN_PER_MILLION = 2.80


def load_rows() -> list[dict]:
    rows = []
    for line in FIXTURE.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rows.append(json.loads(line))
    if len(rows) != 427:
        raise SystemExit(f"mbpp fixture has {len(rows)} rows, want 427")
    seen = set()
    for row in rows:
        if "code" in row or "canonical_solution" in row:
            raise SystemExit("fixture must not contain a solution")
        for key in ("task_id", "source_id", "prompt", "entry_point", "signature", "test_imports", "test_list"):
            if key not in row:
                raise SystemExit(f"fixture missing {key}")
        if row["task_id"] in seen:
            raise SystemExit("duplicate task_id")
        seen.add(row["task_id"])
        if not row["test_list"]:
            raise SystemExit(f"{row['task_id']} has no tests")
    return rows


def grade_mbpp(work: Path, item: dict) -> tuple[bool, str]:
    solution = work / "solution.py"
    if not solution.exists():
        return False, "没有 solution.py"
    spec = {
        "imports": list(item.get("test_imports") or []),
        "tests": list(item["test_list"]),
    }
    (work / "_grade_spec.json").write_text(json.dumps(spec), encoding="utf-8")
    grader = work / "_grade.py"
    grader.write_text(
        "import json\n"
        "import pathlib\n"
        "spec = json.loads(pathlib.Path('_grade_spec.json').read_text(encoding='utf-8'))\n"
        "ns = {}\n"
        "for line in spec['imports']:\n"
        "    exec(line, ns)\n"
        "exec(pathlib.Path('solution.py').read_text(encoding='utf-8'), ns)\n"
        "for test in spec['tests']:\n"
        "    exec(test, ns)\n"
        "print('PASS')\n",
        encoding="utf-8",
    )
    home = work.parent / "grade-home"
    home.mkdir(exist_ok=True)
    run = smoke.run_jailed(
        ["python3", "-I", str(grader)],
        {"PATH": __import__("os").environ.get("PATH", ""), "HOME": str(home), "PYTHONDONTWRITEBYTECODE": "1"},
        work,
        home,
        10,
    )
    if not run["sentinel_ok"]:
        return False, "评分时哨兵被改掉"
    if run["code"] == 0 and "PASS" in run["stdout"]:
        return True, ""
    if run["timed_out"] or run["code"] == 124:
        return False, "测试超时"
    text = (run["stderr"] + "\n" + run["stdout"]).strip().splitlines()
    return False, (text[-1] if text else f"exit_{run['code']}")[:200]


def stub_source(item: dict) -> str:
    prompt = str(item["prompt"]).replace('"""', "'''")
    return f"{item['signature']}\n    \"\"\"{prompt}\"\"\"\n    pass\n"


def one_task(binary: Path, token: str, model: str, item: dict) -> dict:
    task_id = item["task_id"]
    work, home = smoke.setup_task(task_id)
    (work / "solution.py").write_text(stub_source(item), encoding="utf-8")
    prompt = (
        "solution.py 里是一个未完成的 Python 函数。按注释里的题意补全它，使函数行为正确。"
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
        row = smoke.record(task_id, "mbpp", False, run, reason)
        row["category"] = "isolation"
        return run_core.attach_usage(row, run)
    if not run["events"] and run["code"] not in (0, 124):
        reason = run["reason"] or f"exit_{run['code']}"
        row = smoke.record(task_id, "mbpp", False, run, reason)
        row["category"] = run_core.classify(reason, run)
        return run_core.attach_usage(row, run)
    grade_ok, grade_reason = grade_mbpp(work, item)
    reason = "" if grade_ok else (grade_reason or run["reason"])
    if run["timed_out"] and not grade_ok:
        reason = "timeout"
    row = smoke.record(task_id, "mbpp", grade_ok, run, reason)
    row["category"] = "pass" if grade_ok else run_core.classify(reason, run)
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


def estimate_yuan(prompt_tokens: int, completion_tokens: int) -> str:
    if not prompt_tokens and not completion_tokens:
        return "费用未知：`end` 事件没有 token。"
    yuan = prompt_tokens / 1_000_000 * INPUT_YUAN_PER_MILLION + completion_tokens / 1_000_000 * OUTPUT_YUAN_PER_MILLION
    return (
        f"token 合计 prompt {prompt_tokens}、completion {completion_tokens}。"
        f"估算费用 ¥{yuan:.2f}（输入 ¥0.80/百万 token，输出 ¥2.80/百万 token，"
        "按计划 2026-10-15 硅基流动公开价，不是账单）。"
    )


def extrapolate(rows: list[dict], prompt_tokens: int, completion_tokens: int, full_n: int) -> str:
    n = len(rows)
    if not n:
        return "没有题目，不能外推。"
    agent_ms = sum(int(row.get("duration_ms") or 0) for row in rows)
    full_hours = (agent_ms / n) * full_n / 3_600_000
    counted = [row for row in rows if int(row.get("prompt_tokens") or 0) or int(row.get("completion_tokens") or 0)]
    if not counted or not (prompt_tokens or completion_tokens):
        cost = "费用无法外推：这些题的 `end` 事件没有 token。"
    else:
        scale = full_n / len(counted)
        yuan = (
            prompt_tokens * scale / 1_000_000 * INPUT_YUAN_PER_MILLION
            + completion_tokens * scale / 1_000_000 * OUTPUT_YUAN_PER_MILLION
        )
        note = "" if len(counted) == n else f"，其中 {len(counted)}/{n} 题有 token"
        cost = f"估算费用约 ¥{yuan:.0f}{note}"
    return (
        f"按本次每题平均 agent 耗时外推到 sanitized 全量 {full_n} 题：约 {full_hours:.1f} 小时，{cost}。"
        "外推只含各题 agent 时间，不含进程启动；费用按同样的公开单价，不是账单。"
    )


def comparison_lines(rows: list[dict]) -> list[str]:
    prev = run_core.load_jsonl(PREV_JSONL)
    if len(prev) != FULL_N:
        return [f"上次 JSONL 有 {len(prev)} 题，没有逐题对比。", ""]
    gained = []
    lost = []
    changed = []
    for row in rows:
        task_id = row["task_id"]
        old = prev.get(task_id)
        if old is None:
            continue
        old_cat = run_core.category_of(old)
        new_cat = run_core.category_of(row)
        if (not old["ok"]) and row["ok"]:
            gained.append(f"{task_id}（上次 {run_core.CATEGORY_LABEL.get(old_cat, old_cat)}）")
        elif old["ok"] and not row["ok"]:
            lost.append(
                f"{task_id}（这次 {run_core.CATEGORY_LABEL.get(new_cat, new_cat)}：{run_core.safe(row.get('fail_reason') or '')}）"
            )
        elif old_cat != new_cat:
            changed.append(
                f"{task_id}：{run_core.CATEGORY_LABEL.get(old_cat, old_cat)} → {run_core.CATEGORY_LABEL.get(new_cat, new_cat)}"
            )
    old_pass = sum(1 for row in prev.values() if row["ok"])
    return [
        f"与上次（{PREV_LABEL}；JSONL 实计 {old_pass}/{FULL_N}）对比，同一套题、同一模型：",
        "",
        f"- 新增通过 {len(gained)} 题：" + ("、".join(gained) if gained else "没有。"),
        f"- 新增失败 {len(lost)} 题：" + ("、".join(lost) if lost else "没有。"),
        f"- 仍失败但分类变化 {len(changed)} 题：" + ("、".join(changed) if changed else "没有。"),
        "",
    ]


def write_section(meta: dict, rows: list[dict], preliminary: bool) -> None:
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
        key = row.get("category") or run_core.category_of(row)
        counts[key] = counts.get(key, 0) + 1
    rate = (passed / len(rows) * 100) if rows else 0.0
    reused = int(meta.get("reused_prefix") or 0)
    if preliminary:
        scope = f"**初步结果。** 只跑了 sanitized fixture 顺序的前 {len(rows)} 题，不是全量 {FULL_N}。"
        follow = extrapolate(rows, prompt_tokens, completion_tokens, FULL_N)
    elif len(rows) == FULL_N:
        scope = f"sanitized 全量 {FULL_N} 题。"
        if reused:
            follow = (
                f"fixture 顺序的前 {reused} 题复用同日初步运行的 JSONL，没有重跑。"
                "模型、harness、120 秒上限和官方 `test_list` 都与那次相同。其余题目是这次续跑补上的。"
            )
        else:
            follow = "427 题都是本次新跑的，没有复用 e6994c6 或 f1e7c1e 的 JSONL。"
    else:
        scope = f"进行中：已有 {len(rows)}/{FULL_N} 题，还不是全量。"
        follow = "这一节会在 427 题到齐后换成全量结果。"
    lines = [
        MARKER,
        "",
        f"> 日期：2026-10-16。main `{run_core.MAIN_COMMIT}`。本次分支 git `{smoke.git_sha()}`。模型 `{meta['model']}`，网关 host `{meta['base_host']}`。",
        "> Google Research MBPP sanitized。agent 补全 `solution.py` 后，在隔离里用官方 `test_list` 评分（不含 challenge 测试）。pass@1，每题 1 次。不是单次补全，不能直接和公开 MBPP 榜横比。",
        "",
        scope,
        "",
        f"通过 **{passed}/{len(rows)}**（{rate:.1f}%）。题目耗时合计 {agent_ms / 1000:.0f} 秒（各题 agent 耗时相加）。{estimate_yuan(prompt_tokens, completion_tokens)}",
        "",
        follow,
        "",
    ]
    if not preliminary and len(rows) == FULL_N:
        lines.extend(comparison_lines(rows))
    regress = run_core.load_jsonl(REGRESS_JSONL)
    if regress:
        regress_pass = sum(1 for row in regress.values() if row.get("ok"))
        lines.extend(
            [
                f"同一 main 在修复前是 {regress_pass}/{len(regress)}。根因与 HumanEval 相同：30 秒响应头超时把慢首字节当成网关黑洞。",
                "",
            ]
        )
    lines.extend(["失败分类：", ""])
    if not counts:
        lines.append("- 没有失败。")
    else:
        for key, count in sorted(counts.items(), key=lambda item: (-item[1], item[0])):
            lines.append(f"- {run_core.CATEGORY_LABEL.get(key, key)}：{count}")
    lines.extend(
        [
            "",
            "公开基线：Z.ai 的 GLM-5.3-Flash 模型卡没有公布 MBPP。不引用第三方聚合站上的数字。这次分数不是官方榜提交。",
            "",
            "| 题 | 结果 | 耗时 | 步数 | 分类 | 原因 |",
            "| --- | --- | --- | --- | --- | --- |",
        ]
    )
    for row in rows:
        label = "通过" if row["ok"] else "失败"
        category_key = "pass" if row["ok"] else (row.get("category") or run_core.category_of(row))
        category = run_core.CATEGORY_LABEL.get(category_key, category_key)
        reason = "" if row["ok"] else run_core.safe(row.get("fail_reason") or row.get("reason") or "")
        lines.append(
            f"| {row['task_id']} | {label} | {row['duration_ms']} ms | {row['steps']} | {category} | {reason} |"
        )
    lines.append("")
    text = "\n".join(lines)
    for secret in smoke._secret_values:
        if secret and secret in text:
            raise SystemExit("report contained a secret; not written")
    lock_path = smoke.RUN_ROOT / "report.lock"
    with lock_path.open("a", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        existing = smoke.REPORT.read_text(encoding="utf-8")
        smoke.REPORT.write_text(smoke.replace_marked_section(existing, MARKER, text), encoding="utf-8")
        fcntl.flock(lock, fcntl.LOCK_UN)


def self_check() -> None:
    rows = load_rows()
    by_source = {row["source_id"]: row for row in rows}
    for source_id in SELF_CHECK_IDS:
        if source_id not in by_source:
            raise SystemExit(f"fixture missing Mbpp/{source_id}")
    if not CANONICAL.exists():
        print("self-check ok (no canonical json, skipped grade)")
        return
    official = {row["task_id"]: row for row in json.loads(CANONICAL.read_text(encoding="utf-8"))}
    for source_id in SELF_CHECK_IDS:
        item = by_source[source_id]
        canon = official[source_id]
        work, _home = smoke.setup_task(f"mbpp-canonical-{source_id}")
        (work / "solution.py").write_text(canon["code"], encoding="utf-8")
        ok, reason = grade_mbpp(work, item)
        if not ok:
            raise SystemExit(f"canonical grade failed Mbpp/{source_id}: {reason}")
    stub = by_source[2]
    work, _home = smoke.setup_task("mbpp-stub-2")
    (work / "solution.py").write_text(stub_source(stub), encoding="utf-8")
    ok, reason = grade_mbpp(work, stub)
    if ok:
        raise SystemExit("stub for Mbpp/2 unexpectedly passed")
    if "Error" not in reason and "assert" not in reason.lower() and "exit_" not in reason:
        raise SystemExit(f"stub failure reason not classified: {reason}")
    print("self-check ok")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-check", action="store_true")
    parser.add_argument("--no-compile", action="store_true")
    parser.add_argument("--limit", type=int, default=0)
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return
    secrets = smoke.require_secrets()
    model, _model_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL", smoke.DEFAULT_MODEL)
    base, _base_source = smoke.nonsecret("SIMPLEBASE_LLM_PROVIDER_BASE_URL", smoke.DEFAULT_BASE)
    host = base.split("//", 1)[-1].split("/", 1)[0]
    rows_spec = load_rows()
    if args.limit:
        if args.limit < 1:
            raise SystemExit("--limit must be positive")
        rows_spec = rows_spec[: args.limit]
    done = load_done()
    prefix_ids = [item["task_id"] for item in load_rows()[:100]]
    reused_prefix = sum(1 for task_id in prefix_ids if task_id in done)
    if not args.no_compile:
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
        if ordered:
            write_section(
                {
                    "model": model,
                    "base_host": host,
                    "wall_s": int(time.monotonic() - started),
                    "reused_prefix": reused_prefix,
                },
                ordered,
                preliminary=bool(args.limit) and len(rows_spec) < FULL_N,
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
