#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

USER_NAME="exu${RUN_ID}"
# 随机口令只进 stdin，不回显、不写入结果文件。
PASSWORD="$(python3 -c 'import secrets; print(secrets.token_hex(8))')"

step "user-create" user create --username "$USER_NAME" --role user --display-name "Capability example" <<<"$PASSWORD"
unset PASSWORD
expect '.data.id != null and .data.id != "" and .data.username == $n and (.data | has("password") | not) and (.data | has("password_hash") | not)' --arg n "$USER_NAME"
USER_ID="$(jq_out '.data.id')"

if [[ -n "$USER_ID" && "$USER_ID" != "null" ]]; then
  step "user-get" user get --id "$USER_ID"
  expect '.data.id == $id' --arg id "$USER_ID"
  step "user-list" user list --limit 50
  expect '.data.users | map(select(.id == $id)) | length == 1' --arg id "$USER_ID"
else
  step "user-get" user get --id missing
  done_step
  step "user-list" user list --limit 5
  done_step
fi

step "apikey-create" apikey create --permissions database:read
KEY_ID="$(jq_out '.data.id')"
KEY_SECRET="$(jq_out '.data.secret')"
# 失败详情可能含 secret，先打码再记录。
if [[ "$STEP_OK" -eq 1 ]]; then
  if [[ -z "$KEY_SECRET" || "$KEY_SECRET" == "null" ]]; then
    STEP_OK=0
    STEP_WHY="secret missing"
  elif ! printf '%s' "$SB_OUT" | jq -e '.data.permissions | index("database:read") != null' >/dev/null; then
    STEP_OK=0
    STEP_WHY="database:read not in permissions"
  fi
fi
SB_OUT="$(printf '%s' "$SB_OUT" | redact)"
SB_ERR="$(printf '%s' "$SB_ERR" | redact)"
if [[ "$STEP_OK" -ne 1 && -z "$STEP_WHY" ]]; then
  STEP_WHY="$(printf '%s\n%s' "$SB_OUT" "$SB_ERR" | redact)"
fi
done_step

read_with_key() {
  local name="$1"
  shift
  STEP_NAME="$name"
  STEP_CMD="SIMPLEBASE_TOKEN=[redacted] simplebase $*"
  local saved="$SIMPLEBASE_TOKEN"
  SIMPLEBASE_TOKEN="$KEY_SECRET"
  sb_raw "$@"
  SIMPLEBASE_TOKEN="$saved"
  STEP_OK=0
  STEP_WHY=""
  if [[ "$SB_CODE" -eq 0 ]] && printf '%s' "$SB_OUT" | jq -e '.ok == true' >/dev/null 2>&1; then
    STEP_OK=1
  else
    STEP_WHY="$(printf '%s\n%s' "$SB_OUT" "$SB_ERR" | redact)"
  fi
}

if [[ -n "$KEY_SECRET" && "$KEY_SECRET" != "null" ]]; then
  read_with_key "readonly-list" database list --limit 5
  done_step
  read_with_key "readonly-write-rejected" database create --name "should-fail-${RUN_ID}"
  # 写操作必须失败。
  if [[ "$STEP_OK" -eq 1 ]]; then
    STEP_OK=0
    STEP_WHY="readonly key was allowed to create a database"
  else
    STEP_OK=1
    STEP_WHY=""
  fi
  done_step
  step "apikey-revoke" apikey revoke --id "$KEY_ID"
  done_step
  read_with_key "revoked-rejected" database list --limit 5
  if [[ "$STEP_OK" -eq 1 ]]; then
    STEP_OK=0
    STEP_WHY="revoked key still listed databases"
  else
    STEP_OK=1
    STEP_WHY=""
  fi
  done_step
else
  echo "FAIL ${CASE}/apikey-followup (no secret)"
  STEP_FAIL=1
fi
unset KEY_SECRET

if [[ -n "${USER_ID:-}" && "$USER_ID" != "null" ]]; then
  step "user-delete" user delete --id "$USER_ID"
  done_step
fi

finish_case
