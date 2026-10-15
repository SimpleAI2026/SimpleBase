#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

step "list" project list
expect '.data.projects | type == "array"'

step "create" project create --name "cap-${RUN_ID}"
expect '(.data.id | test("^[A-Za-z0-9-]{8}$")) and .data.name == $n' --arg n "cap-${RUN_ID}"
PROJECT_ID="$(jq_out '.data.id')"
if [[ -n "$PROJECT_ID" && "$PROJECT_ID" != "null" ]]; then
  SIMPLEBASE_PROJECT_ID="$PROJECT_ID"
  id_file="${PROJECT_ID_FILE:-/tmp/simplebase-examples/project_id}"
  mkdir -p "$(dirname "$id_file")"
  printf '%s\n' "$PROJECT_ID" >"$id_file"
  chmod 600 "$id_file"
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
