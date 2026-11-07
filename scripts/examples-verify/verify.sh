#!/usr/bin/env bash
# scripts/examples-verify/verify.sh — examples / simplebase skill 逐站验证入口（planv6.4 §4）
#
# 做什么：起 A（本地）与 B（模拟云端）两个实例，按 skill 的工作流把 shop / community /
# iot-telemetry / ops-assistant 四个站点从零跑一遍（S0–S12），再只换 URL + Key 在 B 上重放与
# 业务冒烟，最后输出站点 × 步骤矩阵 report.md。
#
# 设计边界：
#   - 只写 /tmp/sb-verify/<BASE>/<RUN_ID>/ 与新建的 scripts/examples-verify/**；
#     仓库内 examples/**、skills/**、plan/** 与 Go/Vue 代码一律不写（S0 先 cp -R 到副本）。
#   - `sbdev apply` / `simplebase dev apply` 会写 .simplebase/，只能跑在副本里。
#   - Key 只落在副本的 .env*（0600），终端 / jsonl / report 全部过 redact。
#
# 判定（§4.6）：任一站 FAIL → 退出 1；只有 PARTIAL → 退出 0 并警告；
#   LLM / 沙箱 / CLI 技能未启用 → SKIP（写原因），不计 FAIL。
#
# 用法：
#   scripts/examples-verify/verify.sh [--only shop,community] [--skip-cloud]
#       [--with-changes] [--keep] [--ports 18080,18081] [--repo <dir>] [--help]

# 注意：macOS 自带 bash 3.2 在 C.UTF-8 locale 下会把紧跟变量名的中文标点首个字节
# 并进变量名（$A_URL）报 unbound variable）。所以「$变量 + 中文标点」一律写成 ${变量}。
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"

DEFAULT_REPO="$(cd "$SCRIPT_DIR/../.." && pwd)"
REPO="${SB_REPO:-$DEFAULT_REPO}"
BASE="${SB_VERIFY_BASE:-/tmp/sb-verify/harness}"
BFF_BASE="${SB_BFF_BASE:-28100}"

# 默认顺序：community 放最后（当前平台每个项目只允许一个 kind=user 库，
# examples-fixer 正在把 media 并入 community；先跑其它站不阻塞）。
ALL_CASES="shop,iot-telemetry,ops-assistant,community"
ONLY="$ALL_CASES"
SKIP_CLOUD=0
WITH_CHANGES=0
KEEP=0
PORTS_SPEC="18080,18081"
BIN="${SIMPLEBASE_BIN:-}"

usage() {
  cat <<'EOF'
用法: scripts/examples-verify/verify.sh [选项]

  --only <list>      只跑指定站点（逗号分隔）：shop,community,iot-telemetry,ops-assistant
  --skip-cloud       只跑 A 实例的 S0–S8（不要求 B 实例）
  --with-changes     追加 S11（变更回放）/ S12（负向用例），只在 shop、community 上跑
  --keep             保留 /tmp/sb-verify/<RUN_ID> 全部现场与实例/BFF 进程，便于排查
  --ports A,B        实例端口（默认 18080,18081；禁止 8080，本机被 nginx 占用）
  --repo <dir>       仓库根（默认脚本上两级）
  --help             显示本帮助

产物：$RUN/results.jsonl（每步一行）、$RUN/report.md（站点 × 步骤矩阵）、$RUN/logs/。

退出码：0 = 全部 PASS 或仅 PARTIAL；1 = 有站点 FAIL；2 = 用法 / 环境错误。
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --only) [ $# -ge 2 ] || { usage; exit 2; }; ONLY="$2"; shift 2 ;;
    --only=*) ONLY="${1#*=}"; shift ;;
    --skip-cloud) SKIP_CLOUD=1; shift ;;
    --with-changes) WITH_CHANGES=1; shift ;;
    --keep) KEEP=1; shift ;;
    --ports) [ $# -ge 2 ] || { usage; exit 2; }; PORTS_SPEC="$2"; shift 2 ;;
    --ports=*) PORTS_SPEC="${1#*=}"; shift ;;
    --repo) [ $# -ge 2 ] || { usage; exit 2; }; REPO="$2"; shift 2 ;;
    --repo=*) REPO="${1#*=}"; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "examples-verify: 未知参数 $1" >&2; usage; exit 2 ;;
  esac
done

REPO="$(cd "$REPO" 2>/dev/null && pwd)" || { echo "examples-verify: --repo 不存在" >&2; exit 2; }
SK="$REPO/skills/simplebase/scripts/sbdev.py"
[ -n "$BIN" ] || BIN="$REPO/simplebase"

# ── 参数校验 ─────────────────────────────────────────────
A_PORT="${PORTS_SPEC%%,*}"
B_PORT="${PORTS_SPEC##*,}"
if ! [[ "$A_PORT" =~ ^[0-9]+$ ]] || ! [[ "$B_PORT" =~ ^[0-9]+$ ]]; then
  echo "examples-verify: --ports 需要两个数字端口（当前 ${PORTS_SPEC}）" >&2
  exit 2
fi
if [ "$A_PORT" = "$B_PORT" ]; then
  echo "examples-verify: --ports 两个端口不能相同" >&2
  exit 2
fi
if [ "$A_PORT" = "8080" ] || [ "$B_PORT" = "8080" ]; then
  echo "examples-verify: 8080 被本机 nginx 占用，禁止使用；请换 --ports（默认 18080,18081）" >&2
  exit 2
fi

CASES=""
for item in ${ONLY//,/ }; do
  case "$item" in
    shop|community|iot-telemetry|ops-assistant)
      if [ -z "$CASES" ]; then CASES="$item"; else CASES="$CASES,$item"; fi
      ;;
    *) echo "examples-verify: 未知站点 ${item}（可选 shop,community,iot-telemetry,ops-assistant）" >&2; exit 2 ;;
  esac
done
[ -n "$CASES" ] || { echo "examples-verify: --only 为空" >&2; exit 2; }
if [ "$WITH_CHANGES" -eq 1 ] && [ "$SKIP_CLOUD" -eq 1 ]; then
  echo "examples-verify: --with-changes 需要 B 实例（S11 要在云端 plan/apply），不能与 --skip-cloud 同用" >&2
  exit 2
fi

require_cmd python3 node jq curl || exit 2
command -v go >/dev/null 2>&1 || true   # community / ops-assistant 的 Go BFF 需要；用到时会显式报错
[ -x "$BIN" ] || { echo "examples-verify: 找不到可执行二进制 ${BIN}（先 ./build.sh，或用 SIMPLEBASE_BIN 指定）" >&2; exit 2; }
[ -f "$REPO/.env" ] || { echo "examples-verify: 缺少 $REPO/.env（服务端需要 SIMPLEBASE_AUTH_APIKEY_SECRET）" >&2; exit 2; }
[ -f "$SK" ] || { echo "examples-verify: 缺少 $SK" >&2; exit 2; }

# ── 运行目录 ─────────────────────────────────────────────
RUN_ID="${SB_RUN_ID:-$(date +%Y%m%d-%H%M%S)-$$}"
RUN="$BASE/$RUN_ID"
mkdir -p "$RUN/logs" "$RUN/work" "$RUN/bin"
RESULTS="$RUN/results.jsonl"
NOTES="$RUN/notes.md"
META="$RUN/meta.env"
: >"$RESULTS"
: >"$NOTES"
STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# 供 lib.sh / bootstrap.mjs / smoke 共用
export SV_RUN="$RUN" SV_LOG_DIR="$RUN/logs" SV_RESULTS="$RESULTS" SV_REPO="$REPO" SV_BIN="$BIN"
SV_CASE=""
SV_ENV="A"
SV_SUITE="skill"
export SV_CASE SV_ENV SV_SUITE

A_URL="http://127.0.0.1:$A_PORT"
# B 故意写成末尾带点的 localhost.：sbdev / simplebase dev 的 local 判定按 host 字符串精确匹配
# （{127.0.0.1,localhost,::1,0.0.0.0}），localhost. 解析到 127.0.0.1 但会走 remote 分支，
# 这样 apply 无 --yes 才会返回退出码 3（planv6.4 §4.3 S9；report.md 里也会写明）。
B_URL="http://localhost.:$B_PORT"
B_URL_REQUESTS="http://127.0.0.1:$B_PORT"   # 健康检查 / BFF 用普通地址即可

A_PID=""
B_PID=""
BFF_PID=""
BFF_PORT_ACTIVE=""
REPORT_DONE=0
EXIT_CODE=0
SUMMARY_CODE=0
CURRENT_CASE=""
SB_NOTE_B_SEED=0

log "run dir: $RUN"

# ── 静态「文档与实测不一致」观察（会汇总进 report.md） ──
note "S9 的远端判定只看 URL host 字符串：sbdev.py（LOCAL_HOSTS）与 simplebase dev（dev.go devLocalHosts）都把 127.0.0.1 / localhost / ::1 / 0.0.0.0 判成本地；要验证「远端 apply 不带 --yes 退出 3」，B 实例 URL 用 http://localhost.:<port>（末尾带点，解析到 127.0.0.1 但走 remote 分支）。SKILL.md 只写了退出码 3，没有写 127.0.0.1 会被判 local 这个差别。"
note "发布 Key 权限不是层级继承：list/query 校验 database:read、sql exec/batch 校验 database:write、建库校验 database:admin（auth.Principal.HasPermission 精确匹配），所以只给 database:admin 的 Key 跑不了 sbdev apply；bootstrap 签发的发布 Key 同时带 admin/read/write 三个权限。"
note "examples/lib/init/run.mjs 的对象上传对 .csv 声明 text/csv：/s3/objects 的 sniffUploadMIME 会把 CSV 嗅探成 text/plain，声明与嗅探不一致 → 415 mime_mismatch（iot-telemetry 的 seed/exports/readings.csv 会命中）；harness 的 bootstrap 统一用 application/octet-stream 规避。"
note "LLM 相关：harness 默认不向实例注入 SIMPLEBASE_LLM_PROVIDER_*（SB_KEEP_LLM=1 可保留仓库 .env 的 provider 配置），所以 requires.llm 的 community / ops-assistant 记 S8-llm SKIP（助手与助手定时未验证），community 的 summary 断言走「无 provider → 501」分支；按 planv6.4 §4.4 这是可判定口径，不计 FAIL。"
note "iot-telemetry 的脚本（curl.sh / simulate.mjs / worker.mjs）按相对路径从工作区副本启动：Node 会把相对 argv[1] 规范成 realpath；若用绝对路径（如 /tmp/...），macOS 的 /tmp→/private/tmp 符号链接会让 worker.mjs 末尾的 import.meta.url 判定为 false 而静默不执行（README 的相对示例本身没问题，仅记录启动口径）。"
note ".simplebase/env.<label>：label 取自 URL hostname（本地目标固定 local，远程是 hostname 规整后的字符串）；单库才写不带后缀的 SIMPLEBASE_DATABASE_ID，多库只有 SIMPLEBASE_DATABASE_ID_<NAME> 后缀行——bootstrap 写 .env.runtime/.env.cloud.runtime 时遵循同一规则。"

log "A=$A_URL  B=$B_URL  skip-cloud=$SKIP_CLOUD  with-changes=$WITH_CHANGES  cases=$CASES"

# ── 进程管理 ─────────────────────────────────────────────
stop_pid() {
  local pid="$1"
  [ -n "$pid" ] || return 0
  kill -0 "$pid" 2>/dev/null || return 0
  kill -TERM "$pid" 2>/dev/null || true
  local index=0
  while kill -0 "$pid" 2>/dev/null && [ "$index" -lt 50 ]; do
    sleep 0.1
    index=$((index + 1))
  done
  kill -0 "$pid" 2>/dev/null && kill -KILL "$pid" 2>/dev/null || true
  return 0
}

stop_bff() {
  if [ -n "$BFF_PID" ]; then
    stop_pid "$BFF_PID"
    log "停止 BFF（pid=${BFF_PID}）"
    BFF_PID=""
    BFF_PORT_ACTIVE=""
  fi
}

stop_instance() { # stop_instance A|B
  local name="$1"
  if [ "$name" = "A" ] && [ -n "$A_PID" ]; then
    stop_pid "$A_PID"
    log "停止实例 A（pid=${A_PID}）"
    A_PID=""
  elif [ "$name" = "B" ] && [ -n "$B_PID" ]; then
    stop_pid "$B_PID"
    log "停止实例 B（pid=${B_PID}）"
    B_PID=""
  fi
}

start_instance() { # start_instance <A|B> <port> <data-dir> <cache-dir> <log-file>
  local name="$1" port="$2" datadir="$3" cachedir="$4" logfile="$5" pid
  (
    set -a
    # shellcheck disable=SC1091
    . "$REPO/.env"
    set +a
    export SIMPLEBASE_HTTP_ADDRESS="127.0.0.1:$port"
    export SIMPLEBASE_DATA_DIR="$datadir"
    export SIMPLEBASE_DB_CACHE_DIR="$cachedir"
    export SIMPLEBASE_INSTANCE_ID="sb-verify-$(printf '%s' "$name" | tr 'A-Z' 'a-z')"
    # 默认不注入 LLM provider：plan §4.4 要求「无 provider 时 summary 返回 501」可判定，
    # 也让 requires.llm 的站点走明确 skip。SB_KEEP_LLM=1 可保留仓库 .env 里的 provider 配置。
    if [ "${SB_KEEP_LLM:-0}" != "1" ]; then
      unset SIMPLEBASE_LLM_PROVIDER_API_KEY SIMPLEBASE_LLM_PROVIDER_BASE_URL \
        SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL SIMPLEBASE_LLM_PROVIDER
    fi
    exec "$BIN"
  ) >"$logfile" 2>&1 &
  pid=$!
  if [ "$name" = "A" ]; then A_PID=$pid; else B_PID=$pid; fi
  log "实例 $name 启动：pid=$pid port=$port data=$datadir"
}

cleanup() {
  local code=$?
  trap - EXIT INT TERM
  stop_bff
  if [ "$KEEP" -eq 1 ]; then
    [ -n "$A_PID" ] && log "--keep：实例 A 保持运行 pid=${A_PID}（停止：kill ${A_PID}）"
    [ -n "$B_PID" ] && log "--keep：实例 B 保持运行 pid=${B_PID}（停止：kill ${B_PID}）"
  else
    stop_instance B
    stop_instance A
    # 副本里有 .env* 密钥，默认运行不保留；report/results/logs 保留。
    rm -rf "$RUN/work" "$RUN/data-a" "$RUN/cache-a" "$RUN/data-b" "$RUN/cache-b" "$RUN/bin" "$RUN/packages"
  fi
  if [ "$REPORT_DONE" -eq 0 ]; then
    local original="$code"
    finalize_report "$code"
    if [ "$SUMMARY_CODE" = "1" ]; then
      code=1
    else
      code="$original"
    fi
  fi
  exit "$code"
}
trap cleanup EXIT INT TERM

# ── 步骤记录 ─────────────────────────────────────────────
STEP_NAME=""
STEP_START=0
STEP_STATUS="pass"
STEP_DETAIL=""

begin_step() {
  STEP_NAME="$1"
  STEP_START="$(now_ms)"
  STEP_STATUS="pass"
  STEP_DETAIL=""
  SV_SUITE="skill"
}

step_fail() {
  STEP_STATUS="fail"
  STEP_DETAIL="${STEP_DETAIL:+$STEP_DETAIL; }$1"
}

step_skip() {
  [ "$STEP_STATUS" = "fail" ] || STEP_STATUS="skip"
  STEP_DETAIL="${STEP_DETAIL:+$STEP_DETAIL; }$1"
}

step_ok_detail() {
  STEP_DETAIL="${STEP_DETAIL:+$STEP_DETAIL; }$1"
}

end_step() {
  record "$STEP_NAME" "$STEP_STATUS" $(( $(now_ms) - STEP_START )) "$STEP_DETAIL"
}

# ── 通用执行封装 ─────────────────────────────────────────
SBDEV_CODE=0
run_sbdev() { # run_sbdev <log-stem> <args...>
  local stem="$1"
  shift
  ( clean_sb_env SIMPLEBASE_BIN="$BIN" python3 "$SK" "$@" >"$RUN/logs/$stem.out" 2>"$RUN/logs/$stem.err" )
  SBDEV_CODE=$?
}

CLI_CODE=0
run_cli() { # run_cli <log-stem> <args...>（A 的工作区 .env）
  local stem="$1"
  shift
  ( set -a; . "$W/.env"; set +a; "$BIN" "$@" >"$RUN/logs/$stem.out" 2>"$RUN/logs/$stem.err" )
  CLI_CODE=$?
}

run_cli_env() { # run_cli_env <log-stem> <env-file> <args...>
  local stem="$1" envfile="$2"
  shift 2
  ( set -a; . "$envfile"; set +a; "$BIN" "$@" >"$RUN/logs/$stem.out" 2>"$RUN/logs/$stem.err" )
  CLI_CODE=$?
}

# 在指定 URL/Key/项目下跑 CLI（S6 交叉验证、S9 远端交叉验证用）。
run_cli_with() { # run_cli_with <log-stem> <url> <env-file-for-key> <args...>
  local stem="$1" url="$2" envfile="$3"
  shift 3
  (
    set -a
    . "$envfile"
    set +a
    export SIMPLEBASE_URL="$url"
    unset SIMPLEBASE_TOKEN
    "$BIN" "$@" >"$RUN/logs/$stem.out" 2>"$RUN/logs/$stem.err"
  )
  CLI_CODE=$?
}

cli_json() { # cli_json <log-stem> <jq-filter>：CLI 信封最后一行的 data 查询
  local stem="$1" filter="$2"
  tail -n 1 "$RUN/logs/$stem.out" 2>/dev/null | jq -r "$filter" 2>/dev/null
}

cli_ok() { # cli_ok <log-stem>
  local stem="$1"
  [ "$CLI_CODE" -eq 0 ] && [ "$(json_last_ok "$RUN/logs/$stem.out")" = "true" ]
}

cli_failure_detail() { # cli_failure_detail <log-stem>
  local stem="$1"
  error_detail "$RUN/logs/$stem.err" "$RUN/logs/$stem.out"
}

actions_of() { # actions_of <log-stem> <sbdev|cli>
  action_summary "$RUN/logs/$1.out" "$2"
}

actions_equal() { # actions_equal <actions-a> <actions-b>：打印 OK 或 DIFF: ...
  python3 - "$1" "$2" <<'PY'
import json
import sys

a = json.loads(sys.argv[1]) if sys.argv[1] else []
b = json.loads(sys.argv[2]) if sys.argv[2] else []
key = lambda x: (x.get("verb"), x.get("kind"), x.get("name"))
if [key(x) for x in a] == [key(x) for x in b]:
    print("OK")
else:
    for i, (x, y) in enumerate(zip(a, b)):
        if key(x) != key(y):
            print("DIFF: #%d plan=%s apply=%s" % (i + 1, "/".join(key(x)), "/".join(key(y))))
            break
    else:
        print("DIFF: 动作数量 plan=%d apply=%d" % (len(a), len(b)))
PY
}

count_actions() { # count_actions <actions-json> <all|changes|skips>
  python3 - "$1" "$2" <<'PY'
import json
import sys

actions = json.loads(sys.argv[1]) if sys.argv[1] else []
mode = sys.argv[2]
if mode == "changes":
    print(sum(1 for a in actions if a.get("verb") != "skip"))
elif mode == "skips":
    print(sum(1 for a in actions if a.get("verb") == "skip"))
else:
    print(len(actions))
PY
}

file_mode() { # file_mode <path>：输出八进制权限（如 600）
  python3 - "$1" <<'PY'
import os
import stat
import sys

try:
    mode = stat.S_IMODE(os.stat(sys.argv[1]).st_mode)
    print("%o" % mode)
except OSError:
    print("missing")
PY
}

check_private_env() { # check_private_env <path> <label>：存在且 0600
  python3 - "$1" <<'PY'
import os
import stat
import sys

path = sys.argv[1]
try:
    mode = stat.S_IMODE(os.stat(path).st_mode)
except OSError:
    print("missing")
    sys.exit(0)
print("ok" if mode & 0o077 == 0 else "mode-%o" % mode)
PY
}

# 校验一份 env 文件可被 sbdev / simplebase 读取。
write_meta() {
  {
    echo "RUN_ID=$RUN_ID"
    echo "STARTED_AT=$STARTED_AT"
    echo "ENDED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "REPO=$REPO"
    echo "GIT_REV=$(git -C "$REPO" rev-parse --short HEAD 2>/dev/null || echo '-')"
    echo "BIN=$BIN ($(stat -f '%z bytes %Sm' -t '%Y-%m-%dT%H:%M:%S' "$BIN" 2>/dev/null || echo '?'))"
    echo "PORTS=A:$A_PORT B:$B_PORT"
    echo "A_DATA=$RUN/data-a"
    echo "B_DATA=$RUN/data-b"
    echo "FLAGS=--only $CASES --skip-cloud=$SKIP_CLOUD --with-changes=$WITH_CHANGES --keep=$KEEP"
    echo "RESULT=$RESULTS"
    echo "CASES=$CASES"
  } >"$META"
}

finalize_report() {
  local code="${1:-0}"
  write_meta
  local report
  report="$(generate_report "$RUN" "$RESULTS" "$NOTES" "$META")"
  log "report: $report"
  # 站点级判定输出 + 退出码
  local summary
  summary="$(python3 - "$RESULTS" "$CASES" <<'PY'
import json
import sys

results = sys.argv[1]
cases = [c for c in sys.argv[2].split(",") if c]
statuses = {}
try:
    lines = open(results, encoding="utf-8").read().splitlines()
except OSError:
    lines = []
for line in lines:
    line = line.strip()
    if not line:
        continue
    try:
        row = json.loads(line)
    except json.JSONDecodeError:
        continue
    case = row.get("case") or ""
    status = row.get("status") or ("pass" if row.get("ok") else "fail")
    if not case:
        continue
    statuses.setdefault(case, []).append(status)
out = []
fail = 0
partial = 0
for case in cases:
    values = statuses.get(case, [])
    if "fail" in values:
        verdict = "FAIL"
        fail += 1
    elif not values:
        verdict = "SKIP(no steps)"
    elif "skip" in values:
        verdict = "PARTIAL"
        partial += 1
    else:
        verdict = "PASS"
    out.append("%s=%s" % (case, verdict))
print("RESULT " + " ".join(out) + " | fail=%d partial=%d" % (fail, partial))
print("EXIT %d" % (1 if fail else 0))
PY
)"
  log "$(printf '%s' "$summary" | head -1)"
  local exit_line
  exit_line="$(printf '%s' "$summary" | tail -1 | awk '{print $2}')"
  if [ "$exit_line" = "1" ]; then
    log "有站点 FAIL：退出码 1"
  elif printf '%s' "$summary" | grep -q 'PARTIAL'; then
    log "仅有 PARTIAL（SKIP 明细见 report.md）：退出码 0，注意告警"
  fi
  SUMMARY_CODE="${exit_line:-0}"
  REPORT_DONE=1
}

# ── 启动前检查 ───────────────────────────────────────────
check_port_free() { # check_port_free <port> <label>
  local port="$1" label="$2"
  if port_busy "$port"; then
    echo "examples-verify: $label 端口 $port 已被占用；换 --ports 或先释放" >&2
    return 1
  fi
  return 0
}

log "检查端口：A=$A_PORT B=$B_PORT"
check_port_free "$A_PORT" "A" || exit 2
if [ "$SKIP_CLOUD" -eq 0 ]; then
  check_port_free "$B_PORT" "B" || exit 2
fi

mkdir -p "$RUN/data-a" "$RUN/cache-a"
log "启动实例 A（${A_URL}）"
start_instance A "$A_PORT" "$RUN/data-a" "$RUN/cache-a" "$RUN/logs/a-server.log"
if ! wait_health "$A_URL" 60; then
  log "实例 A 60s 未就绪：$(log_tail "$RUN/logs/a-server.log" 5)"
  note "实例 A 启动失败（/health/ready 60s 未 200），检查 .env 的 SIMPLEBASE_AUTH_APIKEY_SECRET 与端口"
  exit 2
fi
log "实例 A ready"

if [ "$SKIP_CLOUD" -eq 0 ]; then
  mkdir -p "$RUN/data-b" "$RUN/cache-b"
  log "启动实例 B（${B_URL}）"
  start_instance B "$B_PORT" "$RUN/data-b" "$RUN/cache-b" "$RUN/logs/b-server.log"
  if ! wait_health "$B_URL_REQUESTS" 60; then
    log "实例 B 60s 未就绪：$(log_tail "$RUN/logs/b-server.log" 5)"
    note "实例 B 启动失败（/health/ready 60s 未 200）"
    exit 2
  fi
  log "实例 B ready"
fi

# JS 站点（shop / iot-telemetry）的脚本按 ../../../packages/js-sdk/dist/index.js 找 SDK；
# 副本放到 /tmp 后相对路径落在 $RUN/packages，这里用符号链接对齐（不复制、不改仓库）。
ln -sfn "$REPO/packages" "$RUN/packages"

# ── 站点公共辅助 ─────────────────────────────────────────
bff_offset() { # bff_offset <case>
  case "$1" in
    shop) echo 2 ;;
    community) echo 6 ;;
    ops-assistant) echo 7 ;;
    *) echo 0 ;;
  esac
}

bff_port() { # bff_port <case> <A|B>
  local offset
  offset="$(bff_offset "$1")"
  if [ "$2" = "B" ]; then
    echo $((BFF_BASE + 100 + offset))
  else
    echo $((BFF_BASE + offset))
  fi
}

build_go_bff() { # build_go_bff <case>
  local case="$1" log="$RUN/logs/$case-build.out"
  mkdir -p "$RUN/gocache" "$RUN/gotmp"
  # 用 RUN 下的 GOCACHE/GOTMPDIR：宿主 ~/Library/Caches/go-build 在受限沙箱里不可写，
  # go build 会因 trim cache 失败退出 1（当前 harness 的常见环境，不写进 report 的 skill 偏差）。
  if ( cd "$REPO" && clean_sb_env GOCACHE="$RUN/gocache" GOTMPDIR="$RUN/gotmp" \
      go build -o "$RUN/bin/$case-server" "$W/server/main.go" ) >"$log" 2>&1; then
    log "$case: Go BFF 构建成功（在临时副本上构建，产物在 $RUN/bin）"
    return 0
  fi
  log "$case: Go BFF 构建失败：$(log_tail "$log" 5)"
  return 1
}

start_bff() { # start_bff <case> <A|B> <env-file> <port>
  local case="$1" label="$2" envfile="$3" port="$4"
  local log="$RUN/logs/$case-bff-$label.log"
  local -a cmd
  if port_busy "$port"; then
    log "$case: BFF 端口 $port 已被占用"
    return 1
  fi
  if [ "$case" = "shop" ]; then
    # 必须用相对路径 + cwd=$W：Node 会把相对 argv[1] 规范成 realpath，与 import.meta.url
    # 一致；直接传绝对路径时，macOS /tmp -> /private/tmp 的符号链接会让 main.mjs 末尾的
    # `import.meta.url === new URL(file://argv[1])` 判断为 false，BFF 静默退出不监听。
    cmd=(node server/main.mjs)
  else
    if ! build_go_bff "$case"; then return 1; fi
    cmd=("$RUN/bin/$case-server")
  fi
  (
    set -a
    # shellcheck disable=SC1090
    . "$envfile"
    set +a
    export EXAMPLE_PORT="$port"
    cd "$W" || exit 1
    exec "${cmd[@]}"
  ) >"$log" 2>&1 &
  BFF_PID=$!
  BFF_PORT_ACTIVE="$port"
  if ! wait_http "http://127.0.0.1:$port/health" 25; then
    log "$case: BFF 未就绪：$(log_tail "$log" 5)"
    stop_bff
    return 1
  fi
  log "$case: BFF ready $label http://127.0.0.1:${port}（pid=${BFF_PID}）"
  return 0
}

run_smoke() { # run_smoke <case> <step> <env-label> <runtime-env-file> <bff-url-or-workdir>
  local case="$1" step="$2" envlabel="$3" envfile="$4" target="$5"
  local log="$RUN/logs/$case-smoke-$envlabel.log"
  (
    set -a
    # shellcheck disable=SC1090
    . "$envfile"
    set +a
    export SMOKE_STEP="$step" SMOKE_ENV="$envlabel" SMOKE_WORK="$W" SMOKE_BFF_URL="$target"
    export SV_SUITE="smoke" SV_CASE="$case" SV_ENV="$envlabel"
    bash "$SCRIPT_DIR/smoke/$case.sh" "$target"
  ) >"$log" 2>&1
  SMOKE_CODE=$?
  [ -f "$log" ] && cat "$log" >&2
  return "$SMOKE_CODE"
}

# ═════════════════════════════════════════════════════════
# 单站步骤
# ═════════════════════════════════════════════════════════
run_case() {
  local case="$1"
  CURRENT_CASE="$case"
  W="$RUN/work/$case"
  export W
  SV_CASE="$case"
  SV_ENV="A"
  export SV_CASE SV_ENV
  PROJECT=""
  PLAN_ACTIONS=""
  B_PLAN_ACTIONS=""

  log "──────── $case ────────"

  # ── S0 准备副本 ──────────────────────────────────────
  begin_step S0
  rm -rf "$W"
  if cp -R "$REPO/examples/$case" "$W"; then
    # 仓库里如果有历史 .simplebase/（其它手工验证留下的），cp 会带进副本；必须剔除，
    # 否则 apply 会误以为已经应用过。这里只清理副本，不动仓库（E16）。
    if [ -d "$W/.simplebase" ]; then
      rm -rf "$W/.simplebase"
      step_ok_detail "副本里带入了历史 .simplebase/，已剔除（E16：只在临时副本重放）"
    else
      step_ok_detail "cp -R examples/$case → $W"
    fi
    local gaps
    gaps="$(grep -h '限制：' "$W/README.md" 2>/dev/null | head -1 | sed 's/.*限制：//' | cut -c1-300)"
    [ -n "$gaps" ] && note "$case 已知缺口（README）：$gaps"
  else
    step_fail "cp -R examples/$case 失败"
  fi
  end_step

  # ── S1 离线校验（两端对账） ───────────────────────────
  begin_step S1
  run_sbdev "$case-S1-sbdev" validate -C "$W"
  local sbdev_code=$SBDEV_CODE
  clean_sb_env "$BIN" dev validate --dir "$W" >"$RUN/logs/$case-S1-cli.out" 2>"$RUN/logs/$case-S1-cli.err"
  local cli_code=$?
  if [ "$sbdev_code" -ne 0 ]; then
    step_fail "sbdev.py validate 退出 ${sbdev_code}：$(error_detail "$RUN/logs/$case-S1-sbdev.err" "$RUN/logs/$case-S1-sbdev.out")"
    if grep -q 'run_at' "$RUN/logs/$case-S1-sbdev.err" 2>/dev/null; then
      note "iot-telemetry：once 任务缺 run_at 时 sbdev/CLI validate 都直接拒绝（E3，待 P1 修）"
    fi
  fi
  if [ "$cli_code" -ne 0 ]; then
    step_fail "simplebase dev validate 退出 ${cli_code}：$(error_detail "$RUN/logs/$case-S1-cli.err" "$RUN/logs/$case-S1-cli.out")"
  fi
  if [ "$sbdev_code" -eq 0 ] && [ "$cli_code" -eq 0 ]; then
    grep '^warning: ' "$RUN/logs/$case-S1-sbdev.err" 2>/dev/null | sed 's/^warning: //' | sort >"$RUN/logs/$case-S1-warn-sbdev.txt"
    python3 - "$RUN/logs/$case-S1-cli.out" >"$RUN/logs/$case-S1-warn-cli.txt" <<'PY'
import json
import sys

lines = [ln for ln in open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines() if ln.strip()]
payload = {}
for line in reversed(lines):
    try:
        payload = json.loads(line)
        break
    except Exception:
        continue
data = payload.get("data") or {}
for item in data.get("warnings") or []:
    print(item)
PY
    sort -o "$RUN/logs/$case-S1-warn-cli.txt" "$RUN/logs/$case-S1-warn-cli.txt"
    if ! diff -q "$RUN/logs/$case-S1-warn-sbdev.txt" "$RUN/logs/$case-S1-warn-cli.txt" >/dev/null 2>&1; then
      step_fail "warning 集合不一致：sbdev=$(tr '\n' '|' <"$RUN/logs/$case-S1-warn-sbdev.txt") cli=$(tr '\n' '|' <"$RUN/logs/$case-S1-warn-cli.txt")"
    fi
    # 计数对账：sbdev 的 "ok: X 个库 / Y 个迁移 / Z 个集合 / F 个函数 / C 个定时任务" 与 CLI JSON。
    local sbdev_summary cli_summary
    sbdev_summary="$(grep -oE 'ok: [0-9]+ 个库.+定时任务' "$RUN/logs/$case-S1-sbdev.out" 2>/dev/null | tail -1)"
    cli_summary="$(tail -n 1 "$RUN/logs/$case-S1-cli.out" 2>/dev/null | jq -r '(.data.databases|tostring)+"/"+(.data.migrations|tostring)+"/"+(.data.collections|tostring)+"/"+(.data.functions|tostring)+"/"+(.data.cron_jobs|tostring)' 2>/dev/null)"
    local sbdev_counts
    sbdev_counts="$(printf '%s' "$sbdev_summary" | sed -E 's/ok: ([0-9]+) 个库 \/ ([0-9]+) 个迁移 \/ ([0-9]+) 个集合 \/ ([0-9]+) 个函数 \/ ([0-9]+) 个定时任务/\1\/\2\/\3\/\4\/\5/')"
    if [ -n "$sbdev_counts" ] && [ -n "$cli_summary" ] && [ "$sbdev_counts" != "$cli_summary" ]; then
      step_fail "库/迁移/集合/函数/任务 计数不一致：sbdev=$sbdev_counts cli=$cli_summary"
    fi
    step_ok_detail "双入口退出 0，warning 集合与计数一致（${cli_summary}）"
    log "$case/S1 通过：$cli_summary"
  fi
  end_step

  # ── S2 建项目 + 签 Key ───────────────────────────────
  begin_step S2
  SV_ENV="A"
  if node "$SCRIPT_DIR/bootstrap.mjs" --url "$A_URL" --dir "$W" --target A \
      --case "$case" --records "$RESULTS" >"$RUN/logs/$case-S2-bootstrap.out" 2>"$RUN/logs/$case-S2-bootstrap.err"; then
    local env_state runtime_state
    env_state="$(check_private_env "$W/.env")"
    runtime_state="$(check_private_env "$W/.env.runtime")"
    if [ "$env_state" != "ok" ]; then step_fail ".env 缺失或权限不对（${env_state}）"; fi
    if [ "$runtime_state" != "ok" ]; then step_fail ".env.runtime 缺失或权限不对（${runtime_state}）"; fi
    PROJECT="$(python3 - "$W/.env" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()
match = re.search(r'^export SIMPLEBASE_PROJECT_ID="?([A-Za-z0-9-]{8})"?', text, re.M)
print(match.group(1) if match else "")
PY
)"
    if [ -z "$PROJECT" ]; then
      step_fail ".env 里没有合法 SIMPLEBASE_PROJECT_ID"
    elif [ "$(printf '%s' "$PROJECT" | wc -c | tr -d ' ')" != "8" ]; then
      step_fail "项目 ID 不是恰好 8 位（${PROJECT}）"
    fi
    if grep -qE 'sb_live_[0-9a-f]{32}' "$RUN/logs/$case-S2-bootstrap.out" "$RUN/logs/$case-S2-bootstrap.err" 2>/dev/null; then
      step_fail "bootstrap 输出泄露了明文 Key"
    fi
    step_ok_detail "project=${PROJECT}，.env/.env.runtime 0600"
    log "$case/S2 通过：project=$PROJECT"
  else
    step_fail "bootstrap 失败：$(log_tail "$RUN/logs/$case-S2-bootstrap.err" 2)"
  fi
  end_step

  # ── S3 自检 ─────────────────────────────────────────
  begin_step S3
  run_sbdev "$case-S3-doctor" doctor -C "$W" --bin "$BIN"
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "doctor 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S3-doctor.err" "$RUN/logs/$case-S3-doctor.out")"
  elif grep -q '\[xx\]' "$RUN/logs/$case-S3-doctor.out" 2>/dev/null; then
    step_fail "doctor 有 [xx]：$(grep '\[xx\]' "$RUN/logs/$case-S3-doctor.out" | head -2 | tr '\n' ' ' | cut -c1-250)"
  else
    step_ok_detail "doctor 全部 [ok]"
  fi
  end_step

  # ── S4 本地 plan ────────────────────────────────────
  begin_step S4
  run_sbdev "$case-S4-plan-A" plan -C "$W" --seed
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "plan 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S4-plan-A.err" "$RUN/logs/$case-S4-plan-A.out")"
  else
    cp "$RUN/logs/$case-S4-plan-A.out" "$RUN/logs/$case-plan-A.txt"
    PLAN_ACTIONS="$(actions_of "$case-S4-plan-A" sbdev)"
    local plan_total plan_changes
    plan_total="$(count_actions "$PLAN_ACTIONS" all)"
    plan_changes="$(count_actions "$PLAN_ACTIONS" changes)"
    if [ "$plan_total" -eq 0 ]; then
      step_fail "plan 没有解析到任何动作（输出格式变了？）"
    elif [ "$plan_changes" -eq 0 ]; then
      step_fail "全新项目 plan 全为 skip（实例不干净或 lock 残留）"
    else
      step_ok_detail "plan $plan_changes 项变更 / $plan_total 项动作"
      log "$case/S4 通过：plan $plan_changes 项变更"
    fi
  fi
  end_step

  # ── S5 本地 apply ───────────────────────────────────
  begin_step S5
  run_sbdev "$case-S5-apply-A" apply -C "$W" --seed
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S5-apply-A.err" "$RUN/logs/$case-S5-apply-A.out")"
  else
    local apply_actions apply_changes apply_match
    apply_actions="$(actions_of "$case-S5-apply-A" sbdev)"
    apply_changes="$(count_actions "$apply_actions" changes)"
    apply_match="$(actions_equal "$PLAN_ACTIONS" "$apply_actions")"
    if [ "$apply_match" != "OK" ]; then
      step_fail "apply 动作序列与 plan 不一致：$apply_match"
    fi
    if [ ! -f "$W/.simplebase/lock.json" ]; then step_fail "缺少 .simplebase/lock.json"; fi
    if [ ! -f "$W/.simplebase/env.local" ]; then step_fail "缺少 .simplebase/env.local"; fi
    step_ok_detail "apply $apply_changes 项变更，动作序列与 plan 一致"
    log "$case/S5 通过：apply $apply_changes 项变更"
  fi
  end_step

  # ── S6 幂等 + 双入口交叉 ────────────────────────────
  # S6a 严格幂等：第二次 apply 不带 --seed，验证结构/迁移/函数/cron；带 --seed 的种子幂等见 S6b。
  begin_step S6
  run_sbdev "$case-S6-apply-A2" apply -C "$W"
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "第二次 apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S6-apply-A2.err" "$RUN/logs/$case-S6-apply-A2.out")"
  else
    local again_actions again_changes
    again_actions="$(actions_of "$case-S6-apply-A2" sbdev)"
    again_changes="$(count_actions "$again_actions" changes)"
    if [ "$again_changes" -ne 0 ]; then
      step_fail "第二次 sbdev apply（不带 --seed）仍有 $again_changes 项非 skip"
    else
      step_ok_detail "sbdev 二次 apply（不带 --seed）全 skip（$(count_actions "$again_actions" skips) 项）"
    fi
  fi
  # 交叉入口：A 是本地目标（127.0.0.1 命中 LOCAL_HOSTS），--yes 冗余但语义正确；
  # 断言重点是两种入口共用 lock 时不报 data_model_mismatch。
  run_cli "$case-S6-cli-A" dev apply --dir "$W" --yes
  if [ "$CLI_CODE" -ne 0 ]; then
    local cli_code
    cli_code="$(last_json_error_code "$RUN/logs/$case-S6-cli-A.err")"
    step_fail "simplebase dev apply 退出 $CLI_CODE ${cli_code:+($cli_code)}：$(error_detail "$RUN/logs/$case-S6-cli-A.err" "$RUN/logs/$case-S6-cli-A.out")"
    if [ "$cli_code" = "data_model_mismatch" ]; then
      note "S6 双入口 data_model_mismatch：清单未显式写 data_model，两种入口推断不一致（E6，待 P1 修）"
    fi
  else
    local cli_actions cli_changes
    cli_actions="$(actions_of "$case-S6-cli-A" cli)"
    cli_changes="$(count_actions "$cli_actions" changes)"
    if [ "$(json_last_ok "$RUN/logs/$case-S6-cli-A.out")" != "true" ]; then
      step_fail "simplebase dev apply 信封 ok!=true"
    elif [ "$cli_changes" -ne 0 ]; then
      step_fail "simplebase dev apply 仍有 $cli_changes 项非 skip（与 sbdev 不互通）"
    else
      step_ok_detail "CLI 交叉 apply 全 skip（$(count_actions "$cli_actions" skips) 项），无 data_model_mismatch"
      log "$case/S6 通过：双入口幂等"
    fi
  fi
  end_step

  # ── S6b 种子重放：集合种子按 _migrations 里的 seed-doc:<集合>/<id> 记账，二次 --seed 必须全 skip ──
  begin_step S6b
  run_sbdev "$case-S6b-apply-seed" apply -C "$W" --seed
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "带 --seed 的 apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S6b-apply-seed.err" "$RUN/logs/$case-S6b-apply-seed.out")"
  else
    local seed_actions seed_changes
    seed_actions="$(actions_of "$case-S6b-apply-seed" sbdev)"
    seed_changes="$(count_actions "$seed_actions" changes)"
    if [ "$seed_changes" -ne 0 ]; then
      step_fail "二次 --seed 仍有 $seed_changes 项非 skip（种子不幂等）"
    else
      step_ok_detail "二次 --seed 全 skip（$(count_actions "$seed_actions" skips) 项）"
    fi
  fi
  end_step

  # ── 取库 ID（S7/S8/S9 共用） ─────────────────────────
  run_cli "$case-databases" database list --limit 200
  if ! cli_ok "$case-databases"; then
    log "$case: database list 失败：$(cli_failure_detail "$case-databases")"
  fi

  # ── S7 资源核验 ─────────────────────────────────────
  begin_step S7
  case "$case" in
    shop) s7_shop ;;
    community) s7_community ;;
    iot-telemetry) s7_iot ;;
    ops-assistant) s7_ops ;;
  esac
  end_step

  # ── S8 补种子 + BFF + 业务冒烟（A） ──────────────────
  begin_step S8
  SV_ENV="A"
  if node "$SCRIPT_DIR/bootstrap.mjs" --url "$A_URL" --dir "$W" --target A --seed-data \
      --case "$case" --records "$RESULTS" >"$RUN/logs/$case-S8-seed.out" 2>"$RUN/logs/$case-S8-seed.err"; then
    step_ok_detail "种子补齐（kv/objects/trigger_once）"
  else
    step_fail "补种子失败：$(log_tail "$RUN/logs/$case-S8-seed.err" 2)"
  fi
  if [ "$STEP_STATUS" = "pass" ] || [ "$STEP_STATUS" = "skip" ]; then
    local runtime_env="$W/.env.runtime"
    if [ "$case" = "iot-telemetry" ]; then
      if run_smoke "$case" S8 A "$runtime_env" "$W"; then
        step_ok_detail "iot 脚本冒烟通过"
      else
        step_fail "iot 脚本冒烟失败：$(log_tail "$RUN/logs/$case-smoke-A.log" 3)"
      fi
    else
      local bffport
      bffport="$(bff_port "$case" A)"
      if start_bff "$case" A "$runtime_env" "$bffport"; then
        if run_smoke "$case" S8 A "$runtime_env" "http://127.0.0.1:$bffport"; then
          step_ok_detail "BFF :$bffport + 业务冒烟通过"
        else
          step_fail "业务冒烟失败：$(log_tail "$RUN/logs/$case-smoke-A.log" 3)"
        fi
      else
        step_fail "BFF 启动失败（:${bffport}）"
      fi
    fi
  else
    step_skip "补种子失败，跳过业务冒烟"
  fi
  stop_bff
  end_step

  # ── S9 云端重放（B） ────────────────────────────────
  if [ "$SKIP_CLOUD" -eq 0 ]; then
    SV_ENV="B"
    begin_step S9
    s9_cloud
    end_step
    SV_ENV="A"
  fi

  # ── S10 只换 URL + Key ──────────────────────────────
  if [ "$SKIP_CLOUD" -eq 0 ]; then
    SV_ENV="B"
    begin_step S10
    s10_swap
    end_step
    SV_ENV="A"
  fi

  # ── S11 / S12 变更与负向（仅 --with-changes + shop/community） ──
  if [ "$WITH_CHANGES" -eq 1 ]; then
    case "$case" in
      shop|community)
        SV_ENV="B"
        begin_step S11
        s11_changes "$case"
        end_step
        begin_step S12
        s12_negative "$case"
        end_step
        SV_ENV="A"
        ;;
      *)
        begin_step S11
        step_skip "S11 只在 shop/community 上跑（planv6.4 §4.3 末段）"
        end_step
        begin_step S12
        step_skip "S12 只在 shop/community 上跑（planv6.4 §4.3 末段）"
        end_step
        ;;
    esac
  fi
}

# ── S7 各站断言 ──────────────────────────────────────────
s7_shop() {
  local db
  db="$(cli_json "$CURRENT_CASE-databases" '.data.databases[]? | select(.name=="shop") | .id')"
  if [ -z "$db" ]; then step_fail "database list 里没有 shop 库"; return; fi
  run_cli "$CURRENT_CASE-S7-sql" sql query --database "$db" --statement 'SELECT COUNT(*) FROM products'
  local count
  count="$(cli_json "$CURRENT_CASE-S7-sql" '.data.rows[0][0]')"
  if ! cli_ok "$CURRENT_CASE-S7-sql"; then
    step_fail "products 查询失败：$(cli_failure_detail "$CURRENT_CASE-S7-sql")"
  elif [ "$count" != "20" ]; then
    step_fail "products 行数=${count}（期望 20）"
  fi
  local table
  for table in orders order_items daily_stats; do
    run_cli "$CURRENT_CASE-S7-sql-$table" sql query --database "$db" --statement "SELECT COUNT(*) FROM $table"
    if ! cli_ok "$CURRENT_CASE-S7-sql-$table"; then
      step_fail "表 $table 不存在或不可查：$(cli_failure_detail "$CURRENT_CASE-S7-sql-$table")"
    fi
  done
  run_cli "$CURRENT_CASE-S7-quote" function invoke --name pricing --export Quote \
    --input '{"items":[{"price_minor":100,"qty":2}],"coupon":"DEMO10"}'
  local total
  total="$(cli_json "$CURRENT_CASE-S7-quote" '.data.total_minor')"
  if ! cli_ok "$CURRENT_CASE-S7-quote"; then
    step_fail "pricing.Quote 调用失败：$(cli_failure_detail "$CURRENT_CASE-S7-quote")"
  elif [ "$total" != "180" ]; then
    step_fail "pricing.Quote total_minor=${total}（期望 180）"
  fi
  run_cli "$CURRENT_CASE-S7-cron" cron list
  if cli_ok "$CURRENT_CASE-S7-cron"; then
    if [ "$(cli_json "$CURRENT_CASE-S7-cron" '[.data.jobs[]? | select(.name=="shop_daily")] | length')" != "1" ]; then
      step_fail "cron shop_daily 不存在"
    fi
  else
    step_fail "cron list 失败：$(cli_failure_detail "$CURRENT_CASE-S7-cron")"
  fi
  [ "$STEP_STATUS" = "pass" ] && step_ok_detail "products=20，orders/order_items/daily_stats 存在，pricing.Quote=180，cron shop_daily 存在"
}

s7_community() {
  # 双库 → 单库适配后（platform：每项目只允许一个 kind=user 库）断言随之变化：
  # 库数量与清单一致；posts 集合在 community 库；likes/post_stats/attachments 同库。
  local expected_db_count count cdb hits
  expected_db_count="$(python3 - "$W/simplebase.json" <<'PY'
import json
import sys

print(len(json.load(open(sys.argv[1], encoding="utf-8")).get("databases") or []))
PY
)"
  count="$(cli_json "$CURRENT_CASE-databases" '.data.databases | length')"
  if [ "${count:-0}" != "$expected_db_count" ]; then
    step_fail "数据库数量=${count:-?}（清单 ${expected_db_count}）"
  fi
  cdb="$(cli_json "$CURRENT_CASE-databases" '.data.databases[]? | select(.name=="community") | .id')"
  if [ -z "$cdb" ]; then step_fail "database list 里没有 community 库"; return; fi
  # 服务端建文档总是生成 UUID、忽略清单里的 id，按 title 内容断言，不写死 id。
  run_cli "$CURRENT_CASE-S7-posts" document list --database "$cdb" --collection posts --limit 100
  if ! cli_ok "$CURRENT_CASE-S7-posts"; then
    step_fail "posts 集合读取失败：$(cli_failure_detail "$CURRENT_CASE-S7-posts")"
  else
    hits="$(cli_json "$CURRENT_CASE-S7-posts" '((.data.rows // []) + ((.data.items // []) | map(.data // .))) | map(select(.title == "欢迎")) | length')"
    if ! [ "${hits:-0}" -ge 1 ] 2>/dev/null; then
      step_fail "posts 集合没有 title=欢迎 的种子帖子"
    fi
  fi
  run_cli "$CURRENT_CASE-S7-likes" sql query --database "$cdb" --statement 'SELECT COUNT(*) FROM likes'
  local likes
  likes="$(cli_json "$CURRENT_CASE-S7-likes" '.data.rows[0][0]')"
  if ! cli_ok "$CURRENT_CASE-S7-likes"; then
    step_fail "likes 查询失败：$(cli_failure_detail "$CURRENT_CASE-S7-likes")"
  elif [ "$likes" != "1" ]; then
    step_fail "likes 行数=${likes}（期望 1）"
  fi
  run_cli "$CURRENT_CASE-S7-stats" sql query --database "$cdb" --statement 'SELECT COUNT(*) FROM post_stats'
  if ! cli_ok "$CURRENT_CASE-S7-stats"; then
    step_fail "post_stats 表不存在或不可查：$(cli_failure_detail "$CURRENT_CASE-S7-stats")"
  fi
  run_cli "$CURRENT_CASE-S7-coll" collection list --database "$cdb"
  if ! cli_ok "$CURRENT_CASE-S7-coll"; then
    step_fail "collection list 失败：$(cli_failure_detail "$CURRENT_CASE-S7-coll")"
  elif [ "$(cli_json "$CURRENT_CASE-S7-coll" '((.data.collections // []) + ((.data.items // []) | map(.name))) | index("posts") != null')" != "true" ]; then
    step_fail "collection list 里没有 posts"
  fi
  # attachments 表在旧双库结构下在 media 库，单库适配后在 community 库；任一库能查通即可。
  local found_att=0 id_
  for id_ in $(cli_json "$CURRENT_CASE-databases" '.data.databases[]?.id'); do
    run_cli "$CURRENT_CASE-S7-att-$id_" sql query --database "$id_" --statement 'SELECT COUNT(*) FROM attachments'
    if cli_ok "$CURRENT_CASE-S7-att-$id_"; then found_att=1; break; fi
  done
  if [ "$found_att" -ne 1 ]; then step_fail "attachments 表在任一库都不可查"; fi
  run_cli "$CURRENT_CASE-S7-mod" function invoke --name moderation --export Check --input '{"title":"spam","body":"spam"}'
  if ! cli_ok "$CURRENT_CASE-S7-mod"; then
    step_fail "moderation.Check 调用失败：$(cli_failure_detail "$CURRENT_CASE-S7-mod")"
  elif [ "$(cli_json "$CURRENT_CASE-S7-mod" '.data.allowed')" != "false" ]; then
    step_fail "moderation.Check 对 spam 未返回 allowed=false"
  fi
  run_cli "$CURRENT_CASE-S7-cron" cron list
  if cli_ok "$CURRENT_CASE-S7-cron"; then
    if [ "$(cli_json "$CURRENT_CASE-S7-cron" '[.data.jobs[]? | select(.name=="community_hot")] | length')" != "1" ]; then
      step_fail "cron community_hot 不存在"
    fi
  else
    step_fail "cron list 失败：$(cli_failure_detail "$CURRENT_CASE-S7-cron")"
  fi
  [ "$STEP_STATUS" = "pass" ] && step_ok_detail "库数=${count}，posts 含 title=欢迎，likes=1，post_stats/attachments 可查，moderation.Check(spam)=false，cron community_hot 存在"
}

s7_iot() {
  local db
  db="$(cli_json "$CURRENT_CASE-databases" '.data.databases[]? | select(.name=="telemetry") | .id')"
  if [ -z "$db" ]; then step_fail "database list 里没有 telemetry 库"; return; fi
  run_cli "$CURRENT_CASE-S7-sql" sql query --database "$db" --statement 'SELECT COUNT(*) FROM devices'
  local count
  count="$(cli_json "$CURRENT_CASE-S7-sql" '.data.rows[0][0]')"
  if ! cli_ok "$CURRENT_CASE-S7-sql"; then
    step_fail "devices 查询失败：$(cli_failure_detail "$CURRENT_CASE-S7-sql")"
  elif [ "$count" != "2" ]; then
    step_fail "devices 行数=${count}（期望 2）"
  fi
  run_cli "$CURRENT_CASE-S7-window" function invoke --name rollup --export Window --input '{}'
  if ! cli_ok "$CURRENT_CASE-S7-window"; then
    step_fail "rollup.Window 调用失败：$(cli_failure_detail "$CURRENT_CASE-S7-window")"
  else
    local start end
    start="$(cli_json "$CURRENT_CASE-S7-window" '.data.start')"
    end="$(cli_json "$CURRENT_CASE-S7-window" '.data.end')"
    if [ -z "$start" ] || [ -z "$end" ]; then step_fail "rollup.Window 未返回 start/end"; fi
  fi
  run_cli "$CURRENT_CASE-S7-cron" cron list
  if ! cli_ok "$CURRENT_CASE-S7-cron"; then
    step_fail "cron list 失败：$(cli_failure_detail "$CURRENT_CASE-S7-cron")"
  else
    if [ "$(cli_json "$CURRENT_CASE-S7-cron" '[.data.jobs[]? | select(.name=="iot_rollup" and .schedule_kind=="interval" and (.interval_seconds|tostring)=="300")] | length')" != "1" ]; then
      step_fail "cron iot_rollup（interval 300）不存在"
    fi
    if [ "$(cli_json "$CURRENT_CASE-S7-cron" '[.data.jobs[]? | select(.name=="iot_export_once" and .schedule_kind=="once")] | length')" != "1" ]; then
      step_fail "cron iot_export_once（once）不存在"
    fi
  fi
  [ "$STEP_STATUS" = "pass" ] && step_ok_detail "devices=2，rollup.Window 返回 start/end，cron iot_rollup/iot_export_once 存在"
}

s7_ops() {
  local db count hits
  db="$(cli_json "$CURRENT_CASE-databases" '.data.databases[]? | select(.name=="ops") | .id')"
  if [ -z "$db" ]; then step_fail "database list 里没有 ops 库"; return; fi
  run_cli "$CURRENT_CASE-S7-sql" sql query --database "$db" --statement 'SELECT COUNT(*) FROM tickets'
  count="$(cli_json "$CURRENT_CASE-S7-sql" '.data.rows[0][0]')"
  if ! cli_ok "$CURRENT_CASE-S7-sql"; then
    step_fail "tickets 查询失败：$(cli_failure_detail "$CURRENT_CASE-S7-sql")"
  elif [ "$count" != "1" ]; then
    step_fail "tickets 行数=${count}（期望 1）"
  fi
  # 服务端建文档总是生成 UUID、忽略清单里的 id，按 owner 内容断言。
  run_cli "$CURRENT_CASE-S7-doc" document list --database "$db" --collection ticket_meta --limit 100
  if ! cli_ok "$CURRENT_CASE-S7-doc"; then
    step_fail "ticket_meta 集合读取失败：$(cli_failure_detail "$CURRENT_CASE-S7-doc")"
  else
    hits="$(cli_json "$CURRENT_CASE-S7-doc" '((.data.rows // []) + ((.data.items // []) | map(.data // .))) | map(select(.owner == "demo-user")) | length')"
    if ! [ "${hits:-0}" -ge 1 ] 2>/dev/null; then
      step_fail "ticket_meta 集合没有 owner=demo-user 的种子文档"
    fi
  fi
  run_cli "$CURRENT_CASE-S7-sla" function invoke --name sla --export Evaluate --input '{"tickets":[{"due_at":"2020-01-01T00:00:00Z","priority":"high"}]}'
  if ! cli_ok "$CURRENT_CASE-S7-sla"; then
    step_fail "sla.Evaluate 调用失败：$(cli_failure_detail "$CURRENT_CASE-S7-sla")"
  elif [ "$(cli_json "$CURRENT_CASE-S7-sla" '.data.levels[0]')" != "overdue" ]; then
    step_fail "sla.Evaluate 对过期工单未返回 overdue"
  fi
  run_cli "$CURRENT_CASE-S7-cron" cron list
  if cli_ok "$CURRENT_CASE-S7-cron"; then
    if [ "$(cli_json "$CURRENT_CASE-S7-cron" '[.data.jobs[]? | select(.name=="ops_sla_scan")] | length')" != "1" ]; then
      step_fail "cron ops_sla_scan 不存在"
    fi
  else
    step_fail "cron list 失败：$(cli_failure_detail "$CURRENT_CASE-S7-cron")"
  fi
  [ "$STEP_STATUS" = "pass" ] && step_ok_detail "tickets=1，ticket_meta 含 owner=demo-user，sla.Evaluate(过期)=overdue，cron ops_sla_scan 存在"
}

# ── S9 云端重放 ──────────────────────────────────────────
s9_cloud() {
  # 1. B 上建项目 + 签 Key（同 project ID）
  if ! node "$SCRIPT_DIR/bootstrap.mjs" --url "$B_URL" --dir "$W" --target B \
      --case "$CURRENT_CASE" --records "$RESULTS" >"$RUN/logs/$CURRENT_CASE-S9-bootstrap.out" 2>"$RUN/logs/$CURRENT_CASE-S9-bootstrap.err"; then
    step_fail "B 建项目/签 Key 失败：$(log_tail "$RUN/logs/$CURRENT_CASE-S9-bootstrap.err" 2)"
    return
  fi
  if [ "$(check_private_env "$W/.env.cloud")" != "ok" ]; then step_fail ".env.cloud 缺失或权限不对"; fi
  if grep -q "$B_URL_REQUESTS" "$W/.env.cloud" 2>/dev/null && ! grep -q 'localhost\.' "$W/.env.cloud" 2>/dev/null; then
    note "B 的 .env.cloud 用了 127.0.0.1 而不是 localhost.：sbdev 会判 local，拿不到远端 apply 的退出码 3"
  fi

  # 2. B 上 plan（干净项目：动作应全部是 create/apply）
  run_sbdev "$CURRENT_CASE-S9-plan-B" plan -C "$W" --seed --env-file "$W/.env.cloud"
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "B plan 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$CURRENT_CASE-S9-plan-B.err" "$RUN/logs/$CURRENT_CASE-S9-plan-B.out")"
    return
  fi
  cp "$RUN/logs/$CURRENT_CASE-S9-plan-B.out" "$RUN/logs/$CURRENT_CASE-plan-B.txt"
  B_PLAN_ACTIONS="$(actions_of "$CURRENT_CASE-S9-plan-B" sbdev)"
  local b_plan_changes
  b_plan_changes="$(count_actions "$B_PLAN_ACTIONS" changes)"
  if [ "$b_plan_changes" -eq 0 ]; then
    step_fail "B 不是干净项目：plan 无变更（0 项），退出码 3 无法验证"
  fi

  # 3. 远端 apply 不带 --yes → 退出码 3
  run_sbdev "$CURRENT_CASE-S9-apply-no" apply -C "$W" --seed --env-file "$W/.env.cloud"
  if [ "$SBDEV_CODE" -ne 3 ]; then
    step_fail "B apply 不带 --yes 退出码=${SBDEV_CODE}（期望 3）$( [ "$SBDEV_CODE" -eq 0 ] && echo '：目标被判成 local 或项目已无变更')"
  else
    step_ok_detail "B apply 无 --yes → 3"
  fi

  # 4. apply --yes → 退出码 0，动作序列与 plan 一致
  run_sbdev "$CURRENT_CASE-S9-apply-yes" apply -C "$W" --seed --env-file "$W/.env.cloud" --yes
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "B apply --yes 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$CURRENT_CASE-S9-apply-yes.err" "$RUN/logs/$CURRENT_CASE-S9-apply-yes.out")"
  else
    local b_apply_actions b_apply_match
    b_apply_actions="$(actions_of "$CURRENT_CASE-S9-apply-yes" sbdev)"
    b_apply_match="$(actions_equal "$B_PLAN_ACTIONS" "$b_apply_actions")"
    if [ "$b_apply_match" != "OK" ]; then step_fail "B apply 动作序列与 plan 不一致：$b_apply_match"; fi
    if [ ! -f "$W/.simplebase/env.localhost" ]; then
      step_fail "B apply 后缺少 .simplebase/env.localhost（label 应取自 hostname localhost）"
    fi
    step_ok_detail "B apply --yes → 0，动作与 plan 一致"
  fi

  # 5. 再跑一次全 skip（带 --seed：种子按 _migrations 记账，必须同样幂等）
  run_sbdev "$CURRENT_CASE-S9-apply-yes2" apply -C "$W" --env-file "$W/.env.cloud" --yes --seed
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "B 二次 apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$CURRENT_CASE-S9-apply-yes2.err" "$RUN/logs/$CURRENT_CASE-S9-apply-yes2.out")"
  else
    local b_again
    b_again="$(count_actions "$(actions_of "$CURRENT_CASE-S9-apply-yes2" sbdev)" changes)"
    if [ "$b_again" -ne 0 ]; then
      step_fail "B 二次 apply 仍有 $b_again 项非 skip"
    else
      step_ok_detail "B 二次 apply 全 skip"
    fi
  fi
  # 6. 交叉入口：simplebase dev apply 走 remote 分支（SIMPLEBASE_URL=localhost.），同样全 skip。
  run_cli_with "$CURRENT_CASE-S9-cli" "$B_URL" "$W/.env.cloud" dev apply --dir "$W" --yes
  if [ "$CLI_CODE" -ne 0 ]; then
    local b_cli_code
    b_cli_code="$(last_json_error_code "$RUN/logs/$CURRENT_CASE-S9-cli.err")"
    step_fail "B simplebase dev apply 退出 $CLI_CODE ${b_cli_code:+($b_cli_code)}：$(error_detail "$RUN/logs/$CURRENT_CASE-S9-cli.err" "$RUN/logs/$CURRENT_CASE-S9-cli.out")"
    if [ "$b_cli_code" = "data_model_mismatch" ]; then
      note "B 交叉入口 data_model_mismatch：清单 data_model 与远端库不一致（E6）"
    fi
  else
    local b_cli_changes
    b_cli_changes="$(count_actions "$(actions_of "$CURRENT_CASE-S9-cli" cli)" changes)"
    if [ "$(json_last_ok "$RUN/logs/$CURRENT_CASE-S9-cli.out")" != "true" ]; then
      step_fail "B simplebase dev apply 信封 ok!=true"
    elif [ "$b_cli_changes" -ne 0 ]; then
      step_fail "B 交叉入口仍有 $b_cli_changes 项非 skip"
    else
      step_ok_detail "B 交叉入口全 skip（remote 分支），无 data_model_mismatch"
    fi
  fi

  # 7. B 侧数据种子 + 运行时 env（S10 用）。
  # skill 重放只同步结构与代码，kv/objects/trigger_once 属于数据、不在重放范围（SKILL.md 边界）；
  # S10 的语义是「只换 SIMPLEBASE_URL/KEY/DB ID」，所以数据由补位脚本在 S9 之后补到 B。
  if ! node "$SCRIPT_DIR/bootstrap.mjs" --url "$B_URL" --dir "$W" --target B --seed-data \
      --case "$CURRENT_CASE" --records "$RESULTS" >"$RUN/logs/$CURRENT_CASE-S9-seed.out" 2>"$RUN/logs/$CURRENT_CASE-S9-seed.err"; then
    step_fail "B 补数据种子/运行时 env 失败：$(log_tail "$RUN/logs/$CURRENT_CASE-S9-seed.err" 2)"
  else
    step_ok_detail "B 已补 kv/objects/trigger_once 种子并写 .env.cloud.runtime"
    if [ "${SB_NOTE_B_SEED:-0}" = "0" ]; then
      SB_NOTE_B_SEED=1
      note "S9→S10 之间对 B 补一次数据种子（bootstrap --seed-data：kv / objects / trigger_once）：kv 与对象属于数据、不在 skill 重放范围（SKILL.md「数据不搬」边界），S10 仍然是「只换 SIMPLEBASE_URL / SIMPLEBASE_API_KEY / 按库名的 DATABASE_ID」。"
    fi
  fi
}

# ── S10 只换 URL + Key ───────────────────────────────────
s10_swap() {
  local runtime_a="$W/.env.runtime" runtime_b="$W/.env.cloud.runtime"
  if [ ! -f "$runtime_b" ]; then
    step_fail "缺少 ${runtime_b}（S9 未生成）"
    return
  fi
  # 断言「只换值、不增删变量」：A/B 运行时 env 的变量名集合必须一致。
  local keys_a keys_b
  keys_a="$(grep -oE '^export [A-Za-z0-9_]+' "$runtime_a" | sort)"
  keys_b="$(grep -oE '^export [A-Za-z0-9_]+' "$runtime_b" | sort)"
  if [ "$keys_a" != "$keys_b" ]; then
    step_fail "A/B 运行时 env 变量名不一致：$(diff <(printf '%s\n' "$keys_a") <(printf '%s\n' "$keys_b") | head -4 | tr '\n' ' ')"
  fi
  local bffport
  bffport="$(bff_port "$CURRENT_CASE" B)"
  stop_bff
  if [ "$CURRENT_CASE" = "iot-telemetry" ]; then
    if run_smoke "$CURRENT_CASE" S10 B "$runtime_b" "$W"; then
      step_ok_detail "只换 URL/Key/库 ID 后 iot 脚本冒烟通过（B）"
    else
      step_fail "B 上 iot 冒烟失败：$(log_tail "$RUN/logs/$CURRENT_CASE-smoke-B.log" 3)"
    fi
  elif start_bff "$CURRENT_CASE" B "$runtime_b" "$bffport"; then
    if run_smoke "$CURRENT_CASE" S10 B "$runtime_b" "http://127.0.0.1:$bffport"; then
      step_ok_detail "只换 URL/Key/库 ID，BFF :$bffport 冒烟通过（B）"
    else
      step_fail "B 上业务冒烟失败：$(log_tail "$RUN/logs/$CURRENT_CASE-smoke-B.log" 3)"
    fi
  else
    step_fail "B 上 BFF 启动失败（:${bffport}）"
  fi
  stop_bff
}

# ── S11 变更回放 ─────────────────────────────────────────
s11_changes() {
  local case="$1"
  local migration_dir="$W/migrations" table function_file marker
  case "$case" in
    shop)
      table="products"
      function_file="$W/functions/pricing.go"
      marker='{"items":[],"coupon":""}'
      ;;
    community)
      table="post_stats"
      function_file="$W/functions/moderation.go"
      marker='{"title":"示例","body":"欢迎"}'
      ;;
    *) step_fail "S11 不支持站点 $case"; return ;;
  esac
  # 迁移编号动态取「现有最大编号 + 1」：examples 的迁移文件可能已被 P1/fixer 增加到 002，
  # 固定写 002_verify.sql 会撞号或乱序。
  local next_seq verify_migration
  next_seq="$(python3 - "$migration_dir" <<'PY'
import os
import re
import sys

names = [name for name in os.listdir(sys.argv[1]) if re.match(r"^\d+_.*\.sql$", name)]
nums = [int(name.split("_", 1)[0]) for name in names]
print((max(nums) if nums else 0) + 1)
PY
)"
  verify_migration="$(printf '%03d_verify.sql' "$next_seq")"
  printf 'ALTER TABLE %s ADD COLUMN verify_note VARCHAR;\n' "$table" >"$migration_dir/$verify_migration"
  # 给函数返回结构加 note 字段（代表「改函数源码 → 新版本试跑后生效」）。
  python3 - "$case" "$function_file" <<'PY'
import sys

case, path = sys.argv[1], sys.argv[2]
text = open(path, encoding="utf-8").read()
if case == "shop":
    text = text.replace(
        'type QuoteResult struct { TotalMinor int64 `json:"total_minor"`; DiscountMinor int64 `json:"discount_minor"` }',
        'type QuoteResult struct { TotalMinor int64 `json:"total_minor"`; DiscountMinor int64 `json:"discount_minor"`; Note string `json:"note"` }')
    text = text.replace("return result\n}", "result.Note = \"verify\"\n\treturn result\n}", 1)
else:
    text = text.replace(
        'type CheckResult struct { Allowed bool `json:"allowed"` }',
        'type CheckResult struct { Allowed bool `json:"allowed"`; Note string `json:"note"` }')
    text = text.replace('return CheckResult{Allowed: ', 'return CheckResult{Note: "verify", Allowed: ', 1)
open(path, "w", encoding="utf-8").write(text)
PY
  # A：apply 只执行 002 + update function
  run_sbdev "$case-S11-apply-A" apply -C "$W"
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "A 变更 apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S11-apply-A.err" "$RUN/logs/$case-S11-apply-A.out")"
    return
  fi
  local a_actions
  a_actions="$(actions_of "$case-S11-apply-A" sbdev)"
  if ! printf '%s' "$a_actions" | jq -e --arg mig "$verify_migration" 'any(.[]; .kind=="migration" and (.name|endswith($mig)) and .verb=="apply")' >/dev/null 2>&1; then
    step_fail "A apply 未执行 ${verify_migration}：$(printf '%s' "$a_actions" | jq -c '[.[]|select(.kind=="migration")]')"
  fi
  if ! printf '%s' "$a_actions" | jq -e 'any(.[]; .kind=="function" and .verb=="update")' >/dev/null 2>&1; then
    step_fail "A apply 未出现 update function"
  fi
  # 函数新版本已试跑并激活：invoke 能看到 note 字段
  run_cli "$case-S11-invoke-A" function invoke --name "$(basename "$function_file" .go)" --export \
    "$( [ "$case" = shop ] && echo Quote || echo Check )" --input "$marker"
  if ! cli_ok "$case-S11-invoke-A"; then
    step_fail "A 新函数版本调用失败：$(cli_failure_detail "$case-S11-invoke-A")"
  elif [ "$(cli_json "$case-S11-invoke-A" '.data.note')" != "verify" ]; then
    step_fail "A 新函数版本未生效（note 字段缺失）"
  fi
  # B：plan 显示 update function 与 002 迁移，apply 后同样生效
  run_sbdev "$case-S11-plan-B" plan -C "$W" --env-file "$W/.env.cloud"
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "B plan 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S11-plan-B.err" "$RUN/logs/$case-S11-plan-B.out")"
    return
  fi
  local b_actions
  b_actions="$(actions_of "$case-S11-plan-B" sbdev)"
  if ! printf '%s' "$b_actions" | jq -e 'any(.[]; .kind=="function" and .verb=="update")' >/dev/null 2>&1; then
    step_fail "B plan 未显示 update function"
  fi
  if ! printf '%s' "$b_actions" | jq -e --arg mig "$verify_migration" 'any(.[]; .kind=="migration" and (.name|endswith($mig)))' >/dev/null 2>&1; then
    step_fail "B plan 未显示 $verify_migration 迁移"
  fi
  run_sbdev "$case-S11-apply-B" apply -C "$W" --env-file "$W/.env.cloud" --yes
  if [ "$SBDEV_CODE" -ne 0 ]; then
    step_fail "B 变更 apply 退出 ${SBDEV_CODE}：$(error_detail "$RUN/logs/$case-S11-apply-B.err" "$RUN/logs/$case-S11-apply-B.out")"
    return
  fi
  run_cli_env "$case-S11-invoke-B" "$W/.env.cloud" function invoke \
    --name "$(basename "$function_file" .go)" --export "$( [ "$case" = shop ] && echo Quote || echo Check )" --input "$marker"
  if ! cli_ok "$case-S11-invoke-B"; then
    step_fail "B 新函数版本调用失败：$(cli_failure_detail "$case-S11-invoke-B")"
  elif [ "$(cli_json "$case-S11-invoke-B" '.data.note')" != "verify" ]; then
    step_fail "B 新函数版本未生效（note 字段缺失）"
  fi
  if [ "$STEP_STATUS" = "pass" ]; then
    step_ok_detail "A 只执行 ${verify_migration} + update function；B plan 显示 update function / ${verify_migration}；两端新版本已激活"
  fi
}

# ── S12 负向用例 ─────────────────────────────────────────
s12_negative() {
  local case="$1"
  local init=""
  local function_file=""
  for candidate in "$W"/migrations/*.sql; do
    [ -f "$candidate" ] || continue
    case "$(basename "$candidate")" in 001_*) init="$candidate" ;; esac
  done
  case "$case" in
    shop) function_file="$W/functions/pricing.go" ;;
    community) function_file="$W/functions/moderation.go" ;;
    *) step_fail "S12 不支持站点 $case"; return ;;
  esac
  if [ -z "$init" ]; then
    step_fail "找不到已应用的 001 迁移"
    return
  fi
  # 1) 改已应用迁移 → migration_modified（退出 1）
  cp "$init" "$RUN/logs/$case-S12-init.bak"
  printf '\n-- verify negative: modify applied migration\n' >>"$init"
  run_sbdev "$case-S12-modified" apply -C "$W" --seed
  local code=$SBDEV_CODE
  local detail
  detail="$(error_detail "$RUN/logs/$case-S12-modified.err" "$RUN/logs/$case-S12-modified.out")"
  cp "$RUN/logs/$case-S12-init.bak" "$init"
  if [ "$code" -ne 1 ] || ! printf '%s' "$detail" | grep -q 'migration_modified'; then
    step_fail "改已应用迁移后 apply 退出=${code}（期望 1 + migration_modified）：$detail"
  fi
  # 2) 函数里写假 sb_live_ → validate 拒绝
  cp "$function_file" "$RUN/logs/$case-S12-function.bak"
  printf '\n// fake key for validation: sb_live_0123456789abcdef0123456789abcdef\n' >>"$function_file"
  run_sbdev "$case-S12-secret" validate -C "$W"
  local secret_code=$SBDEV_CODE
  local secret_detail
  secret_detail="$(error_detail "$RUN/logs/$case-S12-secret.err" "$RUN/logs/$case-S12-secret.out")"
  cp "$RUN/logs/$case-S12-function.bak" "$function_file"
  if [ "$secret_code" -eq 0 ]; then
    step_fail "函数源码含假 sb_live_ 时 validate 仍退出 0（密钥扫描失效）"
  fi
  if [ "$STEP_STATUS" = "pass" ]; then
    step_ok_detail "改已应用迁移 → 退出 1 + migration_modified；假 sb_live_ → validate 退出 $secret_code 拒绝"
  fi
}

# ── 主流程 ───────────────────────────────────────────────
for case in ${CASES//,/ }; do
  run_case "$case"
done

finalize_report 0
EXIT_CODE="$SUMMARY_CODE"
log "完成：$RUN/report.md"
exit "$EXIT_CODE"
