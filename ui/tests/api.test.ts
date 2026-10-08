import { describe, expect, it } from 'vitest'

describe('api entry', () => {
  it('exports httpApi as the only implementation', async () => {
    const mod = await import('@/services/api')
    const { httpApi } = await import('@/services/http-api')
    expect(mod.api).toBe(httpApi)
  })
})
