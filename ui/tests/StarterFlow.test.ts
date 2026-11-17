import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { setI18nLocale } from '@/locales'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp, sampleAgent, uiStubs } from '@/test/helpers'
import Home from '@/pages/Home.vue'
import AgentManager from '@/pages/AgentManager.vue'
import { markStarterSent, savePendingStarter } from '@/utils/starter-storage'
import type { AgentStreamHandlers } from '@/services/types'

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

/* ------------------------------ AgentManager ------------------------------ */

const generalAgent = { ...sampleAgent, id: 'ag-gen', name: '通用助手', module: 'general', builtin_key: 'general' }

/** 捕获 streamRun 的请求体与回调，让测试手动结束每一轮。 */
function captureStream() {
  const runs: { body: Record<string, unknown>; handlers: AgentStreamHandlers }[] = []
  api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, body: Record<string, unknown>, handlers: AgentStreamHandlers) => {
    runs.push({ body, handlers })
    return { close: vi.fn() }
  })
  return runs
}

async function mountManager(path: string) {
  return mountWithApp(AgentManager, {
    path,
    stubs: {
      AgentComposer: { props: ['sending', 'mentionAgents', 'modelValue'], emits: ['send', 'stop', 'update:modelValue'], template: '<div class="ai" />', methods: { focus: () => undefined } },
      ThreadSwitcher: { props: ['threads', 'activeId', 'busy', 'runningThreadIds'], emits: ['create', 'select', 'remove'], template: '<div class="ts" />' }
    }
  })
}

describe('AgentManager：?starter= 搭建会话', () => {
  beforeEach(() => {
    setI18nLocale('zh-CN')
    // 先还原 spy，再重装 api 默认值：restoreAllMocks 会把 vi.fn 的默认实现一并清掉。
    vi.restoreAllMocks()
    resetApiMocks()
    sessionStorage.clear()
    api.agents.list.mockResolvedValue([generalAgent])
    api.agents.modules.mockResolvedValue([])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-old', title: '旧会话', agent_id: 'ag-gen', created_at: 't', updated_at: 't' }])
    api.agentThreads.page.mockResolvedValue({ threads: [{ id: 'th-old', title: '旧会话', agent_id: 'ag-gen', created_at: 't', updated_at: 't' }], next_cursor: '' })
    api.agentThreads.messages.mockResolvedValue([])
    api.agentThreads.create.mockResolvedValue({ id: 'th-new', title: '销售数据看板', agent_id: 'ag-gen', starter_template: 'dashboard', created_at: 't', updated_at: 't' })
    api.cronjobs.list.mockResolvedValue([])
    api.s3.getSite.mockResolvedValue({ enabled: false, baseUrl: undefined, publishable: true })
  })

  it('选中通用助手 → 新建搭建会话（带 starter_template）→ 自动发送第一阶段并带 starter_stage', async () => {
    const runs = captureStream()
    await mountManager('/agents?starter=dashboard')
    await flushPromises()
    // 新建会话（不复用已有会话），标题 = 模板名，starter_template 传给服务端校验
    expect(api.agentThreads.create).toHaveBeenCalledWith('testprj1', '销售数据看板', 'ag-gen', 'dashboard')
    // 自动发送一次，带阶段；技能由服务端按阶段决定，客户端不传 skills
    expect(runs).toHaveLength(1)
    expect(runs[0].body).toMatchObject({ starter_stage: 'plan', mentions: [{ agent_id: 'ag-gen' }] })
    expect(runs[0].body).not.toHaveProperty('skills')
    expect(String(runs[0].body.content)).toContain('规划阶段')
  })

  it('发送后立即去掉 starter query，刷新不会重复发送', async () => {
    captureStream()
    const { router } = await mountManager('/agents?starter=dashboard')
    await flushPromises()
    expect(router.currentRoute.value.query.starter).toBeUndefined()
  })

  it('同一项目同一模板只自动发送一次（sessionStorage 去重）', async () => {
    markStarterSent('testprj1', 'dashboard')
    const runs = captureStream()
    await mountManager('/agents?starter=dashboard')
    await flushPromises()
    expect(api.agentThreads.create).not.toHaveBeenCalledWith('testprj1', expect.anything(), expect.anything(), 'dashboard')
    expect(runs).toHaveLength(0)
  })

  it('未知模板（含 B3 写入类）被忽略，不建会话、不发送', async () => {
    const runs = captureStream()
    const { router } = await mountManager('/agents?starter=todo')
    await flushPromises()
    expect(runs).toHaveLength(0)
    expect(router.currentRoute.value.query.starter).toBeUndefined()
  })

  it('通用助手已被删除：提示并不发送', async () => {
    api.agents.list.mockResolvedValue([{ ...sampleAgent, id: 'ag-db', module: 'database', builtin_key: 'database' }])
    const err = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const runs = captureStream()
    await mountManager('/agents?starter=dashboard')
    await flushPromises()
    expect(err).toHaveBeenCalledWith('通用助手已被删除，请先在云助手里新建一个通用助手')
    expect(runs).toHaveLength(0)
  })

  it('服务端拒绝搭建会话（400 starter_not_allowed）：提示，不发送', async () => {
    api.agentThreads.create.mockImplementation(async (_p: string, _t: string, _a: string, starter?: string) => {
      if (starter) throw Object.assign(new Error('starter session is not allowed'), { status: 400, code: 'starter_not_allowed' })
      return { id: 'th-x', title: 't', created_at: 't', updated_at: 't' }
    })
    const err = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const runs = captureStream()
    await mountManager('/agents?starter=dashboard')
    await flushPromises()
    expect(err).toHaveBeenCalled()
    expect(runs).toHaveLength(0)
  })

  it('上一阶段成功后自动发下一阶段，并显示进度', async () => {
    const runs = captureStream()
    const { wrapper } = await mountManager('/agents?starter=dashboard')
    await flushPromises()
    expect(wrapper.find('[data-starter-progress]').exists()).toBe(true)
    // plan 阶段：助手给出非空清单 → 结束
    runs[0].handlers.onToken?.('数据模型：orders')
    runs[0].handlers.onEnd?.('stop')
    await flushPromises()
    expect(runs).toHaveLength(2)
    expect(runs[1].body.starter_stage).toBe('data')
    expect(wrapper.get('[data-stage="plan"]').attributes('data-status')).toBe('done')
    expect(wrapper.get('[data-stage="data"]').attributes('data-status')).toBe('running')
  })

  it('阶段失败：停住并给出重试 / 跳过 / 我来接手', async () => {
    const runs = captureStream()
    const { wrapper } = await mountManager('/agents?starter=dashboard')
    await flushPromises()
    runs[0].handlers.onError?.(new Error('模型出错'))
    await flushPromises()
    expect(runs).toHaveLength(1)
    expect(wrapper.find('[data-starter-failure]').exists()).toBe(true)
    // 重试：重新发送同一阶段
    await wrapper.get('[data-action="retry"]').trigger('click')
    await flushPromises()
    expect(runs).toHaveLength(2)
    expect(runs[1].body.starter_stage).toBe('plan')
    // 我来接手：退出自动模式
    runs[1].handlers.onError?.(new Error('还是失败'))
    await flushPromises()
    await wrapper.get('[data-action="takeover"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-starter-failure]').exists()).toBe(false)
    expect(runs).toHaveLength(2)
  })

  it('没有 starter 参数时行为不变：不新建会话、不自动发送', async () => {
    const runs = captureStream()
    await mountManager('/agents')
    await flushPromises()
    expect(api.agentThreads.create).not.toHaveBeenCalled()
    expect(runs).toHaveLength(0)
  })
})
