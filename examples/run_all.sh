#!/usr/bin/env bash
# 构建（可选）、启动、登录，然后按顺序跑能力示例与云助手。
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
URL="${SIMPLEBASE_URL:-http://127.0.0.1:8080}"
START=0
BUILD=0
SKIP_BUILD=0
SKIP_AGENT=0
FRESH=0
ONLY=""
API_PORT=""
ADMIN_USER="${SIMPLEBASE_ADMIN_USER:-simplebase2026}"
ADMIN_PASSWORD="${SIMPLEBASE_ADMIN_PASSWORD:-simplebase2026}"
RUN_ID="${RUN_ID:-$(python3 -c 'import secrets; print(secrets.token_hex(3))')}"
RESULT_FILE="${RESULT_FILE:-/tmp/simplebase-examples/results.jsonl}"
STARTED_PID=""

usage() {
  echo "usage: $0 [--start] [--build] [--skip-build] [--api-port N] [--skip-agent] [--fresh] [--only a,b] [--url http://127.0.0.1:8080]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start) START=1 ;;
    --build) BUILD=1 ;;
    --skip-build) SKIP_BUILD=1 ;;
    --api-port) API_PORT="${2:-}"; shift ;;
    --skip-agent) SKIP_AGENT=1 ;;
    --fresh) FRESH=1 ;;
    --only) ONLY="${2:-}"; shift ;;
    --url) URL="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg $1" >&2; usage; exit 2 ;;
  esac
  shift
done

if [[ -n "$API_PORT" ]]; then
  case "$API_PORT" in
    *[!0-9]*) echo "--api-port must be a number, got: $API_PORT" >&2; exit 2 ;;
  esac
  URL="http://127.0.0.1:${API_PORT}"
fi
# --start 沿用旧行为：先 ./build.sh；显式 --skip-build 时跳过。
if [[ "$START" -eq 1 && "$SKIP_BUILD" -eq 0 ]]; then
  BUILD=1
fi
RUN_AGENT=0
if [[ "$SKIP_AGENT" -eq 0 && ( -z "$ONLY" || "$ONLY" == *agent* ) ]]; then
  RUN_AGENT=1
fi
if [[ "$RUN_AGENT" -eq 1 ]] && ! command -v node >/dev/null 2>&1; then
  echo "run_all.sh: 云助手用例需要 node；请安装 Node.js 18+ 并加入 PATH，或加 --skip-agent" >&2
  exit 1
fi

export CGO_ENABLED=1
mkdir -p "$(dirname "$RESULT_FILE")" /tmp/simplebase-examples
: >"$RESULT_FILE"

if [[ "$BUILD" -eq 1 ]]; then
  echo "==> ./build.sh"
  (
    cd "$ROOT"
    set -a
    # shellcheck disable=SC1091
    [[ -f .env ]] && source .env
    set +a
    ./build.sh
  ) || { echo "build failed" >&2; exit 1; }
fi

# 返回 /health/live 的 HTTP 状态码；连不上时 curl 会输出 000。
health_status() {
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$URL/health/live" 2>/dev/null)" || true
  echo "${code:-000}"
}
health() { [[ "$(health_status)" == "200" ]]; }

if [[ "$START" -eq 1 && ! -f "$ROOT/config.yaml" && -f "$ROOT/config.example.yaml" ]]; then
  cp "$ROOT/config.example.yaml" "$ROOT/config.yaml"
fi

if [[ "$START" -eq 1 ]] && ! health; then
  echo "==> start simplebase at $URL"
  (
    cd "$ROOT"
    set -a
    # shellcheck disable=SC1091
    [[ -f .env ]] && source .env
    set +a
    if [[ -n "$API_PORT" ]]; then
      export SIMPLEBASE_HTTP_ADDRESS="127.0.0.1:$API_PORT"
    fi
    export SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT="${SIMPLEBASE_LLM_AGENT_RUN_TIMEOUT:-8m}"
    export SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS="${SIMPLEBASE_LLM_AGENT_MAX_ITERATIONS:-12}"
    if [[ -n "${SIMPLEBASE_LLM_PROVIDER_API_KEY:-}" ]]; then
      export SIMPLEBASE_LLM_PROVIDERS="${SIMPLEBASE_LLM_PROVIDERS:-openai}"
      export SIMPLEBASE_LLM_PROVIDER_OPENAI_API_KEY="$SIMPLEBASE_LLM_PROVIDER_API_KEY"
      export SIMPLEBASE_LLM_PROVIDER_OPENAI_BASE_URL="${SIMPLEBASE_LLM_PROVIDER_BASE_URL:-}"
      export SIMPLEBASE_LLM_PROVIDER_OPENAI_DEFAULT_MODEL="${SIMPLEBASE_LLM_PROVIDER_DEFAULT_MODEL:-}"
      export SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT="${SIMPLEBASE_LLM_PROVIDER_OPENAI_TIMEOUT:-120s}"
    fi
    exec ./simplebase
  ) > /tmp/simplebase-server.log 2>&1 &
  STARTED_PID=$!
  echo "$STARTED_PID" >/tmp/simplebase-server.pid
  for _ in $(seq 1 60); do
    health && break
    sleep 1
  done
  if ! health; then
    echo "server did not become ready at $URL (health/live -> $(health_status)); see /tmp/simplebase-server.log" >&2
    exit 1
  fi
fi

if ! health; then
  echo "simplebase is not reachable at $URL (health/live -> $(health_status); use --start or --url)" >&2
  exit 1
fi

cleanup() {
  if [[ -n "$STARTED_PID" ]]; then
    kill "$STARTED_PID" >/dev/null 2>&1 || true
    wait "$STARTED_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

login_file="$(mktemp)"
chmod 600 "$login_file"
curl -sS --max-time 20 -o "$login_file" -X POST "$URL/v1/auth/login" \
  -H 'content-type: application/json' \
  -d "$(jq -n --arg u "$ADMIN_USER" --arg p "$ADMIN_PASSWORD" '{username:$u,password:$p}')" || true
TOKEN="$(jq -r '.access_token // empty' "$login_file")"
login_code="$(jq -r '.error.code // empty' "$login_file")"
rm -f "$login_file"
if [[ -z "$TOKEN" ]]; then
  echo "login failed code=${login_code:-unknown}" >&2
  exit 1
fi

export SIMPLEBASE_BIN="${SIMPLEBASE_BIN:-$ROOT/simplebase}"
export SIMPLEBASE_URL="$URL"
export SIMPLEBASE_TOKEN="$TOKEN"
export RUN_ID RESULT_FILE
export PROJECT_ID_FILE="/tmp/simplebase-examples/project_id"
# --fresh：丢掉上次 run_all 的项目 ID，让 01-project 新建项目（默认复用仍可访问的项目）。
if [[ "$FRESH" -eq 1 ]]; then rm -f "$PROJECT_ID_FILE"; fi
export FRESH
export SIMPLEBASE_PROJECT_ID="${SIMPLEBASE_PROJECT_ID:-sb-admin}"

# 顺序按 capability 依赖：11-backup 放在 03/04 之前（03/04 结尾 delete 用户数据库会清空项目单文件，
# 备份/ KV 都在同一个文件里）；14/15 是可选用例，前置不满足时打印 SKIP 并退出 0，不计 FAIL。
cases=(01-project 02-auth 11-backup 03-sql 04-documents 05-kv 06-functions 07-storage 08-cron 09-triggers 10-realtime 12-documents-advanced 13-dev-replay 14-sandbox 15-llm)
if [[ -n "$ONLY" ]]; then
  IFS=',' read -r -a cases <<<"$ONLY"
fi

cli_fail=0
for name in "${cases[@]}"; do
  script="$ROOT/examples/capabilities/${name}/run.sh"
  if [[ ! -f "$script" ]]; then
    echo "skip missing $name"
    continue
  fi
  echo "==> $name"
  export CASE="$name"
  if [[ "$name" != "01-project" && -f "$PROJECT_ID_FILE" ]]; then
    SIMPLEBASE_PROJECT_ID="$(cat "$PROJECT_ID_FILE")"
    export SIMPLEBASE_PROJECT_ID
  fi
  if ! bash "$script"; then
    cli_fail=1
  fi
done

agent_fail=0
if [[ "$RUN_AGENT" -eq 1 ]]; then
  if [[ -f "$PROJECT_ID_FILE" ]]; then
    SIMPLEBASE_PROJECT_ID="$(cat "$PROJECT_ID_FILE")"
    export SIMPLEBASE_PROJECT_ID
  fi
  echo "==> agent"
  if ! bash "$ROOT/examples/run_agent.sh"; then
    agent_fail=1
  fi
fi

python3 - "$RESULT_FILE" <<'PY'
import json, sys
path = sys.argv[1]
rows = []
for line in open(path, encoding="utf-8"):
    line = line.strip()
    if line:
        rows.append(json.loads(line))
ok = sum(1 for r in rows if r.get("ok"))
bad = [r for r in rows if not r.get("ok")]
print(f"summary ok={ok} fail={len(bad)} total={len(rows)}")
for r in bad:
    print(f"  FAIL {r.get('suite')}/{r.get('case')}/{r.get('step')}")
PY

if [[ "$cli_fail" -ne 0 || "$agent_fail" -ne 0 ]]; then
  exit 1
fi
exit 0
