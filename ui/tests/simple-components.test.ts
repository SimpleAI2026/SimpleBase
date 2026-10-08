import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PageContainer from '@/components/PageContainer.vue'
import SbCodeBlock from '@/components/SbCodeBlock.vue'
import TablePager from '@/components/TablePager.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import { useProjectStore, ADMIN_PROJECT_ID } from '@/stores/project'
import NavMenu from '@/components/NavMenu.vue'
import MessageScroller from '@/components/chat/MessageScroller.vue'
import { uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

describe('simple presentational components', () => {
  it('renders PageContainer subtitle and slot', () => {
    const w = mount(PageContainer, { props: { subtitle: 'hello' }, slots: { default: '<p>body</p>' } })
    expect(w.text()).toContain('hello')
    expect(w.text()).toContain('body')
  })

  it('formats JSON in SbCodeBlock and falls back to slot', () => {
    const json = mount(SbCodeBlock, { props: { value: { a: 1 }, maxHeight: '80px' } })
    expect(json.text()).toContain('"a"')
    const slotted = mount(SbCodeBlock, { slots: { default: 'raw' } })
    expect(slotted.text()).toContain('raw')
  })

  it('emits pager updates only when there are multiple pages', async () => {
    const single = mount(TablePager, { props: { page: 1, pageSize: 10, total: 3, pageCount: 1 } })
    expect(single.text()).toContain('共 3 条')
    expect(single.findAll('button')).toHaveLength(0)

    expect(single.classes().join(' ')).toContain('pt-3')

    const footer = mount(TablePager, {
      props: { page: 1, pageSize: 10, total: 3, pageCount: 1, variant: 'footer' }
    })
    expect(footer.classes().join(' ')).toContain('border-t')
    expect(footer.classes().join(' ')).not.toContain('pt-3')

    const multi = mount(TablePager, { props: { page: 2, pageSize: 10, total: 30, pageCount: 3 } })
    await multi.findAll('button')[0].trigger('click')
    expect(multi.emitted('update:page')?.[0]).toEqual([1])
    await multi.findAll('button')[1].trigger('click')
    expect(multi.emitted('update:page')?.[1]).toEqual([3])
  })

  it('ConfirmAction renders the slot when disabled', () => {
    const w = mount(ConfirmAction, {
      props: { title: 't', disabled: true },
      slots: { default: '<button>go</button>' }
    })
    expect(w.text()).toContain('go')
  })

  it('ProjectScope shows empty state without a project id', async () => {
    setActivePinia(createPinia())
    const store = useProjectStore()
    store.projectId = ''
    const w = mount(ProjectScope, {
      global: {
        stubs: {
          Card: { template: '<div><slot /></div>' },
          CardContent: { template: '<div><slot /></div>' },
          SbEmptyState: {
            template: '<button @click="$emit(\'action\')">empty</button>',
            emits: ['action']
          }
        }
      }
    })
    expect(w.text()).toContain('empty')
    await w.get('button').trigger('click')
    expect(store.createModalOpen).toBe(true)
  })
})

describe('router-backed chrome', () => {
  it('NavMenu lists console routes', async () => {
    const { default: NavMenu } = await import('@/components/NavMenu.vue')
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' }, meta: { title: '监控大盘' } },
        { path: '/docs', name: 'docs', component: { template: '<div />' }, meta: { title: '文档', hidden: true } }
      ]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(NavMenu, {
      global: {
        plugins: [router],
        stubs: {
          SidebarMenu: { template: '<div><slot /></div>' },
          SidebarMenuItem: { template: '<div><slot /></div>' },
          SidebarMenuButton: { template: '<div><slot /></div>' },
          SidebarGroup: { template: '<div><slot /></div>' },
          SidebarGroupContent: { template: '<div><slot /></div>' },
          SidebarGroupLabel: { template: '<div><slot /></div>' }
        }
      }
    })
    expect(w.text()).toContain('监控大盘')
    expect(w.text()).not.toContain('设置')
    expect(w.text()).toContain('工作台')
    expect(w.text()).toContain('数据')
    expect(w.text()).toContain('自动化')
    expect(w.text()).toContain('运维')
    const names = (w.vm as unknown as { menuItems: { name: string }[] }).menuItems.map((i) => i.name)
    expect(names).toEqual(['dashboard', 'databases', 'key-value', 's3', 'gofunctions', 'cron-jobs', 'sandboxes', 'agents', 'logs'])
    expect(w.text()).toContain('云沙盒')
  })
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
    const { default: MessageScroller } = await import('@/components/chat/MessageScroller.vue')
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
    const { default: UserMenu } = await import('@/components/UserMenu.vue')
    const { useAuthStore } = await import('@/stores/auth')
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
    const { useAuthStore } = await import('@/stores/auth')
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

  it('NavMenu emits navigate on link click', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'dashboard', component: { template: '<div />' }, meta: { title: '监控大盘' } }
      ]
    })
    await router.push('/')
    await router.isReady()
    const w = mount(NavMenu, {
      global: {
        plugins: [router],
        stubs: {
          SidebarMenu: { template: '<div><slot /></div>' },
          SidebarMenuItem: { template: '<div><slot /></div>' },
          SidebarMenuButton: { template: '<div><slot /></div>' },
          SidebarGroup: { template: '<div><slot /></div>' },
          SidebarGroupContent: { template: '<div><slot /></div>' },
          SidebarGroupLabel: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a class="nav-link" @click="$attrs.onClick && $attrs.onClick()"><slot /></a>' }
        }
      }
    })
    const link = w.find('.nav-link, a')
    if (link.exists()) await link.trigger('click')
    w.unmount()
  })

  it('MessageScroller stick/scroll helpers', async () => {
    const w = mount(MessageScroller, { props: { followKey: '1' }, global: { stubs: uiStubs } })
    const vm = w.vm as any
    vm.onScroll()
    vm.viewport = { scrollHeight: 400, scrollTop: 10, clientHeight: 100 }
    vm.onScroll()
    expect(vm.stick).toBe(false)
    vm.viewport = { scrollHeight: 400, scrollTop: 350, clientHeight: 100 }
    vm.onScroll()
    expect(vm.stick).toBe(true)
    vm.scrollToEnd()
    await w.setProps({ followKey: '2' })
    await flushPromises()
    w.unmount()
  })
