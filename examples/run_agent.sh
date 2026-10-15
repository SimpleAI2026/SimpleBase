#!/usr/bin/env bash
# 云助手 SSE 用例，以及一次本地 cloudagent 二进制探测。
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/home/ubuntu/.nvm/versions/node/v22.23.3/bin:${PATH}"
: "${SIMPLEBASE_URL:?}"
: "${SIMPLEBASE_TOKEN:?}"
: "${SIMPLEBASE_PROJECT_ID:?}"
: "${RUN_ID:?}"
RESULT_FILE="${RESULT_FILE:-/tmp/simplebase-examples/results.jsonl}"
export RESULT_FILE
export SIMPLEBASE_BIN="${SIMPLEBASE_BIN:-$ROOT/simplebase}"

node_fail=0
node "$ROOT/examples/capabilities/agent/run.mjs" || node_fail=1

# 本地 cloudagent 走服务端的 OpenAI 兼容出口，本机工具是 shell/文件，不是 @skill。
# headless 下破坏性 shell 不能拿到界面确认。这里只要求它调用 simplebase database list。
local_bin="$ROOT/cloudagent"
if [[ -x "$local_bin" ]]; then
  echo "==> local cloudagent"
  home="$(mktemp -d /tmp/simplebase-ca.XXXXXX)"
  chmod 700 "$home"
  out="$(mktemp)"
  start=$(date +%s%3N)
  set +e
  CLOUDAGENT_HOME="$home" \
    CLOUDAGENT_MODEL="${CLOUDAGENT_MODEL:-${SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL:-zai-org/GLM-5.3-Flash}}" \
    CLOUDAGENT_ENDPOINT="$SIMPLEBASE_URL" \
    CLOUDAGENT_TOKEN="$SIMPLEBASE_TOKEN" \
    CLOUDAGENT_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" \
    timeout 180 "$local_bin" --json -p "请只用 shell 工具执行 ${SIMPLEBASE_BIN} database list。SIMPLEBASE_URL、SIMPLEBASE_TOKEN、SIMPLEBASE_PROJECT_ID 已在环境中，不要打印它们的值，不要执行 env 或 printenv。根据命令输出的 JSON 告诉我 ok 是否为 true。" \
    >"$out" 2>"$out.err"
  code=$?
  set -e
  end=$(date +%s%3N)
  set +e
python3 - "$RESULT_FILE" "$out" "$out.err" "$code" "$((end - start))" <<'PY'
import json, re, sys
path, outp, errp, code, ms = sys.argv[1:]
text = open(outp, encoding="utf-8", errors="replace").read() + "\n" + open(errp, encoding="utf-8", errors="replace").read()
text = re.sub(r"(secret|token|password|api_key|authorization)(\"?[:\s\"]*)[^\s\",}]+", r"\1\2[redacted]", text, flags=re.I)
text = re.sub(r"sb_live_[A-Za-z0-9_\-]+", "sb_live_[redacted]", text)
text = re.sub(r"eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+", "[jwt]", text)
called = "database" in text and "list" in text and ("shell" in text or "tool_call" in text or "simplebase" in text)
ok_flag = '"ok":true' in text.replace(" ", "") or '"ok": true' in text
ok = code == 0 and called and ok_flag
row = {
  "suite": "local-cloudagent",
  "case": "local-cloudagent",
  "step": "database-list",
  "command": "cloudagent --json -p database list",
  "ok": ok,
  "ms": int(ms),
  "detail": "" if ok else text[-700:],
}
open(path, "a", encoding="utf-8").write(json.dumps(row, ensure_ascii=False) + "\n")
print(("PASS" if ok else "FAIL") + f" local-cloudagent/database-list ({ms}ms) exit={code}")
raise SystemExit(0 if ok else 1)
PY
  local_code=$?
  set -e
  rm -rf "$home" "$out" "$out.err"
  if [[ "$local_code" -ne 0 ]]; then
    node_fail=1
  fi
else
  python3 - "$RESULT_FILE" <<'PY'
import json, sys
row = {"suite":"local-cloudagent","case":"local-cloudagent","step":"binary","command":"cloudagent","ok":False,"ms":0,"detail":"cloudagent binary missing"}
open(sys.argv[1], "a", encoding="utf-8").write(json.dumps(row, ensure_ascii=False) + "\n")
print("FAIL local-cloudagent/binary")
PY
  node_fail=1
fi

exit "$node_fail"
