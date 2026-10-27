#!/usr/bin/env bash
# 云助手 SSE 用例，以及一次本地 gocloudagent 二进制探测。
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
if [[ "${SKIP_SSE:-0}" != "1" ]]; then
  node "$ROOT/examples/capabilities/agent/run.mjs" || node_fail=1
fi

# 本地 gocloudagent 走服务端的 OpenAI 兼容出口，本机工具是 shell/文件，不是 @skill。
# headless 下破坏性 shell 不能拿到界面确认。这里只要求它调用 simplebase database list。
local_bin="$ROOT/build/bin/gocloudagent"
# 必须是普通可执行文件：仅用 -x 会把可进入的目录（如模块源码目录）也判成可执行。
if [[ -f "$local_bin" && -x "$local_bin" ]]; then
  echo "==> local gocloudagent"
  home="$(mktemp -d /tmp/simplebase-ca.XXXXXX)"
  chmod 700 "$home"
  # shell 工具会丢掉 SIMPLEBASE_* / GOCLOUDAGENT_*。CLI 改读 XDG 配置，且不要把配置内容写进结果。
  cfg_root="$(mktemp -d /tmp/simplebase-xdg.XXXXXX)"
  chmod 700 "$cfg_root"
  mkdir -p "$cfg_root/simplebase"
  chmod 700 "$cfg_root/simplebase"
  CFG="$cfg_root" python3 - <<'PY'
import json, os
root = os.environ["CFG"]
path = os.path.join(root, "simplebase", "config.json")
with open(path, "w", encoding="utf-8") as f:
    json.dump({
        "endpoint": os.environ["SIMPLEBASE_URL"],
        "token": os.environ["SIMPLEBASE_TOKEN"],
        "project_id": os.environ["SIMPLEBASE_PROJECT_ID"],
        "output": "json",
    }, f)
os.chmod(path, 0o600)
PY
  out="$(mktemp)"
  start=$(date +%s%3N)
  set +e
  GOCLOUDAGENT_HOME="$home" \
    GOCLOUDAGENT_MODEL="${GOCLOUDAGENT_MODEL:-${SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL:-zai-org/GLM-5.3-Flash}}" \
    GOCLOUDAGENT_ENDPOINT="$SIMPLEBASE_URL" \
    GOCLOUDAGENT_TOKEN="$SIMPLEBASE_TOKEN" \
    GOCLOUDAGENT_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" \
    XDG_CONFIG_HOME="$cfg_root" \
    timeout 180 "$local_bin" --json -p "请只用 shell 工具执行 ${SIMPLEBASE_BIN} database list。凭据不在环境变量里（shell 会剥离 SIMPLEBASE_*）。不要执行 env、printenv，不要读取或打印任何配置文件。根据命令输出的 JSON 告诉我 ok 是否为 true。" \
    >"$out" 2>"$out.err"
  code=$?
  set -e
  end=$(date +%s%3N)
  set +e
python3 - "$RESULT_FILE" "$out" "$out.err" "$code" "$((end - start))" <<'PY'
import json, re, sys
path, outp, errp, code, ms = sys.argv[1:]
raw = open(outp, encoding="utf-8", errors="replace").read()
err = open(errp, encoding="utf-8", errors="replace").read()
contents, tools, types = [], [], []
for line in raw.splitlines():
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        ev = json.loads(line)
    except json.JSONDecodeError:
        continue
    if not isinstance(ev, dict):
        continue
    types.append(str(ev.get("type") or ""))
    if ev.get("content"):
        contents.append(str(ev["content"]))
    if ev.get("type") in ("tool_call", "tool_result", "tool_start"):
        tools.append(str(ev.get("name") or ""))
        tools.append(str(ev.get("arguments") or ""))
        tools.append(str(ev.get("content") or ""))
assistant = "".join(contents)
toolblob = "\n".join(tools)
called = ("database" in toolblob and "list" in toolblob) or ("database list" in assistant)
compact = (assistant + "\n" + toolblob).replace(" ", "")
ok_flag = '"ok":true' in compact
ok = str(code) == "0" and called and ok_flag
detail = ""
if not ok:
    detail = f"exit={code} called={called} envelope={ok_flag} events={','.join(types[-12:])} stderr={err[-180:]}"
    detail = re.sub(r"sb_live_[A-Za-z0-9_\-]+", "sb_live_[redacted]", detail)
    detail = re.sub(r"eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+", "[jwt]", detail)
row = {
  "suite": "local-gocloudagent",
  "case": "local-gocloudagent",
  "step": "database-list",
  "command": "gocloudagent --json -p database list",
  "ok": ok,
  "ms": int(ms),
  "detail": detail,
}
open(path, "a", encoding="utf-8").write(json.dumps(row, ensure_ascii=False) + "\n")
print(("PASS" if ok else "FAIL") + f" local-gocloudagent/database-list ({ms}ms) exit={code} called={called} envelope={ok_flag}")
raise SystemExit(0 if ok else 1)
PY
  local_code=$?
  set -e
  rm -rf "$home" "$cfg_root" "$out" "$out.err"
  if [[ "$local_code" -ne 0 ]]; then
    node_fail=1
  fi
else
  python3 - "$RESULT_FILE" <<'PY'
import json, sys
row = {"suite":"local-gocloudagent","case":"local-gocloudagent","step":"binary","command":"gocloudagent","ok":False,"ms":0,"detail":"gocloudagent binary missing"}
open(sys.argv[1], "a", encoding="utf-8").write(json.dumps(row, ensure_ascii=False) + "\n")
print("FAIL local-gocloudagent/binary")
PY
  node_fail=1
fi

exit "$node_fail"
