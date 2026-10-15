#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

FN="tick${RUN_ID}"
JOB_NAME="tick-${RUN_ID}"

step "function" function create --name "$FN" --file "$HERE/tick.go" --description "cron target"
done_step

step "create" cron create --name "$JOB_NAME" --schedule-kind interval --interval-seconds 3600 \
  --func-file "$FN" --func-export Tick --input '{"n":1}' --enabled false
expect '.data.id != "" and .data.enabled == false and .data.func_export == "Tick"'
JOB="$(jq_out '.data.id')"
if [[ -z "$JOB" || "$JOB" == "null" ]]; then
  echo "FAIL ${CASE}/no-job-id"
  exit 1
fi

step "get" cron get --id "$JOB"
expect '.data.id == $id' --arg id "$JOB"

step "trigger" cron trigger --id "$JOB"
expect '.data.triggered == true'

recorded=0
for _ in 1 2 3 4 5 6 7 8 9 10; do
  step "runs" cron runs --id "$JOB"
  if [[ "$STEP_OK" -eq 1 ]] && printf '%s' "$SB_OUT" | jq -e '.data.runs | map(select(.status == "success")) | length >= 1' >/dev/null 2>&1; then
    if ! printf '%s' "$SB_OUT" | jq -e '.data.runs | map(select(.status == "success")) | .[0].response_json | contains("2")' >/dev/null 2>&1; then
      STEP_OK=0
      STEP_WHY="success run did not contain incremented n"
    fi
    done_step
    recorded=1
    break
  fi
  # 还在排队或执行：这一轮不记入结果。
  sleep 2
done
if [[ "$recorded" -ne 1 ]]; then
  STEP_NAME="runs"
  STEP_CMD="simplebase cron runs --id $JOB"
  STEP_MS=0
  if [[ -z "$STEP_WHY" ]]; then
    STEP_WHY="$(printf '%s' "$SB_OUT" | redact)"
  fi
  record 0 "$STEP_WHY"
fi

step "delete" cron delete --id "$JOB"
done_step

finish_case
