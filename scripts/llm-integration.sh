#!/usr/bin/env bash
# llm-integration.sh：真实 LLM 供应商集成测试（planv4.1 §5.5）。
# 与默认 fakellm 测试互补：验证真实上游（DeepSeek/OpenAI 兼容）流式与工具协议。
# 前置：
#   - catalog 中已配置 provider（CredentialRef 指向密钥），或导出 LITELLM_API_KEY
#   - go test -tags llm_integration
# 用法：
#   ./scripts/llm-integration.sh                    # 全部集成用例
#   ./scripts/llm-integration.sh -run TestLLMRealStreamAndToolRoundtrip
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ -n "${LITELLM_API_KEY:-}" ]]; then
  echo "==> LITELLM_API_KEY 已设置（不会回显）"
else
  echo "warn: LITELLM_API_KEY 未设置；集成用例将从 catalog provider 配置读取密钥"
fi

echo "==> go test -tags llm_integration ${*:-(./internal/api)}"
go test -tags llm_integration -timeout 300s -count=1 ${*:-./internal/api}
echo "==> llm integration passed"
