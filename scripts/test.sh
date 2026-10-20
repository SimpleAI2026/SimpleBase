#!/usr/bin/env bash
# test.sh：一键后端测试入口（planv4.1 §5.5）。
# 默认跑云 Agent 相关包的全量测试（含 fakellm 全链路用例，无需 API key）。
# 用法：
#   ./scripts/test.sh              # 云 Agent 相关包（根模块 + cloudagent 嵌套模块）
#   ./scripts/test.sh ./internal/... # 指定根模块包（仍会跑 cloudagent 模块）
#   ./scripts/test.sh -race        # 开启竞态检测
set -euo pipefail
cd "$(dirname "$0")/.."

RACE_FLAG=""
if [[ "${1:-}" == "-race" ]]; then
  RACE_FLAG="-race"
  shift
fi

# 默认包列表必须是多个数组元素：写成单个带空格的元素会被 go test 当成一个包名。
if [[ $# -gt 0 ]]; then
  PACKAGES=("$@")
else
  # internal/cloudagent 已迁出为独立模块（planv6.2），这里列根模块仍存在的包。
  PACKAGES=(./internal/agentbridge ./internal/systemdb ./internal/api ./internal/llmgateway ./internal/agentjob)
fi

echo "==> go test ${RACE_FLAG} ${PACKAGES[*]}"
go test ${RACE_FLAG} -timeout 300s -count=1 "${PACKAGES[@]}"

# cloudagent 与 gosandbox 是嵌套独立模块，根目录的 ./... 与上面的包列表都不含它们（planv6.2）。
echo "==> (cd cloudagent) go build + go vet + go test ${RACE_FLAG}"
(cd cloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./... && \
  GOWORK=off go test ${RACE_FLAG} -timeout 300s -count=1 ./...)

echo "==> (cd gosandbox) go test ${RACE_FLAG}"
(cd gosandbox && GOWORK=off go test ${RACE_FLAG} -timeout 300s -count=1 ./...)
echo "==> all tests passed"
