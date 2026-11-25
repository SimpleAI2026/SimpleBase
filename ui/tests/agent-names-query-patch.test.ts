import { describe, expect, it, vi } from 'vitest'
import { setI18nLocale } from '@/locales'
import { BUILTIN_SEED_NAMES, builtinAliases, displayAgentName, isSeedName } from '@/utils/agent-names'
import { useQueryPatch } from '@/composables/useQueryPatch'

describe('agent-names', () => {
  it('名字等于后端种子名才算「未改名」', () => {
    expect(isSeedName({ builtin_key: 'database', name: 'Database' })).toBe(true)
    expect(isSeedName({ builtin_key: 'database', name: '我的库助手' })).toBe(false)
    expect(isSeedName({ builtin_key: '', name: 'Database' })).toBe(false)
    expect(isSeedName({ builtin_key: undefined, name: 'x' })).toBe(false)
  })

  it('未改名的内置助手随界面语言显示，改过名的原样保留', () => {
    const seed = { builtin_key: 'database', name: BUILTIN_SEED_NAMES.database }
    expect(displayAgentName(seed, 'zh-CN')).toBe('数据库助手')
    expect(displayAgentName(seed, 'en-US')).not.toBe('数据库助手')
    expect(displayAgentName({ builtin_key: 'database', name: '自定义' }, 'zh-CN')).toBe('自定义')
  })

  it('builtinAliases 覆盖两种语言，kv 额外带历史别名 KV助手', () => {
    setI18nLocale('zh-CN')
    expect(builtinAliases('kv')).toContain('KV助手')
    expect(builtinAliases('kv')).toContain('键值助手')
    expect(builtinAliases('database')).toContain('数据库助手')
    expect(builtinAliases('database')).not.toContain('KV助手')
    expect(new Set(builtinAliases('general')).size).toBe(builtinAliases('general').length)
  })

  it('种子名表覆盖全部内置 key（与 locales 的 builtinNames 同步）', () => {
    for (const key of Object.keys(BUILTIN_SEED_NAMES)) {
      expect(builtinAliases(key).length, key).toBeGreaterThan(0)
    }
  })
})

describe('useQueryPatch', () => {
  function fakeRouter(initial: Record<string, unknown> = {}) {
    const state = { query: { ...initial } as Record<string, unknown> }
    const order: string[] = []
    const router = {
      currentRoute: { get value() { return { query: state.query } } },
      replace: vi.fn(async ({ query }: { query: Record<string, unknown> }) => {
        order.push(JSON.stringify(query))
        await Promise.resolve()
        state.query = query
      })
    }
    return { router, state, order }
  }

  it('合并 patch，undefined 删除该键', async () => {
    const { router, state } = fakeRouter({ keep: '1', drop: 'x' })
    const replaceQuery = useQueryPatch(router as never)
    replaceQuery({ thread: 't1', drop: undefined })
    await vi.waitFor(() => expect(router.replace).toHaveBeenCalled())
    await Promise.resolve()
    expect(state.query).toEqual({ keep: '1', thread: 't1' })
  })

  it('连续调用串行执行：后一次基于前一次替换后的结果', async () => {
    const { router, state } = fakeRouter()
    const replaceQuery = useQueryPatch(router as never)
    replaceQuery({ a: '1' })
    replaceQuery({ b: '2' })
    replaceQuery({ a: undefined, c: '3' })
    await vi.waitFor(() => expect(router.replace).toHaveBeenCalledTimes(3))
    await new Promise((r) => setTimeout(r, 0))
    expect(state.query).toEqual({ b: '2', c: '3' })
  })

  it('replace 失败静默，且不阻断后续调用', async () => {
    const { router, state } = fakeRouter()
    router.replace.mockRejectedValueOnce(new Error('nav aborted'))
    const replaceQuery = useQueryPatch(router as never)
    replaceQuery({ a: '1' })
    replaceQuery({ b: '2' })
    await vi.waitFor(() => expect(router.replace).toHaveBeenCalledTimes(2))
    await new Promise((r) => setTimeout(r, 0))
    expect(state.query).toEqual({ b: '2' })
  })

  it('router 没有 replace 时什么都不做', () => {
    const replaceQuery = useQueryPatch({} as never)
    expect(() => replaceQuery({ a: '1' })).not.toThrow()
  })
})
