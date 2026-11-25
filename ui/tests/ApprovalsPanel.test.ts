import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ApprovalsPanel from '@/components/settings/ApprovalsPanel.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, installJsdomPolyfills, mountWithApp, pickSelectOption, realSelectStubs, selectTriggers } from '@/test/helpers'
import { ADMIN_PROJECT_ID } from '@/stores/project'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const pendingApproval = {
  id: 'ap-1',
  run_id: 'run-1',
  call_id: 'call-1',
  thread_id: 'th-1',
  agent_id: 'ag-1',
  tool_name: 'simplebase',
  argv: ['database', 'delete', '--id', 'shop'],
  message: '将执行破坏性命令',
  status: 'pending' as const,
  initiator_id: 'u1',
  created_at: '2026-01-01T00:00:00Z',
  expires_at: '2026-01-01T00:02:00Z'
}

const resolvedApproval = {
  ...pendingApproval,
  id: 'ap-2',
  call_id: 'call-2',
  status: 'approved' as const,
  resolved_by: 'u1',
  resolved_at: '2026-01-01T00:01:00Z'
}

installJsdomPolyfills()

describe('ApprovalsPanel', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentApprovals.list.mockResolvedValue({
      approvals: [pendingApproval, resolvedApproval],
      pending: 1
    })
  })

  it('renders the queue with pending count and resolves an item', async () => {
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    expect(wrapper.text()).toContain('审批队列')
    expect(wrapper.text()).toContain('共 2 条 · 1 条待处理')
    expect(wrapper.text()).toContain('database delete --id shop')
    expect(wrapper.text()).toContain('将执行破坏性命令')
    expect(wrapper.text()).toContain('待处理')
    expect(wrapper.text()).toContain('已放行')

    await clickText(wrapper, '放行')
    await flushPromises()
    expect(api.agentApprovals.resolve).toHaveBeenCalledWith('p1', 'run-1', 'call-1', true)
    // 处理成功后重新拉取列表。
    expect(api.agentApprovals.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('denies an item and surfaces failures', async () => {
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    await clickText(wrapper, '拒绝')
    await flushPromises()
    expect(api.agentApprovals.resolve).toHaveBeenCalledWith('p1', 'run-1', 'call-1', false)

    api.agentApprovals.resolve.mockRejectedValueOnce(new Error('resolve failed'))
    const vm = wrapper.vm as unknown as {
      resolve: (item: typeof pendingApproval, approve: boolean) => Promise<void>
    }
    await vm.resolve(pendingApproval, true)
    expect(wrapper.text()).toContain('审批队列')
  })

  it('filters by status and reloads on project change', async () => {
    const { wrapper, pinia } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    const vm = wrapper.vm as unknown as { statusFilter: string; load: () => Promise<void> }
    vm.statusFilter = 'pending'
    await flushPromises()
    expect(api.agentApprovals.list).toHaveBeenLastCalledWith('p1', 'pending')

    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('p2', 'Second')
    await flushPromises()
    expect(api.agentApprovals.list).toHaveBeenLastCalledWith('p2', '')
  })

  it('shows an admin empty state without requesting approvals', async () => {
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: ADMIN_PROJECT_ID })
    expect(api.agentApprovals.list).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('系统项目不支持审批队列')
  })

  it('handles empty responses and load failures', async () => {
    api.agentApprovals.list.mockResolvedValueOnce({ approvals: [], pending: 0 })
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    expect(wrapper.text()).toContain('暂无审批记录')

    api.agentApprovals.list.mockRejectedValueOnce(new Error('offline'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain('offline')
  })

  it('covers every status badge variant and the unresolved placeholder', async () => {
    api.agentApprovals.list.mockResolvedValueOnce({
      approvals: [
        { ...pendingApproval, id: 'a1', status: 'denied' as const, resolved_by: 'u2' },
        { ...pendingApproval, id: 'a2', status: 'expired' as const },
        { ...pendingApproval, id: 'a3', status: 'canceled' as const }
      ],
      pending: 0
    })
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    expect(wrapper.text()).toContain('已拒绝')
    expect(wrapper.text()).toContain('已过期')
    expect(wrapper.text()).toContain('已作废')
    // 已收敛但无 resolved_by 的行显示占位符。
    expect(wrapper.text()).toContain('—')
  })

  it('renders placeholders for sparse rows and ignores stale responses', async () => {
    api.agentApprovals.list.mockResolvedValueOnce({
      approvals: [
        {
          id: 'a9',
          run_id: 'run-9',
          call_id: 'call-9',
          argv: [],
          status: 'pending' as const,
          created_at: '2026-01-01T00:00:00Z'
        }
      ],
      pending: 1
    })
    const { wrapper, pinia } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    // tool_name / message / initiator / expires_at 全缺省时都走占位符分支。
    expect(wrapper.text()).toContain('—')

    const vm = wrapper.vm as unknown as { load: () => Promise<void> }
    let resolveOld!: (v: { approvals: typeof pendingApproval[]; pending: number }) => void
    api.agentApprovals.list.mockImplementationOnce(
      () => new Promise((resolve) => { resolveOld = resolve })
    )
    const stale = vm.load()
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('p2', 'Second')
    await flushPromises()
    resolveOld({ approvals: [], pending: 0 })
    await stale
    // 旧项目的响应不得覆盖新项目列表。
    expect(api.agentApprovals.list).toHaveBeenLastCalledWith('p2', '')
  })

  it('skips resolve when the principal cannot write', async () => {
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    const original = auth.canWrite
    Object.defineProperty(auth, 'canWrite', { get: () => false, configurable: true })
    const vm = wrapper.vm as unknown as {
      resolve: (item: typeof pendingApproval, approve: boolean) => Promise<void>
    }
    await vm.resolve(pendingApproval, true)
    expect(api.agentApprovals.resolve).not.toHaveBeenCalled()
    Object.defineProperty(auth, 'canWrite', { get: () => original, configurable: true })
  })

  it('tolerates a response without an approvals array', async () => {
    api.agentApprovals.list.mockResolvedValueOnce({ pending: 0 } as never)
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1' })
    expect(wrapper.text()).toContain('暂无审批记录')
  })

  it('drives the status select and the pager through the template handlers', async () => {
    api.agentApprovals.list.mockResolvedValue({
      approvals: Array.from({ length: 12 }, (_, i) => ({
        ...pendingApproval,
        id: `ap-${i}`,
        call_id: `call-${i}`
      })),
      pending: 12
    })
    const { wrapper } = await mountWithApp(ApprovalsPanel, { projectId: 'p1', attachTo: document.body, stubs: realSelectStubs })
    // 真实 Select：选「待处理」，列表按 pending 重新请求
    await pickSelectOption(selectTriggers(wrapper)[0], '待处理')
    expect(api.agentApprovals.list).toHaveBeenLastCalledWith('p1', 'pending')
    // 再选回「全部」：状态筛选清空
    await pickSelectOption(selectTriggers(wrapper)[0], '全部')
    expect(api.agentApprovals.list.mock.calls.at(-1)?.[1]).toBe('')

    // 12 条 / 每页 10 条 → 有第二页，pager 的 update:page 处理器被触发。
    await wrapper.get('.pager-next').trigger('click')
    await flushPromises()
    // 第二页只剩 2 行（表格不渲染 id，用行数断言）。
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
  })
})