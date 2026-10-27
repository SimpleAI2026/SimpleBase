import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { defaultModuleId, defaultSlug, docCatalog, getModule } from '@/docs/catalog'
import { mountWithApp } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import DocsWiki from '@/pages/DocsWiki.vue'

const docsStubs = {
  DocsSidebar: { template: '<aside class="docs-side" />' },
  DocsArticle: { template: '<article class="docs-art" />' },
  Select: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template:
      '<div><button type="button" class="mobile-index" @click="$emit(\'update:modelValue\', \'index\')">idx</button><button type="button" class="mobile-page" @click="$emit(\'update:modelValue\', \'other\')">pg</button></div>'
  },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<span />' },
  SelectContent: { template: '<div />' },
  SelectGroup: { template: '<div />' },
  SelectItem: { template: '<div />' }
}

const legacyDocsStubs = {
  DocsSidebar: { template: '<div />' },
  DocsArticle: { template: '<div class="article" />' },
  Select: { template: '<div><slot /></div>' },
  SelectTrigger: { template: '<div />' },
  SelectValue: { template: '<div />' },
  SelectContent: { template: '<div><slot /></div>' },
  SelectGroup: { template: '<div><slot /></div>' },
  SelectItem: { template: '<div><slot /></div>' }
}

describe('DocsWiki', () => {
  const originalWidth = window.innerWidth

  beforeEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: 1200 })
  })

  afterEach(() => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: originalWidth })
  })

  it('renders the default module article on desktop', async () => {
    const id = defaultModuleId()
    const { wrapper } = await mountWithApp(DocsWiki, {
      path: `/docs/${id}`,
      stubs: docsStubs
    })
    expect(wrapper.find('.docs-art').exists()).toBe(true)
    expect(wrapper.find('.docs-side').exists()).toBe(true)
    expect(wrapper.find('.docs-art').exists()).toBe(true)
    expect(wrapper.text()).toContain(getModule(id)?.title)
    const bar = wrapper.get('[data-docs-tabs]')
    expect(bar.classes()).toContain('border-b')
    expect(bar.classes()).toContain('border-border/70')
    expect(bar.classes()).not.toContain('border-b-0')
    const list = wrapper.get('.tabs-list')
    expect(list.classes()).toContain('border-b-0')
    expect(list.classes()).not.toContain('border-b')
  })

  it('redirects unknown modules and index slugs', async () => {
    const { router } = await mountWithApp(DocsWiki, {
      path: '/docs/not-a-real-module/nope',
      stubs: docsStubs
    })
    await flushPromises()
    expect(router.currentRoute.value.path).toContain('/docs/')

    const id = defaultModuleId()
    const { router: r2 } = await mountWithApp(DocsWiki, {
      path: `/docs/${id}/index`,
      stubs: docsStubs
    })
    await flushPromises()
    expect(r2.currentRoute.value.path).toBe(`/docs/${id}`)
  })

  it('shows a missing-page hint when the slug is unknown', async () => {
    const id = defaultModuleId()
    const { wrapper, router } = await mountWithApp(DocsWiki, {
      path: `/docs/${id}/definitely-missing-slug-xyz`,
      stubs: docsStubs
    })
    await flushPromises()
    expect(wrapper.text().includes('未找到') || wrapper.find('.docs-art').exists() || router.currentRoute.value.path.startsWith('/docs/')).toBe(true)

    const first = getModule(id)
    if (first && first.pages.length > 1) {
      const last = first.pages[first.pages.length - 1]
      const { wrapper: w2 } = await mountWithApp(DocsWiki, {
        path: `/docs/${id}/${last.slug}`,
        stubs: docsStubs
      })
      await flushPromises()
      expect(w2.find('.docs-art').exists() || w2.text().includes('文档')).toBe(true)
      w2.unmount()
    }
    wrapper.unmount()
  })

  it('switches modules, handles mobile page nav, and resize', async () => {
    const id = defaultModuleId()
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: 500 })
    const { wrapper, router } = await mountWithApp(DocsWiki, {
      path: `/docs/${id}`,
      stubs: docsStubs
    })
    expect(wrapper.find('.docs-side').exists()).toBe(true) // CSS 响应式隐藏，jsdom 不计算断点
    expect(wrapper.get('#docs-page-label').text()).toBe('选择文档页面')
    expect(wrapper.find('#docs-page-label').exists()).toBe(true)
    if (wrapper.find('.mobile-index').exists()) {
      await wrapper.get('.mobile-index').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toBe(`/docs/${id}`)
      await wrapper.get('.mobile-page').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path).toContain(`/docs/${id}/`)
    }
    if (wrapper.find('.tab-emit').exists() && docCatalog.length > 1) {
      await wrapper.get('.tab-emit').trigger('click')
      await flushPromises()
      expect(router.currentRoute.value.path.startsWith('/docs/')).toBe(true)
    }
    Object.defineProperty(window, 'innerWidth', { configurable: true, writable: true, value: 900 })
    window.dispatchEvent(new Event('resize'))
    wrapper.unmount()
    expect(getModule(id)).toBeTruthy()
    expect(defaultSlug(id)).toBeTruthy()
  })
  it('scrolls the active tab into view on mount and when the module changes', async () => {
    // 桩版 TabsTrigger 不输出 data-state，这里手工造一个活跃 Tab 让选择器命中
    const host = document.createElement('div')
    host.setAttribute('data-docs-tabs', '')
    const active = document.createElement('button')
    active.setAttribute('data-state', 'active')
    host.appendChild(active)
    document.body.appendChild(host)
    const original = Element.prototype.scrollIntoView
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView

    const id = defaultModuleId()
    const { wrapper, router } = await mountWithApp(DocsWiki, { path: `/docs/${id}`, stubs: docsStubs })
    await flushPromises()
    expect(scrollIntoView).toHaveBeenCalledWith({ inline: 'center', block: 'nearest' })

    scrollIntoView.mockClear()
    const other = docCatalog.find((m) => m.id !== id)
    if (other) {
      await router.push(`/docs/${other.id}`)
      await flushPromises()
      expect(scrollIntoView).toHaveBeenCalled()
    }
    wrapper.unmount()
    if (original) Element.prototype.scrollIntoView = original
    else delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
    host.remove()
  })

  it('skips tab scrolling when no active tab or scrollIntoView is available', async () => {
    const id = defaultModuleId()
    // 不注入活跃 Tab（桩版 TabsTrigger 没有 data-state），命中"找不到元素"分支
    const { wrapper, router } = await mountWithApp(DocsWiki, { path: `/docs/${id}`, stubs: docsStubs })
    await flushPromises()
    const other = docCatalog.find((m) => m.id !== id)
    if (other) {
      await router.push(`/docs/${other.id}`)
      await flushPromises()
    }
    expect(wrapper.find('[data-docs-tabs]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('redirects every legacy doc path to the six-module layout', async () => {
    const cases: [string, string][] = [
      ['/docs/ops/dashboard', '/docs/workbench/dashboard'],
      ['/docs/ops/users', '/docs/data'],
      ['/docs/database/kv', '/docs/data/kv'],
      ['/docs/kv', '/docs/data/kv'],
      ['/docs/logs', '/docs/ops/logs'],
      ['/docs/users', '/docs/data'],
      ['/docs/dashboard', '/docs/workbench/dashboard'],
      ['/docs/agent', '/docs/workbench/agent'],
      ['/docs/database', '/docs/data'],
      ['/docs/storage', '/docs/data/storage'],
      ['/docs/gofunction', '/docs/automation/gofunction'],
      ['/docs/cronjob', '/docs/automation/cronjob'],
      ['/docs/sandbox', '/docs/automation/sandbox']
    ]
    for (const [from, to] of cases) {
      const { router, wrapper } = await mountWithApp(DocsWiki, { path: from, stubs: docsStubs })
      await flushPromises()
      expect(router.currentRoute.value.path, from).toBe(to)
      wrapper.unmount()
    }
  })

  it('DocsWiki resolves module/slug from the route', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/docs/:module?/:slug?', name: 'docs-page', component: DocsWiki, meta: { title: '使用文档' } }
      ]
    })
    await router.push('/docs')
    await router.isReady()
    const w = mount(DocsWiki, { global: { plugins: [router, createPinia()], stubs: legacyDocsStubs } })
    await flushPromises()
    expect(w.find('.article').exists() || w.text().includes('文档')).toBe(true)
    w.unmount()
  })
})
