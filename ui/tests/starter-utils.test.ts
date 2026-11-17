import { beforeEach, describe, expect, it } from 'vitest'
import { genDnsSafeId, isDnsSafeProjectId } from '@/utils/project-id'
import {
  functionCallUrl,
  parseDelivery,
  resolveSiteUrl,
  stripDelivery
} from '@/utils/starter-delivery'
import {
  clearPendingStarter,
  markStarterSent,
  savePendingStarter,
  starterAlreadySent,
  takePendingStarter
} from '@/utils/starter-storage'
import { parseToolCard, stageDoneByCards, stageForArgv, toolResultFailed } from '@/utils/starter-progress'
import { STARTER_STAGES, STARTER_TEMPLATES, findStarterTemplate, stagesFor } from '@/constants/starter-prompts'
import type { AgentToolCallCard } from '@/services/types'

describe('genDnsSafeId', () => {
  it('只产出 [a-z0-9]{8}（站点发布要求 DNS 安全）', () => {
    for (let i = 0; i < 500; i++) {
      const id = genDnsSafeId()
      expect(id).toMatch(/^[a-z0-9]{8}$/)
      expect(isDnsSafeProjectId(id)).toBe(true)
    }
  })

  it('不含大写与连字符，首尾都是字母数字', () => {
    const id = genDnsSafeId(() => new Uint8Array([255, 255, 255, 255, 255, 255, 255, 255]))
    expect(id).toMatch(/^[a-z0-9]+$/)
    expect(id).toHaveLength(8)
  })

  it('isDnsSafeProjectId 拒绝大写、连字符与错误长度', () => {
    for (const bad of ['ABCDEFGH', 'abcd-efg', '-abcdefg', 'abcdefg-', 'abcdefg', 'abcdefghi', '']) {
      expect(isDnsSafeProjectId(bad), bad).toBe(false)
    }
  })

  it('没有 Web Crypto 时仍可生成', () => {
    const g = globalThis as { crypto?: unknown }
    const saved = g.crypto
    Object.defineProperty(globalThis, 'crypto', { value: undefined, configurable: true })
    try {
      expect(genDnsSafeId()).toMatch(/^[a-z0-9]{8}$/)
    } finally {
      Object.defineProperty(globalThis, 'crypto', { value: saved, configurable: true })
    }
  })
})

describe('starter-delivery：交付块协议', () => {
  const block = (o: unknown) => `完成。\n<!-- sb:delivery ${JSON.stringify(o)} -->`

  it('解析完整交付块并逐项规整', () => {
    const d = parseDelivery(
      block({
        site_url: 'https://ab12cd34.app.example.com/',
        functions: [{ name: 'listTodos', url: '/go/ab12cd34/main/listTodos' }],
        tables: ['todos', 7, ' '],
        collections: ['c1'],
        cron: [{ name: 'daily', expr: '0 2 * * *' }],
        next: ['去控制台看看']
      })
    )
    expect(d).not.toBeNull()
    expect(d!.siteUrl).toBe('https://ab12cd34.app.example.com/')
    expect(d!.tables).toEqual(['todos'])
    expect(d!.cron).toEqual([{ name: 'daily', expr: '0 2 * * *' }])
    expect(d!.functions[0].name).toBe('listTodos')
  })

  it('字段缺失逐项隐藏，整体缺失 / 损坏返回 null，不抛错', () => {
    expect(parseDelivery('')).toBeNull()
    expect(parseDelivery('没有交付块')).toBeNull()
    expect(parseDelivery('<!-- sb:delivery {坏的 json -->')).toBeNull()
    expect(parseDelivery('<!-- sb:delivery [1,2] -->')).toBeNull()
    const d = parseDelivery(block({}))
    expect(d).toEqual({ siteUrl: '', functions: [], tables: [], collections: [], cron: [], next: [] })
  })

  it('丢弃没有名字的函数 / 任务条目，限制条目数量', () => {
    const many = Array.from({ length: 200 }, (_, i) => `t${i}`)
    const d = parseDelivery(block({ functions: [{ url: '/x' }, 5, null], cron: [{ expr: '* * * * *' }], tables: many }))
    expect(d!.functions).toEqual([])
    expect(d!.cron).toEqual([])
    expect(d!.tables.length).toBe(50)
  })

  it('超大交付块直接放弃', () => {
    const huge = `<!-- sb:delivery {"next":["${'x'.repeat(20000)}"]} -->`
    expect(parseDelivery(huge)).toBeNull()
  })

  it('stripDelivery 去掉注释，保留正文', () => {
    expect(stripDelivery(block({ a: 1 }))).toBe('完成。')
  })

  it('站点地址必须与后端 baseUrl 同源，否则丢弃改用后端值', () => {
    const base = 'https://ab12cd34.app.example.com'
    expect(resolveSiteUrl('https://ab12cd34.app.example.com/', base)).toBe('https://ab12cd34.app.example.com/web/')
    // 助手给了别的域名：丢弃，用后端值
    expect(resolveSiteUrl('https://evil.example.net/', base)).toBe('https://ab12cd34.app.example.com/web/')
    expect(resolveSiteUrl('javascript:alert(1)', base)).toBe('https://ab12cd34.app.example.com/web/')
    expect(resolveSiteUrl('', base)).toBe('https://ab12cd34.app.example.com/web/')
    // 后端没给 baseUrl（站点未启用）：助手给的地址一律不展示
    expect(resolveSiteUrl('https://ab12cd34.app.example.com/', undefined)).toBe('')
    expect(resolveSiteUrl('https://x.example.com/', '')).toBe('')
    // 后端值不是 http(s) 也拒绝
    expect(resolveSiteUrl('', 'javascript:alert(1)')).toBe('')
    expect(resolveSiteUrl('', 'not a url')).toBe('')
  })

  it('函数地址由前端按当前 origin 重新拼接，不用助手给的 url', () => {
    const fn = { name: 'listTodos', url: 'https://evil.example.net/steal' }
    expect(functionCallUrl('http://localhost:5173/', 'ab12cd34', fn)).toBe('http://localhost:5173/go/ab12cd34/main/listTodos')
    expect(functionCallUrl('https://c.example.com', 'ab12cd34', { name: 'f', url: '/go/ab12cd34/shop/f' })).toBe('https://c.example.com/go/ab12cd34/shop/f')
    // 名字里的特殊字符被转义
    expect(functionCallUrl('https://c.example.com', 'p', { name: 'a/b', url: '' })).toBe('https://c.example.com/go/p/main/a%2Fb')
  })
})

describe('starter-storage', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('pending 读取即清除，只续跑一次', () => {
    savePendingStarter({ templateId: 'dashboard', name: '看板', engine: 'sqlite' })
    expect(takePendingStarter()).toEqual({ templateId: 'dashboard', name: '看板', engine: 'sqlite' })
    expect(takePendingStarter()).toBeNull()
  })

  it('格式损坏视为无', () => {
    sessionStorage.setItem('sb_pending_starter', '{坏')
    expect(takePendingStarter()).toBeNull()
    sessionStorage.setItem('sb_pending_starter', JSON.stringify({ name: 'x' }))
    expect(takePendingStarter()).toBeNull()
  })

  it('未知引擎回落 duckdb；clear 清除', () => {
    sessionStorage.setItem('sb_pending_starter', JSON.stringify({ templateId: 't', engine: 'oracle' }))
    expect(takePendingStarter()).toEqual({ templateId: 't', name: '', engine: 'duckdb' })
    savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })
    clearPendingStarter()
    expect(takePendingStarter()).toBeNull()
  })

  it('去重键按「项目 + 模板」区分', () => {
    expect(starterAlreadySent('p1', 'dashboard')).toBe(false)
    markStarterSent('p1', 'dashboard')
    expect(starterAlreadySent('p1', 'dashboard')).toBe(true)
    expect(starterAlreadySent('p2', 'dashboard')).toBe(false)
    expect(starterAlreadySent('p1', 'notice')).toBe(false)
  })

  it('Storage 读写抛错（配额 / 隐私模式）时同样降级，不影响主流程', () => {
    const boom = () => {
      throw new Error('QuotaExceeded')
    }
    const real = Object.getOwnPropertyDescriptor(window, 'sessionStorage')!
    // jsdom 的 sessionStorage 是 getter，spy 不到实例方法：整体换成会抛错的实现。
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      value: { getItem: boom, setItem: boom, removeItem: boom }
    })
    try {
      expect(() => savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })).not.toThrow()
      expect(takePendingStarter()).toBeNull()
      expect(() => clearPendingStarter()).not.toThrow()
      expect(starterAlreadySent('p', 'a')).toBe(false)
      expect(() => markStarterSent('p', 'a')).not.toThrow()
    } finally {
      Object.defineProperty(window, 'sessionStorage', real)
    }
  })

  it('sessionStorage 不可用时降级而不抛错', () => {
    const real = Object.getOwnPropertyDescriptor(window, 'sessionStorage')!
    Object.defineProperty(window, 'sessionStorage', {
      configurable: true,
      get() {
        throw new Error('denied')
      }
    })
    try {
      expect(() => savePendingStarter({ templateId: 'a', name: '', engine: 'duckdb' })).not.toThrow()
      expect(takePendingStarter()).toBeNull()
      expect(starterAlreadySent('p', 'a')).toBe(false)
      expect(() => markStarterSent('p', 'a')).not.toThrow()
      expect(() => clearPendingStarter()).not.toThrow()
    } finally {
      Object.defineProperty(window, 'sessionStorage', real)
    }
  })
})

describe('starter-progress：进度来自真实工具调用', () => {
  const card = (argv: string[], status = 'success', content = '{"ok":true,"data":{}}'): AgentToolCallCard => ({
    name: 'simplebase',
    arguments: JSON.stringify({ argv }),
    status,
    content
  })

  it('parseToolCard 只认 simplebase 且 argv 全是字符串', () => {
    expect(parseToolCard({ name: 'readonly_sql', arguments: '{}' })).toBeNull()
    expect(parseToolCard({ name: 'simplebase', arguments: '不是 json' })).toBeNull()
    expect(parseToolCard({ name: 'simplebase', arguments: '{"argv":[1,2]}' })).toBeNull()
    expect(parseToolCard(card(['sql', 'exec']))).toEqual({ argv: ['sql', 'exec'], ok: true })
  })

  it('CLI 信封 ok:false 即使工具本身 success 也算失败', () => {
    expect(toolResultFailed('{"ok":false,"error":{"code":"x"}}')).toBe(true)
    expect(toolResultFailed('{"is_error":true}')).toBe(true)
    expect(toolResultFailed('{"ok":true}')).toBe(false)
    expect(toolResultFailed('普通文本')).toBe(false)
    expect(toolResultFailed(undefined)).toBe(false)
    expect(parseToolCard(card(['sql', 'exec'], 'success', '{"ok":false}'))!.ok).toBe(false)
    expect(parseToolCard(card(['sql', 'exec'], 'error'))!.ok).toBe(false)
  })

  it('各阶段的完成判定', () => {
    expect(stageDoneByCards('data', [card(['sql', 'exec', '--database', 'd'])])).toBe(true)
    expect(stageDoneByCards('data', [card(['collection', 'create'])])).toBe(true)
    expect(stageDoneByCards('data', [card(['sql', 'query'])])).toBe(false)
    expect(stageDoneByCards('func', [card(['function', 'create'])])).toBe(false)
    expect(stageDoneByCards('func', [card(['function', 'test'])])).toBe(true)
    expect(stageDoneByCards('cron', [card(['cron', 'create'])])).toBe(true)
    expect(stageDoneByCards('cron', [card(['cron', 'trigger'])])).toBe(false)
    expect(stageDoneByCards('site', [card(['site', 'enable'])])).toBe(false)
    expect(stageDoneByCards('site', [card(['site', 'publish', '--prefix', 'web/'])])).toBe(true)
    expect(stageDoneByCards('verify', [card(['site', 'status'])])).toBe(true)
    expect(stageDoneByCards('plan', [card(['sql', 'exec'])])).toBe(false)
    // 失败的调用不算
    expect(stageDoneByCards('func', [card(['function', 'test'], 'success', '{"ok":false}')])).toBe(false)
    expect(stageDoneByCards('data', [])).toBe(false)
  })

  it('stageForArgv 把命令归到阶段', () => {
    expect(stageForArgv(['sql', 'exec'])).toBe('data')
    expect(stageForArgv(['collection', 'create'])).toBe('data')
    expect(stageForArgv(['function', 'test'])).toBe('func')
    expect(stageForArgv(['cron', 'create'])).toBe('cron')
    expect(stageForArgv(['object', 'upload'])).toBe('site')
    expect(stageForArgv(['site', 'publish'])).toBe('site')
    expect(stageForArgv(['kv', 'exec'])).toBeNull()
  })
})

describe('starter-prompts：模板与阶段', () => {
  it('B1 只展示数据通道为只读快照的三个模板', () => {
    expect(STARTER_TEMPLATES.map((t) => t.id)).toEqual(['dashboard', 'portfolio', 'notice'])
    // 需要匿名写入的模板依赖 B3，不能出现在前端
    for (const id of ['todo', 'booking', 'survey']) expect(findStarterTemplate(id)).toBeUndefined()
    expect(findStarterTemplate(' dashboard ')?.id).toBe('dashboard')
    expect(findStarterTemplate(undefined)).toBeUndefined()
  })

  it('阶段顺序与后端 StarterStages 一致', () => {
    expect([...STARTER_STAGES]).toEqual(['plan', 'data', 'func', 'cron', 'site', 'verify'])
  })

  it('没有定时任务的模板跳过 cron；站点不可发布跳过 site', () => {
    const portfolio = findStarterTemplate('portfolio')!
    const dashboard = findStarterTemplate('dashboard')!
    expect(stagesFor(portfolio, true)).not.toContain('cron')
    expect(stagesFor(dashboard, true)).toContain('cron')
    expect(stagesFor(dashboard, false)).not.toContain('site')
    expect(stagesFor(dashboard, false)).toContain('verify')
  })
})
