#!/usr/bin/env bash
# test.sh：一键后端测试入口（planv4.1 §5.5）。
# 默认跑云 Agent 相关包的全量测试（含 fakellm 全链路用例，无需 API key）。
# 用法：
#   ./scripts/test.sh              # 云 Agent 相关包
#   ./scripts/test.sh ./internal/... # 指定包
#   ./scripts/test.sh -race        # 开启竞态检测
set -euo pipefail
cd "$(dirname "$0")/.."

PACKAGES=("${@:-./internal/cloudagent ./internal/systemdb ./internal/api ./internal/llmgateway}")
RACE_FLAG=""
if [[ "${1:-}" == "-race" ]]; then
  RACE_FLAG="-race"
  shift
  PACKAGES=("${@:-./internal/cloudagent ./internal/systemdb ./internal/api ./internal/llmgateway}")
fi

echo "==> go test ${RACE_FLAG} ${PACKAGES[*]}"
go test ${RACE_FLAG} -timeout 300s -count=1 "${PACKAGES[@]}"
echo "==> all tests passed"
