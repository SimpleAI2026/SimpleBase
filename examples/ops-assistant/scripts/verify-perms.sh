#!/usr/bin/env bash
set -euo pipefail
: "${SIMPLEBASE_URL:?source examples/ops-assistant/.env}"
: "${SIMPLEBASE_READONLY_KEY:?source examples/ops-assistant/.env}"
: "${SIMPLEBASE_DATABASE_ID:?source examples/ops-assistant/.env}"
# 跨项目 403 用例需要一个同实例上存在的其他项目；默认 ex-shop1，可用环境变量覆盖。
# 目标项目不存在时该用例 skip（用 --project-prefix 初始化后可能没有 ex-shop1），不影响其余断言。
OTHER_PROJECT_ID="${SIMPLEBASE_OTHER_PROJECT_ID:-ex-shop1}"
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
cross_status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $SIMPLEBASE_READONLY_KEY" "$SIMPLEBASE_URL/v1/projects/$OTHER_PROJECT_ID/s3/objects")"
case "$cross_status" in
  403) printf 'PASS GET /v1/projects/%s/s3/objects: 403\n' "$OTHER_PROJECT_ID" ;;
  404) printf 'SKIP GET /v1/projects/%s/s3/objects: 项目不存在（用 SIMPLEBASE_OTHER_PROJECT_ID 指定同实例另一个项目）\n' "$OTHER_PROJECT_ID" ;;
  *) printf 'FAIL GET /v1/projects/%s/s3/objects: %s != 403\n' "$OTHER_PROJECT_ID" "$cross_status" >&2; exit 1 ;;
esac
