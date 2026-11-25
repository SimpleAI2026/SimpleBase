// 协作群 UI 组件测试。
// 覆盖：群成员弹窗（两个页签、成员置灰、新建校验）、Plan 卡（目标 / 步骤 / 分工 / 进度与重试）、
// 人工待办卡（回复/无法处理、只读态）、群设置弹窗的新字段（路由提示词/兜底/人工策略）。
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type { Component } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia } from 'pinia'
import { setI18nLocale } from '@/locales'
import AgentGroupMembersModal from '@/components/modal/AgentGroupMembersModal.vue'
import AgentPlanCard from '@/components/agent/AgentPlanCard.vue'
import AgentHumanTaskCard from '@/components/agent/AgentHumanTaskCard.vue'
import AgentTagModal from '@/components/modal/AgentTagModal.vue'
import { Select } from '@/components/ui/select'
import type { AgentPlanSnapshot, AgentModuleInfo, AgentTag, AgentTask } from '@/services/types'

function task(over: Partial<AgentTask> = {}): AgentTask {
  return {
    id: 't1',
    turnId: 'turn-1',
    seq: 1,
    targetKind: 'agent',
    targetId: 'a1',
    instruction: '核对订单',
    status: 'running',
    attempt: 0,
    dependsOn: [],
    version: 1,
    createdAt: 't',
    ...over
  }
}

const MODULES: AgentModuleInfo[] = [
  { id: 'database', name: 'Database', description: '', default_tools: [], team_supported: false },
  { id: 'logs', name: 'Logs', description: '', default_tools: [], team_supported: false }
]

/**
 * SbModal 是业务封装的弹窗壳（内部 Dialog 会 teleport）。
 * 这里只桩壳、保留内容与「确定」按钮，断言落在真实业务模板上；
 * 不桩 components/ui 下的交互组件。
 */
const SbModalStub = {
  props: ['open', 'title', 'confirmLoading'],
  emits: ['ok', 'update:open'],
  template: `
    <div v-if="open" data-modal-stub>
      <p data-modal-title>{{ title }}</p>
      <slot />
      <button type="button" class="modal-ok" @click="$emit('ok')">ok</button>
    </div>
  `
}

function mountWithI18n(component: Component, props: Record<string, unknown>) {
  return mount(component, {
    props,
    global: { plugins: [createPinia()], stubs: { SbModal: SbModalStub } }
  })
}

/** 真实 reka Select：值由 modelValue 承载；触发器上的 data-* 钩子用来定位第几个下拉。 */
function selects(wrapper: VueWrapper) {
  return wrapper.findAllComponents(Select)
}
const FALLBACK = 0
const HUMAN_POLICY = 1

describe('AgentGroupMembersModal', () => {
  beforeEach(() => setI18nLocale('zh-CN'))

  it('列出全部助手，并把已在群内的置灰', async () => {
    const wrapper = mountWithI18n(AgentGroupMembersModal, {
      open: true,
      groupName: '告警',
      agents: [
        { id: 'a1', name: '数据库助手', module: 'database' },
        { id: 'a2', name: '日志助手', module: 'logs' }
      ],
      memberIds: ['a1'],
      modules: MODULES
    })
    await flushPromises()
    const checks = wrapper.findAll('[data-group-agent-check]')
    expect(checks).toHaveLength(2)
    expect((checks[0].element as HTMLInputElement).disabled).toBe(true)
    expect((checks[1].element as HTMLInputElement).disabled).toBe(false)
    // 已在群内不能再选。
    await checks[0].trigger('change')
    expect(wrapper.find('[data-group-agent-check="a1"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('选中已有助手后提交 agentIds', async () => {
    const wrapper = mountWithI18n(AgentGroupMembersModal, {
      open: true,
      groupName: '告警',
      agents: [{ id: 'a2', name: '日志助手', module: 'logs' }],
      memberIds: [],
      modules: MODULES
    })
    await flushPromises()
    await wrapper.get('[data-group-agent-check="a2"]').trigger('change')
    await wrapper.get('.modal-ok').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual([{ agentIds: ['a2'] }])
    wrapper.unmount()
  })

  it('切到新建页签时按模块预填名称并提交 newAgent', async () => {
    const wrapper = mountWithI18n(AgentGroupMembersModal, {
      open: true,
      groupName: '告警',
      agents: [],
      memberIds: [],
      modules: MODULES,
      initialMode: 'create'
    })
    await flushPromises()
    await wrapper.get('[data-group-new-module="logs"]').trigger('click')
    expect((wrapper.get('[data-group-new-name]').element as HTMLInputElement).value).toBe('Logs')
    await wrapper.get('[data-group-new-desc]').setValue('看日志')
    await wrapper.get('.modal-ok').trigger('click')
    expect(wrapper.emitted('save')?.[0]).toEqual([
      { newAgent: { name: 'Logs', module: 'logs', description: '看日志' } }
    ])
    wrapper.unmount()
  })

  it('切页签再切回时保留已选（符合「失败/切换不丢选择」）', async () => {
    const wrapper = mountWithI18n(AgentGroupMembersModal, {
      open: true,
      groupName: '告警',
      agents: [{ id: 'a2', name: '日志助手', module: 'logs' }],
      memberIds: [],
      modules: MODULES
    })
    await flushPromises()
    await wrapper.get('[data-group-agent-check="a2"]').trigger('change')
    await wrapper.get('[data-group-tab="create"]').trigger('click')
    await wrapper.get('[data-group-tab="existing"]').trigger('click')
    expect((wrapper.get('[data-group-agent-check="a2"]').element as HTMLInputElement).checked).toBe(true)
    wrapper.unmount()
  })
})

/** Plan 快照：默认 simple、无确认。 */
function plan(over: Partial<AgentPlanSnapshot['decision']> = {}): AgentPlanSnapshot {
  return {
    version: 3,
    confirmState: '',
    replans: 0,
    decision: {
      action: 'create', goal: '核对订单', complexity: 'simple', steps: [], dispatches: [],
      confirm: { required: false, reasons: [] }, progress: { percent: 0, note: '' },
      findings: [], unknowns: [], shouldContinue: false, shouldSummarize: false, sources: [],
      ...over
    }
  }
}

describe('AgentPlanCard', () => {
  beforeEach(() => setI18nLocale('zh-CN'))

  it('渲染目标、任务分工与进度；没有 Plan 快照时不渲染', () => {
    const wrapper = mountWithI18n(AgentPlanCard, {
      plan: plan(),
      tasks: [
        task({ id: 't1', seq: 1, status: 'completed' }),
        task({ id: 't2', seq: 2, targetId: 'a2', status: 'running' })
      ],
      names: { a1: '数据库助手', a2: '日志助手' }
    })
    expect(wrapper.get('[data-plan-goal]').text()).toBe('核对订单')
    expect(wrapper.findAll('[data-dispatch-step]')).toHaveLength(2)
    expect(wrapper.text()).toContain('数据库助手')
    expect(wrapper.text()).toContain('日志助手')
    expect(wrapper.text()).toContain('1/2')
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')
    expect(wrapper.find('[data-plan-complexity]').exists()).toBe(false)
    expect(wrapper.find('[data-plan-steps]').exists()).toBe(false)
    wrapper.unmount()

    const empty = mountWithI18n(AgentPlanCard, { plan: null, tasks: [task()] })
    expect(empty.find('[data-agent-plan-card]').exists()).toBe(false)
    empty.unmount()
  })

  it('复杂计划显示步骤清单，步骤状态按关联任务归约；发现可展开', () => {
    const wrapper = mountWithI18n(AgentPlanCard, {
      plan: plan({
        complexity: 'complex',
        steps: [
          { id: 's1', title: '建表', status: 'pending', note: '' },
          { id: 's2', title: '发布站点', status: 'done', note: '已验证' }
        ],
        findings: ['订单表缺索引'],
        progress: { percent: 40, note: '建表中' }
      }),
      tasks: [task({ id: 't1', stepId: 's1', status: 'completed' })]
    })
    expect(wrapper.get('[data-plan-complexity]').text()).toBe('复杂任务')
    expect(wrapper.get('[data-plan-summary]').text()).toBe('建表中')
    expect(wrapper.get('[data-plan-step="s1"]').attributes('data-plan-step-status')).toBe('done')
    expect(wrapper.get('[data-plan-step="s2"]').attributes('data-plan-step-status')).toBe('done')
    expect(wrapper.get('[data-plan-step="s2"]').text()).toContain('已验证')
    expect(wrapper.get('[data-plan-findings]').text()).toContain('订单表缺索引')
    wrapper.unmount()
  })

  it('失败步骤可重试，完成步骤不显示重试', async () => {
    const wrapper = mountWithI18n(AgentPlanCard, {
      plan: plan(),
      tasks: [task({ id: 't1', status: 'failed' }), task({ id: 't2', seq: 2, status: 'completed' })]
    })
    const retry = wrapper.findAll('[data-dispatch-retry]')
    expect(retry).toHaveLength(1)
    await retry[0].trigger('click')
    expect(wrapper.emitted('retry-task')?.[0]).toEqual(['t1'])
    wrapper.unmount()
  })

  it('人工任务显示为「需要你处理」', () => {
    const wrapper = mountWithI18n(AgentPlanCard, {
      plan: plan(),
      tasks: [task({ id: 'h1', targetKind: 'human', targetId: 'u1', status: 'waiting_human' })]
    })
    expect(wrapper.text()).toContain('需要你处理')
    wrapper.unmount()
  })
})

describe('AgentHumanTaskCard', () => {
  beforeEach(() => setI18nLocale('zh-CN'))

  it('回复并继续带上说明', async () => {
    const wrapper = mountWithI18n(AgentHumanTaskCard, {
      task: task({ id: 'h1', targetKind: 'human', targetId: 'u1', status: 'waiting_human', assigneeId: 'u1' })
    })
    await wrapper.get('[data-human-reply-input]').setValue('确认是正常波动')
    await wrapper.get('[data-human-continue]').trigger('click')
    expect(wrapper.emitted('reply')?.[0]).toEqual(['h1', 'continue', '确认是正常波动'])
    wrapper.unmount()
  })

  it('无法处理时不带空字符串', async () => {
    const wrapper = mountWithI18n(AgentHumanTaskCard, {
      task: task({ id: 'h1', targetKind: 'human', status: 'waiting_human' })
    })
    await wrapper.get('[data-human-unable]').trigger('click')
    expect(wrapper.emitted('reply')?.[0]).toEqual(['h1', 'unable', undefined])
    wrapper.unmount()
  })

  it('已处理的任务显示结论，不再给输入框', () => {
    const wrapper = mountWithI18n(AgentHumanTaskCard, {
      task: task({
        id: 'h1',
        targetKind: 'human',
        status: 'completed',
        replyText: '已确认',
        replyOutcome: 'continue'
      })
    })
    expect(wrapper.find('[data-human-reply-input]').exists()).toBe(false)
    expect(wrapper.get('[data-human-done]').text()).toContain('已确认')
    wrapper.unmount()
  })

  it('提交中禁用按钮', () => {
    const wrapper = mountWithI18n(AgentHumanTaskCard, {
      task: task({ id: 'h1', targetKind: 'human', status: 'waiting_human' }),
      busy: true
    })
    expect((wrapper.get('[data-human-continue]').element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })
})

describe('AgentTagModal（群设置）', () => {
  beforeEach(() => setI18nLocale('zh-CN'))

  const tag: AgentTag = {
    id: 'g1',
    name: '告警群',
    agentIds: ['a1', 'a2'],
    prompts: [],
    unavailableAgentIds: [],
    routingPrompt: '数据问题交给数据库',
    fallbackAgentId: 'a1',
    humanPolicy: 'disabled',
    threadKinds: ['alert'],
    version: 3
  }

  it('回填路由提示词、兜底与人工策略', async () => {
    const wrapper = mountWithI18n(AgentTagModal, {
      open: true,
      tag,
      agents: [
        { id: 'a1', name: '数据库助手', module: 'database' },
        { id: 'a2', name: '日志助手', module: 'logs' }
      ]
    })
    await flushPromises()
    expect((wrapper.get('[data-tag-routing-prompt]').element as HTMLTextAreaElement).value).toBe('数据问题交给数据库')
    expect(selects(wrapper)[FALLBACK].props('modelValue')).toBe('a1')
    expect(selects(wrapper)[HUMAN_POLICY].props('modelValue')).toBe('disabled')
    // 触发器按钮仍带原有的 data 钩子（样式与定位不变）。
    expect(wrapper.find('[data-tag-fallback]').exists()).toBe(true)
    expect(wrapper.find('[data-tag-human-policy]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('取消勾选兜底助手时自动清空兜底', async () => {
    const wrapper = mountWithI18n(AgentTagModal, {
      open: true,
      tag,
      agents: [
        { id: 'a1', name: '数据库助手', module: 'database' },
        { id: 'a2', name: '日志助手', module: 'logs' }
      ]
    })
    await flushPromises()
    await wrapper.get('[data-tag-agent-check="a1"]').trigger('change')
    await flushPromises()
    // 空值用哨兵 __none__ 表示「无兜底」（reka 不允许空串作 item value）。
    expect(selects(wrapper)[FALLBACK].props('modelValue')).toBe('__none__')
    wrapper.unmount()
  })

  it('选择兜底 / 人工策略后随保存提交；选回「无」「允许人工」则清空', async () => {
    const wrapper = mountWithI18n(AgentTagModal, {
      open: true,
      tag: { ...tag, fallbackAgentId: '', humanPolicy: undefined },
      agents: [
        { id: 'a1', name: '数据库助手', module: 'database' },
        { id: 'a2', name: '日志助手', module: 'logs' }
      ]
    })
    await flushPromises()
    selects(wrapper)[FALLBACK].vm.$emit('update:modelValue', 'a2')
    selects(wrapper)[HUMAN_POLICY].vm.$emit('update:modelValue', 'disabled')
    await flushPromises()
    await wrapper.get('.modal-ok').trigger('click')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ fallbackAgentId: 'a2', humanPolicy: 'disabled' })

    selects(wrapper)[FALLBACK].vm.$emit('update:modelValue', '__none__')
    selects(wrapper)[HUMAN_POLICY].vm.$emit('update:modelValue', '__none__')
    await flushPromises()
    await wrapper.get('.modal-ok').trigger('click')
    const second = wrapper.emitted('save')?.[1]?.[0] as { fallbackAgentId: string; humanPolicy?: string }
    expect(second.fallbackAgentId).toBe('')
    expect(second.humanPolicy).toBeUndefined()
    wrapper.unmount()
  })

  it('兜底下拉只列当前成员', async () => {
    const wrapper = mountWithI18n(AgentTagModal, {
      open: true,
      tag: { ...tag, fallbackAgentId: '' },
      agents: [
        { id: 'a1', name: '数据库助手', module: 'database' },
        { id: 'a2', name: '日志助手', module: 'logs' },
        { id: 'a3', name: '群外助手', module: 's3' }
      ]
    })
    await flushPromises()
    // 候选来自 memberAgents：只列当前成员，群外助手 a3 不在其中。
    // （下拉内容在 portal 里、关闭时不挂载，这里断言真实的数据来源。）
    const vm = wrapper.vm as unknown as { memberAgents: Array<{ id: string }> }
    expect(vm.memberAgents.map((a) => a.id)).toEqual(['a1', 'a2'])
    wrapper.unmount()
  })
})
