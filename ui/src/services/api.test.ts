import { describe, expect, it, vi } from 'vitest'

describe('api selector', () => {
  it('defaults to httpApi when mock is off', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_USE_MOCK', 'false')
    const mod = await import('./api')
    expect(mod.isMock).toBe(false)
    expect(mod.api.projects).toBeDefined()
    vi.unstubAllEnvs()
  })
})
