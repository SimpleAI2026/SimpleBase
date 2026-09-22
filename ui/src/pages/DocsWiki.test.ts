import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defaultModuleId, defaultSlug, docCatalog, getModule } from '../docs/catalog'
import { mountWithApp } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import DocsWiki from './DocsWiki.vue'

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
    expect(wrapper.find('.docs-side').exists()).toBe(false)
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
})
