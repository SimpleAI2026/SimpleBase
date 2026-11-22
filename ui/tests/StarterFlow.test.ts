import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { setI18nLocale } from '@/locales'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import Home from '@/pages/Home.vue'
import { savePendingStarter } from '@/utils/starter-storage'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

/* ---------------------------------- Home ---------------------------------- */

async function mountHome(opts: { apiKey?: boolean } = {}) {
  setActivePinia(createPinia())
  const pinia = createPinia()
  setActivePinia(pinia)
  if (opts.apiKey) useAuthStore().apiKey = 'k'
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: Home },
      { path: '/console/agents', name: 'agents', component: { template: '<div>agents</div>' } },
      { path: '/docs/:pathMatch(.*)*', name: 'docs', component: { template: '<div>docs</div>' } }
    ]
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(Home, { global: { plugins: [router, pinia], stubs: { ...uiStubs, RouterLink: false, LoginModal: { template: '<div data-login />' } } } })
  await flushPromises()
  return { wrapper, router }
}

describe('Home：一句话建应用', () => {
  beforeEach(() => {
    setI18nLocale('zh-CN')
    // 先还原 spy，再重装 api 默认值：restoreAllMocks 会把 vi.fn 的默认实现一并清掉。
    vi.restoreAllMocks()
    resetApiMocks()
    sessionStorage.clear()
  })

  it('卡片区位于首屏之后、capabilities 之前', async () => {
    const { wrapper } = await mountHome()
    const sections = wrapper.findAll('main > section').map((s) => s.attributes('aria-labelledby') || 'hero')
    expect(sections).toEqual(['hero', 'starters-heading', 'capabilities-heading'])
    expect(wrapper.findAll('[data-starter]')).toHaveLength(3)
    expect(wrapper.get('#starters-heading').text()).toBe('试试这样说')
  })

  it('点卡片打开确认弹窗（还没建项目）', async () => {
    const { wrapper } = await mountHome({ apiKey: true })
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
    await wrapper.get('[data-starter="dashboard"]').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    expect(api.projects.create).not.toHaveBeenCalled()
  })

  it('已登录：确认后建项目并跳转 agents?starter=', async () => {
    api.projects.create.mockResolvedValue({ id: 'ab12cd34', name: '销售数据看板', createdAt: 't' })
    api.projects.list.mockResolvedValue([])
    const { wrapper, router } = await mountHome({ apiKey: true })
    await wrapper.get('[data-starter="dashboard"]').trigger('click')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.projects.create).toHaveBeenCalledTimes(1)
    expect(api.projects.create.mock.calls[0][0].id).toMatch(/^[a-z0-9]{8}$/)
    expect(router.currentRoute.value.name).toBe('agents')
    expect(router.currentRoute.value.query.starter).toBe('dashboard')
    expect(useProjectStore().id).toBe('ab12cd34')
  })

  it('未登录：关闭确认弹窗、打开登录框并记住意图，不建项目', async () => {
    const { wrapper } = await mountHome({ apiKey: false })
    await wrapper.get('[data-starter="notice"]').trigger('click')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.projects.create).not.toHaveBeenCalled()
    expect(useAuthStore().loginOpen).toBe(true)
    expect(wrapper.find('.sb-modal').exists()).toBe(false)
    expect(sessionStorage.getItem('sb_pending_starter')).toContain('notice')
  })

  it('登录成功后续跑同一张卡片，并预填之前的项目名与引擎', async () => {
    const { wrapper } = await mountHome({ apiKey: false })
    savePendingStarter({ templateId: 'portfolio', name: '我的作品集', engine: 'sqlite' })
    // 模拟登录成功：登录态变化触发续跑。
    const auth = useAuthStore()
    auth.user = { id: 'u', username: 'a', role: 'user' } as never
    auth.accessToken = 't'
    await flushPromises()
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    expect((wrapper.get('#starter-project-name').element as HTMLInputElement).value).toBe('我的作品集')
    // 只续跑一次：pending 已被消费
    expect(sessionStorage.getItem('sb_pending_starter')).toBeNull()
  })

  it('进入页面时已有 pending 且已登录：自动续跑', async () => {
    savePendingStarter({ templateId: 'notice', name: '公告', engine: 'duckdb' })
    const { wrapper } = await mountHome({ apiKey: true })
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
  })

  it('建项目失败：提示错误，不跳转', async () => {
    api.projects.create.mockRejectedValue(Object.assign(new Error('配额不足'), { status: 403 }))
    const err = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const { wrapper, router } = await mountHome({ apiKey: true })
    await wrapper.get('[data-starter="dashboard"]').trigger('click')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(err).toHaveBeenCalledWith('配额不足')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('原有首屏与文档入口不受影响', async () => {
    const { wrapper } = await mountHome()
    expect(wrapper.find('h1').text()).toContain('为 SaaS 构建的 AI 工作台')
    expect(wrapper.find('main a[href="/console/agents"]').text()).toContain('进入工作台')
    expect(wrapper.findAll('[data-capability]').map((n) => n.attributes('data-capability'))).toEqual(['agent', 'ship', 'cost'])
  })
})
