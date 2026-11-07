#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 备份恢复：项目级单文件快照（项目级 KV 数据面）。
# 流程：KV SET before → backup create → KV 覆盖 after → backup restore → KV GET 回到 before
#       → 下载快照校验大小 → backup delete 后列表不再包含。
#
# 备份默认关闭（config.backup.enabled=false），接口返回 503 backup_disabled；此时整体 SKIP。
#
# 已知平台约束：所有用户数据库与项目级 KV 共用「项目单文件」，删除任意用户数据库会清理
# 该文件（catalog.DeleteDatabaseSync → PurgeProjectFiles）。若前面的用例删过数据库，当前项目
# 的文件可能已被 unlink，backup create 会返回 500 internal_error。此时本用例新建一个专用项目
# （需要 token 有 project:admin），在干净项目上验证备份；无建项目权限则打印 SKIP。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
KEY="cap11:${SUF}:k"
TMPDIR_CASE="$(mktemp -d "${TMPDIR:-/tmp}/sb-backup.XXXXXX")"
BID=""
PROJECT_CREATED=""
CLEANED_BACKUP=0

sb_json() {
  SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="${SIMPLEBASE_PROJECT_ID:-}" SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" 2>&1 || true
}

cleanup() {
  if [[ -n "$BID" && "$CLEANED_BACKUP" != "1" ]]; then
    sb_json backup delete --id "$BID" >/dev/null 2>&1 || true
  fi
  sb_json kv exec --command DEL --arg "$KEY" >/dev/null 2>&1 || true
  rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

skip_case() { # skip_case <reason>：写一条 ok=true 的 skip 记录并 exit 0，不计 FAIL
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

record_assert() { # record_assert <name> <ok> <detail> [cmd]
  STEP_NAME="$1"
  STEP_OK="$2"
  STEP_WHY="$3"
  STEP_CMD="${4:-}"
  STEP_MS=0
  record "$STEP_OK" "$STEP_WHY"
}

# ── 0. 前置：备份 API 是否启用 ────────────────────────────
# 备份默认关闭：config.backup.enabled=false 时 internal/app 不挂载 /backups 路由，
# CLI 拿到的不是 503 backup_disabled，而是 HTTP 404 not_found。两者都按「未启用」SKIP。
PRE="$(sb_json backup list)"
if ! printf '%s' "$PRE" | jq -e '.ok == true' >/dev/null 2>&1; then
  PRE_CODE="$(printf '%s' "$PRE" | jq -r '.error.code // empty' 2>/dev/null || true)"
  case "$PRE_CODE" in
    backup_disabled|not_found|404)
      skip_case "备份 API 未启用：config backup.enabled=true / SIMPLEBASE_BACKUP_ENABLED=true 才挂载 /backups（当前 backup list: ${PRE_CODE}）" ;;
  esac
  echo "FAIL ${CASE}/preflight (backup list: ${PRE_CODE:-unknown})"
  printf '%s\n' "$PRE" | redact >&2
  exit 1
fi

# ── 1. 项目健康探针：项目文件可快照吗？ ───────────────────
# 备份是项目级操作；前面的用例删除用户数据库时会清理项目单文件，使快照一直 500。
sb_json kv exec --command SET --arg "cap11:probe:${SUF}" --arg probe >/dev/null 2>&1 || true
PROBE="$(sb_json backup create)"
if printf '%s' "$PROBE" | jq -e '.ok == true and .data.id != ""' >/dev/null 2>&1; then
  PROBE_ID="$(printf '%s' "$PROBE" | jq -r '.data.id')"
  sb_json backup delete --id "$PROBE_ID" >/dev/null 2>&1 || true
  record_assert "project" 1 "沿用当前项目（项目文件可快照）" "simplebase backup create (probe)"
else
  PROBE_CODE="$(printf '%s' "$PROBE" | jq -r '.error.code // empty' 2>/dev/null || true)"
  if [[ "$PROBE_CODE" != "internal_error" ]]; then
    echo "FAIL ${CASE}/project (backup create probe: ${PROBE_CODE:-unknown})"
    printf '%s\n' "$PROBE" | redact >&2
    exit 1
  fi
  # 当前项目文件已被删库清理，换个干净项目（备份按项目隔离）。
  # 服务端生成 8 位项目 ID，不传 --id，避免 ID 形态/长度问题。
  NEW_PROJECT="$(sb_json project create --name "cap11-${SUF}")"
  if ! printf '%s' "$NEW_PROJECT" | jq -e '.ok == true and .data.id != ""' >/dev/null 2>&1; then
    NEW_CODE="$(printf '%s' "$NEW_PROJECT" | jq -r '.error.code // empty' 2>/dev/null || true)"
    skip_case "当前项目文件不可快照（删库副作用），且无法新建项目（project create: ${NEW_CODE:-unknown}）"
  fi
  PROJECT_CREATED="$(printf '%s' "$NEW_PROJECT" | jq -r '.data.id')"
  export SIMPLEBASE_PROJECT_ID="$PROJECT_CREATED"
  STEP_NAME="create-project"
  STEP_CMD="project create --name cap11-${SUF}"
  STEP_MS=0
  record 1 "当前项目文件不可快照（删库副作用），改用新建项目 $SIMPLEBASE_PROJECT_ID"
  STEP_NAME="project"
  STEP_CMD="simplebase backup create (probe)"
  STEP_MS=0
  record 1 "备份在专用项目 $SIMPLEBASE_PROJECT_ID 上验证"
fi

STEP_NAME="preflight"
STEP_CMD="simplebase backup list"
STEP_MS=0
record 1 "备份 API 可用（列表返回 items 数组）"

# ── 2. 写入项目级 KV，作为快照内容 ────────────────────────
step "kv-set" kv exec --command SET --arg "$KEY" --arg before
expect '.data == "OK"'
step "kv-get-before" kv exec --command GET --arg "$KEY"
expect '.data == "before"'

# ── 3. 创建快照 ───────────────────────────────────────────
step "create" backup create
expect '.data.id != "" and .data.status == "ready" and (.data.sha256 | length) == 64 and .data.size_bytes > 0 and .data.engine != ""'
BID="$(jq_out '.data.id')"
SIZE="$(jq_out '.data.size_bytes')"
if [[ -z "$BID" || "$BID" == "null" ]]; then
  echo "FAIL ${CASE}/no-backup-id"
  finish_case
fi

step "list" backup list
expect '.data.items | map(select(.id == $id)) | length == 1' --arg id "$BID"

# ── 4. 改数据后恢复 ───────────────────────────────────────
step "kv-overwrite" kv exec --command SET --arg "$KEY" --arg after
expect '.data == "OK"'
step "kv-get-after" kv exec --command GET --arg "$KEY"
expect '.data == "after"'

step "restore" backup restore --id "$BID"
expect '.data.ok == true'

# 恢复会关闭并重开项目数据库；重新 GET 应回到快照里的 before。
step "kv-get-restored" kv exec --command GET --arg "$KEY"
expect '.data == "before"'

# ── 5. 下载快照（GET /backups/:id/download）───────────────
DL_FILE="$TMPDIR_CASE/backup.bin"
DL_CODE="$(curl -sS -o "$DL_FILE" -w '%{http_code}' --max-time 60 \
  "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/backups/$BID/download" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" 2>"$TMPDIR_CASE/dl.err" || true)"
DL_SIZE=0
[[ -f "$DL_FILE" ]] && DL_SIZE="$(wc -c <"$DL_FILE" | tr -d ' ')"
STEP_NAME="download"
STEP_CMD="curl -o backup.bin .../backups/$BID/download"
STEP_MS=0
STEP_OK=0
STEP_WHY=""
if [[ "$DL_CODE" != "200" ]]; then
  STEP_WHY="http $DL_CODE: $(redact <"$TMPDIR_CASE/dl.err" 2>/dev/null || true)"
elif [[ -n "$SIZE" && "$SIZE" != "null" && "$DL_SIZE" != "$SIZE" ]]; then
  STEP_WHY="downloaded $DL_SIZE bytes, metadata says $SIZE"
else
  STEP_OK=1
fi
record "$STEP_OK" "$STEP_WHY"

# ── 6. 删除备份 ───────────────────────────────────────────
step "delete" backup delete --id "$BID"
expect '.data.ok == true'
CLEANED_BACKUP=1

step "list-after-delete" backup list
expect '.data.items | map(select(.id == $id)) | length == 0' --arg id "$BID"

step "kv-del" kv exec --command DEL --arg "$KEY"
done_step

finish_case
