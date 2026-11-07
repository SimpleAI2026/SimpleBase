#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 可选能力：OpenAI 兼容接口（/v1/projects/:p/v1/models、/chat/completions）。
# 依赖服务端配置 LLM provider（外部网络）。未启用 / 无模型 / token 无 llm:invoke /
# provider 暂不可用时整体 SKIP exit 0，不计 FAIL。

BASE="$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/v1"

skip_case() {
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

# llm_probe：非 0 退出时把响应体放到 LLM_BODY（供调用方判断 skip 原因）。
llm_probe() { # llm_probe <method> <path> [json-body]
  local method="$1" path="$2" body="${3:-}"
  local outf code
  outf="$(mktemp)"
  set +e
  if [[ -n "$body" ]]; then
    code="$(curl -sS -o "$outf" -w '%{http_code}' --max-time 120 -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H 'content-type: application/json' \
      --data "$body" 2>"$TMPDIR_CASE/curl.err")"
  else
    code="$(curl -sS -o "$outf" -w '%{http_code}' --max-time 120 -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $SIMPLEBASE_TOKEN" 2>"$TMPDIR_CASE/curl.err")"
  fi
  set -e
  LLM_CODE="${code:-0}"
  LLM_BODY="$(cat "$outf")"
  rm -f "$outf"
}

record_assert() {
  STEP_NAME="$1"
  STEP_OK="$2"
  STEP_WHY="$3"
  STEP_CMD="${4:-}"
  STEP_MS=0
  record "$STEP_OK" "$STEP_WHY"
}

TMPDIR_CASE="$(mktemp -d "${TMPDIR:-/tmp}/sb-llm.XXXXXX")"
trap 'rm -rf "$TMPDIR_CASE"' EXIT

# ── 0. 前置：models 可用且至少一个模型 ─────────────────────
llm_probe GET /models
if [[ "$LLM_CODE" != "200" ]]; then
  CODE="$(printf '%s' "$LLM_BODY" | jq -r '.error.code // empty' 2>/dev/null || true)"
  skip_case "OpenAI 兼容接口不可用或 token 无 llm:invoke（http $LLM_CODE${CODE:+, code=$CODE}）"
fi
MODEL="$(printf '%s' "$LLM_BODY" | jq -r '.data[0].id // empty' 2>/dev/null || true)"
if [[ -z "$MODEL" ]]; then
  skip_case "项目没有可用模型（LLM provider 未配置或模型列表为空）"
fi

STEP_NAME="models"
STEP_CMD="curl $BASE/models"
STEP_MS=0
STEP_OK=0
STEP_WHY=""
if printf '%s' "$LLM_BODY" | jq -e '.object == "list" and (.data | type == "array") and (.data | length) >= 1 and (.data[0].id != "")' >/dev/null 2>&1; then
  STEP_OK=1
else
  STEP_WHY="模型列表不是 OpenAI 形状: $(printf '%s' "$LLM_BODY" | redact)"
fi
record "$STEP_OK" "$STEP_WHY"

# ── 1. 非流式 chat completion（最多重试 3 次；上游不可用则 SKIP）──
CHAT_BODY="$(jq -nc --arg m "$MODEL" '{model:$m, messages:[{role:"user",content:"Reply with exactly: pong"}], max_tokens:32}')"
CHAT_OK=0
CHAT_WHY=""
for attempt in 1 2 3; do
  llm_probe POST /chat/completions "$CHAT_BODY"
  if [[ "$LLM_CODE" == "200" ]] && printf '%s' "$LLM_BODY" | jq -e '.object == "chat.completion" and .choices[0].message.role == "assistant" and (.choices[0].message.content | length) > 0 and .usage.total_tokens > 0' >/dev/null 2>&1; then
    CHAT_OK=1
    break
  fi
  UP_CODE="$(printf '%s' "$LLM_BODY" | jq -r '.error.code // empty' 2>/dev/null || true)"
  CHAT_WHY="http $LLM_CODE${UP_CODE:+, code=$UP_CODE}"
  if [[ "$UP_CODE" == "llm_upstream_error" || "$UP_CODE" == "llm_provider_unavailable" || "$LLM_CODE" == "502" || "$LLM_CODE" == "504" ]]; then
    skip_case "LLM provider 暂不可用（${CHAT_WHY}），跳过外部依赖断言"
  fi
  sleep 1
done
STEP_NAME="chat-completion"
STEP_CMD="curl -X POST $BASE/chat/completions"
STEP_MS=0
STEP_OK="$CHAT_OK"
STEP_WHY="$CHAT_WHY"
if [[ "$CHAT_OK" -eq 1 ]]; then
  STEP_WHY=""
fi
record "$STEP_OK" "$STEP_WHY"
CONTENT="$(printf '%s' "$LLM_BODY" | jq -r '.choices[0].message.content' 2>/dev/null || true)"

# ── 2. 流式 chat completion：delta 分块 + [DONE] ──────────
STREAM_FILE="$TMPDIR_CASE/stream.txt"
STREAM_CODE="$(curl -sS -N -o "$STREAM_FILE" -w '%{http_code}' --max-time 120 \
  -X POST "$BASE/chat/completions" -H "Authorization: Bearer $SIMPLEBASE_TOKEN" \
  -H 'content-type: application/json' \
  --data "$(jq -nc --arg m "$MODEL" '{model:$m, messages:[{role:"user",content:"Reply with exactly: pong"}], max_tokens:32, stream:true}')" \
  2>"$TMPDIR_CASE/stream.err" || true)"
STREAM_JSON_OK="$(python3 - "$STREAM_FILE" <<'PY'
import json
import sys

text = ""
try:
    text = open(sys.argv[1], encoding="utf-8", errors="replace").read()
except OSError:
    pass
chunks = []
done = False
for line in text.splitlines():
    if not line.startswith("data: "):
        continue
    raw = line[len("data: "):].strip()
    if raw == "[DONE]":
        done = True
        continue
    try:
        chunks.append(json.loads(raw))
    except json.JSONDecodeError:
        print("bad-json")
        sys.exit(1)
content = "".join(
    (c.get("choices") or [{}])[0].get("delta", {}).get("content") or "" for c in chunks
)
ok = (
    done
    and len(chunks) >= 1
    and all(c.get("object") == "chat.completion.chunk" for c in chunks)
    and len(content) > 0
)
print("ok" if ok else f"bad(frames={len(chunks)},done={done},content={len(content)})")
PY
)"
STEP_NAME="chat-stream"
STEP_CMD="curl -N -X POST $BASE/chat/completions (stream=true)"
STEP_MS=0
STEP_OK=0
STEP_WHY=""
if [[ "$STREAM_CODE" == "200" && "$STREAM_JSON_OK" == "ok" ]]; then
  STEP_OK=1
else
  STEP_WHY="http $STREAM_CODE stream=$STREAM_JSON_OK: $(head -c 200 "$STREAM_FILE" 2>/dev/null | redact)"
fi
record "$STEP_OK" "$STEP_WHY"

# ── 3. 鉴权：不带 Key 返回 401 ────────────────────────────
NOAUTH_CODE="$(curl -sS -o "$TMPDIR_CASE/noauth.json" -w '%{http_code}' --max-time 20 \
  -X POST "$BASE/chat/completions" -H 'content-type: application/json' \
  -d "$CHAT_BODY" 2>/dev/null || true)"
STEP_NAME="no-auth-401"
STEP_CMD="curl -X POST $BASE/chat/completions (no auth)"
STEP_MS=0
STEP_OK=0
STEP_WHY=""
if [[ "$NOAUTH_CODE" == "401" ]] && jq -e '.error.code == "unauthenticated"' "$TMPDIR_CASE/noauth.json" >/dev/null 2>&1; then
  STEP_OK=1
else
  STEP_WHY="期望 401 unauthenticated，实际 http=$NOAUTH_CODE $(redact <"$TMPDIR_CASE/noauth.json" 2>/dev/null || true)"
fi
record "$STEP_OK" "$STEP_WHY"

finish_case
