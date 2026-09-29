import { describe, expect, it } from 'vitest'
import { getProviderPreset, LLM_PROVIDER_PRESETS, maskSecret } from './llmProviders'

describe('llm provider catalog', () => {
  it('looks up presets by id', () => {
    expect(getProviderPreset('openai')?.name).toBe('OpenAI')
    expect(getProviderPreset('missing')).toBeUndefined()
    expect(LLM_PROVIDER_PRESETS.length).toBeGreaterThanOrEqual(8)
  })

  it('masks secrets without leaking the full value', () => {
    expect(maskSecret(undefined)).toBe('')
    expect(maskSecret('')).toBe('')
    expect(maskSecret('short')).toBe('••••••••')
    expect(maskSecret('sk-abcdefghijklmnopqrstuvwxyz')).toBe('sk-...wxyz')
  })
})
