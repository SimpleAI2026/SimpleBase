#!/usr/bin/env bash
# smoke/ops-assistant.sh — 运维助理冒烟（planv6.4 §4.4）
#
# 断言：GET /api/tickets、/api/health 200；POST /api/tickets 成功；verify-perms.sh 全 PASS
#       （跨项目用例的目标改为参数 VERIFY_CROSS_PROJECT，不写死 ex-shop1）。
#
# 前置：SIMPLEBASE_URL / SIMPLEBASE_API_KEY / SIMPLEBASE_READONLY_KEY /
#       SIMPLEBASE_PROJECT_ID / SIMPLEBASE_DATABASE_ID
# 用法：SMOKE_STEP=S8 SMOKE_ENV=A SMOKE_BFF_URL=http://127.0.0.1:28107 bash smoke/ops-assistant.sh

set -uo pipefail
# shellcheck source=../lib.sh
. "$(cd "$(dirname "$0")" && pwd)/../lib.sh"
SV_SUITE="${SV_SUITE:-smoke}"

: "${SIMPLEBASE_URL:?需要 SIMPLEBASE_URL（先 source 运行时 env）}"
: "${SIMPLEBASE_API_KEY:?需要 SIMPLEBASE_API_KEY}"
: "${SIMPLEBASE_READONLY_KEY:?需要 SIMPLEBASE_READONLY_KEY}"
: "${SIMPLEBASE_PROJECT_ID:?需要 SIMPLEBASE_PROJECT_ID}"
: "${SIMPLEBASE_DATABASE_ID:?需要 SIMPLEBASE_DATABASE_ID（单库站点）}"

BFF="${1:-${SMOKE_BFF_URL:-}}"
if [ -z "$BFF" ]; then
  echo "smoke/ops-assistant.sh: 需要 BFF URL（参数或 SMOKE_BFF_URL）" >&2
  exit 1
fi
BFF="${BFF%/}"
WORK="${SMOKE_WORK:-$PWD}"
VERIFY_CROSS_PROJECT="${VERIFY_CROSS_PROJECT:-ex-xproj}"

smoke_start
smoke_init_tmp
SMOKE_STARTED="$(now_ms)"

# ── 1. GET /api/tickets 200 ─────────────────────────────
code="$(sb_request GET "$BFF/api/tickets")"
tickets_before="$(sb_body | jq 'length' 2>/dev/null || echo -1)"
[ "$code" = "200" ] && [ "$tickets_before" -ge 1 ]
smoke_check "get-tickets" $? "code=$code tickets=$tickets_before"

# ── 2. GET /api/health 200（logs/metrics/audit/quota）────
code="$(sb_request GET "$BFF/api/health")"
health_ok="$(sb_body | jq -r '[has("logs"), has("metrics"), has("trend"), has("audit"), has("quota")] | all' 2>/dev/null || echo false)"
[ "$code" = "200" ] && [ "$health_ok" = "true" ]
smoke_check "api-health" $? "code=$code keys_ok=$health_ok"

# ── 3. POST /api/tickets 事务成功 ───────────────────────
code="$(sb_request POST "$BFF/api/tickets" '{"title":"verify 冒烟工单","priority":"low"}')"
ticket_id="$(sb_body | jq -r '.id // empty' 2>/dev/null)"
code2="$(sb_request GET "$BFF/api/tickets")"
tickets_after="$(sb_body | jq 'length' 2>/dev/null || echo -1)"
[ "$code" = "201" ] && [ -n "$ticket_id" ] && [ "$code2" = "200" ] && [ "$tickets_after" = "$((tickets_before + 1))" ]
smoke_check "create-ticket" $? "code=$code id=${ticket_id:0:8} tickets=${tickets_before}→$tickets_after"

# ── 4. verify-perms.sh 全 PASS（跨项目目标参数化）───────
perms_src="$WORK/scripts/verify-perms.sh"
perms_run="$SMOKE_TMP/verify-perms.verify.sh"
if [ -f "$perms_src" ]; then
  python3 - "$perms_src" "$perms_run" <<'PY'
import sys

src, dst = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()
needle = 'check 403 GET "$SIMPLEBASE_URL/v1/projects/ex-shop1/s3/objects"'
replacement = 'check 403 GET "$SIMPLEBASE_URL/v1/projects/${VERIFY_CROSS_PROJECT:-ex-xproj}/s3/objects"'
if needle in text:
    text = text.replace(needle, replacement)
else:
    # 上游脚本已参数化或行格式变了：原样复制，记录观察。
    sys.stderr.write("verify-perms: 没找到写死的 ex-shop1 行，按原样运行\n")
open(dst, "w", encoding="utf-8").write(text)
PY
  chmod +x "$perms_run"
  perms_code=0
  VERIFY_CROSS_PROJECT="$VERIFY_CROSS_PROJECT" bash "$perms_run" >"$SMOKE_TMP/verify-perms.out" 2>"$SMOKE_TMP/verify-perms.err" || perms_code=$?
  pass_count="$(grep -c '^PASS ' "$SMOKE_TMP/verify-perms.out" 2>/dev/null || echo 0)"
  fail_line="$(grep '^FAIL ' "$SMOKE_TMP/verify-perms.out" "$SMOKE_TMP/verify-perms.err" 2>/dev/null | head -1)"
  [ "$perms_code" -eq 0 ] && [ "$pass_count" -ge 12 ] && [ -z "$fail_line" ]
  smoke_check "verify-perms-all-pass" $? "exit=$perms_code pass=$pass_count${fail_line:+ $fail_line}"
  perms_note="verify-perms ${pass_count} PASS（跨项目目标 ${VERIFY_CROSS_PROJECT}）"
else
  smoke_check "verify-perms-all-pass" 1 "缺少 $perms_src"
  perms_note=""
fi

SMOKE_MS=$(( $(now_ms) - SMOKE_STARTED ))
extra="tickets ${tickets_before}→${tickets_after}${perms_note:+；$perms_note}"
smoke_finish "$extra"
