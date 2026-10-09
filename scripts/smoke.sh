#!/usr/bin/env bash
# smoke.sh：构建 + 全量冒烟（planv4.1 §5.5）。
# 本地验收：go build 全仓 + go vet + 云 Agent 全链路测试 + 前端构建。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> go build ./..."
go build ./...
echo "==> go vet（云 Agent 相关包）"
go vet ./internal/cloudagent ./internal/systemdb ./internal/api ./internal/llmgateway
echo "==> 云 Agent 全链路测试（fakellm，无需 API key）"
go test -timeout 300s -count=1 ./internal/cloudagent ./internal/systemdb ./internal/api ./internal/llmgateway
go test -timeout 300s -count=1 -run TestAgentSkillsE2E ./internal/api

if [[ -d ui && -f ui/package.json ]]; then
  echo "==> 前端构建"
  (cd ui && npm run build)
fi
echo "==> smoke passed"
