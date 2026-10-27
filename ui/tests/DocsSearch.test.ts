import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DocsSearch from '@/components/docs/DocsSearch.vue'

const searchDocs = vi.hoisted(() => vi.fn())
vi.mock('@/docs/catalog', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/docs/catalog')>()
  return { ...actual, searchDocs }
})

async function mounted() {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/docs/:module?/:slug?', component: { template: '<div />' } }
  ] })
  await router.push('/docs')
  await router.isReady()
  const wrapper = mount(DocsSearch, {
    attachTo: document.body,
    global: { plugins: [router], stubs: { teleport: false } }
  })
  return { wrapper, router }
}

function input() {
  return new DOMWrapper(document.body.querySelector<HTMLInputElement>('input[type="search"]')!)
}

describe('DocsSearch', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    searchDocs.mockReset()
    searchDocs.mockResolvedValue([{ page: { moduleId: 'automation', slug: 'sandbox-api', title: '云沙盒 API', filePath: 'automation/sandbox-api.md' },
      heading: '错误码', hash: '错误码', snippet: 'sandbox_busy' }])
  })

  it('searches with keyboard shortcut, navigates on enter and closes on escape', async () => {
    const { wrapper, router } = await mounted()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true }))
    await flushPromises()
    expect(input().exists()).toBe(true)
    await input().setValue('ErrBusy')
    await flushPromises()
    expect(searchDocs).toHaveBeenCalledWith('ErrBusy')
    expect(document.body.textContent).toContain('错误码')
    await input().trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/docs/automation/sandbox-api')
    expect(router.currentRoute.value.hash).toBe('#错误码')
    await wrapper.get('button[aria-label="搜索文档"]').trigger('click')
    await input().trigger('keydown', { key: 'Escape' })
    expect(document.body.querySelector('input[type="search"]')).toBeNull()
    wrapper.unmount()
  })

  it('shows empty result and does not build an index for blank text', async () => {
    searchDocs.mockResolvedValueOnce([])
    const { wrapper } = await mounted()
    await wrapper.get('button[aria-label="搜索文档"]').trigger('click')
    await input().setValue('nothing')
    await flushPromises()
    expect(document.body.textContent).toContain('没有匹配的文档')
    await input().setValue('')
    expect(document.body.querySelectorAll('[role="option"]')).toHaveLength(0)
    wrapper.unmount()
  })
})
