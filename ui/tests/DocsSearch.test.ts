import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DocsSearch from '@/components/docs/DocsSearch.vue'

const searchDocs = vi.hoisted(() => vi.fn())
vi.mock('@/docs/catalog', () => ({ searchDocs }))

async function mounted() {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/docs/:module?/:slug?', component: { template: '<div />' } }
  ] })
  await router.push('/docs')
  await router.isReady()
  const wrapper = mount(DocsSearch, { global: { plugins: [router] } })
  return { wrapper, router }
}

describe('DocsSearch', () => {
  beforeEach(() => {
    searchDocs.mockReset()
    searchDocs.mockResolvedValue([{ page: { moduleId: 'sandbox', slug: 'api', title: '云沙盒 API', filePath: 'sandbox/api.md' },
      heading: '错误码', hash: '错误码', snippet: 'sandbox_busy' }])
  })

  it('searches with keyboard shortcut, navigates on enter and closes on escape', async () => {
    const { wrapper, router } = await mounted()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true }))
    await flushPromises()
    expect(wrapper.find('input[type="search"]').exists()).toBe(true)
    await wrapper.get('input').setValue('ErrBusy')
    await flushPromises()
    expect(searchDocs).toHaveBeenCalledWith('ErrBusy')
    expect(wrapper.text()).toContain('错误码')
    await wrapper.get('input').trigger('keydown.enter')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/docs/sandbox/api')
    expect(router.currentRoute.value.hash).toBe('#错误码')
    await wrapper.get('button[aria-label="搜索文档"]').trigger('click')
    await wrapper.get('input').trigger('keydown.esc')
    expect(wrapper.find('input').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows empty result and does not build an index for blank text', async () => {
    searchDocs.mockResolvedValueOnce([])
    const { wrapper } = await mounted()
    await wrapper.get('button[aria-label="搜索文档"]').trigger('click')
    await wrapper.get('input').setValue('nothing')
    await flushPromises()
    expect(wrapper.text()).toContain('没有匹配的文档')
    await wrapper.get('input').setValue('')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(0)
    wrapper.unmount()
  })
})
