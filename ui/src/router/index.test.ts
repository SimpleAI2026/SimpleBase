import { describe, expect, it } from 'vitest'
import router from './index'

describe('router', () => {
  it('registers console pages, docs, and legacy redirects', () => {
    const names = router.getRoutes().map((r) => r.name).filter(Boolean)
    expect(names).toEqual(
      expect.arrayContaining([
        'dashboard',
        'databases',
        's3',
        'gofunctions',
        'cron-jobs',
        'agents',
        'settings',
        'logs',
        'docs',
        'docs-module',
        'docs-page'
      ])
    )
    const sql = router.getRoutes().find((r) => r.path === '/sql' || r.path.endsWith('/sql'))
    const data = router.getRoutes().find((r) => r.path === '/data' || r.path.endsWith('/data'))
    const llm = router.getRoutes().find((r) => r.path === '/llm' || r.path.endsWith('/llm'))
    expect(sql?.redirect || data?.redirect || llm?.redirect).toBeTruthy()
    const docs = router.getRoutes().find((r) => r.path === '/docs')
    expect(docs?.meta?.hidden).toBe(true)
  })
})
