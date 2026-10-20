#!/usr/bin/env bash
# smoke.sh：构建 + 全量冒烟（planv4.1 §5.5）。
# 本地验收：go build 全仓 + go vet + 云 Agent 全链路测试 + 前端构建。
# cloudagent 与 gosandbox 是嵌套独立模块（planv6.2），根目录 ./... 不含它们，需单独进入目录。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> go build ./..."
go build ./...
echo "==> (cd cloudagent) go build + go vet（独立模块）"
(cd cloudagent && GOWORK=off go build ./... && GOWORK=off go vet ./...)
echo "==> (cd gosandbox) go build + go vet（独立模块）"
(cd gosandbox && GOWORK=off go build ./... && GOWORK=off go vet ./...)
echo "==> go vet（云 Agent 相关包）"
go vet ./internal/agentbridge ./internal/systemdb ./internal/api ./internal/llmgateway ./internal/agentjob
echo "==> 云 Agent 全链路测试（fakellm，无需 API key）"
(cd cloudagent && GOWORK=off go test -timeout 300s -count=1 ./...)
go test -timeout 300s -count=1 ./internal/agentbridge ./internal/systemdb ./internal/api ./internal/llmgateway ./internal/agentjob
go test -timeout 300s -count=1 -run TestAgentSkillsE2E ./internal/api

if [[ -d ui && -f ui/package.json ]]; then
  echo "==> 前端构建"
  (cd ui && npm run build)
fi
echo "==> smoke passed"
