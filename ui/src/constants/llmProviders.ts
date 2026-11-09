/** 预置 LLM 厂商 Token Plan 模板（前端静态 catalog，见 ui-settings-chat-plan.md §四） */

export type LlmProviderFieldKey =
  | 'api_key'
  | 'base_url'
  | 'organization'
  | 'endpoint'
  | 'deployment'
  | 'api_version'
  | 'default_model'

export interface LlmProviderField {
  key: LlmProviderFieldKey
  label: string
  secret?: boolean
  required?: boolean
  placeholder?: string
}

export interface LlmProviderPreset {
  id: string
  name: string
  protocol: 'openai_chat' | 'anthropic_messages' | 'azure_openai' | 'google_genai' | 'openai_compatible'
  fields: LlmProviderField[]
  suggestedModels: string[]
}

export const LLM_PROVIDER_PRESETS: LlmProviderPreset[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    protocol: 'openai_chat',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true, placeholder: 'sk-...' },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://api.openai.com/v1' },
      { key: 'organization', label: 'Organization' }
    ],
    suggestedModels: ['gpt-4o-mini', 'gpt-4o', 'o4-mini']
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    protocol: 'anthropic_messages',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true, placeholder: 'sk-ant-...' },
      { key: 'base_url', label: 'Base URL' }
    ],
    suggestedModels: ['claude-sonnet-4-5', 'claude-opus-4-5', 'claude-haiku-4-5']
  },
  {
    id: 'azure_openai',
    name: 'Azure OpenAI',
    protocol: 'azure_openai',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'endpoint', label: 'Endpoint', required: true, placeholder: 'https://xxx.openai.azure.com' },
      { key: 'deployment', label: 'Deployment', required: true },
      { key: 'api_version', label: 'API Version', placeholder: '2024-10-21' }
    ],
    suggestedModels: []
  },
  {
    id: 'google',
    name: 'Google Gemini',
    protocol: 'google_genai',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL' }
    ],
    suggestedModels: ['gemini-2.0-flash', 'gemini-2.5-pro']
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://api.deepseek.com' }
    ],
    suggestedModels: ['deepseek-chat', 'deepseek-reasoner']
  },
  {
    id: 'moonshot',
    name: 'Moonshot (Kimi)',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://api.moonshot.cn/v1' }
    ],
    suggestedModels: ['moonshot-v1-auto', 'moonshot-v1-128k']
  },
  {
    id: 'zhipu',
    name: '智谱',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://open.bigmodel.cn/api/paas/v4' }
    ],
    suggestedModels: ['glm-4-flash', 'glm-4-plus']
  },
  {
    id: 'dashscope',
    name: '阿里云百炼',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://dashscope.aliyuncs.com/compatible-mode/v1' }
    ],
    suggestedModels: ['qwen-plus', 'qwen-turbo', 'qwen-max']
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', placeholder: 'https://openrouter.ai/api/v1' }
    ],
    suggestedModels: ['openai/gpt-4o-mini', 'anthropic/claude-sonnet-4']
  },
  {
    id: 'custom_openai',
    name: '自定义 OpenAI 兼容',
    protocol: 'openai_compatible',
    fields: [
      { key: 'api_key', label: 'API Key', secret: true, required: true },
      { key: 'base_url', label: 'Base URL', required: true, placeholder: 'https://example.com/v1' },
      { key: 'default_model', label: '默认模型' }
    ],
    suggestedModels: []
  }
]

export function getProviderPreset(id: string): LlmProviderPreset | undefined {
  return LLM_PROVIDER_PRESETS.find((p) => p.id === id)
}

/** 掩码展示：仅保留首尾，避免完整 key 出现在 UI */
export function maskSecret(value: string | undefined): string {
  if (!value) return ''
  const v = value.trim()
  if (v.length <= 8) return '••••••••'
  return `${v.slice(0, 3)}...${v.slice(-4)}`
}
