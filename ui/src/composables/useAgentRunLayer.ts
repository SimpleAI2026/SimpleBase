import { computed } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { useAgentConversation } from '@/composables/useAgentConversation'
import { useAgentCollaboration } from '@/composables/useAgentCollaboration'
import { useActiveRunsPoll } from '@/composables/useActiveRunsPoll'
import { notifyCollabError } from '@/composables/useAgentSender'
import type { ChatMsg } from '@/services/types'

export interface UseAgentRunLayerOptions {
  projectId: () => string
  threadId: Ref<string>
  chatMessages: Ref<ChatMsg[]>
  /** 一轮结束（协作）：页面据此刷新会话列表与待办。 */
  onTurnEnd: () => void
  /** 轮询每 tick 顺带执行（待办角标与运行指示同一轮拉取）。 */
  onTick: () => void | Promise<void>
}

/**
 * 运行层：单助手 run（conv）+ 协作轮次（collab）+ 「正在回复」指示（activeRuns 轮询）。
 *
 * conv 与 collab 并存：按会话类型二选一，互不干扰；`sending` 是两者的并集。
 */
export function useAgentRunLayer(opts: UseAgentRunLayerOptions) {
  const conv = useAgentConversation({
    projectId: opts.projectId,
    threadId: () => opts.threadId.value,
    messages: () => opts.chatMessages.value
  })
  const collab = useAgentCollaboration({
    projectId: opts.projectId,
    messages: () => opts.chatMessages.value,
    onTurnEnd: opts.onTurnEnd,
    onError: notifyCollabError
  })
  const sending: ComputedRef<boolean> = computed(() => conv.sending.value || collab.running.value)

  /** 服务端 /agent-runs/active 覆盖后台续跑与定时任务。 */
  const { activeRuns, refresh: refreshActiveRuns } = useActiveRunsPoll({
    projectId: opts.projectId,
    sending,
    onTick: opts.onTick
  })

  const runningAgentIds = computed(() => new Set(activeRuns.value.map((r) => r.agent_id)))
  const runningThreadIds = computed(() => {
    const ids = new Set(activeRuns.value.map((r) => r.thread_id))
    if (sending.value && opts.threadId.value) ids.add(opts.threadId.value)
    return ids
  })

  return { conv, collab, sending, activeRuns, refreshActiveRuns, runningAgentIds, runningThreadIds }
}
