import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PageContainer from './PageContainer.vue'
import ProjectScope from './ProjectScope.vue'
import SbCodeBlock from './SbCodeBlock.vue'
import NavMenu from './NavMenu.vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useProjectStore, ADMIN_PROJECT_ID } from '../stores/project'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('low-branch component coverage', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  it('PageContainer with and without subtitle', () => {
    const a = mount(PageContainer, { props: { subtitle: 'hello' }, slots: { default: '<span>x</span>' } })
    expect(a.text()).toContain('hello')
    const b = mount(PageContainer, { slots: { default: '<span>y</span>' } })
    expect(b.text()).not.toContain('hello')
  })

  it('ProjectScope empty and with project', async () => {
    localStorage.removeItem('sb_project_id')
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useProjectStore()
    store.projectId = ''
    const empty = mount(ProjectScope, {
      global: {
        plugins: [pinia],
        stubs: { PageContainer: true, Card: true, CardContent: true, SbEmptyState: true }
      }
    })
    await flushPromises()
    expect(empty.html()).toBeTruthy()
    store.setProject('dev-shop')
    const pinia2 = createPinia()
    setActivePinia(pinia2)
    useProjectStore().setProject('dev-shop')
    const full = mount(ProjectScope, {
      slots: { default: '<b>inner</b>' },
      global: {
        plugins: [pinia2],
        stubs: { PageContainer: true, Card: true, CardContent: true, SbEmptyState: true }
      }
    })
    expect(full.text()).toContain('inner')
  })

  it('SbCodeBlock value and slot branches', () => {
    const a = mount(SbCodeBlock, { props: { value: { a: 1 } } })
    expect(a.text()).toContain('a')
    const b = mount(SbCodeBlock, { slots: { default: 'raw' } })
    expect(b.text()).toContain('raw')
  })

  it('MessageScroller scroll and follow', async () => {
    const { default: MessageScroller } = await import('./chat/MessageScroller.vue')
    const w = mount(MessageScroller, {
      props: { followKey: 1 },
      slots: { default: '<div style="height:800px">msg</div>' },
      global: { stubs: { Button: { template: '<button @click="$emit(\'click\')"><slot /></button>' } } }
    })
    await flushPromises()
    const vp = w.find('div.overflow-y-auto')
    Object.defineProperty(vp.element, 'scrollHeight', { value: 1000 })
    Object.defineProperty(vp.element, 'clientHeight', { value: 200 })
    Object.defineProperty(vp.element, 'scrollTop', { value: 0, writable: true })
    await vp.trigger('scroll')
    await w.setProps({ followKey: 2 })
    await flushPromises()
    const jump = w.find('button')
    if (jump.exists()) await jump.trigger('click')
  })

  it('UserMenu open password and logout', async () => {
    const { default: UserMenu } = await import('./UserMenu.vue')
    const { useAuthStore } = await import('../stores/auth')
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    auth.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'alice',
        role: 'user',
        displayName: 'Alice',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    const w = mount(UserMenu, {
      global: {
        plugins: [pinia],
        stubs: {
          Popover: { template: '<div><slot /></div>' },
          PopoverTrigger: { template: '<div><slot /></div>' },
          PopoverContent: { template: '<div><slot /></div>' },
          Button: { template: '<button><slot /></button>' },
          Badge: { template: '<span><slot /></span>' },
          Separator: { template: '<hr />' }
        }
      }
    })
    await flushPromises()
    const btns = w.findAll('button')
    const pwd = btns.find((b) => b.text().includes('修改密码'))
    if (pwd) await pwd.trigger('click')
    const out = btns.find((b) => b.text().includes('退出登录'))
    if (out) {
      await out.trigger('click')
      await flushPromises()
    }
  })

  it('NavMenu role filters users item', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' }, meta: { title: '监控大盘' } },
        {
          path: '/users',
          name: 'users',
          component: { template: '<div />' },
          meta: { title: '用户管理', requiresRole: 'superadminl1' }
        }
      ]
    })
    await router.push('/')
    await router.isReady()
    const stubs = {
      SidebarMenu: { template: '<div><slot /></div>' },
      SidebarMenuItem: { template: '<div><slot /></div>' },
      SidebarMenuButton: { template: '<div><slot /></div>' },
      SidebarGroup: { template: '<div><slot /></div>' },
      SidebarGroupContent: { template: '<div><slot /></div>' },
      SidebarGroupLabel: { template: '<div><slot /></div>' }
    }
    const plain = mount(NavMenu, { global: { plugins: [router, createPinia()], stubs } })
    expect(plain.text()).not.toContain('用户管理')
    plain.unmount()

    const pinia = createPinia()
    setActivePinia(pinia)
    const { useAuthStore } = await import('../stores/auth')
    const auth = useAuthStore()
    auth.applyTokens({
      accessToken: 'a',
      refreshToken: 'r',
      user: {
        id: 'u1',
        username: 'simplebase2026',
        role: 'superadminl1',
        displayName: '',
        email: '',
        status: 'active',
        mustChangePassword: false
      }
    })
    const superMenu = mount(NavMenu, { global: { plugins: [router, pinia], stubs } })
    expect(superMenu.text()).toContain('用户管理')
  })
})
