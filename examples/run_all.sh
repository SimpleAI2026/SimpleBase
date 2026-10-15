#!/usr/bin/env bash
# 构建（可选）、启动、登录，然后按顺序跑能力示例与云助手。
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
URL="${SIMPLEBASE_URL:-http://127.0.0.1:8080}"
START=0
BUILD=0
SKIP_AGENT=0
ONLY=""
ADMIN_USER="${SIMPLEBASE_ADMIN_USER:-simplebase2026}"
ADMIN_PASSWORD="${SIMPLEBASE_ADMIN_PASSWORD:-simplebase2026}"
RUN_ID="${RUN_ID:-$(python3 -c 'import secrets; print(secrets.token_hex(3))')}"
RESULT_FILE="${RESULT_FILE:-/tmp/simplebase-examples/results.jsonl}"
STARTED_PID=""

usage() {
  echo "usage: $0 [--start] [--build] [--skip-agent] [--only a,b] [--url http://127.0.0.1:8080]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start) START=1; BUILD=1 ;;
    --build) BUILD=1 ;;
    --skip-agent) SKIP_AGENT=1 ;;
    --only) ONLY="${2:-}"; shift ;;
    --url) URL="${2:-}"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg $1" >&2; usage; exit 2 ;;
  esac
  shift
done

export PATH="/home/ubuntu/.nvm/versions/node/v22.23.3/bin:${PATH}"
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

health() { curl -sf --max-time 2 "$URL/health/live" >/dev/null 2>&1; }

if [[ "$START" -eq 1 && ! -f "$ROOT/config.yaml" && -f "$ROOT/config.example.yaml" ]]; then
  cp "$ROOT/config.example.yaml" "$ROOT/config.yaml"
fi

if [[ "$START" -eq 1 ]] && ! health; then
  echo "==> start simplebase"
  (
    cd "$ROOT"
    set -a
    # shellcheck disable=SC1091
    [[ -f .env ]] && source .env
    set +a
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
    echo "server did not become ready; see /tmp/simplebase-server.log" >&2
    exit 1
  fi
fi

if ! health; then
  echo "simplebase is not reachable at $URL (use --start)" >&2
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
export SIMPLEBASE_PROJECT_ID="${SIMPLEBASE_PROJECT_ID:-sb-admin}"

cases=(01-project 02-auth 03-sql 04-documents 05-kv 06-functions 07-storage 08-cron)
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
if [[ "$SKIP_AGENT" -eq 0 && ( -z "$ONLY" || "$ONLY" == *agent* ) ]]; then
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
