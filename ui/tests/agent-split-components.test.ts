import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { i18n } from '@/locales'
import { uiStubs, sampleAgent } from '@/test/helpers'
import AgentListPanel from '@/components/agent/AgentListPanel.vue'
import AgentFormModal from '@/components/modal/AgentFormModal.vue'
import type { AgentSchedule } from '@/services/types'

/**
 * 本次拆分出的两个 agent 组件（plan v3 §3.6）：
 * AgentListPanel 负责左栏渲染与事件转发，AgentFormModal 负责编辑弹窗布局。
 * 它们此前只被宿主测试间接覆盖，这里按自身契约直接验证。
 */

function mountList(overrides: Record<string, unknown> = {}) {
  return mount(AgentListPanel, {
    props: {
      agents: [sampleAgent as never],
      activeId: 'ag-1',
      pending: false,
      showSkeleton: false,
      showEmpty: false,
      showError: false,
      refreshing: false,
      error: '',
      scheduleByAgent: {},
      scheduleSummary: (s: AgentSchedule) => `${s.cron_expr}:${s.enabled}`,
      ...overrides
    },
    global: { stubs: uiStubs, plugins: [i18n] }
  })
}

describe('AgentListPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('渲染条目并转发 select；编辑/定时/删除不再由左栏发出', async () => {
    const sched: AgentSchedule = { id: 's1', agent_id: 'ag-1', cron_expr: '0 8 * * *', enabled: true } as never
    const w = mountList({ scheduleByAgent: { 'ag-1': sched } })
    const card = w.findComponent({ name: 'AgentCard' })
    expect(card.exists()).toBe(true)
    // 传入了定时任务时摘要走宿主注入的格式化函数
    expect(card.props('scheduleSummary')).toBe('0 8 * * *:true')
    expect(card.props('scheduleEnabled')).toBe(true)

    card.vm.$emit('select', 'ag-1')
    expect(w.emitted('select')).toEqual([['ag-1']])
    // planv6.0 §3.3：卡片只剩 select，宿主头部的「⋯」菜单接管其余动作。
    expect(card.emitted('edit')).toBeUndefined()
    expect(card.emitted('schedule')).toBeUndefined()
    expect(card.emitted('remove')).toBeUndefined()
    w.unmount()
  })

  it('无定时任务时不传摘要', () => {
    const w = mountList()
    const card = w.findComponent({ name: 'AgentCard' })
    expect(card.props('scheduleSummary')).toBeUndefined()
    expect(card.props('scheduleEnabled')).toBe(false)
    w.unmount()
  })

  it('顶部「新建」图标按钮向上抛事件；刷新改由宿主菜单负责', async () => {
    const w = mountList()
    // 需求 #1：搜索 + 新建都在左栏顶部；新建只留图标，按 aria-label 定位。
    expect(w.find('input[aria-label="搜索助手"]').exists()).toBe(true)
    await w.get('button[aria-label="新建"]').trigger('click')
    expect(w.emitted('create')).toHaveLength(1)
    expect(w.findAll('button').find((b) => b.text().includes('刷新'))).toBeUndefined()
    w.unmount()
  })

  it('搜索按名称与描述过滤条目', async () => {
    const w = mountList()
    const input = w.get('input[aria-label="搜索助手"]')
    await input.setValue('Database')
    expect(w.findAllComponents({ name: 'AgentCard' })).toHaveLength(1)
    // 描述也能命中（sampleAgent.description = 'db helper'）
    await input.setValue('db helper')
    expect(w.findAllComponents({ name: 'AgentCard' })).toHaveLength(1)
    await input.setValue('不存在')
    expect(w.findAllComponents({ name: 'AgentCard' })).toHaveLength(0)
    expect(w.text()).toContain('没有匹配的助手')
    // 清除按钮恢复全部
    await w.get('button[aria-label="清除搜索"]').trigger('click')
    expect(w.findAllComponents({ name: 'AgentCard' })).toHaveLength(1)
    w.unmount()
  })

  it('空态提供创建入口，重试事件透传到宿主', async () => {
    const w = mountList({ agents: [], showEmpty: true })
    expect(w.text()).toContain('暂无 Agent')
    await w.get('.empty-action').trigger('click')
    expect(w.emitted('create')).toHaveLength(1)

    const err = mountList({ showError: true, error: 'boom' })
    const region = err.findComponent({ name: 'SbAsyncRegion' })
    region.vm.$emit('retry')
    expect(err.emitted('reload')).toHaveLength(1)
    w.unmount()
    err.unmount()
  })
})

const form = {
  name: 'Database',
  module: 'database',
  description: '',
  system_prompt: '',
  model_override: '',
  tool_ids: ['list_databases'],
  writable: false
}

function mountForm(overrides: Record<string, unknown> = {}) {
  return mount(AgentFormModal, {
    props: {
      open: true,
      editing: false,
      saving: false,
      form,
      moduleOptions: [
        { label: 'Database（database）', value: 'database', disabled: false },
        { label: 'Sandbox（sandbox）', value: 'sandbox', disabled: true }
      ],
      toolOptions: [{ label: 'list_databases', value: 'list_databases' }],
      modelOptions: ['gpt-4o-mini'],
      defaultModel: '',
      ...overrides
    },
    global: { stubs: uiStubs, plugins: [i18n] }
  })
}

describe('AgentFormModal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('新建与编辑使用不同标题', () => {
    const create = mountForm()
    expect(create.find('.sb-modal').attributes('data-title')).toContain('新建')
    create.unmount()

    const edit = mountForm({ editing: true })
    expect(edit.find('.sb-modal').attributes('data-title')).toContain('编辑')
    edit.unmount()
  })

  it('模型占位符在已知默认模型时带上模型名', () => {
    const plain = mountForm()
    expect(plain.get('#agent-model').attributes('placeholder')).toContain('使用项目默认模型')
    plain.unmount()

    const known = mountForm({ defaultModel: 'gpt-4o-mini' })
    expect(known.get('#agent-model').attributes('placeholder')).toContain('gpt-4o-mini')
    known.unmount()
  })

  it('被禁用的模块选项带未配置提示，已选中工具为默认样式', () => {
    const w = mountForm()
    const options = w.findAll('option')
    expect(options.some((o) => o.text().includes('未配置云沙盒'))).toBe(true)
    // Badge 桩渲染为 span，选中态经 title/文本可辨
    const toolBadge = w.findAll('.badge').find((b) => b.text().includes('list_databases'))
    expect(toolBadge?.attributes('title')).toContain('工具：')
    w.unmount()
  })

  it('模块切换、工具切换与保存事件向上抛', async () => {
    const w = mountForm()
    // Select 桩以 .select-emit 暴露一次 update:modelValue
    await w.get('.select-emit').trigger('click')
    const tool = w.findAll('button, span').find((b) => b.text().trim() === 'list_databases')
      ?? w.findAll('*').find((b) => b.text().trim() === 'list_databases')!
    await tool.trigger('click')
    expect(w.emitted('module-change')).toBeTruthy()
    expect(w.emitted('toggle-tool')).toEqual([['list_databases']])
    w.unmount()
  })

  it('SbModal 的 ok 与 update:open 由宿主的 save / 关闭处理', async () => {
    const w = mountForm()
    // SbModal 桩渲染 .sb-ok / .sb-modal-close，事件即组件对外契约
    await w.get('.sb-ok').trigger('click')
    await w.get('.sb-modal-close').trigger('click')
    expect(w.emitted('save')).toHaveLength(1)
    expect(w.emitted('update:open')).toEqual([[false]])
    w.unmount()
  })

  it('writable 开关按 modelValue 渲染并向上抛 update:writable', async () => {
    // planv6.0 v2 §3.2：助手级写开关。Switch 桩用 reka 真实的 modelValue 契约，
    // 点击后 emit 取反值。
    const off = mountForm()
    expect(off.get('[data-agent-writable]').exists()).toBe(true)
    await off.get('[data-agent-writable]').trigger('click')
    expect(off.emitted('update:writable')).toEqual([[true]])
    off.unmount()

    const on = mountForm({ form: { ...form, writable: true } })
    await on.get('[data-agent-writable]').trigger('click')
    expect(on.emitted('update:writable')).toEqual([[false]])
    on.unmount()
  })

  it('saving 状态下确认按钮禁用', () => {
    const w = mountForm({ saving: true })
    // SbModal 桩会渲染 .sb-ok，禁用态由 okDisabled(confirmLoading) 驱动
    expect(w.find('.sb-ok').exists()).toBe(true)
    w.unmount()
  })
})

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})
