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

  it('assigns Chinese titles to the seven console feature areas', () => {
    const titleOf = (name: string) => router.getRoutes().find((r) => r.name === name)?.meta?.title
    expect(titleOf('dashboard')).toBe('监控大盘')
    expect(titleOf('databases')).toBe('数据库管理')
    expect(titleOf('s3')).toBe('S3 对象存储')
    expect(titleOf('gofunctions')).toBe('云函数')
    expect(titleOf('cron-jobs')).toBe('定时任务')
    expect(titleOf('agents')).toBe('云 Agent')
    expect(titleOf('logs')).toBe('日志管理')
  })

  it('resolves lazy page loaders', async () => {
    const loaders = router.getRoutes()
      .map((r) => r.components?.default)
      .filter((fn): fn is () => Promise<unknown> => typeof fn === 'function')
    expect(loaders.length).toBeGreaterThan(0)
    const loaded = await Promise.all(loaders.map((fn) => fn()))
    expect(loaded.every(Boolean)).toBe(true)
  })
})
