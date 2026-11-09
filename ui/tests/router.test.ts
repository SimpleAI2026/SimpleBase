import { describe, expect, it } from 'vitest'
import router from '@/router/index'
import { setI18nLocale, t } from '@/locales'

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
    await router.push('/console')
    expect(router.currentRoute.value.name).toBe('agents')
    expect(router.currentRoute.value.path).toBe('/console/agents')
    await router.push('/console?settings=1')
    expect(router.currentRoute.value.name).toBe('agents')
    expect(router.currentRoute.value.fullPath).toBe('/console/agents?settings=1')
    await router.push({ name: 'dashboard' })
    expect(router.currentRoute.value.path).toBe('/console/dashboard')
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
      ['/data', '/console/data'],
      ['/llm', '/console/agents']
    ]) {
      await router.push(`${source}?from=bookmark`)
      expect(router.currentRoute.value.fullPath).toBe(`${target}?from=bookmark`)
    }
    await router.push('/console/databases?from=bookmark')
    expect(router.currentRoute.value.fullPath).toBe('/console/databases?from=bookmark')
    await router.push('/console/dashboard?from=bookmark')
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.fullPath).toBe('/console/dashboard?from=bookmark')
    await router.push('/settings?settings=appearance&from=bookmark')
    expect(router.currentRoute.value.fullPath).toBe('/console/agents?settings=appearance&from=bookmark')
    await router.push('/?settings=models&from=bookmark')
    expect(router.currentRoute.value.fullPath).toBe('/console/agents?settings=models&from=bookmark')
    await router.push('/')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('assigns i18n title keys to the seven console feature areas', () => {
    const keyOf = (name: string) => router.getRoutes().find((r) => r.name === name)?.meta?.titleKey
    expect(keyOf('dashboard')).toBe('nav.dashboard')
    expect(keyOf('databases')).toBe('nav.databases')
    expect(keyOf('s3')).toBe('nav.s3')
    expect(keyOf('gofunctions')).toBe('nav.gofunctions')
    expect(keyOf('cron-jobs')).toBe('nav.cronJobs')
    expect(keyOf('agents')).toBe('nav.agents')
    expect(keyOf('logs')).toBe('nav.logs')
    // 路由只存 key，标题文案在语言包里；中文界面下求值就是原来的中文标题。
    setI18nLocale('zh-CN')
    expect(t(keyOf('dashboard') as string)).toBe('监控大盘')
    expect(t(keyOf('databases') as string)).toBe('数据管理')
    setI18nLocale('en-US')
    expect(t(keyOf('dashboard') as string)).toBe('Dashboard')
    setI18nLocale('zh-CN')
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
