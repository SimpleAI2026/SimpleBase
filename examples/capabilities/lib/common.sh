#!/usr/bin/env bash
# 能力示例的公共步骤记录。由各 run.sh source，不要直接执行。
# 调用方需要：SIMPLEBASE_BIN、SIMPLEBASE_URL、SIMPLEBASE_TOKEN、
# SIMPLEBASE_PROJECT_ID、RUN_ID、RESULT_FILE、CASE。
if [[ -z "${SIMPLEBASE_BIN:-}" || -z "${SIMPLEBASE_URL:-}" || -z "${SIMPLEBASE_TOKEN:-}" || -z "${SIMPLEBASE_PROJECT_ID:-}" || -z "${RUN_ID:-}" || -z "${RESULT_FILE:-}" || -z "${CASE:-}" ]]; then
  echo "capabilities/lib/common.sh: missing SIMPLEBASE_BIN/URL/TOKEN/PROJECT_ID, RUN_ID, RESULT_FILE or CASE" >&2
  exit 1
fi

STEP_FAIL=0
SB_OUT=""
SB_ERR=""
SB_CODE=0
STEP_NAME=""
STEP_CMD=""
STEP_MS=0
STEP_OK=0
STEP_WHY=""

redact() {
  python3 -c '
import sys, re
s = sys.stdin.read()
s = re.sub(r"(?i)(secret|token|password|api_key|authorization|refresh_token)(\"?:?\s*\"?)[^\s\",}]+", r"\1\2[redacted]", s)
s = re.sub(r"sb_live_[A-Za-z0-9_\-]+", "sb_live_[redacted]", s)
s = re.sub(r"eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+", "[jwt]", s)
sys.stdout.write(s[:900])
'
}

record() {
  local ok="$1" detail="$2"
  DETAIL="$detail" OK="$ok" NAME="$STEP_NAME" CMD="$STEP_CMD" MS="$STEP_MS" \
    CASE="$CASE" FILE="$RESULT_FILE" python3 -c '
import json, os
row = {
  "suite": "cli",
  "case": os.environ["CASE"],
  "step": os.environ["NAME"],
  "command": os.environ["CMD"],
  "ok": os.environ["OK"] == "1",
  "ms": int(os.environ["MS"] or "0"),
  "detail": os.environ.get("DETAIL", ""),
}
with open(os.environ["FILE"], "a", encoding="utf-8") as f:
    f.write(json.dumps(row, ensure_ascii=False) + "\n")
'
  if [[ "$ok" != "1" ]]; then
    STEP_FAIL=1
    echo "FAIL ${CASE}/${STEP_NAME}"
  else
    echo "PASS ${CASE}/${STEP_NAME} (${STEP_MS}ms)"
  fi
}

sb_raw() {
  local errf outf
  errf="$(mktemp)"
  outf="$(mktemp)"
  local start end
  start=$(date +%s%3N)
  set +e
  SIMPLEBASE_URL="$SIMPLEBASE_URL" \
    SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" \
    SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" >"$outf" 2>"$errf"
  SB_CODE=$?
  set -e
  end=$(date +%s%3N)
  STEP_MS=$((end - start))
  SB_OUT="$(cat "$outf")"
  SB_ERR="$(cat "$errf")"
  rm -f "$outf" "$errf"
}

# step <name> <simplebase argv...>
# 成功条件是退出码 0 且信封 ok=true。随后可用 expect 追加 jq 断言。
step() {
  STEP_NAME="$1"
  shift
  STEP_CMD="simplebase $*"
  STEP_WHY=""
  sb_raw "$@"
  STEP_OK=0
  if [[ "$SB_CODE" -eq 0 ]] && printf '%s' "$SB_OUT" | jq -e '.ok == true' >/dev/null 2>&1; then
    STEP_OK=1
  else
    STEP_WHY="$(printf '%s\n%s' "$SB_OUT" "$SB_ERR" | redact)"
  fi
}

# expect <jq-filter> [jq 选项...]
# 例：expect '.data.name == $n' --arg n "$name"
expect() {
  local filter="$1"
  shift
  if [[ "$STEP_OK" -eq 1 ]] && ! printf '%s' "$SB_OUT" | jq "$@" -e "$filter" >/dev/null 2>&1; then
    STEP_OK=0
    STEP_WHY="assertion failed: $filter"
  fi
  record "$STEP_OK" "$STEP_WHY"
}

done_step() {
  record "$STEP_OK" "$STEP_WHY"
}

jq_out() {
  printf '%s' "$SB_OUT" | jq -r "$1"
}

finish_case() {
  if [[ "$STEP_FAIL" -ne 0 ]]; then
    exit 1
  fi
}
