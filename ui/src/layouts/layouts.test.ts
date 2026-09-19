import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import DefaultLayout from './DefaultLayout.vue'
import DocsLayout from './DocsLayout.vue'
import { useAuthStore } from '../stores/auth'

vi.mock('../services/api', () => ({
  isMock: true,
  api: {}
}))

describe('layouts', () => {
  it('DefaultLayout shows route title and opens the key drawer', async () => {
    setActivePinia(createPinia())
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/',
          component: DefaultLayout,
          children: [{ path: '', name: 'dashboard', component: { template: '<div>dash</div>' }, meta: { title: '监控大盘' } }]
        }
      ]
    })
    await router.push('/')
    await router.isReady()
    const reload = vi.fn()
    Object.defineProperty(window, 'location', { value: { reload }, writable: true })
    const w = mount(DefaultLayout, {
      global: {
        plugins: [router, createPinia()],
        stubs: {
          SidebarProvider: { template: '<div><slot /></div>' },
          Sidebar: { template: '<div><slot /></div>' },
          SidebarHeader: { template: '<div><slot /></div>' },
          SidebarContent: { template: '<div><slot /></div>' },
          SidebarGroup: { template: '<div><slot /></div>' },
          SidebarGroupContent: { template: '<div><slot /></div>' },
          SidebarRail: { template: '<div />' },
          SidebarInset: { template: '<div><slot /></div>' },
          SidebarTrigger: { template: '<button />' },
          Breadcrumb: { template: '<div><slot /></div>' },
          BreadcrumbList: { template: '<div><slot /></div>' },
          BreadcrumbItem: { template: '<div><slot /></div>' },
          BreadcrumbPage: { template: '<div><slot /></div>' },
          Tooltip: { template: '<div><slot /></div>' },
          TooltipTrigger: { template: '<div><slot /></div>' },
          TooltipContent: { template: '<div><slot /></div>' },
          Badge: { template: '<span><slot /></span>' },
          Button: { template: '<button @click="$emit(\'click\')"><slot /></button>' },
          NavMenu: { template: '<div />' },
          ApiKeyDrawer: { template: '<div />' },
          GlobalProjectSwitcher: { template: '<div />' }
        }
      }
    })
    await flushPromises()
    expect(w.text()).toContain('监控大盘')
    expect(w.text()).toContain('Mock')
    const store = useAuthStore()
    const settingsBtn = w.findAll('button').find((b) => b.html().includes('Settings') || true)
    await w.findAll('button').at(-2)?.trigger('click')
    store.openDrawer()
    expect(store.keyDrawerOpen).toBe(true)
    w.unmount()
  })

  it('DocsLayout renders console and github links', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/docs', component: DocsLayout, children: [{ path: '', component: { template: '<div>doc</div>' } }] }]
    })
    await router.push('/docs')
    await router.isReady()
    const w = mount(DocsLayout, {
      global: {
        plugins: [router],
        stubs: {
          Button: { template: '<button><slot /></button>' },
          GithubMark: { template: '<span>gh</span>' }
        }
      }
    })
    expect(w.text()).toContain('使用文档')
    expect(w.text()).toContain('doc')
  })
})
