#!/usr/bin/env bash
# SimpleBase 一体化构建 / 本地开发脚本。
#
# 构建流程：
#   1. 安装并构建前端（ui/）→ 输出到 ui/dist
#   2. 将 ui/dist 同步到 internal/web/dist（供 go:embed 嵌入后端二进制）
#   3. 编译服务和 CLI ./cmd/simplebase → 输出到 ./simplebase
#
# 开发流程（./build.sh dev）：
#   并行启动后端 (:8080) + Vite 前端 (:5173)，浏览器打开前端地址；
#   API 经 vite proxy 转发到后端，可用完整前后端功能。
#
# 重置（./build.sh reset）：
#   删除 database.cache_dir（默认 .cache）下的全部本地数据。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$ROOT_DIR/ui"
WEB_DIST_DIR="$ROOT_DIR/internal/web/dist"
OUTPUT_BIN="$ROOT_DIR/simplebase"
# gocloudagent CLI 与 gosandbox helper 的产物目录（planv6.2 模块拆分）。
# 不能输出到仓库根的 "gocloudagent"：那里现在是独立模块的源码目录。
CLI_BIN_DIR="$ROOT_DIR/build/bin"

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
  ./build.sh [构建选项]       生产构建（默认）
  ./build.sh dev [开发选项]   启动本地前后端开发环境
  ./build.sh reset [重置选项] 删除本地数据库缓存，清空全部数据

构建选项:
  -h, --help     显示本帮助并退出
  --skip-ui      跳过前端构建与同步，仅编译后端
  --ui-only      仅构建前端并同步到 embed 目录，不编译后端

构建产物:
  simplebase        服务端二进制（cmd/simplebase）
  build/bin/gocloudagent  云 Agent 本地 CLI（gocloudagent/cmd/gocloudagent；会话存
                          ~/.gocloudagent/，模型出口走 simplebase HTTP API，不进服务镜像）
  build/bin/gosandbox-inner  Linux 上的本机 shell 隔离 helper（与 CLI 同目录）

开发选项（跟在 dev 后）:
  -h, --help     显示本帮助并退出
  --no-open      启动后不自动打开浏览器
  --host HOST    前端监听/打开主机（默认 127.0.0.1，也可用 SIMPLEBASE_DEV_UI_HOST）
  --port PORT    前端端口（默认 5173，也可用 SIMPLEBASE_DEV_UI_PORT）
  --api-port PORT  后端端口（默认 8080，也可用 SIMPLEBASE_DEV_API_PORT）

开发说明:
  - 前端: http://HOST:UI_PORT （Vite，代理 /v1 /health → 后端）
  - 后端: http://API_HOST:API_PORT （go run ./cmd/simplebase）
  - 自动加载仓库根目录 .env（若存在；主要用于密钥）
  - 非密钥配置以 config.yaml 为准；环境变量覆盖 YAML
  - 仅 dev 子命令在缺少 config.yaml 时从 config.example.yaml 生成本地副本（gitignore）；
    生产构建 / 直接运行 ./simplebase 不会生成，请先 cp config.example.yaml config.yaml
  - 存储模式由 config.yaml storage.database/files 决定（默认 local，数据落 storage.data_dir）
  - SIMPLEBASE_LOG_LEVEL 仅在 yaml/.env 都未设置时默认 debug（生产仍为 info）
  - --api-port 会导出 SIMPLEBASE_HTTP_ADDRESS；未传则尊重 yaml http.address
  - 后端日志输出到终端 stdout/stderr（同时写入临时文件，供启动失败排查）
  - API Key 请在控制台「设置 → 连接」中重置签发（首次使用请先登录控制台）
  - Ctrl+C 结束前后端进程

重置选项（跟在 reset 后）:
  -h, --help     显示本帮助并退出
  -y, --yes      不询问确认，直接删除
  --remote       同时清理远端对象（仅 storage.database=s3 或 storage.files=s3 时可用，
                 需设置 SIMPLEBASE_ALLOW_RESET=1 由脚本自动注入）

说明:
  reset 会删除 storage.data_dir 与 database.cache_dir（本地持久层是唯一副本），
  不迁移数据。切换存储模式后必须执行一次；
  s3.prefix 为空时远端清理会作用于整个桶，请确保桶由本实例独占。

重置说明:
  - 删除 database.cache_dir 整目录（系统库、用户库 catalog/parquet、dev 本地文件）
  - 路径来源：SIMPLEBASE_DB_CACHE_DIR > config.yaml database.cache_dir > .cache
  - 保留 config.yaml 与 .env；下次 ./build.sh dev 会重建空库并重新种子
  - 不清理远程 S3 对象（远程清理请用 ./simplebase reset --confirm=<instance_id>）
  - 目录被进程占用时拒绝执行，请先 Ctrl+C 停止 ./build.sh dev
  - 交互终端需输入 yes；非交互请加 -y

环境要求: yarn、go；开发另需 curl（探测就绪）、tee（后端日志）；重置占用检测需 lsof
默认构建产物: ./simplebase
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
  # 开发副本默认把存储模式收敛到本地盘（config.example.yaml 已是 local；
  # 这里只确保 log_level 更详细，便于排查）。
  local tmp
  tmp="$(mktemp "${TMPDIR:-/tmp}/simplebase-config.XXXXXX")"
  awk '
    BEGIN { done_lvl=0 }
    /^[[:space:]]*log_level:[[:space:]]*info/ && !done_lvl { sub(/info/, "debug"); done_lvl=1 }
    { print }
  ' "$dst" > "$tmp"
  mv "$tmp" "$dst"
}

# ensure_dev_env 把 AUTH 盐写入 .env（绝不写进 YAML：YAML 里的密钥会被拒绝）。
ensure_dev_env() {
  local env_file="$ROOT_DIR/.env"
  if [[ -n "${SIMPLEBASE_AUTH_APIKEY_SECRET:-}" ]]; then
    return 0
  fi
  local secret=""
  if [[ -f "$env_file" ]]; then
    secret="$(grep -E '^SIMPLEBASE_AUTH_APIKEY_SECRET=' "$env_file" | tail -n 1 | cut -d= -f2- || true)"
  fi
  if [[ -n "$secret" ]]; then
    export SIMPLEBASE_AUTH_APIKEY_SECRET="$secret"
    return 0
  fi
  secret="$(openssl rand -hex 16 2>/dev/null || printf 'dev-local-%s' "$")"
  touch "$env_file"
  printf '\n# 本地开发用 auth 盐（build.sh 自动生成，禁止提交）。\nSIMPLEBASE_AUTH_APIKEY_SECRET=%s\n' "$secret" >> "$env_file"
  export SIMPLEBASE_AUTH_APIKEY_SECRET="$secret"
  echo "    已写入 SIMPLEBASE_AUTH_APIKEY_SECRET 到 .env（文件已被 gitignore）"
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
    # 整目录替换前备份占位文件。它被 git 跟踪，用来让未构建前端时
    # //go:embed all:dist 仍能编译；真实 index.html 决定运行时走已构建控制台。
    KEEP_BACKUP=""
    if [[ -f "$WEB_DIST_DIR/.keep" ]]; then
      KEEP_BACKUP="$(mktemp "${TMPDIR:-/tmp}/simplebase-web-keep.XXXXXX")"
      cp "$WEB_DIST_DIR/.keep" "$KEEP_BACKUP"
    fi
    TMP_DIST="$(mktemp -d "${TMPDIR:-/tmp}/simplebase-web-dist.XXXXXX")"
    cleanup_tmp() {
      rm -rf "$TMP_DIST"
      if [[ -n "$KEEP_BACKUP" ]]; then
        rm -f "$KEEP_BACKUP"
      fi
    }
    trap cleanup_tmp EXIT
    cp -R "$UI_DIR/dist/." "$TMP_DIST/"
    rm -rf "$WEB_DIST_DIR"
    mkdir -p "$(dirname "$WEB_DIST_DIR")"
    mv "$TMP_DIST" "$WEB_DIST_DIR"
    if [[ -n "$KEEP_BACKUP" ]]; then
      cp "$KEEP_BACKUP" "$WEB_DIST_DIR/.keep"
      rm -f "$KEEP_BACKUP"
      KEEP_BACKUP=""
    fi
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
  if [[ ! -d "$ROOT_DIR/cmd/simplebase" ]]; then
    echo "错误: 后端入口不存在: $ROOT_DIR/cmd/simplebase" >&2
    exit 1
  fi

  echo "==> [3/3] 编译后端 (go build)"
  cd "$ROOT_DIR"
  go build -o "$OUTPUT_BIN" ./cmd/simplebase
  echo "    产物: $OUTPUT_BIN"

  # 云 Agent 本地 CLI：独立模块（planv6.2），不进服务镜像（planv6.0 §5.1）。
  # 产物固定到 build/bin：仓库根的 gocloudagent 是模块源码目录。
  if [[ -d "$ROOT_DIR/gocloudagent/cmd/gocloudagent" ]]; then
    if [[ -d "$CLI_BIN_DIR" && ! -w "$CLI_BIN_DIR" ]]; then
      echo "错误: 产物目录不可写: $CLI_BIN_DIR" >&2
      exit 1
    fi
    mkdir -p "$CLI_BIN_DIR"
    local agent_bin="$CLI_BIN_DIR/gocloudagent"
    if [[ -d "$agent_bin" ]]; then
      echo "错误: CLI 产物路径是目录而非文件: $agent_bin" >&2
      exit 1
    fi
    echo "==> 编译 gocloudagent CLI (go build)"
    (cd "$ROOT_DIR/gocloudagent" && go build -o "$agent_bin" ./cmd/gocloudagent)
    echo "    产物: $agent_bin"
    # 本机 shell 隔离的沙盒内 helper（planv6.2）：Linux 上 CLI 在同目录找它；
    # 纯 Go、CGO_ENABLED=0，不链接 DuckDB。macOS 走 sandbox-exec，不需要 helper。
    if [[ "$(uname -s)" == "Linux" && -d "$ROOT_DIR/gosandbox/cmd/gosandbox-inner" ]]; then
      local inner_bin="$CLI_BIN_DIR/gosandbox-inner"
      echo "==> 编译 gosandbox-inner (go build)"
      (cd "$ROOT_DIR/gosandbox" && CGO_ENABLED=0 go build -o "$inner_bin" ./cmd/gosandbox-inner)
      echo "    产物: $inner_bin"
    fi
  fi

  echo "==> 构建完成: $OUTPUT_BIN"
  if [[ ! -f "$ROOT_DIR/config.yaml" && "$UI_ONLY" -eq 0 ]]; then
    echo "    提示: 当前目录没有 config.yaml，直接运行会报 'open config.yaml: no such file or directory'。"
    echo "          请先 cp config.example.yaml config.yaml，或使用 ./build.sh dev（会自动生成）。"
  fi
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
  if [[ ! -d "$ROOT_DIR/cmd/simplebase" ]]; then
    echo "错误: 后端入口不存在: $ROOT_DIR/cmd/simplebase" >&2
    exit 1
  fi

  cd "$ROOT_DIR"
  load_dotenv
  ensure_dev_config
  ensure_dev_env

  local cfg="$ROOT_DIR/config.yaml"

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
  echo "==> storage.database=$(resolve_storage_database 2>/dev/null || echo local) files=$(resolve_storage_files 2>/dev/null || echo local)"
  echo "==> data_dir=$(resolve_data_dir 2>/dev/null || echo '<default>')"
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
  # 远端（storage.database=s3）冷启动要逐表拉 parquet 文件，
  # system→HTTP 监听可达 60-90s；本地模式快得多。等待预算按模式区分。
  local backend_wait_tries=60 # 本地：60 × 0.5s = 30s
  if [[ "$(resolve_storage_database 2>/dev/null || echo local)" == "s3" ]]; then
    backend_wait_tries=300 # 远端：300 × 0.5s = 150s
  fi
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
    exec go run ./cmd/simplebase
  ) > >(tee "$backend_log") 2>&1 &
  backend_pid=$!

  if ! wait_http "$health_url" "后端" "$backend_wait_tries"; then
    echo "错误: 后端 ${api_url} 未在 $((backend_wait_tries / 2))s 内就绪。" >&2
    echo "    远端 S3 部署首次启动需逐表拉取数据文件，可能较慢；" >&2
    echo "    若日志仍在推进（无 Fatal/panic），可加大等待或直接复用该进程。" >&2
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
  echo "    API Key:  登录控制台后于「设置 → 连接」重置签发"
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

resolve_cache_dir() {
  # 在命令替换中调用，提示必须走 stderr，避免污染路径。
  load_dotenv >&2
  local cfg="$ROOT_DIR/config.yaml"
  local dir="${SIMPLEBASE_DB_CACHE_DIR:-}"
  if [[ -z "$dir" ]]; then
    dir="$(yaml_scalar_under "$cfg" "database" "cache_dir" || true)"
  fi
  if [[ -z "$dir" ]]; then
    dir=".cache"
  fi
  if [[ "$dir" != /* ]]; then
    dir="$ROOT_DIR/$dir"
  fi
  # 去掉尾部斜杠，避免 rm 顺着符号链接删到目标内容
  while [[ "$dir" == */ && "$dir" != "/" ]]; do
    dir="${dir%/}"
  done
  if [[ -d "$dir" && ! -L "$dir" ]]; then
    dir="$(cd "$dir" && pwd -P)"
  else
    local parent base
    parent="$(cd "$(dirname "$dir")" && pwd -P)"
    base="$(basename "$dir")"
    dir="$parent/$base"
  fi
  printf '%s\n' "$dir"
}

resolve_data_dir() {
  # 在命令替换中调用，提示必须走 stderr，避免污染路径。
  load_dotenv >&2
  local cfg="$ROOT_DIR/config.yaml"
  local dir="${SIMPLEBASE_DATA_DIR:-}"
  if [[ -z "$dir" ]]; then
    dir="$(yaml_scalar_under "$cfg" "storage" "data_dir" || true)"
  fi
  if [[ -z "$dir" ]]; then
    dir="./data"
  fi
  if [[ "$dir" != /* ]]; then
    dir="$ROOT_DIR/$dir"
  fi
  while [[ "$dir" == */ && "$dir" != "/" ]]; do
    dir="${dir%/}"
  done
  if [[ -d "$dir" && ! -L "$dir" ]]; then
    dir="$(cd "$dir" && pwd -P)"
  else
    local parent base
    parent="$(cd "$(dirname "$dir")" && pwd -P)"
    base="$(basename "$dir")"
    dir="$parent/$base"
  fi
  printf '%s\n' "$dir"
}

resolve_storage_database() {
  # 返回生效的 DB 持久层模式：local | s3（环境变量 > config.yaml > 默认 local）。
  load_dotenv >&2
  local cfg="$ROOT_DIR/config.yaml"
  local mode="${SIMPLEBASE_STORAGE_DATABASE:-}"
  if [[ -z "$mode" ]]; then
    mode="$(yaml_scalar_under "$cfg" "storage" "database" || true)"
  fi
  if [[ -z "$mode" ]]; then
    mode="local"
  fi
  printf '%s\n' "$(printf '%s' "$mode" | tr '[:upper:]' '[:lower:]')"
}

resolve_storage_files() {
  load_dotenv >&2
  local cfg="$ROOT_DIR/config.yaml"
  local mode="${SIMPLEBASE_STORAGE_FILES:-}"
  if [[ -z "$mode" ]]; then
    mode="$(yaml_scalar_under "$cfg" "storage" "files" || true)"
  fi
  if [[ -z "$mode" ]]; then
    mode="local"
  fi
  printf '%s\n' "$(printf '%s' "$mode" | tr '[:upper:]' '[:lower:]')"
}

resolve_instance_id() {
  # 返回实例 ID：环境变量 > config.yaml > 默认值（与 config 包默认保持一致）。
  load_dotenv >&2
  local cfg="$ROOT_DIR/config.yaml"
  local id="${SIMPLEBASE_INSTANCE_ID:-}"
  if [[ -z "$id" ]]; then
    id="$(yaml_scalar_under "$cfg" "instance" "id" || true)"
  fi
  printf '%s\n' "$id"
}

assert_safe_conf_dir() {
  # 拒绝删除根目录、仓库根、家目录、符号链接，以及包含 config.yaml/.env 的目录。
  local dir="$1"
  local label="$2"
  if [[ -z "$dir" || "$dir" == "/" || "$dir" == "$ROOT_DIR" || "$dir" == "${HOME:-}" ]]; then
    echo "错误: 拒绝删除该路径 ($label): ${dir:-<空>}" >&2
    exit 1
  fi
  case "$ROOT_DIR/" in
    "$dir/"*)
      echo "错误: $label 覆盖了仓库目录，拒绝删除: $dir" >&2
      exit 1
      ;;
  esac
  if [[ -L "$dir" ]]; then
    echo "错误: $label 是符号链接，拒绝删除: $dir -> $(readlink "$dir")" >&2
    exit 1
  fi
  local f
  for f in config.yaml .env; do
    if [[ -e "$dir/$f" ]]; then
      echo "错误: $label 包含 $f，拒绝删除: $dir" >&2
      exit 1
    fi
  done
}

assert_safe_cache_dir() {
  assert_safe_conf_dir "$1" "cache_dir"
}

cache_dir_holders() {
  local dir="$1"
  if ! command -v lsof >/dev/null 2>&1; then
    echo "警告: 未找到 lsof，无法检测占用进程。请确认已停止 ./build.sh dev" >&2
    return 0
  fi
  lsof -t +D "$dir" 2>/dev/null || true
}

cmd_reset() {
  local YES=0
  local REMOTE=0

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -h|--help)
        usage
        exit 0
        ;;
      -y|--yes)
        YES=1
        ;;
      --remote)
        REMOTE=1
        ;;
      *)
        echo "错误: 未知重置选项 '$1'" >&2
        echo >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done

  local cache_dir data_dir db_mode files_mode
  cache_dir="$(resolve_cache_dir)"
  data_dir="$(resolve_data_dir)"
  db_mode="$(resolve_storage_database)"
  files_mode="$(resolve_storage_files)"
  assert_safe_cache_dir "$cache_dir"
  assert_safe_conf_dir "$data_dir" "data_dir"

  echo "==> 将重置本地数据并清空全部数据（不可恢复）"
  echo "    存储模式: database=$db_mode files=$files_mode"
  echo "    数据目录: $data_dir"
  echo "    缓存目录: $cache_dir"
  echo "    提示: 切换存储模式后必须执行本命令；旧数据不会被迁移。"

  # 收集需要清理的目录（去重）。
  local dirs=()
  local d
  for d in "$data_dir" "$cache_dir"; do
    local dup=0
    local existing
    for existing in "${dirs[@]:-}"; do
      [[ "$existing" == "$d" ]] && dup=1
    done
    [[ "$dup" -eq 0 ]] && dirs+=("$d")
  done

  local holders=""
  for d in "${dirs[@]}"; do
    if [[ -n "$(cache_dir_holders "$d")" ]]; then
      holders="$d"
      break
    fi
  done
  if [[ -n "$holders" ]]; then
    echo "错误: $holders 正被进程占用，请先停止后端（Ctrl+C 结束 ./build.sh dev）" >&2
    if command -v lsof >/dev/null 2>&1; then
      lsof +D "$holders" 2>/dev/null | head -n 20 >&2 || true
    fi
    exit 1
  fi

  if command -v du >/dev/null 2>&1; then
    for d in "${dirs[@]}"; do
      [[ -d "$d" ]] || continue
      echo "    大小: $(du -sh "$d" 2>/dev/null | awk '{print $1}' || true)  $d"
    done
  fi

  if [[ "$REMOTE" -eq 1 ]]; then
    if [[ "$db_mode" != "s3" && "$files_mode" != "s3" ]]; then
      echo "错误: --remote 仅在 storage.database=s3 或 storage.files=s3 时可用（当前 database=$db_mode files=$files_mode）" >&2
      exit 2
    fi
    echo "    远端: 将删除 s3 前缀下的全部对象（SIMPLEBASE_ALLOW_RESET=1）"
  fi

  if [[ "$YES" -ne 1 ]]; then
    if [[ ! -t 0 ]]; then
      echo "错误: 非交互终端请加 -y 确认删除" >&2
      exit 2
    fi
    printf '输入 yes 确认删除: '
    local ans=""
    read -r ans
    if [[ "$ans" != "yes" ]]; then
      echo "已取消"
      exit 1
    fi
  fi

  if [[ "$REMOTE" -eq 1 ]]; then
    local instance_id
    instance_id="$(resolve_instance_id)"
    echo "==> 清理远端对象..."
    (
      cd "$ROOT_DIR"
      SIMPLEBASE_ALLOW_RESET=1 go run ./cmd/simplebase reset --confirm="$instance_id"
    ) || { echo "错误: 远端清理失败" >&2; exit 1; }
  fi

  local removed=0
  for d in "${dirs[@]}"; do
    if [[ -d "$d" ]]; then
      rm -rf -- "$d"
      echo "==> 已删除 $d"
      removed=1
    fi
  done
  if [[ "$removed" -eq 0 ]]; then
    echo "==> 目录不存在，无需清理"
  fi
  echo "    已保留 config.yaml 与 .env。下次 ./build.sh dev 会重建空库并重新种子。"
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
    reset)
      shift
      cmd_reset "$@"
      ;;
    *)
      # 兼容旧用法: ./build.sh --skip-ui
      cmd_build "$@"
      ;;
  esac
else
  cmd_build
fi
