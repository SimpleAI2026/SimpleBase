import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { ACTIVE_POLL_MS, useActiveRunsPoll } from '@/composables/useActiveRunsPoll'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
}

const run = (id: string) => ({ run_id: id, agent_id: 'ag', thread_id: 'th' })

function setup(initial = 'p1') {
  const projectId = ref(initial)
  const sending = ref(false)
  const onTick = vi.fn()
  const scope = effectScope()
  const poll = scope.run(() => useActiveRunsPoll({ projectId: () => projectId.value, sending, onTick }))!
  return { projectId, sending, onTick, scope, poll }
}

describe('useActiveRunsPoll', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetApiMocks()
    setHidden(false)
  })
  afterEach(() => {
    vi.useRealTimers()
    setHidden(false)
  })

  it('有运行中的 run 时按间隔继续轮询，全部结束即停', async () => {
    api.agentThreads.active.mockResolvedValueOnce([run('r1')]).mockResolvedValueOnce([run('r1')]).mockResolvedValue([])
    const { poll, onTick, scope } = setup()
    await poll.refresh()
    expect(poll.activeRuns.value).toHaveLength(1)
    expect(onTick).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(3)
    expect(poll.activeRuns.value).toEqual([])

    // 没有运行中的 run 且不在发送：不再续约定时器。
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS * 3)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(3)
    scope.stop()
  })

  it('本地正在发送时即使服务端没有 run 也继续轮询', async () => {
    api.agentThreads.active.mockResolvedValue([])
    const { poll, sending, scope } = setup()
    sending.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(0)
    const first = api.agentThreads.active.mock.calls.length
    expect(first).toBeGreaterThanOrEqual(1)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(api.agentThreads.active.mock.calls.length).toBeGreaterThan(first)
    void poll
    scope.stop()
  })

  it('页面隐藏时暂停，回到前台由 visibilitychange 续上', async () => {
    api.agentThreads.active.mockResolvedValue([run('r1')])
    const { poll, scope } = setup()
    await poll.refresh()
    expect(api.agentThreads.active).toHaveBeenCalledTimes(1)

    // 定时器到点时页面已隐藏：本轮不请求，也不再续约。
    setHidden(true)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS * 3)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(1)

    // 回到前台：visibilitychange 立即补一次请求，之后恢复定时轮询。
    setHidden(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(0)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(3)
    scope.stop()
  })

  it('项目切换后到达的旧响应不落地，也不再续约定时器', async () => {
    let resolveOld: (v: unknown) => void = () => undefined
    api.agentThreads.active.mockImplementationOnce(() => new Promise((r) => { resolveOld = r }))
    const { poll, projectId, onTick, scope } = setup('p1')
    const pending = poll.refresh()
    projectId.value = 'p2'
    resolveOld([run('stale')])
    await pending
    expect(poll.activeRuns.value).toEqual([])
    // 项目已切换：旧响应在落地前就被丢弃，既不触发 onTick，也不会续约。
    expect(onTick).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS * 2)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(1)
    scope.stop()
  })

  it('请求失败静默：保留上一次结果，仍按规则续约', async () => {
    api.agentThreads.active.mockResolvedValueOnce([run('r1')]).mockRejectedValueOnce(new Error('down')).mockResolvedValue([])
    const { poll, scope } = setup()
    await poll.refresh()
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(poll.activeRuns.value).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(ACTIVE_POLL_MS)
    expect(poll.activeRuns.value).toEqual([])
    scope.stop()
  })

  it('没有项目 / 已 dispose 时不发请求，且移除 visibilitychange 监听', async () => {
    const off = vi.spyOn(document, 'removeEventListener')
    const { poll, projectId, scope } = setup('')
    await poll.refresh()
    expect(api.agentThreads.active).not.toHaveBeenCalled()
    projectId.value = 'p1'
    poll.dispose()
    await poll.refresh()
    expect(api.agentThreads.active).not.toHaveBeenCalled()
    expect(off).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    scope.stop()
    off.mockRestore()
  })
})
