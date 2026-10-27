import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uiStubs } from '@/test/helpers'

const loadMarkdown = vi.hoisted(() => vi.fn())
vi.mock('@/docs/catalog', () => ({
  githubBlobUrl: (p: string) => `https://github.com/x/blob/${p}`,
  isLargeDoc: () => false,
  loadMarkdown
}))

vi.mock('@/docs/render', () => ({
  renderMarkdown: () => {
    throw new Error('boom-render')
  }
}))

import DocsArticle from '@/components/docs/DocsArticle.vue'

async function mountArticle(filePath: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/docs/:module?/:slug?', component: { template: '<div />' } }]
  })
  await router.push('/docs')
  await router.isReady()
  return mount(DocsArticle, {
    props: {
      moduleId: 'm',
      moduleTitle: 'M',
      page: { moduleId: 'm', slug: 'p', title: 'T', order: 1, filePath }
    },
    global: { plugins: [router], stubs: uiStubs }
  })
}

describe('DocsArticle error paths', () => {
  beforeEach(() => {
    loadMarkdown.mockReset()
    loadMarkdown.mockImplementation(async (p: string) => (p.includes('empty') ? '   ' : p.includes('missing') ? null : '# Hi'))
  })

  it('renders empty, missing, and markdown-failure states', async () => {
    const missing = await mountArticle('missing.md')
    await flushPromises()
    expect(missing.text()).toContain('文档文件缺失')

    const empty = await mountArticle('empty.md')
    await flushPromises()
    expect(empty.text()).toContain('这篇文档没有正文')

    const fail = await mountArticle('ok.md')
    await flushPromises()
    expect(fail.html()).toContain('文档渲染失败')
  })

  it('retries loading from the error region', async () => {
    const wrapper = await mountArticle('missing.md')
    await flushPromises()
    expect(loadMarkdown).toHaveBeenCalledTimes(1)

    wrapper.findComponent({ name: 'SbAsyncRegion' }).vm.$emit('retry')
    await flushPromises()
    expect(loadMarkdown).toHaveBeenCalledTimes(2)
    expect(loadMarkdown).toHaveBeenLastCalledWith('missing.md')
    wrapper.unmount()
  })
})
