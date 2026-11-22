import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { i18n } from '@/locales'
import { uiStubs, sampleAgent } from '@/test/helpers'
import AgentListPanel from '@/components/agent/AgentListPanel.vue'
import type { AgentSchedule } from '@/services/types'

/**
 * 本次拆分出的两个 agent 组件（plan v3 §3.6）：
 * AgentListPanel 负责左栏渲染与事件转发（助手编辑表单已随 v2 下线）。
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
