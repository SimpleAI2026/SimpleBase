import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { defaultModuleId, defaultSlug, getPage } from '../../docs/catalog'
import { uiStubs } from '../../test/helpers'
import DocsArticle from './DocsArticle.vue'
import DocsSidebar from './DocsSidebar.vue'

const routes = [
  { path: '/docs/:module?/:slug?', name: 'docs', component: { template: '<div />' } }
]

async function withRouter() {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push('/docs')
  await router.isReady()
  return router
}

describe('docs components', () => {
  it('renders an article, prev/next links, and github url', async () => {
    const router = await withRouter()
    const id = defaultModuleId()
    const page = getPage(id, defaultSlug(id))!
    const w = mount(DocsArticle, {
      props: {
        moduleId: id,
        moduleTitle: 'Mod',
        page,
        prev: { ...page, slug: 'index', title: 'Prev' },
        next: { ...page, slug: 'next-page', title: 'Next' }
      },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(w.text()).toContain('Mod')
    expect(w.find('a[href*="github.com"]').exists()).toBe(true)
    expect(w.html()).toContain(`/docs/${id}`)
  })

  it('shows load-error and empty-body alerts', async () => {
    const router = await withRouter()
    const missing = mount(DocsArticle, {
      props: {
        moduleId: 'm',
        moduleTitle: 'M',
        page: { moduleId: 'm', slug: 'x', title: 'X', order: 1, filePath: 'no/such.md' }
      },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(missing.text()).toMatch(/没有正文|缺失|未找到/)

    vi.mock('../../docs/catalog', async (orig) => {
      const actual = await orig()
      return {
        ...actual,
        loadMarkdownRaw: () => '   '
      }
    })
  })

  it('builds sidebar links', async () => {
    const router = await withRouter()
    const side = mount(DocsSidebar, {
      props: {
        moduleId: 'ops',
        moduleTitle: 'Ops',
        activeSlug: 'index',
        pages: [
          { moduleId: 'ops', slug: 'index', title: 'Index', order: 0, filePath: 'ops/index.md' },
          { moduleId: 'ops', slug: 'deploy', title: 'Deploy', order: 1, filePath: 'ops/deploy.md' }
        ]
      },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(side.html()).toContain('/docs/ops')
    expect(side.html()).toContain('/docs/ops/deploy')
  })
})
