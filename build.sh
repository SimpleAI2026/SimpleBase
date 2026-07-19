#!/usr/bin/env bash
# SimpleBase 一体化构建脚本。
#
# 流程：
#   1. 安装并构建前端（ui/）→ 输出到 ui/dist
#   2. 将 ui/dist 同步到 internal/web/dist（供 go:embed 嵌入后端二进制）
#   3. 编译后端 ./cmd/simplebased → 输出到 ./simplebased
#
# 产物 ./simplebased 内嵌管理端静态资源，单进程同时提供 API 与管理界面。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$ROOT_DIR/ui"
WEB_DIST_DIR="$ROOT_DIR/internal/web/dist"
OUTPUT_BIN="$ROOT_DIR/simplebased"

echo "==> [1/3] 构建前端 (vite build)"
cd "$UI_DIR"
if [ ! -d node_modules ]; then
  echo "    node_modules 缺失，执行 yarn install..."
  yarn install
fi
yarn build

echo "==> [2/3] 同步前端产物到 internal/web/dist"
rm -rf "$WEB_DIST_DIR"
mkdir -p "$WEB_DIST_DIR"
cp -R "$UI_DIR/dist/." "$WEB_DIST_DIR/"
echo "    已复制 $(find "$WEB_DIST_DIR" -type f | wc -l | tr -d ' ') 个文件"

echo "==> [3/3] 编译后端 (go build)"
cd "$ROOT_DIR"
go build -o "$OUTPUT_BIN" ./cmd/simplebased
echo "    产物: $OUTPUT_BIN"

echo "==> 构建完成: $OUTPUT_BIN"
