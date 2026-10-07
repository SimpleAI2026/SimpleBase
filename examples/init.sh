#!/usr/bin/env bash
# 清单驱动初始化；管理凭据仅传给进程，不写入场景 .env。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
command -v curl >/dev/null || { echo '需要 curl' >&2; exit 1; }
command -v node >/dev/null || { echo '需要 node' >&2; exit 1; }
exec node "$ROOT/examples/lib/init/run.mjs" "$@"
