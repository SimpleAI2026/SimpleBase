#!/usr/bin/env bash
set -euo pipefail
: "${SIMPLEBASE_URL:?source examples/ops-assistant/.env}"
: "${SIMPLEBASE_READONLY_KEY:?source examples/ops-assistant/.env}"
: "${SIMPLEBASE_DATABASE_ID:?source examples/ops-assistant/.env}"
base="$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID"
check() {
  local expected="$1" method="$2" path="$3" body="${4:-}" status
  local args=(-sS -o /dev/null -w '%{http_code}' -X "$method" -H "Authorization: Bearer $SIMPLEBASE_READONLY_KEY" -H 'Content-Type: application/json')
  if [ -n "$body" ]; then args+=(-d "$body"); fi
  status="$(curl "${args[@]}" "$path")"
  if [ "$status" != "$expected" ]; then printf 'FAIL %s %s: %s != %s\n' "$method" "$path" "$status" "$expected" >&2; exit 1; fi
  printf 'PASS %s %s: %s\n' "$method" "$path" "$status"
}
check 200 POST "$base/databases/$SIMPLEBASE_DATABASE_ID/query" '{"sql":"SELECT COUNT(*) FROM tickets"}'
check 403 POST "$base/databases/$SIMPLEBASE_DATABASE_ID/execute" '{"sql":"DELETE FROM tickets WHERE id = '\''nothing'\''"}'
for argv in '["GET","idem:demo"]' '["HGETALL","health:demo"]' '["ZRANGE","priority:demo","0","-1"]' '["SCAN","0"]'; do check 200 POST "$base/kv" "{\"type\":\"cmd\",\"argvs\":$argv}"; done
check 403 POST "$base/kv" '{"type":"cmd","argvs":["SET","example:denied","1"]}'
check 403 POST "$base/kv" '{"type":"String","args":{"key":"example:denied","value":"1"}}'
check 200 POST "$SIMPLEBASE_URL/go/$SIMPLEBASE_PROJECT_ID/sla/Evaluate" '{"tickets":[]}'
check 200 GET "$base/s3/objects"
check 403 POST "$base/gofunctions" '{"name":"denied","source":"package main"}'
check 403 POST "$base/s3/objects"
check 403 POST "$base/cron-jobs" '{"name":"denied"}'
check 403 GET "$SIMPLEBASE_URL/v1/projects/ex-shop1/s3/objects"
