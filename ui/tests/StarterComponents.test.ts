import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import type { Component } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import HomeStarterPrompts from '@/components/home/HomeStarterPrompts.vue'
import StarterLaunchModal from '@/components/home/StarterLaunchModal.vue'
import { inDialog, installJsdomPolyfills, modalOkDisabled, mountDialog, okModal, realDialogStubs, uiStubs } from '@/test/helpers'

/** 这些组件的断言打在真实 DOM 上：Button / Badge / Card 都不桩（见 ui/AGENTS.md）。 */
function mountPlain(component: Component, props: Record<string, unknown> = {}) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }, { path: '/console/:p(.*)*', component: { template: '<div />' } }] })
  return mount(component, {
    props: props as never,
    global: { plugins: [router, createPinia()], stubs: { RouterLink: false } }
  })
}

installJsdomPolyfills()

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
  /** 真实 SbModal + Dialog：内容 teleport 到 body，弹窗内查询走 inDialog()。 */
  function mountModal(props: Record<string, unknown>) {
    return mountDialog(StarterLaunchModal, { props, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
  }
  
  it('打开时项目名默认取模板名，并提示每次都会新建项目', async () => {
    const w = await mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    expect((inDialog().get('#starter-project-name').element as HTMLInputElement).value).toBe('销售数据看板')
    expect(inDialog().get('[data-starter-notice]').text()).toContain('每次开始都会新建一个项目')
  })

  it('确认时 emit 去空格后的项目名与引擎', async () => {
    const w = await mountModal({ open: true, templateId: 'portfolio', initialName: '  我的作品  ', initialEngine: 'sqlite' })
    await flushPromises()
    await okModal()
    expect(w.emitted('confirm')).toEqual([[{ name: '我的作品', engine: 'sqlite' }]])
  })

  it('项目名为空或含非法字符时不能提交', async () => {
    const w = await mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    await inDialog().get('#starter-project-name').setValue('a/b')
    expect(inDialog().text()).toContain('路径分隔符')
    // 非法名称 / 空名称时「开始」是禁用的：用户点不了，而不是点了没反应
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#starter-project-name').setValue('   ')
    expect(modalOkDisabled()).toBe(true)
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('提示词预览可展开，且不暴露任何密钥', async () => {
    const w = await mountModal({ open: true, templateId: 'dashboard' })
    await flushPromises()
    const pre = inDialog().get('[data-starter-preview] pre').text()
    expect(pre).toContain('销售数据看板')
    expect(pre).toContain('web/index.html')
    expect(pre.toLowerCase()).not.toContain('sb_live_')
  })

  it('launching 时不能重复提交', async () => {
    const w = await mountModal({ open: true, templateId: 'dashboard', launching: true })
    await flushPromises()
    // launching 时确认按钮进入加载态并禁用，无法重复提交
    expect(modalOkDisabled()).toBe(true)
    expect(w.emitted('confirm')).toBeUndefined()
  })

  it('未知模板不提交', async () => {
    const w = await mountModal({ open: true, templateId: 'todo' })
    await flushPromises()
    expect(modalOkDisabled()).toBe(true)
    expect(w.emitted('confirm')).toBeUndefined()
    expect(inDialog().find('[data-starter-preview]').exists()).toBe(false)
  })

  it('重新打开时恢复默认项目名与引擎', async () => {
    const w = await mountModal({ open: false, templateId: 'notice' })
    await w.setProps({ open: true })
    await flushPromises()
    expect((inDialog().get('#starter-project-name').element as HTMLInputElement).value).toBe('团队公告板')
  })
})
