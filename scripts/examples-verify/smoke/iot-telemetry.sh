#!/usr/bin/env bash
# smoke/iot-telemetry.sh — 物联网遥测冒烟（planv6.4 §4.4）
#
# 断言：scripts/curl.sh 两次请求 200；simulate.mjs 写入 1 行；worker.mjs 连续跑两次，
#       第二次不重复处理同一 run（processed_runs / rollup_5m 不增长）。
#
# 前置：SIMPLEBASE_URL / SIMPLEBASE_API_KEY / SIMPLEBASE_PROJECT_ID / SIMPLEBASE_DATABASE_ID
# 用法：SMOKE_STEP=S8 SMOKE_ENV=A SMOKE_WORK=/tmp/.../work/iot-telemetry bash smoke/iot-telemetry.sh
#       （$1 可覆盖工作区目录；脚本没有 BFF）

set -uo pipefail
# shellcheck source=../lib.sh
. "$(cd "$(dirname "$0")" && pwd)/../lib.sh"
SV_SUITE="${SV_SUITE:-smoke}"

: "${SIMPLEBASE_URL:?需要 SIMPLEBASE_URL（先 source 运行时 env）}"
: "${SIMPLEBASE_API_KEY:?需要 SIMPLEBASE_API_KEY}"
: "${SIMPLEBASE_PROJECT_ID:?需要 SIMPLEBASE_PROJECT_ID}"
: "${SIMPLEBASE_DATABASE_ID:?需要 SIMPLEBASE_DATABASE_ID（单库站点）}"

WORK="${1:-${SMOKE_WORK:-}}"
if [ -z "$WORK" ] || [ ! -d "$WORK" ]; then
  echo "smoke/iot-telemetry.sh: 需要工作区目录（参数或 SMOKE_WORK），当前 '$WORK'" >&2
  exit 1
fi

smoke_start
smoke_init_tmp
SMOKE_STARTED="$(now_ms)"
SQL_DB="$SIMPLEBASE_DATABASE_ID"
PROJECT_API="${SIMPLEBASE_URL%/}/v1/projects/$SIMPLEBASE_PROJECT_ID"

# ── 1. scripts/curl.sh 两次 200（curl -f 失败即非 0）──
( cd "$WORK" && bash scripts/curl.sh >"$SMOKE_TMP/curl1.json" 2>"$SMOKE_TMP/curl1.err" )
code1=$?
( cd "$WORK" && bash scripts/curl.sh >"$SMOKE_TMP/curl2.json" 2>"$SMOKE_TMP/curl2.err" )
code2=$?
[ "$code1" -eq 0 ] && [ "$code2" -eq 0 ]
smoke_check "curl-script-twice-200" $? "run1=$code1 run2=$code2"

# ── 2. simulate.mjs 写入 1 行 readings ──────────────────
readings_before="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM readings')"
( cd "$WORK" && node scripts/simulate.mjs device-01 >"$SMOKE_TMP/simulate.out" 2>"$SMOKE_TMP/simulate.err" )
simulate_code=$?
readings_after="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM readings')"
[ "$simulate_code" -eq 0 ] && [ -n "$readings_before" ] && [ "$((readings_after - readings_before))" = "1" ]
smoke_check "simulate-writes-one-reading" $? "code=$simulate_code before=$readings_before after=$readings_after"

# ── 3. 至少有一个 completed 的 run（bootstrap trigger_once）──
code="$(sb_request GET "$PROJECT_API/cron-jobs")"
job_id="$(sb_body | jq -r '.jobs[]? | select(.name == "iot_rollup") | .id' 2>/dev/null | head -1)"
completed=-1
if [ -n "$job_id" ]; then
  code="$(sb_request GET "$PROJECT_API/cron-jobs/$job_id/runs?limit=100")"
  completed="$(sb_body | jq '[.runs[]? | select(.status == "completed")] | length' 2>/dev/null || echo -1)"
fi
[ -n "$job_id" ] && [ "$completed" -ge 1 ]
smoke_check "rollup-run-completed" $? "job=${job_id:0:8} completed=$completed"

# ── 4. worker.mjs 连续两次：第二次不重复处理同一 run ────
processed_before="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM processed_runs')"
rollup_before="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM rollup_5m')"
( cd "$WORK" && node scripts/worker.mjs >"$SMOKE_TMP/worker1.out" 2>"$SMOKE_TMP/worker1.err" )
worker1=$?
processed_mid="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM processed_runs')"
rollup_mid="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM rollup_5m')"
( cd "$WORK" && node scripts/worker.mjs >"$SMOKE_TMP/worker2.out" 2>"$SMOKE_TMP/worker2.err" )
worker2=$?
processed_after="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM processed_runs')"
rollup_after="$(sql_scalar "$SQL_DB" 'SELECT COUNT(*) FROM rollup_5m')"
[ "$worker1" -eq 0 ] && [ "$worker2" -eq 0 ] && [ "$processed_mid" -gt "$processed_before" ]
smoke_check "worker-processes-runs" $? "worker1=$worker1 worker2=$worker2 processed=${processed_before}→$processed_mid"

[ "$processed_after" = "$processed_mid" ] && [ "$rollup_after" = "$rollup_mid" ]
smoke_check "worker-idempotent-second-run" $? "processed=${processed_mid}→$processed_after rollup=${rollup_mid}→$rollup_after"

# ── 5. KV 在线标记（simulate 写入 TTL key）──────────────
key="$(sql_scalar "$SQL_DB" "SELECT COUNT(*) FROM readings WHERE device_id = 'device-01'")"
[ -n "$key" ] && [ "$key" -ge 1 ]
smoke_check "readings-has-device-01" $? "readings=$key"

SMOKE_MS=$(( $(now_ms) - SMOKE_STARTED ))
extra="readings ${readings_before}→${readings_after}，processed ${processed_before}→${processed_mid}→${processed_after}"
smoke_finish "$extra"
