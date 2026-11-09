import { describe, expect, it, beforeEach } from 'vitest'
import {
  clearTokens,
  getAccessToken,
  getApiKey,
  getRefreshToken,
  setApiKey,
  setTokens
} from '@/services/http'

describe('http token helpers', () => {
  beforeEach(() => {
    clearTokens()
  })

  it('stores and clears tokens', () => {
    setTokens('acc', 'ref')
    expect(getAccessToken()).toBe('acc')
    expect(getRefreshToken()).toBe('ref')
    clearTokens()
    expect(getAccessToken()).toBe('')
    expect(getRefreshToken()).toBe('')
  })

  it('api key fallback', () => {
    expect(getApiKey()).toBe('') // 无本地 Key 时为空（不再回退内置种子）
    setApiKey('  sb_live_x  ')
    // setApiKey 不 trim（与历史行为一致）
    expect(getApiKey()).toBe('  sb_live_x  ')
  })
})
