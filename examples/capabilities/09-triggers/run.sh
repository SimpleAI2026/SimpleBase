#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 事件触发器：record_change（集合 create）与 http invoke 两条路径。
# 断言方式：被触发的云函数用 net/http 回调本机的一次性 HTTP sink，按 record_id 计数。
# 服务端不在本机时（URL 主机不是 loopback）跳过 sink 断言，只验证管理面与入队结果。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
COL="orders_${SUF}"
FN="trig_${SUF}"
TMPDIR_CASE="$(mktemp -d "${TMPDIR:-/tmp}/sb-triggers.XXXXXX")"
SINK_FILE=""
SINK_PID=""
DB=""
CREATED_DB=0
TRIGGER_IDS=()

cleanup() {
  local id
  for id in "${TRIGGER_IDS[@]:-}"; do
    [[ -n "$id" ]] || continue
    curl -sS -o /dev/null --max-time 5 -X DELETE \
      "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers/$id" \
      -H "Authorization: Bearer $SIMPLEBASE_TOKEN" >/dev/null 2>&1 || true
  done
  if [[ -n "${FN:-}" ]]; then
    SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
      SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
      "$SIMPLEBASE_BIN" function delete --name "$FN" >/dev/null 2>&1 || true
  fi
  if [[ "$CREATED_DB" == "1" && -n "$DB" ]]; then
    SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
      SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
      "$SIMPLEBASE_BIN" database delete --id "$DB" >/dev/null 2>&1 || true
  fi
  if [[ -n "$SINK_PID" ]]; then
    kill "$SINK_PID" >/dev/null 2>&1 || true
    wait "$SINK_PID" 2>/dev/null || true
  fi
  rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

# ── 直接读（record 之外）────────────────────────────────────
sb_json() {
  SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" 2>/dev/null || true
}

api_call() { # api_call <method> <path> [body]；结果放 SB_CODE/SB_OUT/SB_ERR
  local method="$1" path="$2" body="${3:-}"
  local outf errf code
  outf="$(mktemp)"
  errf="$(mktemp)"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl -sS -o "$outf" -w '%{http_code}' --max-time 20 -X "$method" \
      "$SIMPLEBASE_URL$path" -H "Authorization: Bearer $SIMPLEBASE_TOKEN" \
      -H 'content-type: application/json' --data "$body" 2>"$errf")"
  else
    code="$(curl -sS -o "$outf" -w '%{http_code}' --max-time 20 -X "$method" \
      "$SIMPLEBASE_URL$path" -H "Authorization: Bearer $SIMPLEBASE_TOKEN" 2>"$errf")"
  fi
  set -e
  SB_CODE="${code:-0}"
  SB_OUT="$(cat "$outf")"
  SB_ERR="$(cat "$errf")"
  rm -f "$outf" "$errf"
}

api_step() { # api_step <name> <expected-http-code> <method> <path> [json-body]
  local name="$1" want="$2" method="$3" path="$4" body="${5:-}"
  STEP_NAME="$name"
  STEP_CMD="curl -X $method $path"
  STEP_MS=0
  api_call "$method" "$path" "$body"
  STEP_OK=0
  STEP_WHY=""
  if [[ "$SB_CODE" == "$want" ]]; then
    STEP_OK=1
  else
    STEP_WHY="expected http $want, got $SB_CODE: $(printf '%s%s' "$SB_OUT" "$SB_ERR" | redact)"
  fi
}

wait_file_has() { # wait_file_has <file> <needle> [tries]；每轮 100ms
  local file="$1" needle="$2" tries="${3:-50}" i
  for ((i = 0; i < tries; i++)); do
    if [[ -f "$file" ]] && grep -q -- "$needle" "$file" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

sink_step() { # sink_step <name> <needle>；本地 sink 命中计数断言
  local name="$1" needle="$2" hits
  STEP_NAME="$name"
  STEP_CMD="grep $needle $SINK_FILE"
  STEP_MS=0
  STEP_OK=0
  STEP_WHY=""
  if [[ -z "$SINK_FILE" ]]; then
    echo "SKIP ${CASE}/${name} (服务端不在本机，无法回调 sink)"
    return
  fi
  if ! wait_file_has "$SINK_FILE" "$needle" 100; then
    STEP_WHY="5s 内没有收到触发器回调（sink=${SINK_FILE}）"
    record 0 "$STEP_WHY"
    return
  fi
  hits="$(grep -c -- "$needle" "$SINK_FILE" 2>/dev/null || true)"
  if [[ "$hits" -eq 1 ]]; then
    STEP_OK=1
  else
    STEP_WHY="期望恰好 1 次回调，实际 $hits 次"
  fi
  record "$STEP_OK" "$STEP_WHY"
}

# ── 1. 数据库与集合 ───────────────────────────────────────
DB="$(sb_json database list | jq -r '.data.databases[0].id // empty')"
if [[ -n "$DB" ]]; then
  STEP_NAME="reuse-db"
  STEP_CMD="simplebase database list"
  STEP_MS=0
  record 1 "复用已有数据库 ${DB}（项目单文件限制：每个项目只有一个用户库）"
else
  step "create-db" database create --name "cap09-${SUF}" --data-model collection
  expect '.data.id != "" and .data.data_model == "collection"'
  DB="$(jq_out '.data.id')"
  CREATED_DB=1
fi
if [[ -z "$DB" || "$DB" == "null" ]]; then
  echo "FAIL ${CASE}/no-database"
  exit 1
fi

step "create-collection" collection create --database "$DB" --name "$COL"
done_step

# ── 2. sink + 被触发的云函数 ──────────────────────────────
IS_LOCAL="$(python3 - "$SIMPLEBASE_URL" <<'PY'
import sys, urllib.parse
host = urllib.parse.urlparse(sys.argv[1]).hostname or ""
print("1" if host in {"127.0.0.1", "localhost", "::1", "0.0.0.0"} else "0")
PY
)"

if [[ "$IS_LOCAL" == "1" ]]; then
  SINK_FILE="$TMPDIR_CASE/hits.log"
  : >"$SINK_FILE"
  SINK_PORT="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
  cat >"$TMPDIR_CASE/sink.py" <<'PY'
import http.server
import sys

hits, port = sys.argv[1], int(sys.argv[2])


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        with open(hits, "a", encoding="utf-8") as f:
            f.write(self.path + "\n")
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, *args):
        pass


http.server.HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
  python3 "$TMPDIR_CASE/sink.py" "$SINK_FILE" "$SINK_PORT" >"$TMPDIR_CASE/sink.out" 2>&1 &
  SINK_PID=$!
  SINK_UP=0
  for _ in $(seq 1 50); do
    if curl -sS -o /dev/null --max-time 1 "http://127.0.0.1:$SINK_PORT/ping" 2>/dev/null; then
      SINK_UP=1
      break
    fi
    sleep 0.1
  done
  if [[ "$SINK_UP" -ne 1 ]]; then
    echo "FAIL ${CASE}/sink-start (port=$SINK_PORT)"
    cat "$TMPDIR_CASE/sink.out" >&2 || true
    exit 1
  fi
  : >"$SINK_FILE"
fi

if [[ -n "$SINK_FILE" ]]; then
  cat >"$TMPDIR_CASE/trigger.go" <<GO
//go:build ignore

package main

import (
	"fmt"
	"net/http"
)

type Event struct {
	Kind     string         \`json:"kind"\`
	Target   string         \`json:"target"\`
	Action   string         \`json:"action"\`
	RecordID string         \`json:"record_id"\`
	Changed  []string       \`json:"changed"\`
	Data     map[string]any \`json:"data"\`
}

type Result struct {
	Accepted bool \`json:"accepted"\`
}

func Handle(event Event) Result {
	_, err := http.Get(fmt.Sprintf("http://127.0.0.1:$SINK_PORT/hit?record_id=%s&action=%s", event.RecordID, event.Action))
	return Result{Accepted: err == nil}
}
GO
else
  cat >"$TMPDIR_CASE/trigger.go" <<'GO'
//go:build ignore

package main

type Event struct {
	Kind     string         `json:"kind"`
	Target   string         `json:"target"`
	Action   string         `json:"action"`
	RecordID string         `json:"record_id"`
	Data     map[string]any `json:"data"`
}

type Result struct {
	Accepted bool `json:"accepted"`
}

func Handle(event Event) Result {
	return Result{Accepted: event.Action != ""}
}
GO
fi

step "function" function create --name "$FN" --file "$TMPDIR_CASE/trigger.go" --description "event trigger target"
expect '.data.name == $n and (.data.exports | index("Handle") != null)' --arg n "$FN"
if [[ "$STEP_OK" -ne 1 ]]; then
  echo "FAIL ${CASE}/function-create"
  finish_case
fi

# ── 3. record_change 触发器：文档 create 后被调用一次 ─────
api_step "record-trigger-create" 201 POST "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers" \
  "$(jq -nc --arg col "$COL" --arg fn "$FN" \
    '{kind:"record_change", target:$col, events:["create"], func_file:$fn, func_export:"Handle", enabled:true}')"
expect '.id != "" and .kind == "record_change" and .target == $t and .enabled == true' --arg t "$COL"
TID="$(jq_out '.id')"
if [[ -n "$TID" && "$TID" != "null" ]]; then
  TRIGGER_IDS+=("$TID")
fi

step "insert-doc" document insert --database "$DB" --collection "$COL" --json '{"item":"book","qty":2}'
expect '.data.id != ""'
DOC="$(jq_out '.data.id')"
if [[ -n "${DOC:-}" && "$DOC" != "null" ]]; then
  sink_step "record-change-fired" "$DOC"
else
  STEP_NAME="record-change-fired"
  STEP_CMD="(no document id)"
  STEP_OK=0
  record 0 "document insert 没有返回 id"
fi

api_step "trigger-list" 200 GET "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers"
expect '.dropped == 0 and (.items | map(select(.id == $id)) | length == 1)' --arg id "${TID:-}"

# ── 4. http 触发器：显式 invoke 入队 ─────────────────────
api_step "http-trigger-create" 201 POST "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers" \
  "$(jq -nc --arg fn "$FN" '{kind:"http", func_file:$fn, func_export:"Handle", enabled:true}')"
expect '.id != "" and .kind == "http"'
TID2="$(jq_out '.id')"
if [[ -n "$TID2" && "$TID2" != "null" ]]; then
  TRIGGER_IDS+=("$TID2")
fi

api_step "http-trigger-invoke" 202 POST "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers/${TID2:-missing}/invoke" \
  '{"source":"capability-example"}'
expect '.queued == true'
sink_step "http-invoke-fired" "action=http"

api_step "trigger-update" 200 PATCH "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers/${TID2:-missing}" \
  '{"enabled":false}'
expect '.enabled == false'

api_step "trigger-delete-record" 204 DELETE "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers/${TID:-missing}"
done_step
api_step "trigger-delete-http" 204 DELETE "/v1/projects/$SIMPLEBASE_PROJECT_ID/function-triggers/${TID2:-missing}"
done_step
TRIGGER_IDS=()

step "function-delete" function delete --name "$FN"
done_step

if [[ "$CREATED_DB" == "1" ]]; then
  step "delete-db" database delete --id "$DB"
  done_step
  CREATED_DB=0
fi

finish_case
