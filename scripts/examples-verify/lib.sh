#!/usr/bin/env bash
# scripts/examples-verify/lib.sh — 逐站验证 harness 公共函数（planv6.4 §4.2/§4.6）
#
# 由 verify.sh 与 smoke/*.sh source，不直接执行。兼容 macOS 自带 bash 3.2：
# 不使用关联数组、mapfile、${var,,} 等 bash 4 特性。
#
# 调用方约定（verify.sh 会 export）：
#   SV_RUN        本次运行目录（/tmp/sb-verify/harness/<RUN_ID>）
#   SV_LOG_DIR    日志目录（默认 $SV_RUN/logs）
#   SV_RESULTS    results.jsonl 路径
#   SV_CASE       当前站点（shop/community/iot-telemetry/ops-assistant）
#   SV_ENV        当前环境（A=本地实例 / B=云端重放实例）
#   SV_SUITE      suite 名（skill / bootstrap / smoke）
#   SV_REPO       仓库根
#   SV_BIN        simplebase 二进制
# 记录 schema：{suite,case,env,step,ok,ms,detail,status}，每步一行（jsonl）。
#   ok=true 覆盖 pass 与 skip；status 用 pass|skip|fail 区分三态（skip 不计 FAIL）。

now_ms() {
  python3 -c 'import time; print(int(time.time() * 1000))'
}

# redact：把密钥类文本替换成 [redacted]，并截断到 900 字符。
# 所有写到终端 / jsonl / report 的命令输出都要先过这里。
redact() {
  # 用 -c 而不是 `python3 - <<'PY'`：heredoc 会顶掉管道 stdin，导致读不到待脱敏文本。
  python3 -c '
import re
import sys

s = sys.stdin.read()
s = re.sub(r"(?i)(secret|token|password|api_key|authorization|refresh_token|readonly_key)(\"?[:=]\s*\"?)[^\s\",}]+",
           r"\1\2[redacted]", s)
s = re.sub(r"sb_live_[A-Za-z0-9_\-]+", "sb_live_[redacted]", s)
s = re.sub(r"eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+", "[jwt]", s)
sys.stdout.write(s[:900])
'
}

# log <message...>：带时间戳打到 stderr（stdout 留给 report 相关输出）。
log() {
  printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2
}

# record <step> <pass|skip|fail> <ms> <detail>
record() {
  local step="$1" status="$2" ms="$3" detail="${4:-}"
  local file="${SV_RESULTS:-}"
  [ -n "$file" ] || return 0
  local clean
  clean="$(printf '%s' "$detail" | redact)"
  SV_STEP="$step" SV_STATUS="$status" SV_MS="$ms" SV_DETAIL="$clean" \
    SV_CASE="${SV_CASE:-}" SV_ENV="${SV_ENV:-}" SV_SUITE="${SV_SUITE:-skill}" SV_FILE="$file" \
    python3 - <<'PY'
import json
import os

status = os.environ["SV_STATUS"]
row = {
    "suite": os.environ.get("SV_SUITE") or "skill",
    "case": os.environ.get("SV_CASE") or "",
    "env": os.environ.get("SV_ENV") or "",
    "step": os.environ["SV_STEP"],
    "ok": status != "fail",
    "ms": int(os.environ.get("SV_MS") or "0"),
    "detail": os.environ.get("SV_DETAIL") or "",
    "status": status,
}
with open(os.environ["SV_FILE"], "a", encoding="utf-8") as handle:
    handle.write(json.dumps(row, ensure_ascii=False) + "\n")
PY
  local mark="PASS"
  case "$status" in
    skip) mark="SKIP" ;;
    fail) mark="FAIL" ;;
  esac
  local short
  short="$(printf '%s' "$clean" | tr '\n' ' ' | cut -c1-220)"
  printf '%-4s %s/%s %-5s %s %s%s\n' "$mark" "${SV_ENV:-A}" "${SV_CASE:-?}" "$step" "$(printf '%4d' "$ms")ms" "$short" >&2
}

# note <text>：追加一条「skill 文档与实测不一致」/已知缺口观察，汇总进 report.md。
note() {
  [ -n "${SV_RUN:-}" ] || return 0
  printf -- '- %s\n' "$*" | redact >>"$SV_RUN/notes.md"
}

# port_busy <port>：0 = 被监听。
port_busy() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1 && return 0
    return 1
  fi
  python3 - "$port" <<'PY'
import socket
import sys

sock = socket.socket()
try:
    sock.bind(("127.0.0.1", int(sys.argv[1])))
except OSError:
    sys.exit(0)
finally:
    sock.close()
sys.exit(1)
PY
}

# wait_health <base-url> <timeout-seconds>：/health/ready == 200 返回 0。
wait_health() {
  local url="$1" timeout="$2" deadline code
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 2 "$url/health/ready" 2>/dev/null || true)"
    [ "$code" = "200" ] && return 0
    sleep 0.25
  done
  return 1
}

# wait_http <url> <timeout-seconds>：任意 2xx/3xx/4xx 说明端口已监听（用于 BFF /health）。
wait_http() {
  local url="$1" timeout="$2" deadline code
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 2 "$url" 2>/dev/null || true)"
    case "$code" in
      2??|3??|4??) return 0 ;;
    esac
    sleep 0.25
  done
  return 1
}

# http_code <method> <url> [curl args...]
http_code() {
  local method="$1" url="$2"
  shift 2
  curl -sS -o /dev/null -w '%{http_code}' -X "$method" --max-time 20 "$@" "$url" 2>/dev/null || true
}

# http_body <method> <url> [curl args...]：stdout 输出 body。
http_body() {
  local method="$1" url="$2"
  shift 2
  curl -sS -X "$method" --max-time 20 "$@" "$url" 2>/dev/null || true
}

# clean_sb_env <command...>：清掉从仓库 .env 可能残留的 SIMPLEBASE_* 连接变量，
# 避免进程环境覆盖工作区 .env（两套入口都按 --env-file > 进程环境 > 工作区 .env 取凭据）。
clean_sb_env() {
  env -u SIMPLEBASE_URL -u SIMPLEBASE_API_KEY -u SIMPLEBASE_TOKEN \
    -u SIMPLEBASE_PROJECT_ID -u SIMPLEBASE_DATABASE_ID -u SIMPLEBASE_READONLY_KEY \
    -u SIMPLEBASE_API_KEY_ID -u SIMPLEBASE_READONLY_KEY_ID -u SIMPLEBASE_BIN "$@"
}

# json_get <file> <jq-filter> [jq args...]：取 JSON 文件里的值（失败返回空）。
json_get() {
  local file="$1" filter="$2"
  shift 2
  jq -r "$@" "$filter" "$file" 2>/dev/null | head -c 4000
}

# json_last_ok <file>：最后一行 JSON 信封的 .ok（true/false/空）。
json_last_ok() {
  python3 - "$1" <<'PY'
import json
import sys

try:
    lines = [ln for ln in open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines() if ln.strip()]
except OSError:
    sys.exit(0)
if not lines:
    sys.exit(0)
try:
    data = json.loads(lines[-1])
except Exception:
    sys.exit(0)
print("true" if data.get("ok") is True else "false")
PY
}

# json_last_step_count <file>：data.actions 里非 skip 的动作数（数值，失败为空）。
# 兼容 sbdev 的易读输出：`^  (create|skip|update|apply) ` 行也计入。
action_summary() {
  # action_summary <file> <sbdev|cli>
  python3 - "$1" "$2" <<'PY'
import json
import re
import sys

path, kind = sys.argv[1], sys.argv[2]
lines = []
try:
    lines = [ln for ln in open(path, encoding="utf-8", errors="replace").read().splitlines() if ln.strip()]
except OSError:
    pass
actions = []
if kind == "cli":
    payload = None
    for ln in reversed(lines):
        try:
            payload = json.loads(ln)
            break
        except Exception:
            continue
    data = (payload or {}).get("data") or {}
    for item in data.get("actions") or []:
        actions.append({"verb": item.get("verb") or "", "kind": item.get("kind") or "", "name": item.get("name") or ""})
else:
    pattern = re.compile(r"^\s{2}(create|skip|update|apply)\s+([a-z]+)\s+(.+?)(?:\s{2}\(.*\))?$")
    for ln in lines:
        match = pattern.match(ln)
        if match:
            actions.append({"verb": match.group(1), "kind": match.group(2), "name": match.group(3).strip()})
print(json.dumps(actions, ensure_ascii=False))
PY
}

# action_count_changes <actions-json>：非 skip 数目。
action_count_changes() {
  python3 - "$1" <<'PY'
import json
import sys

try:
    actions = json.loads(sys.argv[1])
except Exception:
    actions = []
print(sum(1 for a in actions if a.get("verb") != "skip"))
PY
}

# error_detail <stderr-file> [stdout-file]：提取 error.code: message（CLI JSON 或 sbdev 文本）。
error_detail() {
  python3 - "$1" "${2:-}" <<'PY'
import json
import re
import sys

parts = []
for path in sys.argv[1:]:
    if not path:
        continue
    try:
        text = open(path, encoding="utf-8", errors="replace").read()
    except OSError:
        continue
    for line in reversed([ln for ln in text.splitlines() if ln.strip()]):
        try:
            payload = json.loads(line)
        except Exception:
            continue
        err = payload.get("error")
        if isinstance(err, dict):
            parts.append("%s: %s" % (err.get("code") or "error", (err.get("message") or "")[:300]))
            break
        break
    if parts:
        break
if not parts:
    # sbdev / 普通文本：取最后一条含 error code 的行。
    for path in sys.argv[1:]:
        if not path:
            continue
        try:
            text = open(path, encoding="utf-8", errors="replace").read()
        except OSError:
            continue
        lines = [ln.strip() for ln in text.splitlines() if ln.strip()]
        if not lines:
            continue
        last = lines[-1]
        match = re.search(r"(?:sbdev:|error:)\s*([a-zA-Z_]+):\s*(.*)", last)
        if match:
            parts.append("%s: %s" % (match.group(1), match.group(2)[:300]))
        else:
            parts.append(last[:300])
        break
print(parts[0] if parts else "")
PY
}

# last_json_error_code <file>：最后一行 JSON 信封的 error.code。
last_json_error_code() {
  python3 - "$1" <<'PY'
import json
import sys

try:
    lines = [ln for ln in open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines() if ln.strip()]
except OSError:
    sys.exit(0)
for line in reversed(lines):
    try:
        payload = json.loads(line)
    except Exception:
        continue
    err = payload.get("error") or {}
    print(err.get("code") or "")
    break
PY
}

# log_tail <file> [lines]：脱敏后的日志尾部（用于失败详情）。
log_tail() {
  local file="$1" lines="${2:-3}"
  [ -f "$file" ] || return 0
  tail -n "$lines" "$file" | redact | tr '\n' ' ' | cut -c1-500
}

# require_cmd <name...>
require_cmd() {
  local name
  for name in "$@"; do
    command -v "$name" >/dev/null 2>&1 || {
      echo "examples-verify: 缺少命令 $name" >&2
      return 1
    }
  done
}

# ── 冒烟脚本共用 ─────────────────────────────────────────
# smoke 脚本约定：
#   - 需要进程环境已具备 SIMPLEBASE_URL / SIMPLEBASE_API_KEY / SIMPLEBASE_PROJECT_ID
#     以及 SIMPLEBASE_DATABASE_ID（或 SIMPLEBASE_DATABASE_ID_<NAME>）。
#   - shop / community / ops-assistant 用 SMOKE_BFF_URL（也可用 $1 覆盖）；
#     iot-telemetry 用 SMOKE_WORK 指向工作区副本（$1 覆盖）。
#   - SMOKE_STEP / SMOKE_ENV 决定记录到 results.jsonl 的步骤名与环境。
SMOKE_PASS=0
SMOKE_FAIL=0
SMOKE_DETAIL=""

smoke_start() {
  SMOKE_PASS=0
  SMOKE_FAIL=0
  SMOKE_DETAIL=""
}

# smoke_check <name> <exit-code> [detail]
smoke_check() {
  local name="$1" code="$2" detail="${3:-}"
  if [ "$code" -eq 0 ]; then
    SMOKE_PASS=$((SMOKE_PASS + 1))
    printf 'PASS %s\n' "$name"
  else
    SMOKE_FAIL=$((SMOKE_FAIL + 1))
    SMOKE_DETAIL="${SMOKE_DETAIL}FAIL ${name}${detail:+ (${detail})}; "
    printf 'FAIL %s%s\n' "$name" "${detail:+ ($detail)}" >&2
  fi
}

# smoke_finish [额外说明]
smoke_finish() {
  local extra="${1:-}"
  local status="pass" detail
  if [ "$SMOKE_FAIL" -gt 0 ]; then
    status="fail"
  fi
  detail="${SMOKE_DETAIL}${SMOKE_PASS} assertions passed"
  if [ -n "$extra" ]; then
    detail="${detail}; ${extra}"
  fi
  record "${SMOKE_STEP:-S8}" "$status" "${SMOKE_MS:-0}" "$detail"
  # 冒烟脚本总耗时由 verify.sh 计算后单独记录：这里只把断言结果写进结果文件。
  [ "$SMOKE_FAIL" -eq 0 ]
}

# ── report.md ────────────────────────────────────────────
# generate_report <run-dir> <results-jsonl> <notes-file> <meta-env-file>
# meta-env-file 是 KEY=VALUE 行，键：RUN_ID、REPO、BIN、A_URL、B_URL、PORTS、
#   FLAGS、STARTED_AT、ENDED_AT、GIT_REV、A_DATA、B_DATA。
generate_report() {
  local run="$1" results="$2" notes="$3" meta="$4"
  SV_REPORT_RUN="$run" SV_REPORT_RESULTS="$results" SV_REPORT_NOTES="$notes" SV_REPORT_META="$meta" \
    python3 - <<'PY'
import json
import os
import re
from datetime import datetime

run = os.environ["SV_REPORT_RUN"]
results_path = os.environ["SV_REPORT_RESULTS"]
notes_path = os.environ["SV_REPORT_NOTES"]

meta = {}
try:
    for line in open(os.environ["SV_REPORT_META"], encoding="utf-8"):
        if "=" in line:
            key, value = line.rstrip("\n").split("=", 1)
            meta[key] = value
except OSError:
    pass

rows = []
try:
    for line in open(results_path, encoding="utf-8"):
        line = line.strip()
        if not line:
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError:
            continue
except OSError:
    pass

step_order = ["S0", "S1", "S2", "S3", "S4", "S5", "S6", "S6b", "S7", "S8", "S8-llm",
              "S9", "S10", "S11", "S12"]
case_order = [c for c in (meta.get("CASES") or "").split(",") if c]
seen_cases = []
for row in rows:
    name = row.get("case") or ""
    if name and name not in seen_cases:
        seen_cases.append(name)
for name in seen_cases:
    if name not in case_order:
        case_order.append(name)
extra_steps = []
for row in rows:
    step = row.get("step") or ""
    if step and step not in step_order and step not in extra_steps:
        extra_steps.append(step)

W = "\u2705"   # pass
K = "\u23ed"   # skip
F = "\u274c"   # fail
D = "\u2014"   # not run

merged = {}
order_keys = []
for row in rows:
    key = (row.get("case") or "", row.get("env") or "", row.get("step") or "")
    item = merged.get(key)
    if item is None:
        merged[key] = {
            "statuses": [row.get("status") or ("pass" if row.get("ok") else "fail")],
            "details": [row.get("detail") or ""],
            "ms": int(row.get("ms") or 0),
        }
        order_keys.append(key)
    else:
        item["statuses"].append(row.get("status") or ("pass" if row.get("ok") else "fail"))
        if row.get("detail"):
            item["details"].append(row["detail"])
        item["ms"] += int(row.get("ms") or 0)

RANK = {"pass": 0, "skip": 1, "fail": 2}
SYMBOL = {"pass": W, "skip": K, "fail": F}


def merged_status(item):
    return max(item["statuses"], key=lambda s: RANK.get(s, 2))


def merged_detail(item):
    out = []
    for detail in item["details"]:
        if detail and detail not in out:
            out.append(detail)
    return "; ".join(out)


envs = []
for (case, env, step) in order_keys:
    if env not in envs:
        envs.append(env)
if not envs:
    envs = ["A"]

# 判定：FAIL 优先；有 skip 则 PARTIAL；全 pass 则 PASS。
verdict = {}
for case in case_order:
    statuses = []
    for (c, env, step), item in merged.items():
        if c == case:
            statuses.extend(item["statuses"])
    if not statuses:
        verdict[case] = "—"
    elif "fail" in statuses:
        verdict[case] = "FAIL"
    elif "skip" in statuses:
        verdict[case] = "PARTIAL"
    else:
        verdict[case] = "PASS"

lines = []
lines.append("# examples / simplebase skill 逐站验证报告")
lines.append("")
lines.append("> 自动生成（`scripts/examples-verify/verify.sh`，planv6.4 §4）。"
             "判定口径：PASS = 已执行步骤全过；PARTIAL = 无 FAIL 但有 SKIP（LLM/沙箱/CLI 技能缺失或已知缺口，"
             "附缺口编号）；FAIL = 任一步失败。任一站 FAIL → 退出码 1；仅 PARTIAL → 退出码 0 并警告。")
lines.append("")
lines.append("| 项 | 值 |")
lines.append("|---|---|")
for key, label in (("RUN_ID", "Run ID"), ("STARTED_AT", "开始"), ("ENDED_AT", "结束"),
                   ("REPO", "仓库"), ("GIT_REV", "git"), ("BIN", "二进制"),
                   ("PORTS", "实例端口"), ("A_DATA", "A 数据目录"), ("B_DATA", "B 数据目录"),
                   ("FLAGS", "参数"), ("RESULT", "结果文件")):
    value = meta.get(key, "")
    if value:
        lines.append("| %s | %s |" % (label, value))
lines.append("| 汇总 | %s |" % ", ".join("%s: %s" % (c, verdict[c]) for c in case_order))
lines.append("")

lines.append("## 总表（站点 × 步骤）")
lines.append("")
header = "| 站点 | 环境 | " + " | ".join(step_order + extra_steps) + " | 判定 |"
lines.append(header)
lines.append("|" + "---|" * (len(step_order) + len(extra_steps) + 3))
for case in case_order:
    case_verdict = verdict.get(case, "—")
    for env in envs:
        cells = []
        for step in step_order + extra_steps:
            item = merged.get((case, env, step))
            cells.append(SYMBOL[merged_status(item)] if item else D)
        lines.append("| %s | %s | %s | %s |" % (case, env, " | ".join(cells), case_verdict))
lines.append("")

lines.append("图例：%s pass，%s skip（不计 FAIL），%s fail，%s 未执行。" % (W, K, F, D))
lines.append("")

fail_rows = [(k, v) for k, v in merged.items() if merged_status(v) == "fail"]
skip_rows = [(k, v) for k, v in merged.items() if merged_status(v) == "skip"]
lines.append("## 失败明细")
lines.append("")
if fail_rows:
    lines.append("| 站点 | 环境 | 步骤 | error.code / 原因 |")
    lines.append("|---|---|---|---|")
    for key, item in fail_rows:
        case, env, step = key
        detail = merged_detail(item) or "(no detail)"
        code = ""
        match = re.match(r"([a-zA-Z_]+):", detail)
        if match:
            code = match.group(1)
        lines.append("| %s | %s | %s | %s%s |" % (case, env, step, ("`%s` " % code) if code else "", detail))
    lines.append("")
else:
    lines.append("无 FAIL。")
    lines.append("")

lines.append("## SKIP 明细（不计 FAIL）")
lines.append("")
if skip_rows:
    lines.append("| 站点 | 环境 | 步骤 | 原因 |")
    lines.append("|---|---|---|---|")
    for key, item in skip_rows:
        case, env, step = key
        lines.append("| %s | %s | %s | %s |" % (case, env, step, merged_detail(item)))
    lines.append("")
else:
    lines.append("无 SKIP。")
    lines.append("")

lines.append("## 步骤耗时（ms）")
lines.append("")
lines.append("| 站点 | 环境 | " + " | ".join(step_order + extra_steps) + " |")
lines.append("|" + "---|" * (len(step_order) + len(extra_steps) + 2))
for case in case_order:
    for env in envs:
        cells = []
        for step in step_order + extra_steps:
            item = merged.get((case, env, step))
            cells.append(str(item["ms"]) if item else D)
        lines.append("| %s | %s | %s |" % (case, env, " | ".join(cells)))
lines.append("")

lines.append("## 证据文件")
lines.append("")
lines.append("```text")
lines.append(run)
lines.append("  results.jsonl      每步一行（schema: suite,case,env,step,ok,ms,detail,status）")
lines.append("  report.md          本文件")
lines.append("  logs/              每个命令的 stdout/stderr（已脱敏，不含 Key）")
lines.append("  work/<case>/       临时工作区副本（.simplebase/、.env* 都只落在副本里）")
lines.append("```")
lines.append("")
docs = []
if os.path.isfile(notes_path):
    for line in open(notes_path, encoding="utf-8"):
        line = line.rstrip("\n")
        if line.startswith("- "):
            docs.append(line[2:])
lines.append("## skill 文档与实测不一致 / 已知缺口")
lines.append("")
if docs:
    for doc in docs:
        lines.append("- " + doc)
else:
    lines.append("- （无记录）")
lines.append("")

with open(os.path.join(run, "report.md"), "w", encoding="utf-8") as handle:
    handle.write("\n".join(lines) + "\n")
print(os.path.join(run, "report.md"))
PY
}

# ── 冒烟脚本 HTTP 辅助（smoke/*.sh 共用） ─────────────────
SMOKE_TMP=""
SMOKE_BODY_FILE=""

smoke_init_tmp() {
  SMOKE_TMP="$(mktemp -d "${TMPDIR:-/tmp}/sb-smoke.XXXXXX")"
  SMOKE_BODY_FILE="$SMOKE_TMP/body"
  trap 'rm -rf "$SMOKE_TMP"' EXIT
}

# sb_request <method> <url> [json-body] [api-key] [额外 curl 参数...]
# stdout 输出 HTTP 状态码，响应 body 落在 ${SMOKE_BODY_FILE}（用 sb_body 读）。
sb_request() {
  local method="$1" url="$2"
  local body="${3:-}" key="${4:-${SIMPLEBASE_API_KEY:-}}"
  local total=$#
  local shiftn=$(( total < 4 ? total : 4 ))
  shift "$shiftn"
  local -a args=(-sS -o "$SMOKE_BODY_FILE" -w '%{http_code}' -X "$method" "$url")
  [ -n "$key" ] && args+=(-H "Authorization: Bearer $key")
  [ -n "$body" ] && args+=(-H 'content-type: application/json' --data "$body")
  args+=("$@")
  curl "${args[@]}"
}

sb_body() {
  cat "$SMOKE_BODY_FILE" 2>/dev/null || true
}

# sql_json <database-id> <statement>：返回查询响应 JSON（body）。
sql_json() {
  local database="$1" statement="$2"
  local payload
  payload="$(jq -cn --arg sql "$statement" '{sql:$sql}')"
  sb_request POST "${SIMPLEBASE_URL%/}/v1/projects/$SIMPLEBASE_PROJECT_ID/databases/$database/query" "$payload" >/dev/null
  sb_body
}

# sql_scalar <database-id> <statement>：取第一行第一列。
sql_scalar() {
  sql_json "$1" "$2" | jq -r '.rows[0][0] // empty' 2>/dev/null
}
