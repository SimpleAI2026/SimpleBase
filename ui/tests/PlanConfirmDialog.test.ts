import { describe, expect, it } from 'vitest'
import { inDialog, installJsdomPolyfills, mountDialog, realDialogStubs, uiStubs } from '@/test/helpers'
import PlanConfirmDialog from '@/components/agent/PlanConfirmDialog.vue'
import type { AgentPlanDecision } from '@/services/types'

installJsdomPolyfills()

function plan(): AgentPlanDecision {
  return {
    action: 'create', goal: '删除前核对', complexity: 'complex',
    steps: [{ id: 's1', title: '核对', status: 'pending', note: '' }],
    dispatches: [
      { ref: 'd1', targetKind: 'agent', targetId: 'a1', message: '原任务一', dependsOn: [], contextFrom: [], stepId: 's1', risk: 'destructive', expectedOutput: '' },
      { ref: 'd2', targetKind: 'agent', targetId: 'a2', message: '原任务二', dependsOn: ['d1'], contextFrom: ['d1'], stepId: 's1', risk: '', expectedOutput: '' }
    ],
    confirm: { required: true, reasons: ['包含破坏性操作'] },
    progress: { percent: 0, note: '' }, findings: [], unknowns: [],
    shouldContinue: false, shouldSummarize: false, sources: []
  }
}

describe('PlanConfirmDialog', () => {
  it('编辑 message 并删除任务后提交 edits', async () => {
    const wrapper = await mountDialog(PlanConfirmDialog, {
      props: { open: true, plan: plan(), names: { a1: '数据库', a2: '文件' } },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    expect(inDialog().text()).toContain('删除前核对')
    expect(inDialog().text()).toContain('包含破坏性操作')
    await inDialog().get('[data-plan-message="d1"]').setValue('编辑后的任务')
    await inDialog().get('[data-plan-drop="d2"]').trigger('click')
    await inDialog().get('[data-plan-approve]').trigger('click')
    expect(wrapper.emitted('submit')?.[0]).toEqual([true, [
      { ref: 'd1', message: '编辑后的任务' },
      { ref: 'd2', drop: true }
    ]])
    wrapper.unmount()
  })

  it('拒绝整轮不带编辑', async () => {
    const wrapper = await mountDialog(PlanConfirmDialog, {
      props: { open: true, plan: plan() },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    await inDialog().get('[data-plan-reject]').trigger('click')
    expect(wrapper.emitted('submit')?.[0]).toEqual([false, []])
    wrapper.unmount()
  })
})
