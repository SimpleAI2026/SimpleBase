import { describe, expect, it } from 'vitest'
import { docCatalog, legacyDocRedirects, resolveLegacyDocPath } from '@/docs/catalog'

/**
 * 文档分为六大模块（planv6.1）：入门 / 工作台 / 数据 / 自动化 / 运维 / SDK。
 * 左：控制台侧栏分组（NavMenu GROUP_DEFS）；右：该分组在文档站里的模块 id。
 * 调整侧栏分组或新增页面时，请同步 docs/_meta.json 与此表。
 */
const EXPECTED_MODULES: { id: string; title: string }[] = [
  { id: 'getting-started', title: '入门' },
  { id: 'workbench', title: '工作台' },
  { id: 'data', title: '数据' },
  { id: 'automation', title: '自动化' },
  { id: 'ops', title: '运维' },
  { id: 'sdk', title: 'SDK' }
]

/** 每个模块里必须出现的子主题页（slug 为路由末段，index 即模块首页）。 */
const REQUIRED_PAGES: Record<string, string[]> = {
  workbench: ['index', 'agent', 'dashboard'],
  data: ['index', 'sql', 'kv', 'storage'],
  automation: ['index', 'gofunction', 'cronjob', 'sandbox'],
  ops: ['index', 'logs', 'deployment']
}

describe('文档六大模块', () => {
  it('Tab 只有六个，且顺序与标题固定', () => {
    expect(docCatalog.map((m) => ({ id: m.id, title: m.title }))).toEqual(EXPECTED_MODULES)
  })

  it('每个模块包含约定的子主题页', () => {
    for (const [moduleId, slugs] of Object.entries(REQUIRED_PAGES)) {
      const mod = docCatalog.find((m) => m.id === moduleId)
      expect(mod, moduleId).toBeTruthy()
      const actual = new Set(mod!.pages.map((p) => p.slug))
      for (const slug of slugs) expect(actual.has(slug), `${moduleId}/${slug}`).toBe(true)
    }
  })

  it('不再存在已删除的用户管理文档', () => {
    const slugs = docCatalog.flatMap((m) => m.pages.map((p) => `${p.moduleId}/${p.slug}`))
    expect(slugs.some((s) => s.includes('users'))).toBe(false)
  })

  it('同一模块内用 group 把子主题归类，且分组连续不交错', () => {
    for (const mod of docCatalog) {
      const seen: string[] = []
      for (const page of mod.pages) {
        if (!page.group) continue
        const last = seen[seen.length - 1]
        if (last !== page.group) {
          expect(seen.includes(page.group), `${mod.id}: 分组 ${page.group} 被拆开`).toBe(false)
          seen.push(page.group)
        }
      }
    }
  })

  it('旧路径重定向目标都存在', () => {
    const routes = new Set(
      docCatalog.flatMap((m) =>
        m.pages.map((p) => (p.slug === 'index' ? `/docs/${m.id}` : `/docs/${m.id}/${p.slug}`))
      )
    )
    for (const [from, to] of Object.entries(legacyDocRedirects)) {
      expect(routes.has(to), `${from} -> ${to}`).toBe(true)
    }
    expect(resolveLegacyDocPath('ops', 'dashboard')).toBe('/docs/workbench/dashboard')
    expect(resolveLegacyDocPath('kv', 'index')).toBe('/docs/data/kv')
    expect(resolveLegacyDocPath('ops', 'deployment')).toBeNull()
    expect(resolveLegacyDocPath('ops', 'logs')).toBe('/docs/ops/logs')
  })
})
