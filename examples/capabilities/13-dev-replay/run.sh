#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 本地→云端重放：把内置最小工作区（1 迁移 + 1 函数 + 1 定时任务）重放到当前实例。
# 依次执行 dev validate → dev plan → dev apply --seed --yes → 再 apply 一次，
# 断言第二次全部 skip；随后用 CLI 核对资源，最后删除本次创建的资源并清理临时工作区。
#
# 工作区是仓库内 workspace/ 的临时副本（mktemp），.simplebase/lock.json 与 env 提示文件
# 只落在副本里，跑完删除，不污染仓库。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
DBNAME="devmain-${SUF}"
FNAME="devfn${SUF}"
CNAME="devcron-${SUF}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/sb-dev-replay.XXXXXX")"
DBID=""
CRONID=""
CLEANED_DB=0
CLEANED_FN=0
CLEANED_CRON=0

sb_json() {
  SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" 2>&1 || true
}

cleanup() {
  if [[ "$CLEANED_DB" != "1" && -n "$DBID" ]]; then
    sb_json database delete --id "$DBID" >/dev/null 2>&1 || true
  fi
  if [[ "$CLEANED_FN" != "1" ]]; then
    sb_json function delete --name "$FNAME" >/dev/null 2>&1 || true
  fi
  if [[ "$CLEANED_CRON" != "1" && -n "$CRONID" ]]; then
    sb_json cron delete --id "$CRONID" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ── 0. 准备临时工作区（替换占位符）────────────────────────
mkdir -p "$WORK"
cp -R "$HERE/workspace/." "$WORK/"
python3 - "$WORK" "$SIMPLEBASE_PROJECT_ID" "$SUF" <<'PY'
import sys

root, project_id, suffix = sys.argv[1], sys.argv[2], sys.argv[3]
for rel in ("simplebase.json", "migrations/001_init.sql"):
    path = root + "/" + rel
    with open(path, encoding="utf-8") as f:
        text = f.read()
    text = text.replace("__PROJECT_ID__", project_id).replace("__SUFFIX__", suffix)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
PY

# 同一项目重复跑本用例时，清掉上次留下、名字为 devmain-* 的用户库（项目只允许一个用户库）。
for id in $(sb_json database list | jq -r '.data.databases[]? | select(.name | startswith("devmain-")) | .id'); do
  sb_json database delete --id "$id" >/dev/null 2>&1 || true
done

# ── 1. 离线校验 ───────────────────────────────────────────
step "validate" dev validate --dir "$WORK"
expect '(.data.errors | length) == 0 and .data.databases == 1 and .data.migrations == 1 and .data.collections == 1 and .data.functions == 1 and .data.cron_jobs == 1'

# 与 skill 的 sbdev.py 对同一份清单做一次离线对账（skills 目录存在时才跑）。
SKILL="$(cd "$HERE/../../.." && pwd)/skills/simplebase/scripts/sbdev.py"
if [[ -f "$SKILL" ]] && command -v python3 >/dev/null 2>&1; then
  set +e
  SKILL_OUT="$(python3 "$SKILL" validate -C "$WORK" 2>&1)"
  SKILL_CODE=$?
  set -e
  STEP_NAME="skill-validate"
  STEP_CMD="python3 skills/simplebase/scripts/sbdev.py validate -C <work>"
  STEP_MS=0
  STEP_OK=0
  STEP_WHY=""
  if [[ "$SKILL_CODE" -eq 0 ]]; then
    STEP_OK=1
  else
    STEP_WHY="sbdev.py validate 退出码 $SKILL_CODE: $(printf '%s' "$SKILL_OUT" | redact)"
  fi
  record "$STEP_OK" "$STEP_WHY"
else
  echo "SKIP ${CASE}/skill-validate (skills/simplebase/scripts/sbdev.py 不存在)"
fi

# ── 2. plan：只读列出动作 ─────────────────────────────────
step "plan" dev plan --dir "$WORK" --seed
expect '.data.mode == "plan" and (.data.actions | length) >= 4 and ((.data.actions | map(.kind) | unique) == ["collection","cron","database","document","function","migration"]) and .data.seed == true'

# ── 3. 第一次 apply：创建资源 ─────────────────────────────
step "apply" dev apply --dir "$WORK" --seed --yes
expect '.data.mode == "apply" and .data.changes >= 1'
DBID="$(sb_json database list | jq -r --arg n "$DBNAME" '.data.databases[]? | select(.name == $n) | .id' | head -1)"
CRONID="$(sb_json cron list | jq -r --arg n "$CNAME" '.data.jobs[]? | select(.name == $n) | .id' | head -1)"

# ── 4. 第二次 apply：全部 skip（幂等）────────────────────
step "apply-idempotent" dev apply --dir "$WORK" --seed --yes
expect '.data.mode == "apply" and .data.changes == 0 and (.data.actions | length) >= 4 and (.data.skips == (.data.actions | length)) and (.data.actions | all(.verb == "skip"))'

# ── 5. lock 状态 ──────────────────────────────────────────
step "status" dev status --dir "$WORK"
expect '.data.current != "" and (.data.keys | length) >= 1 and (.data.targets | length) >= 1'

# ── 6. 资源核对（CLI 读回）────────────────────────────────
step "resource-db" database list
expect '(.data.databases | map(select(.name == $n)) | length) == 1' --arg n "$DBNAME"
step "resource-function" function list
expect '(.data.functions | map(select(.name == $n)) | length) == 1' --arg n "$FNAME"
step "resource-cron" cron list
expect '(.data.jobs | map(select(.name == $n)) | length) == 1' --arg n "$CNAME"

# ── 7. 清理本次创建的资源 ─────────────────────────────────
if [[ -n "$DBID" ]]; then
  step "delete-db" database delete --id "$DBID"
  done_step
  CLEANED_DB=1
fi
step "delete-function" function delete --name "$FNAME"
done_step
CLEANED_FN=1
if [[ -n "$CRONID" ]]; then
  step "delete-cron" cron delete --id "$CRONID"
  done_step
  CLEANED_CRON=1
fi

finish_case
