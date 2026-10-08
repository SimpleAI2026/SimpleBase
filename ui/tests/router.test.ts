import { describe, expect, it } from 'vitest'
import router from '@/router/index'

describe('router', () => {
  it('registers console pages, docs, and legacy redirects', () => {
    const names = router.getRoutes().map((r) => r.name).filter(Boolean)
    expect(names).toEqual(
      expect.arrayContaining([
        'home',
        'dashboard',
        'databases',
        's3',
        'gofunctions',
        'cron-jobs',
        'agents',
        'settings',
        'logs',
        'users',
        'docs',
        'docs-module',
        'docs-page'
      ])
    )
    const sql = router.getRoutes().find((r) => r.path === '/sql' || r.path.endsWith('/sql'))
    const data = router.getRoutes().find((r) => r.path === '/data' || r.path.endsWith('/data'))
    const llm = router.getRoutes().find((r) => r.path === '/llm' || r.path.endsWith('/llm'))
    expect(sql?.redirect || data?.redirect || llm?.redirect).toBeTruthy()
    const settings = router.getRoutes().find((r) => r.name === 'settings')
    expect(settings?.redirect).toBeTruthy()
    expect(settings?.components?.default).toBeUndefined()
    const docs = router.getRoutes().find((r) => r.path === '/docs')
    expect(docs?.meta?.hidden).toBe(true)
  })

  it('separates the public homepage from the existing console', async () => {
    await router.push('/')
    expect(router.currentRoute.value.name).toBe('home')
    expect(router.currentRoute.value.matched.some((record) => record.path === '/console')).toBe(false)
    await router.push({ name: 'dashboard' })
    expect(router.currentRoute.value.path).toBe('/console')
    expect(router.currentRoute.value.name).toBe('dashboard')
  })

  it('keeps old console links and settings deep-links working', { timeout: 15000 }, async () => {
    const oldPaths = ['databases', 'key-value', 's3', 'gofunctions', 'cron-jobs', 'agents', 'logs', 'users']
    for (const path of oldPaths) {
      await router.push(`/${path}?from=bookmark`)
      expect(router.currentRoute.value.fullPath).toBe(`/console/${path}?from=bookmark`)
    }
    for (const [source, target] of [
      ['/sql', '/console/databases'],
      ['/data', '/console/databases'],
      ['/llm', '/console/agents']
    ]) {
      await router.push(`${source}?from=bookmark`)
      expect(router.currentRoute.value.fullPath).toBe(`${target}?from=bookmark`)
    }
    await router.push('/settings?settings=appearance&from=bookmark')
    expect(router.currentRoute.value.fullPath).toBe('/console?settings=appearance&from=bookmark')
    await router.push('/?settings=models&from=bookmark')
    expect(router.currentRoute.value.fullPath).toBe('/console?settings=models&from=bookmark')
    await router.push('/')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('assigns Chinese titles to the seven console feature areas', () => {
    const titleOf = (name: string) => router.getRoutes().find((r) => r.name === name)?.meta?.title
    expect(titleOf('dashboard')).toBe('监控大盘')
    expect(titleOf('databases')).toBe('数据库管理')
    expect(titleOf('s3')).toBe('对象存储')
    expect(titleOf('gofunctions')).toBe('云函数')
    expect(titleOf('cron-jobs')).toBe('定时任务')
    expect(titleOf('agents')).toBe('云助手')
    expect(titleOf('logs')).toBe('日志管理')
  })

  it('resolves lazy page loaders', async () => {
    const loaders = router.getRoutes()
      .map((r) => r.components?.default)
      .filter((fn): fn is () => Promise<unknown> => typeof fn === 'function')
    expect(loaders.length).toBeGreaterThan(0)
    const loaded = await Promise.all(loaders.map((fn) => fn()))
    expect(loaded.every(Boolean)).toBe(true)
  }, 20000)
})
