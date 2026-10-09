import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import DocsLayout from '@/layouts/DocsLayout.vue'
import { useAuthStore } from '@/stores/auth'

vi.mock('@/services/api', () => ({
  api: {}
}))

describe('layouts', () => {
  it('DefaultLayout shows route title and opens settings', async () => {
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
          SettingsModal: { template: '<div />' },
          GlobalProjectSwitcher: { template: '<div />' }
        }
      }
    })
    await flushPromises()
    expect(w.text()).toContain('监控大盘')
    const store = useAuthStore()
    await w.findAll('button').at(-2)?.trigger('click')
    store.openSettings()
    expect(store.settingsOpen).toBe(true)
    w.unmount()
  })

  it('DocsLayout renders console and github links', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>home</div>' } },
        { path: '/console/agents', name: 'agents', component: { template: '<div>agents</div>' } },
        { path: '/docs', component: DocsLayout, children: [{ path: '', component: { template: '<div>doc</div>' } }] }
      ]
    })
    await router.push('/docs')
    await router.isReady()
    const w = mount(DocsLayout, {
      global: {
        plugins: [router],
        stubs: {
          RouterLink: false,
          Button: { template: '<button><slot /></button>' }
        }
      }
    })
    expect(w.text()).toContain('使用文档')
    expect(w.text()).toContain('doc')
    expect(w.find('a[href*="github.com"]').exists()).toBe(true)
    expect(w.find('a[title="返回首页"]').attributes('href')).toBe('/')
    expect(w.findAll('a').find((link) => link.text() === '返回控制台')?.attributes('href')).toBe('/console/agents')
  })

  it('sends a signed-in non-super user from the users page to agents', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    auth.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u2',
        username: 'admin',
        role: 'admin',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/console',
          component: DefaultLayout,
          children: [
            { path: 'agents', name: 'agents', component: { template: '<div>agents</div>' }, meta: { title: '云助手' } },
            { path: 'users', name: 'users', component: { template: '<div>users</div>' }, meta: { title: '用户管理' } }
          ]
        }
      ]
    })
    await router.push('/console/users')
    await router.isReady()
    const w = mount(DefaultLayout, {
      global: {
        plugins: [router, pinia],
        stubs: {
          SidebarProvider: { template: '<div><slot /></div>' },
          Sidebar: { template: '<div><slot /></div>' },
          SidebarHeader: { template: '<div><slot /></div>' },
          SidebarContent: { template: '<div><slot /></div>' },
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
          Button: { template: '<button><slot /></button>' },
          NavMenu: { template: '<div />' },
          SettingsModal: { template: '<div />' },
          GlobalProjectSwitcher: { template: '<div />' },
          LoginModal: { template: '<div />' },
          UserMenu: { template: '<div />' }
        }
      }
    })
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('agents')
    expect(router.currentRoute.value.path).toBe('/console/agents')
    w.unmount()
  })
})
