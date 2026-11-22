// planv6.7 v2：协作群 UI 组件测试。
// 覆盖：群成员弹窗（两个页签、成员置灰、新建校验）、路由分工卡（步骤与重试）、
// 人工待办卡（回复/无法处理、只读态）、群设置弹窗的新字段（路由提示词/兜底/人工策略）。
import { flushPromises, mount } from '@vue/test-utils'
import type { Component } from 'vue'
import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia } from 'pinia'
import { setI18nLocale } from '@/locales'
import AgentGroupMembersModal from '@/components/modal/AgentGroupMembersModal.vue'
import AgentDispatchCard from '@/components/agent/AgentDispatchCard.vue'
import AgentHumanTaskCard from '@/components/agent/AgentHumanTaskCard.vue'
import AgentTagModal from '@/components/modal/AgentTagModal.vue'
import type { AgentModuleInfo, AgentTag, AgentTask } from '@/services/types'

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

describe('AgentDispatchCard', () => {
  beforeEach(() => setI18nLocale('zh-CN'))

  it('渲染每一步与状态、进度', () => {
    const wrapper = mountWithI18n(AgentDispatchCard, {
      tasks: [
        task({ id: 't1', seq: 1, status: 'completed' }),
        task({ id: 't2', seq: 2, targetId: 'a2', status: 'running' })
      ],
      names: { a1: '数据库助手', a2: '日志助手' }
    })
    expect(wrapper.findAll('[data-dispatch-step]')).toHaveLength(2)
    expect(wrapper.text()).toContain('数据库助手')
    expect(wrapper.text()).toContain('日志助手')
    expect(wrapper.text()).toContain('1/2')
    wrapper.unmount()
  })

  it('失败步骤可重试，完成步骤不显示重试', async () => {
    const wrapper = mountWithI18n(AgentDispatchCard, {
      tasks: [task({ id: 't1', status: 'failed' }), task({ id: 't2', seq: 2, status: 'completed' })]
    })
    const retry = wrapper.findAll('[data-dispatch-retry]')
    expect(retry).toHaveLength(1)
    await retry[0].trigger('click')
    expect(wrapper.emitted('retry-task')?.[0]).toEqual(['t1'])
    wrapper.unmount()
  })

  it('人工任务显示为「需要你处理」', () => {
    const wrapper = mountWithI18n(AgentDispatchCard, {
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
    expect((wrapper.get('[data-tag-fallback]').element as HTMLSelectElement).value).toBe('a1')
    expect((wrapper.get('[data-tag-human-policy]').element as HTMLSelectElement).value).toBe('disabled')
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
    expect((wrapper.get('[data-tag-fallback]').element as HTMLSelectElement).value).toBe('')
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
    const options = wrapper.get('[data-tag-fallback]').findAll('option')
    expect(options.map((o) => o.attributes('value'))).toEqual(['', 'a1', 'a2'])
    wrapper.unmount()
  })
})
