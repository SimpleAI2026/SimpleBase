#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

step "list" project list
expect '.data.projects | type == "array"'

# 复用上一次 run_all 建的项目（PROJECT_ID_FILE），避免每轮都新建 cap-<RUN_ID>；
# 传 --fresh（或 FRESH=1）才强制新建。文件里的项目可能来自另一个实例（共享 /tmp 路径），
# 解析不到时清掉文件、回退为新建，不算失败。
id_file="${PROJECT_ID_FILE:-/tmp/simplebase-examples/project_id}"
FRESH="${FRESH:-0}"
[[ "${1:-}" == "--fresh" ]] && FRESH=1
reused=0
if [[ "$FRESH" != "1" && -s "$id_file" ]]; then
  SIMPLEBASE_PROJECT_ID="$(cat "$id_file")"
  step "reuse" settings get
  if [[ "$STEP_OK" -eq 1 ]]; then
    done_step
    reused=1
  else
    echo "stale project id in $id_file; creating a new project"
    rm -f "$id_file"
  fi
fi

if [[ "$reused" -eq 0 ]]; then
  step "create" project create --name "cap-${RUN_ID}"
  expect '(.data.id | test("^[A-Za-z0-9-]{8}$")) and .data.name == $n' --arg n "cap-${RUN_ID}"
  PROJECT_ID="$(jq_out '.data.id')"
  if [[ -n "$PROJECT_ID" && "$PROJECT_ID" != "null" ]]; then
    SIMPLEBASE_PROJECT_ID="$PROJECT_ID"
    mkdir -p "$(dirname "$id_file")"
    printf '%s\n' "$PROJECT_ID" >"$id_file"
    chmod 600 "$id_file"
  fi
fi

step "settings-set" settings set --json '{"key":"examples.note","value_json":"\"cap\""}'
done_step

step "settings-get" settings get
expect '.data.settings | map(select(.key == "examples.note")) | length >= 1'

step "quota" quota get
done_step

step "audit" audit list --limit 5
done_step

step "log-search" log search --limit 5
done_step

step "log-retention" log retention get
expect '.data.keep_days | type == "number"'

finish_case
