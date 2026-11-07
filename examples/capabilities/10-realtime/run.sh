#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 文档 Realtime：SSE 订阅集合 create/update/delete 事件。
# 流程：连上 /realtime → 插文档 → 收 record.create → 改订阅 → patch → 收 record.update
#       → 删除 → 收 record.delete → 用 Last-Event-ID 重连验证回放。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
COL="notes_${SUF}"
TMPDIR_CASE="$(mktemp -d "${TMPDIR:-/tmp}/sb-realtime.XXXXXX")"
SSE_FILE="$TMPDIR_CASE/sse.log"
REPLAY_FILE="$TMPDIR_CASE/replay.log"
SSE_PID=""
DB=""
CREATED_DB=0
DOC=""

cleanup() {
  if [[ -n "$SSE_PID" ]]; then
    kill "$SSE_PID" >/dev/null 2>&1 || true
    wait "$SSE_PID" 2>/dev/null || true
  fi
  if [[ "$CREATED_DB" == "1" && -n "$DB" ]]; then
    SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
      SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
      "$SIMPLEBASE_BIN" database delete --id "$DB" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

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

# sse_frame_ok <file> <record_id> <event-name> [data-key=data-value] [frame-no]
# 解析 SSE 帧并断言：匹配 record_id 的帧存在、事件名正确；给出 key=value 时还要求
# data.{key} 相等（data 需 include_data=1）。
sse_frame_ok() {
  local file="$1" rid="$2" event="$3" pair="${4:-}" want="${5:-0}"
  EVENT="$event" RID="$rid" PAIR="$pair" WANT="$want" python3 - "$file" <<'PY'
import json
import os
import sys

path = sys.argv[1]
event = os.environ["EVENT"]
rid = os.environ["RID"]
pair = os.environ.get("PAIR", "")
want = int(os.environ.get("WANT", "0") or "0")
frames = []
name = None
data = None
try:
    lines = open(path, encoding="utf-8", errors="replace").read().splitlines()
except OSError:
    lines = []
for line in lines:
    if line.startswith("event: "):
        name = line[len("event: "):]
    elif line.startswith("data: "):
        raw = line[len("data: "):]
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            payload = None
        if name and name.startswith("record."):
            frames.append({"event": name, "data": payload})
        name = None

hits = [f for f in frames if isinstance(f["data"], dict) and f["data"].get("record_id") == rid and f["event"] == event]
if not hits:
    print(f"no {event} frame for record_id={rid} (frames={[f['event'] for f in frames][-8:]})")
    sys.exit(1)
payload = hits[0]["data"]
for key in ("id", "collection", "action", "at"):
    if key not in payload:
        print(f"event payload missing {key}: {payload}")
        sys.exit(1)
if pair:
    key, expected = pair.split("=", 1)
    actual = payload.get("data", {}).get(key)
    if str(actual) != expected:
        print(f"data.{key}={actual!r} != {expected!r}")
        sys.exit(1)
if want and len(hits) != want:
    print(f"expected {want} frame(s), got {len(hits)}")
    sys.exit(1)
sys.exit(0)
PY
}

wait_sse() { # wait_sse <record_id> <event> [pair] [tries]；每轮 100ms
  local rid="$1" event="$2" pair="${3:-}" tries="${4:-60}" i
  for ((i = 0; i < tries; i++)); do
    if sse_frame_ok "$SSE_FILE" "$rid" "$event" "$pair" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

record_assert() { # record_assert <name> <ok(0|1)> <detail> [ms]
  STEP_NAME="$1"
  STEP_OK="$2"
  STEP_WHY="$3"
  STEP_CMD="${4:-}"
  STEP_MS=0
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
  step "create-db" database create --name "cap10-${SUF}" --data-model collection
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

# ── 2. 建立 SSE 订阅 ──────────────────────────────────────
curl -sS -N --max-time 90 \
  "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/realtime?collections=$COL&events=create,update,delete&include_data=1" \
  -H "Authorization: Bearer $SIMPLEBASE_TOKEN" >"$SSE_FILE" 2>"$TMPDIR_CASE/sse.err" &
SSE_PID=$!

CONNECTED=0
for _ in $(seq 1 50); do
  if grep -q '^event: connect' "$SSE_FILE" 2>/dev/null; then
    CONNECTED=1
    break
  fi
  sleep 0.1
done
if [[ "$CONNECTED" -eq 1 ]]; then
  record_assert "sse-connect" 1 "" "curl -N .../realtime?collections=$COL"
else
  record_assert "sse-connect" 0 "SSE 连接没有收到 connect 帧: $(redact <"$TMPDIR_CASE/sse.err" 2>/dev/null || true)" \
    "curl -N .../realtime?collections=$COL"
  finish_case
fi

CLIENT_ID="$(python3 - "$SSE_FILE" <<'PY'
import json
import sys
try:
    lines = open(sys.argv[1], encoding="utf-8", errors="replace").read().splitlines()
except OSError:
    lines = []
for line in lines:
    if line.startswith("data: "):
        try:
            payload = json.loads(line[len("data: "):])
        except json.JSONDecodeError:
            continue
        if isinstance(payload, dict) and payload.get("client_id"):
            print(payload["client_id"])
            break
PY
)"

# ── 3. create 事件 ────────────────────────────────────────
step "insert-doc" document insert --database "$DB" --collection "$COL" --json '{"title":"rt-live","n":1}'
expect '.data.id != ""'
DOC="$(jq_out '.data.id')"

if [[ -n "$DOC" && "$DOC" != "null" ]]; then
  if wait_sse "$DOC" "record.create" "title=rt-live"; then
    record_assert "live-create" 1 ""
  else
    record_assert "live-create" 0 "5s 内没有收到 record.create（record_id=${DOC}）"
  fi
  if sse_frame_ok "$SSE_FILE" "$DOC" "record.create" "title=rt-live" 1; then
    record_assert "event-payload" 1 ""
  else
    record_assert "event-payload" 0 "事件字段或 include_data 正文不符合预期"
  fi
else
  record_assert "live-create" 0 "document insert 没有返回 id"
  record_assert "event-payload" 0 "document insert 没有返回 id"
fi

# ── 4. 更新订阅 → update 事件 ─────────────────────────────
if [[ -n "$CLIENT_ID" ]]; then
  api_step "subscription-update" 204 POST "/v1/projects/$SIMPLEBASE_PROJECT_ID/realtime/subscriptions" \
    "$(jq -nc --arg cid "$CLIENT_ID" --arg col "$COL" '{client_id:$cid, collections:[$col], events:["update","delete"]}')"
  done_step
else
  record_assert "subscription-update" 0 "connect 帧里没有 client_id"
fi

step "patch-doc" document patch --database "$DB" --collection "$COL" --id "$DOC" --json '{"title":"rt-updated"}'
expect '.data.title == "rt-updated" and .data.n == 1'
if wait_sse "$DOC" "record.update" "title=rt-updated"; then
  record_assert "live-update" 1 ""
else
  record_assert "live-update" 0 "5s 内没有收到 record.update（record_id=${DOC}）"
fi

# ── 5. delete 事件 ────────────────────────────────────────
step "delete-doc" document delete --database "$DB" --collection "$COL" --id "$DOC"
done_step
if wait_sse "$DOC" "record.delete"; then
  record_assert "live-delete" 1 ""
else
  record_assert "live-delete" 0 "5s 内没有收到 record.delete（record_id=${DOC}）"
fi

if [[ -n "$SSE_PID" ]]; then
  kill "$SSE_PID" >/dev/null 2>&1 || true
  wait "$SSE_PID" 2>/dev/null || true
  SSE_PID=""
fi

# ── 6. 断线续传：Last-Event-ID: 0 回放 create 帧 ─────────
set +e
curl -sS -N --max-time 3 -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'Last-Event-ID: 0' \
  "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/realtime?collections=$COL&events=create" \
  >"$REPLAY_FILE" 2>/dev/null
set -e
set +e
sse_frame_ok "$REPLAY_FILE" "$DOC" "record.create" >/dev/null 2>&1
REPLAY_OK=$?
set -e
if [[ "$REPLAY_OK" -eq 0 ]]; then
  record_assert "replay" 1 "" "curl -N -H 'Last-Event-ID: 0' ..."
else
  record_assert "replay" 0 "Last-Event-ID 回放没有带回 create 事件" "curl -N -H 'Last-Event-ID: 0' ..."
fi

# ── 7. 清理 ───────────────────────────────────────────────
if [[ "$CREATED_DB" == "1" ]]; then
  step "delete-db" database delete --id "$DB"
  done_step
  CREATED_DB=0
fi

finish_case
