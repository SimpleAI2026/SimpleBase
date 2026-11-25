// 运行层：conv 与 collab 并存，sending 取并集；「正在回复」的助手 / 会话指示由服务端活动 run 与路由中消息合成。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentRunLayer } from '@/composables/useAgentRunLayer'
import type { ChatMsg } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function setup() {
  const threadId = ref('th-current')
  const chatMessages = ref<ChatMsg[]>([])
  const onTurnEnd = vi.fn()
  const onTick = vi.fn()
  const scope = effectScope()
  const layer = scope.run(() => useAgentRunLayer({ projectId: () => 'p1', threadId, chatMessages, onTurnEnd, onTick }))!
  return { scope, layer, threadId, chatMessages, onTick }
}

describe('useAgentRunLayer', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetApiMocks()
    api.agentThreads.active.mockResolvedValue([])
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('运行中的助手 / 会话来自服务端活动 run；路由中的助手消息也计入', async () => {
    api.agentThreads.active.mockResolvedValue([
      { run_id: 'r1', agent_id: 'a1', thread_id: 'th-live' },
      { run_id: 'r2', agent_id: 'a2', thread_id: 'th-live' }
    ])
    const { layer, chatMessages, threadId, onTick, scope } = setup()
    await layer.refreshActiveRuns()

    expect(onTick).toHaveBeenCalledTimes(1)
    expect([...layer.runningAgentIds.value].sort()).toEqual(['a1', 'a2'])
    expect([...layer.runningThreadIds.value]).toEqual(['th-live'])
    expect(layer.sending.value).toBe(false)

    // 「运行中」只由服务端活动 run 派生（CB-FE-11）：本地气泡不再标记路由中。
    chatMessages.value = [{ role: 'assistant', content: '', agentId: 'a3' }]
    await nextTick()
    expect(layer.runningAgentIds.value.has('a3')).toBe(false)
    // 没有本地发送时，当前会话不会仅因被选中就标记为「正在回复」。
    expect(layer.runningThreadIds.value.has(threadId.value)).toBe(false)
    scope.stop()
  })

  it('sending 是单助手 run 与协作轮次的并集；本地发送中时当前会话计入运行指示并触发一次活动 run 刷新', async () => {
    const { layer, threadId, scope } = setup()
    expect(layer.sending.value).toBe(false)

    layer.collab.running.value = true
    await nextTick()
    expect(layer.sending.value).toBe(true)
    expect(layer.runningThreadIds.value.has(threadId.value)).toBe(true)
    expect(api.agentThreads.active).toHaveBeenCalledTimes(1)

    layer.collab.running.value = false
    await nextTick()
    expect(layer.sending.value).toBe(false)
    expect(layer.runningThreadIds.value.has(threadId.value)).toBe(false)
    scope.stop()
  })
})
