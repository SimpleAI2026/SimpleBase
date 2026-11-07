#!/usr/bin/env bash
# 云助手 SSE 用例，以及一次本地 gocloudagent 二进制探测。
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
command -v node >/dev/null 2>&1 || { echo "run_agent.sh: 需要 node（云助手用例）；请安装 Node.js 18+ 并加入 PATH" >&2; exit 1; }
# macOS 自带的 date 不支持 GNU 的 +%s%3N，统一用 python3 取毫秒。
now_ms() {
  python3 -c 'import time; print(int(time.time() * 1000))'
}
: "${SIMPLEBASE_URL:?}"
: "${SIMPLEBASE_TOKEN:?}"
: "${SIMPLEBASE_PROJECT_ID:?}"
: "${RUN_ID:?}"
RESULT_FILE="${RESULT_FILE:-/tmp/simplebase-examples/results.jsonl}"
export RESULT_FILE
export SIMPLEBASE_BIN="${SIMPLEBASE_BIN:-$ROOT/simplebase}"

# 云助手与 CLI 技能是服务端可选项（agent.skills_cli）：/agents 缺少用例需要的内置助手时，
# agent 与 local-gocloudagent 整体 skip 并打印原因，不记 FAIL。
agents_json="$(curl -sS --max-time 10 -H "Authorization: Bearer $SIMPLEBASE_TOKEN" \
  "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/agents" 2>/dev/null || true)"
skip_reason="$(AGENTS_JSON="$agents_json" CASES_FILE="$ROOT/examples/capabilities/agent/cases.json" python3 - <<'PY'
import json, os, sys
try:
    data = json.loads(os.environ.get("AGENTS_JSON") or "null")
    cases = json.load(open(os.environ["CASES_FILE"], encoding="utf-8"))
except Exception:
    print("无法读取 /agents（服务端未启用云助手）")
    sys.exit(0)
modules = {a.get("builtin_key") for a in (data or {}).get("agents") or [] if a.get("builtin_key")}
need = sorted({c.get("module") for c in cases if c.get("module")})
missing = [m for m in need if m not in modules]
if missing:
    print("服务端缺少内置助手 " + ",".join(missing) + "（需启用 agent.skills_cli / CLI 技能）")
PY
)"
if [[ -n "$skip_reason" ]]; then
  echo "==> SKIP agent: $skip_reason"
  python3 - "$RESULT_FILE" "$skip_reason" <<'PY'
import json, sys
path, reason = sys.argv[1], sys.argv[2]
for suite in ("agent", "local-gocloudagent"):
    row = {"suite": suite, "case": suite, "step": "skip", "command": "GET /agents",
           "ok": True, "skip": True, "ms": 0, "detail": "skipped: " + reason}
    open(path, "a", encoding="utf-8").write(json.dumps(row, ensure_ascii=False) + "\n")
PY
  exit 0
fi

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
  start=$(now_ms)
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
  end=$(now_ms)
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
