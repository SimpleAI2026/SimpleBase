import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import type { Component } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import HomeStarterPrompts from '@/components/home/HomeStarterPrompts.vue'
import StarterLaunchModal from '@/components/home/StarterLaunchModal.vue'
import StarterProgress from '@/components/agent/StarterProgress.vue'
import StarterDeliveryCard from '@/components/agent/StarterDeliveryCard.vue'
import { uiStubs } from '@/test/helpers'
import type { StarterDelivery } from '@/utils/starter-delivery'

/** 这些组件的断言打在真实 DOM 上：Button / Badge / Card 都不桩（见 ui/AGENTS.md）。 */
function mountPlain(component: Component, props: Record<string, unknown> = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }, { path: '/console/:p(.*)*', component: { template: '<div />' } }] })
  return mount(component, {
    props: props as never,
    global: { plugins: [router, createPinia()], stubs: { RouterLink: false } }
  })
}

describe('HomeStarterPrompts', () => {
  it('渲染 3 张卡片，带标题、说明与产出徽标', () => {
    const w = mountPlain(HomeStarterPrompts)
    const cards = w.findAll('[data-starter]')
    expect(cards.map((c) => c.attributes('data-starter'))).toEqual(['dashboard', 'portfolio', 'notice'])
    expect(w.get('[data-starter="dashboard"]').text()).toContain('销售数据看板')
    const outputs = w.get('[data-starter="dashboard"]').findAll('[data-output]').map((b) => b.attributes('data-output'))
    expect(outputs).toEqual(['table', 'cron', 'site'])
    expect(w.get('[data-starter="portfolio"]').find('[data-output="cron"]').exists()).toBe(false)
  })

  it('点击只 emit pick(templateId)，不做任何建项目动作', async () => {
    const w = mountPlain(HomeStarterPrompts)
    await w.get('[data-starter="notice"]').trigger('click')
    expect(w.emitted('pick')).toEqual([['notice']])
  })

  it('响应式网格：sm 两列、lg 三列', () => {
    const cls = mountPlain(HomeStarterPrompts).get('[data-starter-grid]').classes()
    expect(cls).toContain('sm:grid-cols-2')
    expect(cls).toContain('lg:grid-cols-3')
  })
})

describe('StarterLaunchModal', () => {
  /** 弹窗类组件走 uiStubs（Dialog 经 Teleport，真实 DOM 不在 wrapper 内）。 */
  function mountModal(props: Record<string, unknown>) {
    return mount(StarterLaunchModal, { props, global: { stubs: uiStubs } })
  }
  const okBtn = (w: ReturnType<typeof mountModal>) => w.get('.sb-ok')

  it('打开时项目名默认取模板名，并提示每次都会新建项目', async () => {
    const w = mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    expect((w.get('#starter-project-name').element as HTMLInputElement).value).toBe('销售数据看板')
    expect(w.get('[data-starter-notice]').text()).toContain('每次开始都会新建一个项目')
  })

  it('确认时 emit 去空格后的项目名与引擎', async () => {
    const w = mountModal({ open: true, templateId: 'portfolio', initialName: '  我的作品  ', initialEngine: 'sqlite' })
    await flushPromises()
    await okBtn(w).trigger('click')
    expect(w.emitted('confirm')).toEqual([[{ name: '我的作品', engine: 'sqlite' }]])
  })

  it('项目名为空或含非法字符时不能提交', async () => {
    const w = mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    await w.get('#starter-project-name').setValue('a/b')
    expect(w.text()).toContain('路径分隔符')
    await okBtn(w).trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
    await w.get('#starter-project-name').setValue('   ')
    await okBtn(w).trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('提示词预览可展开，且不暴露任何密钥', async () => {
    const w = mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    const pre = w.get('[data-starter-preview] pre').text()
    expect(pre).toContain('销售数据看板')
    expect(pre).toContain('web/index.html')
    expect(pre.toLowerCase()).not.toContain('sb_live_')
  })

  it('launching 时不能重复提交', async () => {
    const w = mountModal({ open: true, templateId: 'dashboard', launching: true })
    await flushPromises()
    await okBtn(w).trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('未知模板不提交', async () => {
    const w = mountModal({ open: true, templateId: 'todo' })
    await flushPromises()
    await okBtn(w).trigger('click')
    expect(w.emitted('confirm')).toBeUndefined()
    expect(w.find('[data-starter-preview]').exists()).toBe(false)
  })

  it('重新打开时恢复默认项目名与引擎', async () => {
    const w = mountModal({ open: false, templateId: 'notice' })
    await w.setProps({ open: true })
    await flushPromises()
    expect((w.get('#starter-project-name').element as HTMLInputElement).value).toBe('团队公告板')
  })
})

describe('StarterProgress', () => {
  const items = [
    { stage: 'plan', status: 'done' },
    { stage: 'data', status: 'running' },
    { stage: 'func', status: 'pending' },
    { stage: 'cron', status: 'skipped' },
    { stage: 'site', status: 'failed' },
    { stage: 'verify', status: 'pending' }
  ] as const

  it('按状态渲染 6 个阶段', () => {
    const w = mountPlain(StarterProgress, { items: [...items] })
    const rows = w.findAll('[data-stage]')
    expect(rows.map((r) => r.attributes('data-status'))).toEqual(['done', 'running', 'pending', 'skipped', 'failed', 'pending'])
    expect(rows[0].text()).toContain('规划')
    expect(w.get('[data-stage="cron"] span span').classes()).toContain('line-through')
  })

  it('失败时给出三个操作并各自 emit', async () => {
    const w = mountPlain(StarterProgress, { items: [...items], failure: { stage: 'site', reason: '站点没发布' } })
    expect(w.get('[data-starter-failure]').text()).toContain('站点没发布')
    await w.get('[data-action="retry"]').trigger('click')
    await w.get('[data-action="skip"]').trigger('click')
    await w.get('[data-action="takeover"]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    expect(w.emitted('skip')).toHaveLength(1)
    expect(w.emitted('takeover')).toHaveLength(1)
  })

  it('没有失败时不显示操作区', () => {
    expect(mountPlain(StarterProgress, { items: [...items] }).find('[data-starter-failure]').exists()).toBe(false)
  })
})

describe('StarterDeliveryCard', () => {
  const delivery: StarterDelivery = {
    siteUrl: 'https://evil.example.net/',
    functions: [{ name: 'listTodos', url: 'https://evil.example.net/x' }],
    tables: ['orders'],
    collections: ['notes'],
    cron: [{ name: 'daily', expr: '0 2 * * *' }],
    next: ['去控制台看看']
  }

  it('站点地址与后端不同源时丢弃，改用后端 baseUrl；函数地址按当前 origin 重新拼接', () => {
    const w = mountPlain(StarterDeliveryCard, { delivery, projectId: 'ab12cd34', siteBaseUrl: 'https://ab12cd34.app.example.com', origin: 'https://console.example.com' })
    const site = w.get('[data-delivery="site"]')
    expect(site.text()).toContain('https://ab12cd34.app.example.com/web/')
    expect(site.text()).not.toContain('evil')
    expect(site.get('a').attributes('href')).toBe('https://ab12cd34.app.example.com/web/')
    expect(site.get('a').attributes('rel')).toContain('noopener')
    const fn = w.get('[data-delivery="functions"]').text()
    expect(fn).toContain('https://console.example.com/go/ab12cd34/main/listTodos')
    expect(fn).not.toContain('evil')
  })

  it('后端没给 baseUrl：不展示助手给的站点地址', () => {
    const w = mountPlain(StarterDeliveryCard, { delivery, projectId: 'p', origin: 'https://c.example.com' })
    expect(w.find('[data-delivery="site"]').exists()).toBe(false)
    expect(w.html()).not.toContain('evil.example.net')
  })

  it('站点被跳过时给出说明', () => {
    const w = mountPlain(StarterDeliveryCard, { delivery: { ...delivery, siteUrl: '' }, projectId: 'p', siteSkipped: true, origin: 'https://c.example.com' })
    expect(w.get('[data-delivery="site-skipped"]').text()).toContain('站点不可发布')
  })

  it('字段缺失逐项隐藏，但资源入口总在', () => {
    const empty: StarterDelivery = { siteUrl: '', functions: [], tables: [], collections: [], cron: [], next: [] }
    const w = mountPlain(StarterDeliveryCard, { delivery: empty, projectId: 'p', origin: 'https://c.example.com' })
    for (const k of ['site', 'functions', 'data', 'cron', 'next']) expect(w.find(`[data-delivery="${k}"]`).exists(), k).toBe(false)
    expect(w.findAll('[data-resource]').map((a) => a.attributes('data-resource'))).toEqual(['/console/databases', '/console/gofunctions', '/console/cron-jobs', '/console/s3'])
  })

  it('curl 示例不含任何真实密钥，只用占位符', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    vi.spyOn(toast, 'success').mockImplementation(() => '' as never)
    const w = mountPlain(StarterDeliveryCard, { delivery, projectId: 'ab12cd34', siteBaseUrl: 'https://ab12cd34.app.example.com', origin: 'https://c.example.com' })
    await w.get('[data-action="copy-curl"]').trigger('click')
    await flushPromises()
    const text = writeText.mock.calls[0][0] as string
    expect(text).toContain('<API_KEY>')
    expect(text).not.toMatch(/sb_live_/)
    expect(toast.success).toHaveBeenCalled()
  })

  it('复制失败时提示而不是静默', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) }, configurable: true })
    const err = vi.spyOn(toast, 'error').mockImplementation(() => '' as never)
    const w = mountPlain(StarterDeliveryCard, { delivery, projectId: 'p', siteBaseUrl: 'https://p.example.com', origin: 'https://c.example.com' })
    await w.get('[data-action="copy-site"]').trigger('click')
    await flushPromises()
    expect(err).toHaveBeenCalled()
  })
})
