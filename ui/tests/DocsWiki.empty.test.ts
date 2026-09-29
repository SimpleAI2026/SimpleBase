import { describe, expect, it, vi } from 'vitest'
import { mountWithApp } from '../test/helpers'

vi.mock('../docs/catalog', () => ({
  docCatalog: [],
  defaultModuleId: () => 'getting-started',
  defaultSlug: () => 'index',
  getModule: () => undefined,
  getPage: () => undefined,
  loadedMarkdownCount: 0
}))

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import DocsWiki from './DocsWiki.vue'

describe('DocsWiki empty catalog', () => {
  it('shows the failed-load alert when no markdown matched', async () => {
    const { wrapper } = await mountWithApp(DocsWiki, { path: '/docs/mod/slug' })
    expect(wrapper.text()).toContain('未能加载文档')
    expect(wrapper.text()).toContain('0')
    const { wrapper: w2 } = await mountWithApp(DocsWiki, { path: '/docs/mod' })
    expect(w2.text()).toContain('未能加载文档')
    const { wrapper: w3 } = await mountWithApp(DocsWiki, { path: '/docs' })
    expect(w3.text()).toContain('未能加载文档')
  })
})
