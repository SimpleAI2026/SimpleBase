import { describe, expect, it } from 'vitest'
import { getProviderPreset, LLM_PROVIDER_PRESETS } from '@/constants/llmProviders'

describe('llm provider catalog', () => {
  it('looks up presets by id', () => {
    expect(getProviderPreset('openai')?.name).toBe('OpenAI')
    expect(getProviderPreset('missing')).toBeUndefined()
    expect(LLM_PROVIDER_PRESETS.length).toBeGreaterThanOrEqual(8)
  })

  it('exposes claude and codex presets with matching protocols', () => {
    expect(getProviderPreset('claude')?.protocol).toBe('anthropic_messages')
    expect(getProviderPreset('codex')?.protocol).toBe('openai_chat')
    // 两者都必须有 api_key 字段，否则设置页无法保存连接。
    for (const id of ['claude', 'codex']) {
      const preset = getProviderPreset(id)
      expect(preset?.fields.some((f) => f.key === 'api_key' && f.required)).toBe(true)
      expect(preset?.suggestedModels.length).toBeGreaterThan(0)
    }
  })
})
