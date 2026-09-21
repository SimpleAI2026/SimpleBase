#!/usr/bin/env bash
# SimpleBase 一体化构建 / 本地开发脚本。
#
# 构建流程：
#   1. 安装并构建前端（ui/）→ 输出到 ui/dist
#   2. 将 ui/dist 同步到 internal/web/dist（供 go:embed 嵌入后端二进制）
#   3. 编译后端 ./cmd/simplebased → 输出到 ./simplebased
#
# 开发流程（./build.sh dev）：
#   并行启动后端 (:8080) + Vite 前端 (:5173)，浏览器打开前端地址；
#   API 经 vite proxy 转发到后端，可用完整前后端功能。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$ROOT_DIR/ui"
WEB_DIST_DIR="$ROOT_DIR/internal/web/dist"
OUTPUT_BIN="$ROOT_DIR/simplebased"

# 开发默认：前端入口（经 proxy 访问后端）
DEV_UI_HOST="${SIMPLEBASE_DEV_UI_HOST:-127.0.0.1}"
DEV_UI_PORT="${SIMPLEBASE_DEV_UI_PORT:-5173}"
DEV_API_HOST="${SIMPLEBASE_DEV_API_HOST:-127.0.0.1}"
DEV_API_PORT="${SIMPLEBASE_DEV_API_PORT:-8080}"
DEV_OPEN=1
API_PORT_EXPLICIT=0

usage() {
  cat <<'USAGE'
用法:
  ./build.sh [构建选项]     生产构建（默认）
  ./build.sh dev [开发选项] 启动本地前后端开发环境

构建选项:
  -h, --help     显示本帮助并退出
  --skip-ui      跳过前端构建与同步，仅编译后端
  --ui-only      仅构建前端并同步到 embed 目录，不编译后端

开发选项（跟在 dev 后）:
  -h, --help     显示本帮助并退出
  --no-open      启动后不自动打开浏览器
  --host HOST    前端监听/打开主机（默认 127.0.0.1，也可用 SIMPLEBASE_DEV_UI_HOST）
  --port PORT    前端端口（默认 5173，也可用 SIMPLEBASE_DEV_UI_PORT）
  --api-port PORT  后端端口（默认 8080，也可用 SIMPLEBASE_DEV_API_PORT）

开发说明:
  - 前端: http://HOST:UI_PORT （Vite，代理 /v1 /health → 后端）
  - 后端: http://API_HOST:API_PORT （go run ./cmd/simplebased）
  - 自动加载仓库根目录 .env（若存在；主要用于密钥）
  - 非密钥配置以 config.yaml 为准；环境变量覆盖 YAML
  - 缺少 config.yaml 时从 config.example.yaml 生成本地副本（gitignore）
  - SIMPLEBASE_DEV_MODE 仅在 yaml/.env 都未设置时默认 true
  - SIMPLEBASE_LOG_LEVEL 仅在 yaml/.env 都未设置时默认 debug（生产仍为 info）
  - --api-port 会导出 SIMPLEBASE_HTTP_ADDRESS；未传则尊重 yaml http.address
  - 后端日志输出到终端 stdout/stderr（同时写入临时文件，供启动失败排查）
  - DevMode 种子 Key: sb_live_dev_key_12345
  - Ctrl+C 结束前后端进程

环境要求: yarn、go；开发另需 curl（探测就绪）、tee（后端日志）
默认构建产物: ./simplebased
USAGE
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "错误: 未找到命令 '$1'，请先安装后再运行 build.sh" >&2
    exit 1
  fi
}

yaml_scalar_under() {
  # yaml_scalar_under FILE SECTION KEY — very small nested scalar reader (no yq).
  local file="$1"
  local section="$2"
  local key="$3"
  [[ -f "$file" ]] || return 1
  awk -v section="$section" -v key="$key" '
    BEGIN { insec=0 }
    /^[[:space:]]*#/ { next }
    $0 ~ "^"section":[[:space:]]*$" { insec=1; next }
    insec && /^[^[:space:]#]/ { insec=0 }
    insec && $0 ~ "^[[:space:]]+"key":[[:space:]]*" {
      line=$0
      sub("^[[:space:]]+"key":[[:space:]]*", "", line)
      gsub(/["\047]/, "", line)
      sub(/[[:space:]]+#.*$/, "", line)
      gsub(/[[:space:]]+$/, "", line)
      print line
      exit
    }
  ' "$file"
}

yaml_top_scalar() {
  local file="$1"
  local key="$2"
  [[ -f "$file" ]] || return 1
  awk -v key="$key" '
    /^[[:space:]]*#/ { next }
    $0 ~ "^"key":[[:space:]]*" {
      line=$0
      sub("^"key":[[:space:]]*", "", line)
      gsub(/["\047]/, "", line)
      sub(/[[:space:]]+#.*$/, "", line)
      gsub(/[[:space:]]+$/, "", line)
      print line
      exit
    }
  ' "$file"
}

ensure_dev_config() {
  local src="$ROOT_DIR/config.example.yaml"
  local dst="$ROOT_DIR/config.yaml"
  if [[ -f "$dst" ]]; then
    return 0
  fi
  if [[ ! -f "$src" ]]; then
    echo "警告: 未找到 config.yaml 与 config.example.yaml。请确保 AUTH 等必填项已在环境中配置。" >&2
    return 0
  fi
  echo "==> 未找到 config.yaml，从 config.example.yaml 生成本地开发副本"
  cp "$src" "$dst"
  local tmp
  tmp="$(mktemp "${TMPDIR:-/tmp}/simplebase-config.XXXXXX")"
  awk '
    BEGIN { done_dev=0; done_lvl=0 }
    /^dev_mode:[[:space:]]*false/ && !done_dev { sub(/false/, "true"); done_dev=1 }
    /^[[:space:]]*log_level:[[:space:]]*info/ && !done_lvl { sub(/info/, "debug"); done_lvl=1 }
    { print }
  ' "$dst" > "$tmp"
  mv "$tmp" "$dst"
  if grep -qE '^[[:space:]]*api_key_hash_secret:[[:space:]]*""' "$dst" && [[ -z "${SIMPLEBASE_AUTH_APIKEY_SECRET:-}" ]]; then
    local secret
    secret="$(openssl rand -hex 16 2>/dev/null || printf 'dev-local-%s' "$$")"
    tmp="$(mktemp "${TMPDIR:-/tmp}/simplebase-config.XXXXXX")"
    awk -v secret="$secret" '
      /^[[:space:]]*api_key_hash_secret:[[:space:]]*""/ && !done {
        sub(/""/, "\"" secret "\"")
        done=1
      }
      { print }
    ' "$dst" > "$tmp"
    mv "$tmp" "$dst"
    echo "    已写入本地 api_key_hash_secret（文件已被 gitignore）"
  fi
}

load_dotenv() {
  local env_file="$ROOT_DIR/.env"
  if [[ -f "$env_file" ]]; then
    echo "==> 加载 $env_file"
    set -a
    # shellcheck disable=SC1090
    source "$env_file"
    set +a
  else
    echo "==> 未找到 .env，使用当前环境变量 / config.yaml"
  fi
}

open_url() {
  local url="$1"
  if [[ "$DEV_OPEN" -ne 1 ]]; then
    return 0
  fi
  if command -v open >/dev/null 2>&1; then
    open "$url" >/dev/null 2>&1 || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 || true
  else
    echo "    （无 open/xdg-open，请手动打开 $url）"
  fi
}

wait_http() {
  local url="$1"
  local name="$2"
  local tries="${3:-60}"
  local i
  for ((i = 1; i <= tries; i++)); do
    if curl -sf --max-time 1 "$url" >/dev/null 2>&1; then
      echo "    $name 就绪: $url"
      return 0
    fi
    sleep 0.5
  done
  echo "错误: 等待 $name 就绪超时 ($url)" >&2
  return 1
}

cmd_build() {
  local SKIP_UI=0
  local UI_ONLY=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      --skip-ui)
        SKIP_UI=1
        ;;
      --ui-only)
        UI_ONLY=1
        ;;
      *)
        echo "错误: 未知选项 '$1'" >&2
        echo >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  if [[ "$SKIP_UI" -eq 1 && "$UI_ONLY" -eq 1 ]]; then
    echo "错误: --skip-ui 与 --ui-only 不能同时使用" >&2
    exit 2
  fi

  if [[ "$SKIP_UI" -eq 0 ]]; then
    need_cmd yarn
    if [[ ! -d "$UI_DIR" ]]; then
      echo "错误: 前端目录不存在: $UI_DIR" >&2
      exit 1
    fi
    if [[ ! -f "$UI_DIR/package.json" ]]; then
      echo "错误: 缺少 $UI_DIR/package.json" >&2
      exit 1
    fi

    echo "==> [1/3] 构建前端 (vite build)"
    cd "$UI_DIR"
    if [[ ! -d node_modules ]]; then
      echo "    node_modules 缺失，执行 yarn install..."
      yarn install
    fi
    yarn build

    if [[ ! -f "$UI_DIR/dist/index.html" ]]; then
      echo "错误: 前端构建未产出 $UI_DIR/dist/index.html" >&2
      exit 1
    fi

    echo "==> [2/3] 同步前端产物到 internal/web/dist"
    TMP_DIST="$(mktemp -d "${TMPDIR:-/tmp}/simplebase-web-dist.XXXXXX")"
    cleanup_tmp() { rm -rf "$TMP_DIST"; }
    trap cleanup_tmp EXIT
    cp -R "$UI_DIR/dist/." "$TMP_DIST/"
    rm -rf "$WEB_DIST_DIR"
    mkdir -p "$(dirname "$WEB_DIST_DIR")"
    mv "$TMP_DIST" "$WEB_DIST_DIR"
    trap - EXIT
    echo "    已复制 $(find "$WEB_DIST_DIR" -type f | wc -l | tr -d ' ') 个文件"
  else
    echo "==> 跳过前端构建 (--skip-ui)"
    if [[ ! -f "$WEB_DIST_DIR/index.html" ]]; then
      echo "错误: $WEB_DIST_DIR/index.html 不存在，无法 --skip-ui；请先完整构建一次" >&2
      exit 1
    fi
  fi

  if [[ "$UI_ONLY" -eq 1 ]]; then
    echo "==> 已跳过后端编译 (--ui-only)"
    echo "==> 前端同步完成: $WEB_DIST_DIR"
    exit 0
  fi

  need_cmd go
  if [[ ! -d "$ROOT_DIR/cmd/simplebased" ]]; then
    echo "错误: 后端入口不存在: $ROOT_DIR/cmd/simplebased" >&2
    exit 1
  fi

  echo "==> [3/3] 编译后端 (go build)"
  cd "$ROOT_DIR"
  go build -o "$OUTPUT_BIN" ./cmd/simplebased
  echo "    产物: $OUTPUT_BIN"

  echo "==> 构建完成: $OUTPUT_BIN"
}

cmd_dev() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      --no-open)
        DEV_OPEN=0
        ;;
      --host)
        shift
        [[ $# -gt 0 ]] || { echo "错误: --host 需要参数" >&2; exit 2; }
        DEV_UI_HOST="$1"
        ;;
      --port)
        shift
        [[ $# -gt 0 ]] || { echo "错误: --port 需要参数" >&2; exit 2; }
        DEV_UI_PORT="$1"
        ;;
      --api-port)
        shift
        [[ $# -gt 0 ]] || { echo "错误: --api-port 需要参数" >&2; exit 2; }
        DEV_API_PORT="$1"
        API_PORT_EXPLICIT=1
        ;;
      *)
        echo "错误: 未知开发选项 '$1'" >&2
        echo >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  need_cmd yarn
  need_cmd go
  need_cmd curl
  need_cmd tee

  if [[ ! -f "$UI_DIR/package.json" ]]; then
    echo "错误: 缺少 $UI_DIR/package.json" >&2
    exit 1
  fi
  if [[ ! -d "$ROOT_DIR/cmd/simplebased" ]]; then
    echo "错误: 后端入口不存在: $ROOT_DIR/cmd/simplebased" >&2
    exit 1
  fi

  cd "$ROOT_DIR"
  load_dotenv
  ensure_dev_config

  local cfg="$ROOT_DIR/config.yaml"

  # DevMode：仅在 yaml 与 env 都未设置时默认 true。
  if [[ -z "${SIMPLEBASE_DEV_MODE:-}" ]]; then
    local yaml_dev
    yaml_dev="$(yaml_top_scalar "$cfg" "dev_mode" || true)"
    if [[ -z "$yaml_dev" ]]; then
      export SIMPLEBASE_DEV_MODE=true
    fi
  fi

  # Log level：仅在 yaml 与 env 都未设置时默认 debug，避免永远盖住 yaml。
  if [[ -z "${SIMPLEBASE_LOG_LEVEL:-}" ]]; then
    local yaml_level
    yaml_level="$(yaml_scalar_under "$cfg" "observability" "log_level" || true)"
    if [[ -z "$yaml_level" ]]; then
      export SIMPLEBASE_LOG_LEVEL=debug
    fi
  fi

  # HTTP 地址：--api-port 显式覆盖；否则尊重 env / yaml http.address。
  if [[ "$API_PORT_EXPLICIT" -eq 1 ]]; then
    export SIMPLEBASE_HTTP_ADDRESS=":${DEV_API_PORT}"
  else
    local effective_addr="${SIMPLEBASE_HTTP_ADDRESS:-}"
    if [[ -z "$effective_addr" ]]; then
      effective_addr="$(yaml_scalar_under "$cfg" "http" "address" || true)"
    fi
    if [[ -n "$effective_addr" ]]; then
      local yaml_port="${effective_addr##*:}"
      if [[ "$yaml_port" =~ ^[0-9]+$ ]]; then
        DEV_API_PORT="$yaml_port"
      fi
    else
      export SIMPLEBASE_HTTP_ADDRESS=":${DEV_API_PORT}"
    fi
  fi
  export SIMPLEBASE_DEV_API_PROXY="http://${DEV_API_HOST}:${DEV_API_PORT}"
  echo "==> SIMPLEBASE_DEV_MODE=${SIMPLEBASE_DEV_MODE:-<yaml>}"
  echo "==> log_level=${SIMPLEBASE_LOG_LEVEL:-<yaml observability.log_level>}"
  echo "==> backend ${DEV_API_HOST}:${DEV_API_PORT}"

  if [[ ! -d "$UI_DIR/node_modules" ]]; then
    echo "==> node_modules 缺失，执行 yarn install..."
    (cd "$UI_DIR" && yarn install)
  fi

  local ui_url="http://${DEV_UI_HOST}:${DEV_UI_PORT}"
  local api_url="http://${DEV_API_HOST}:${DEV_API_PORT}"
  local health_url="${api_url}/health/live"

  local backend_pid=""
  local frontend_pid=""
  local log_dir
  log_dir="$(mktemp -d "${TMPDIR:-/tmp}/simplebase-dev-logs.XXXXXX")"
  local backend_log="$log_dir/backend.log"
  local frontend_log="$log_dir/frontend.log"

  # 递归结束进程树（go run / yarn 会再拉子进程）
  kill_tree() {
    local pid="$1"
    local child
    [[ -n "$pid" ]] || return 0
    for child in $(pgrep -P "$pid" 2>/dev/null || true); do
      kill_tree "$child"
    done
    kill "$pid" 2>/dev/null || true
  }

  cleanup_dev() {
    local code=$?
    trap - EXIT INT TERM
    echo
    echo "==> 正在停止开发进程..."
    kill_tree "$frontend_pid"
    kill_tree "$backend_pid"
    sleep 0.3
    # 兜底：按端口强杀残留监听（go run / vite 子进程偶发未收到信号）
    # 故意不再 wait 子进程，避免 macOS bash 在已杀进程上 wait 卡住
    local p pid
    for p in "${DEV_API_PORT}" "${DEV_UI_PORT}"; do
      for pid in $(lsof -nP -iTCP:"$p" -sTCP:LISTEN -t 2>/dev/null || true); do
        kill -9 "$pid" 2>/dev/null || true
      done
    done
    echo "==> 已停止。日志: ${log_dir}"
    exit "$code"
  }
  trap cleanup_dev EXIT INT TERM

  echo "==> 启动后端 ${api_url}"
  echo "    日志: 终端 stdout/stderr（副本 ${backend_log}）"
  (
    cd "$ROOT_DIR"
    # 同时写终端与文件：启动失败时仍可 tail 副本；go run 用于本地热改后重启
    exec go run ./cmd/simplebased
  ) > >(tee "$backend_log") 2>&1 &
  backend_pid=$!

  if ! wait_http "$health_url" "后端" 90; then
    echo "---- 后端日志尾部 ----" >&2
    tail -n 40 "$backend_log" >&2 || true
    exit 1
  fi

  echo "==> 启动前端 ${ui_url} (proxy -> ${api_url})"
  echo "    日志: ${frontend_log}"
  (
    cd "$UI_DIR"
    # --host / --port 覆盖 vite.config；strictPort 避免静默改端口
    exec yarn dev --host "$DEV_UI_HOST" --port "$DEV_UI_PORT" --strictPort
  ) >"$frontend_log" 2>&1 &
  frontend_pid=$!

  if ! wait_http "$ui_url" "前端" 90; then
    echo "---- 前端日志尾部 ----" >&2
    tail -n 40 "$frontend_log" >&2 || true
    exit 1
  fi

  echo
  echo "==> 开发环境已就绪"
  echo "    打开:     ${ui_url}"
  echo "    后端 API: ${api_url}"
  echo "    健康检查: ${health_url}"
  echo "    日志级别: ${SIMPLEBASE_LOG_LEVEL:-config.yaml}（后端输出到终端）"
  echo "    Dev Key:  sb_live_dev_key_12345"
  echo "    停止:     Ctrl+C"
  echo

  open_url "$ui_url"

  # 任一子进程退出则收尾
  while true; do
    if ! kill -0 "$backend_pid" 2>/dev/null; then
      echo "错误: 后端进程已退出，见 ${backend_log}" >&2
      tail -n 40 "${backend_log}" >&2 || true
      exit 1
    fi
    if ! kill -0 "$frontend_pid" 2>/dev/null; then
      echo "错误: 前端进程已退出，见 ${frontend_log}" >&2
      tail -n 40 "${frontend_log}" >&2 || true
      exit 1
    fi
    sleep 1
  done
}

# ── 入口 ──
if [[ $# -gt 0 ]]; then
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    dev)
      shift
      cmd_dev "$@"
      ;;
    build)
      shift
      cmd_build "$@"
      ;;
    *)
      # 兼容旧用法: ./build.sh --skip-ui
      cmd_build "$@"
      ;;
  esac
else
  cmd_build
fi
