#!/usr/bin/env bash
# examples/init.sh — 将 examples/ 下的业务案例一键初始化到 SimpleBase。
#
# 做什么：
#   1. 为每个案例在目标项目（默认 dev-shop）创建同名用户数据库（幂等，可重复执行）
#   2. 生成 examples/<case>/.env（SIMPLEBASE_URL / API_KEY / PROJECT_ID / DATABASE_ID，
#      权限 600；已被根 .gitignore 的 .env 规则忽略，勿提交）
#   3. 构建各案例静态页 dist/（Node 案例 npm run build；Go 案例 goexample/cmd/build）
#   4. --publish 时顺带发布各案例的云函数与定时任务（即各 README 的 publish 步骤）
#
# 不做什么（以及原因）：
#   - 不创建项目 / API Key：项目请在控制台创建；Key 在控制台「设置 → 连接 →
#     签发新 Key」获取（登录态或持 ProjectAdmin 的 Key 均可签发）。DevMode
#     种子 Key 只绑定 dev-shop 项目且权限齐全，因此 DevMode 下所有案例共用该项目。
#   - 不建表：各案例 BFF 启动时 init() 幂等建表（CREATE TABLE IF NOT EXISTS）。
#   - 不上传对象存储（--upload）：静态页私有归档由各案例 README 单独说明。
set -euo pipefail

EXAMPLES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$EXAMPLES_DIR/.." && pwd)"

# 案例清单（不含 _template / shared）。Node 案例与 Go 案例的构建/发布命令不同。
NODE_CASES=(booking-service shop-service ticket-service)
GO_CASES=(community-service mini-game-service)
ALL_CASES=(booking-service shop-service ticket-service community-service mini-game-service)

# base_url 等三项：CLI 参数 > 环境变量 > 默认值（默认对接 ./build.sh dev 的本地实例）
BASE_URL="${SIMPLEBASE_URL:-http://127.0.0.1:8080}"
API_KEY="${SIMPLEBASE_API_KEY:-sb_live_dev_key_12345}"
PROJECT_ID="${SIMPLEBASE_PROJECT_ID:-dev-shop}"
DO_PUBLISH=0
KEY_FROM="${SIMPLEBASE_API_KEY:+SIMPLEBASE_API_KEY}"
declare -a FAILED=()

usage() {
  cat <<'USAGE'
用法:
  ./examples/init.sh [选项]     在 SimpleBase 中初始化全部业务案例

为每个案例在目标项目创建同名用户数据库（幂等），生成 examples/<case>/.env，
并构建案例静态页 dist/。表结构由案例启动时自动创建，无需本脚本处理。

选项:
  --url URL       SimpleBase 地址（默认 http://127.0.0.1:8080；亦可用 SIMPLEBASE_URL）
  --api-key KEY   API Key（默认 DevMode 种子 Key；亦可用 SIMPLEBASE_API_KEY）
  --project ID    项目 ID（默认 dev-shop；亦可用 SIMPLEBASE_PROJECT_ID）
  --publish       数据库就绪后发布各案例云函数与定时任务（Node 案例需 js-sdk）
  -h, --help      显示本帮助并退出

案例（各建一个同名用户数据库）:
  booking-service shop-service ticket-service community-service mini-game-service
USAGE
}

log() { printf '==> %s\n' "$*"; }
die() { printf 'init.sh: %s\n' "$*" >&2; exit 1; }
fail() { FAILED+=("$1"); printf 'init.sh: 警告: %s\n' "$*" >&2; }
short_body() { printf '%s' "$RESP_BODY" | head -c 300; }

case_is_go() { [[ " ${GO_CASES[*]} " == *" $1 "* ]]; }

port_of() {
  case "$1" in
    shop-service) echo 8092 ;;
    booking-service) echo 8093 ;;
    mini-game-service) echo 8094 ;;
    ticket-service) echo 8095 ;;
    community-service) echo 8096 ;;
  esac
}

# env_quote VALUE — 转义双引号内的特殊字符，供 .env 的 export 使用
env_quote() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/\$/\\$/g'; }

# api_call METHOD PATH [JSON_BODY] — 设置 RESP_CODE（HTTP 状态码）与 RESP_BODY
api_call() {
  local method="$1" path="$2" body="${3:-}" out
  local args=(-sS -X "$method" -H "Authorization: Bearer $API_KEY" -w '\n%{http_code}')
  if [ -n "$body" ]; then
    args+=(-H 'Content-Type: application/json' -d "$body")
  fi
  if ! out="$(curl "${args[@]}" "$BASE_URL$path" 2>&1)"; then
    die "请求 $BASE_URL$path 失败：$(printf '%s' "$out" | head -c 300)"
  fi
  RESP_CODE="${out##*$'\n'}"
  RESP_BODY="${out%$'\n'*}"
}

# json_path KEY[.KEY...] — 从 stdin 的 JSON 提取字段值；缺失时退出 1
json_path() {
  node -e '
    const path = process.argv[1].split(".");
    let s = "";
    process.stdin.on("data", (d) => (s += d));
    process.stdin.on("end", () => {
      let v = JSON.parse(s);
      for (const key of path) v = v == null ? undefined : v[key];
      if (v === undefined || v === null) process.exit(1);
      console.log(String(v));
    });
  ' "$1"
}

# db_id_by_name NAME — 按名称在项目下查找数据库，输出其 ID；未找到退出 1
db_id_by_name() {
  local name="$1" cursor="" db_name db_id
  while :; do
    api_call GET "/v1/projects/$PROJECT_ID/databases?limit=200${cursor:+&cursor=$(node -e 'process.stdout.write(encodeURIComponent(process.argv[1]))' "$cursor")}"
    [ "$RESP_CODE" = "200" ] || return 1
    while IFS=$'\t' read -r db_name db_id; do
      if [ "$db_name" = "$name" ]; then
        printf '%s' "$db_id"
        return 0
      fi
    done < <(node -e '
      let s = "";
      process.stdin.on("data", (d) => (s += d));
      process.stdin.on("end", () => {
        const d = JSON.parse(s);
        for (const db of d.databases || []) console.log(db.name + "\t" + db.id);
      });
    ' <<<"$RESP_BODY")
    cursor="$(json_path next_cursor <<<"$RESP_BODY" || true)"
    [ -z "$cursor" ] && return 1
  done
}

# find_or_create_db NAME — 幂等获取（或创建）案例数据库，输出其 ID
find_or_create_db() {
  local name="$1" db_id
  db_id="$(db_id_by_name "$name" || true)"
  if [ -n "$db_id" ]; then
    printf '%s' "$db_id"
    return 0
  fi
  api_call POST "/v1/projects/$PROJECT_ID/databases" "{\"name\":\"$name\"}"
  case "$RESP_CODE" in
    201) json_path id <<<"$RESP_BODY" ;;
    409)
      if db_id="$(db_id_by_name "$name" || true)" && [ -n "$db_id" ]; then
        printf '%s' "$db_id"
      else
        die "数据库 $name 创建冲突且无法解析 ID：$(short_body)"
      fi
      ;;
    *) die "创建数据库 ${name} 失败（HTTP ${RESP_CODE}）：$(short_body)" ;;
  esac
}

# write_env CASE DB_ID — 生成案例环境文件（含密钥，权限 600）
write_env() {
  local file="$EXAMPLES_DIR/$1/.env"
  cat > "$file" <<EOF
# 由 examples/init.sh 生成（含密钥，勿提交）。启动案例前：set -a; . 本文件; set +a
export SIMPLEBASE_URL="$(env_quote "$BASE_URL")"
export SIMPLEBASE_API_KEY="$(env_quote "$API_KEY")"
export SIMPLEBASE_PROJECT_ID="$(env_quote "$PROJECT_ID")"
export SIMPLEBASE_DATABASE_ID="$(env_quote "$2")"
EOF
  chmod 600 "$file"
}

# load_env [CASE] — 在当前（子）shell 导出案例环境变量
load_env() {
  local file
  if [ $# -gt 0 ]; then file="$EXAMPLES_DIR/$1/.env"; else file="./.env"; fi
  set -a
  # shellcheck disable=SC1090
  . "$file"
  set +a
}

# build_dist CASE — 构建案例静态页（失败返回非零，不终止整体）
build_dist() {
  local case="$1"
  if case_is_go "$case"; then
    command -v go >/dev/null 2>&1 || { log "[$case] 未安装 go，跳过 dist 构建"; return 1; }
    (cd "$ROOT_DIR" && go run ./examples/shared/goexample/cmd/build "examples/$case")
  else
    (cd "$EXAMPLES_DIR/$case" && npm run build)
  fi
}

# publish_case CASE — 发布该案例的云函数与定时任务（失败返回非零，不终止整体）
publish_case() {
  local case="$1"
  if case_is_go "$case"; then
    command -v go >/dev/null 2>&1 || { log "[$case] 未安装 go，跳过云函数发布"; return 1; }
    (cd "$ROOT_DIR" && load_env "$case" && go run ./examples/shared/goexample/cmd/publish "$case" "examples/$case")
    return
  fi
  if [ "$case" = "shop-service" ]; then
    (cd "$EXAMPLES_DIR/$case" && load_env && node deploy/publish.mjs)
  else
    (cd "$EXAMPLES_DIR/$case" && load_env && npm run publish)
  fi
}

# ── 参数解析 ──────────────────────────────────────────────
while [ $# -gt 0 ]; do
  case "$1" in
    --url)
      [ $# -ge 2 ] || die "--url 需要一个值"
      BASE_URL="$2"; shift 2 ;;
    --api-key)
      [ $# -ge 2 ] || die "--api-key 需要一个值"
      API_KEY="$2"; KEY_FROM="--api-key 参数"; shift 2 ;;
    --project)
      [ $# -ge 2 ] || die "--project 需要一个值"
      PROJECT_ID="$2"; shift 2 ;;
    --publish) DO_PUBLISH=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数 $1（用 --help 查看用法）" ;;
  esac
done
BASE_URL="${BASE_URL%/}"

command -v curl >/dev/null 2>&1 || die "需要 curl"
command -v node >/dev/null 2>&1 || die "需要 node（JSON 解析与 Node 案例构建）"

# ── 前置校验 ──────────────────────────────────────────────
log "SimpleBase: ${BASE_URL}（项目 ${PROJECT_ID}，Key 来源：${KEY_FROM:-DevMode 种子}）"
if ! curl -sSf "$BASE_URL/health/live" >/dev/null 2>&1; then
  die "无法访问 $BASE_URL/health/live；请先 ./build.sh dev 启动 SimpleBase，或用 --url 指定地址"
fi
api_call GET "/v1/projects/$PROJECT_ID/databases?limit=1"
case "$RESP_CODE" in
  200) : ;;
  401|403) die "API Key 无权访问项目 ${PROJECT_ID}（HTTP ${RESP_CODE}）。
    - DevMode（./build.sh dev）用种子 Key sb_live_dev_key_12345（默认值，无需 --api-key）
    - 生产实例（dev_mode=false）没有种子 Key：登录控制台 → 设置 → 连接 →
      「签发新 Key」获取 sb_live_... 后再传 --api-key；或先用种子流程切换 DevMode" ;;
  404) die "项目 $PROJECT_ID 不存在；检查 --project（示例需普通项目，不要用 admin 系统项目）" ;;
  *) die "初始化前校验失败（HTTP ${RESP_CODE}）：$(short_body)" ;;
esac

if [ "$DO_PUBLISH" = "1" ] && [ ! -f "$ROOT_DIR/packages/js-sdk/dist/index.js" ]; then
  log "构建 packages/js-sdk（Node 案例发布依赖）..."
  (cd "$ROOT_DIR/packages/js-sdk" && npm run build) || die "js-sdk 构建失败；请先在 packages/js-sdk 执行 npm install"
fi

# ── 逐案例初始化 ──────────────────────────────────────────
for case in "${ALL_CASES[@]}"; do
  log "[$case] 初始化用户数据库..."
  db_id="$(find_or_create_db "$case")"
  write_env "$case" "$db_id"
  log "[$case] 数据库 ${case}（ID ${db_id}），已生成 .env"
  if ! build_dist "$case"; then
    fail "$case: dist 构建失败"
  fi
done

# ── 可选：发布云函数与定时任务 ─────────────────────────────
if [ "$DO_PUBLISH" = "1" ]; then
  for case in "${ALL_CASES[@]}"; do
    log "[$case] 发布云函数与定时任务..."
    if ! publish_case "$case"; then
      fail "$case: 云函数发布失败"
    fi
  done
fi

if [ "${#FAILED[@]}" -gt 0 ]; then
  die "部分步骤失败：${FAILED[*]}；其余步骤已完成，可修复后重跑（本脚本幂等）"
fi

# ── 后续指引 ──────────────────────────────────────────────
cat <<EOF

初始化完成。在仓库根目录为每个案例开独立终端启动：

EOF
for case in "${ALL_CASES[@]}"; do
  port="$(port_of "$case")"
  if case_is_go "$case"; then
    printf '  set -a && . examples/%s/.env && set +a\n  go run ./examples/%s/server    # http://127.0.0.1:%s\n\n' "$case" "$case" "$port"
  else
    printf '  set -a && . examples/%s/.env && set +a\n  cd examples/%s && npm start    # http://127.0.0.1:%s\n\n' "$case" "$case" "$port"
  fi
done
cat <<'EOF'
提示：
  - 表结构由案例首次启动时自动创建；消费 cron 运行记录（仅单实例）：启动前设置 EXAMPLE_WORKER=1
  - 未用 --publish 时，云函数与定时任务的发布命令见各案例 README
  - 重新执行本脚本幂等：已存在的数据库直接复用，.env 会按当前参数覆盖
EOF
