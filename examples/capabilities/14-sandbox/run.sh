#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 可选能力：云沙盒（E2B 兼容 API）。
# 前置是实例具备本机进程隔离能力（Linux bubblewrap+Landlock / macOS sandbox-exec）。
# 不满足时整体 SKIP exit 0，不计 FAIL；满足时走 create → get → list → pause → delete。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
NAME="cap14-${SUF}"
SID=""
CLEANED=0

sb_json() {
  SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" 2>&1 || true
}

skip_case() {
  local reason="$1"
  DETAIL="skipped: $reason" NAME="skip" CASE="$CASE" FILE="$RESULT_FILE" python3 -c '
import json, os
row = {"suite": "cli", "case": os.environ["CASE"], "step": os.environ["NAME"],
       "command": "", "ok": True, "ms": 0, "detail": os.environ.get("DETAIL", "")}
with open(os.environ["FILE"], "a", encoding="utf-8") as f:
    f.write(json.dumps(row, ensure_ascii=False) + "\n")
'
  echo "SKIP ${CASE} ($reason)"
  exit 0
}

cleanup() {
  if [[ "$CLEANED" != "1" && -n "$SID" ]]; then
    sb_json sandbox delete --id "$SID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

# ── 0. 前置：沙盒不可用就 SKIP ────────────────────────────
CAPS="$(sb_json sandbox capabilities)"
if ! printf '%s' "$CAPS" | jq -e '.ok == true and .data.available == true' >/dev/null 2>&1; then
  if printf '%s' "$CAPS" | jq -e '.ok == true and .data.available == false' >/dev/null 2>&1; then
    REASON="$(printf '%s' "$CAPS" | jq -r '.data.reason // .data.backend // "sandbox unavailable"')"
    skip_case "云沙盒不可用（${REASON}）"
  fi
  CODE="$(printf '%s' "$CAPS" | jq -r '.error.code // "unknown"' 2>/dev/null || echo unknown)"
  if [[ "$CODE" == "sandbox_unavailable" || "$CODE" == "not_found" || "$CODE" == "forbidden" ]]; then
    skip_case "云沙盒未启用或无权限（${CODE}）"
  fi
  echo "FAIL ${CASE}/capabilities (${CODE})"
  printf '%s\n' "$CAPS" | redact >&2
  exit 1
fi

# ── 1. 完整走一遍控制面 ───────────────────────────────────
step "capabilities" sandbox capabilities
expect '.data.available == true and .data.backend != ""'

step "create" sandbox create --name "$NAME" --timeout 60
expect '.data.sandboxID != "" and ((.data.state // "") != "" or (.data.x_simplebase.status // "") != "")'
SID="$(jq_out '.data.sandboxID')"
if [[ -z "$SID" || "$SID" == "null" ]]; then
  echo "FAIL ${CASE}/no-sandbox-id"
  finish_case
fi

step "get" sandbox get --id "$SID"
expect '.data.sandboxID == $id' --arg id "$SID"

step "list" sandbox list
expect '(.data | map(select(.sandboxID == $id)) | length) == 1' --arg id "$SID"

step "pause" sandbox pause --id "$SID"
expect '.data.ok == true'

step "delete" sandbox delete --id "$SID"
expect '.data.ok == true'
CLEANED=1

step "list-after-delete" sandbox list
expect '(.data | map(select(.sandboxID == $id)) | length) == 0' --arg id "$SID"

finish_case
