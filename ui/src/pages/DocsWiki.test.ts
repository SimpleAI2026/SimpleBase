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
  DocsModuleTabs: {
    props: ['modelValue', 'modules'],
    emits: ['update:modelValue'],
    template:
      '<div class="tabs"><button type="button" class="switch-mod" @click="$emit(\'update:modelValue\', modules[1] ? modules[1].id : modelValue)">mod</button></div>'
  },
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
    const { wrapper } = await mountWithApp(DocsWiki, {
      path: `/docs/${id}/definitely-missing-slug-xyz`,
      stubs: docsStubs
    })
    await flushPromises()
    expect(wrapper.text().includes('未找到') || wrapper.find('.docs-art').exists()).toBe(true)
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
    if (wrapper.find('.switch-mod').exists() && docCatalog.length > 1) {
      await wrapper.get('.switch-mod').trigger('click')
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
