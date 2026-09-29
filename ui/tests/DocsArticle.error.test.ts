import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { uiStubs } from '../../test/helpers'

vi.mock('../../docs/catalog', () => ({
  githubBlobUrl: (p: string) => `https://github.com/x/blob/${p}`,
  loadMarkdownRaw: (p: string) => (p.includes('empty') ? '   ' : p.includes('missing') ? null : '# Hi')
}))

vi.mock('../../docs/render', () => ({
  renderMarkdown: () => {
    throw new Error('boom-render')
  }
}))

import DocsArticle from './DocsArticle.vue'

describe('DocsArticle error paths', () => {
  it('renders empty, missing, and markdown-failure states', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/docs/:module?/:slug?', component: { template: '<div />' } }]
    })
    await router.push('/docs')
    await router.isReady()
    const page = (filePath: string, slug = 'p') => ({
      moduleId: 'm',
      slug,
      title: 'T',
      order: 1,
      filePath
    })

    const missing = mount(DocsArticle, {
      props: { moduleId: 'm', moduleTitle: 'M', page: page('missing.md') },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(missing.text()).toContain('文档文件缺失')

    const empty = mount(DocsArticle, {
      props: { moduleId: 'm', moduleTitle: 'M', page: page('empty.md') },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(empty.text()).toContain('这篇文档没有正文')

    const fail = mount(DocsArticle, {
      props: { moduleId: 'm', moduleTitle: 'M', page: page('ok.md') },
      global: { plugins: [router], stubs: uiStubs }
    })
    expect(fail.html()).toContain('文档渲染失败')
  })
})
