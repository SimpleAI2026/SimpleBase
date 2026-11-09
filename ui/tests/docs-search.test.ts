import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DocsSearch from '@/components/docs/DocsSearch.vue'
import { searchDocs, type DocSearchResult } from '@/docs/catalog'

vi.mock('@/docs/catalog', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/docs/catalog')>()
  return { ...actual, searchDocs: vi.fn() }
})

const results: DocSearchResult[] = [
  {
    page: { moduleId: 'data', slug: 'realtime', title: '文档 Realtime', order: 2, filePath: 'data/realtime.md' },
    heading: '建立连接',
    hash: '建立连接',
    snippet: '通过 SSE 订阅文档事件'
  },
  {
    page: { moduleId: 'automation', slug: 'function-triggers', title: '事件触发器', order: 15, filePath: 'automation/function-triggers.md' },
    snippet: '响应文档与对象事件'
  }
]

async function mountSearch() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/docs/:module?/:slug?', component: { template: '<div />' } }]
  })
  await router.push('/docs')
  await router.isReady()
  const wrapper = mount(DocsSearch, {
    attachTo: document.body,
    global: { plugins: [router], stubs: { teleport: false } }
  })
  return { router, wrapper }
}

function searchInput(): DOMWrapper<HTMLInputElement> {
  const element = document.body.querySelector<HTMLInputElement>('input[role="combobox"]')
  expect(element).toBeTruthy()
  return new DOMWrapper(element!)
}

describe('DocsSearch', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    vi.mocked(searchDocs).mockReset()
    vi.mocked(searchDocs).mockResolvedValue(results)
  })

  it('searches, navigates with arrow keys and Enter, then closes and resets', async () => {
    const { router, wrapper } = await mountSearch()
    await wrapper.find('button[aria-label="搜索文档"]').trigger('click')
    await flushPromises()
    const input = searchInput()
    await input.setValue('realtime')
    await flushPromises()

    expect(searchDocs).toHaveBeenCalledWith('realtime')
    expect(document.body.querySelectorAll('[role="option"]')).toHaveLength(2)
    expect(input.attributes('aria-activedescendant')).toBe('docs-search-result-0')

    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(input.attributes('aria-activedescendant')).toBe('docs-search-result-1')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/docs/automation/function-triggers')
    expect(document.body.querySelector('input[role="combobox"]')).toBeNull()
    await wrapper.find('button[aria-label="搜索文档"]').trigger('click')
    await flushPromises()
    expect(searchInput().element.value).toBe('')
    expect(document.body.textContent).toContain('输入关键词')
    wrapper.unmount()
  })

  it('opens with Ctrl/Cmd+K and closes with Escape', async () => {
    const { wrapper } = await mountSearch()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))
    await flushPromises()
    const input = searchInput()
    expect(document.activeElement).toBe(input.element)

    await input.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(document.body.querySelector('input[role="combobox"]')).toBeNull()
    wrapper.unmount()
  })

  it('shows an empty result state and wraps keyboard selection', async () => {
    vi.mocked(searchDocs).mockResolvedValueOnce([])
    const { wrapper } = await mountSearch()
    await wrapper.find('button[aria-label="搜索文档"]').trigger('click')
    await flushPromises()
    const input = searchInput()
    await input.setValue('missing')
    await flushPromises()
    expect(document.body.textContent).toContain('没有匹配的文档')
    expect(input.attributes('aria-activedescendant')).toBeUndefined()

    await input.setValue('realtime')
    await flushPromises()
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(input.attributes('aria-activedescendant')).toBe('docs-search-result-1')
    wrapper.unmount()
  })
})
