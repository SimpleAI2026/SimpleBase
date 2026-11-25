import { onScopeDispose, ref, watch } from 'vue'
import type { Ref } from 'vue'
import { api } from '@/services/api'
import type { ActiveAgentRun } from '@/services/types'

/** 有运行中的 run 时的轮询间隔。 */
export const ACTIVE_POLL_MS = 3000

export interface UseActiveRunsPollOptions {
  projectId: () => string
  /** 本地是否正在发送：发送中即使服务端还没登记也要继续轮询。 */
  sending: Ref<boolean>
  /** 每轮轮询后顺带执行（待办角标与运行指示同一轮拉取）。 */
  onTick?: () => void | Promise<void>
}

/**
 * 「正在回复」指示的数据源：服务端 /agent-runs/active 覆盖后台续跑与定时任务。
 *
 * - 有运行中的 run（或本地正在发送）时每 3s 轮询，全部结束即停；
 * - 页面隐藏时暂停，回到前台由 visibilitychange 续上；
 * - 项目切换后到达的旧响应一律丢弃；
 * - 失败静默：指示性数据，保留上一次结果，下轮再试。
 */
export function useActiveRunsPoll(opts: UseActiveRunsPollOptions) {
  const activeRuns = ref<ActiveAgentRun[]>([])
  let timer: ReturnType<typeof setTimeout> | null = null
  let disposed = false

  function clearTimer() {
    if (timer) clearTimeout(timer)
    timer = null
  }

  async function refresh() {
    clearTimer()
    const projectId = opts.projectId()
    if (!projectId || disposed) return
    try {
      const list = await api.agentThreads.active(projectId)
      if (disposed || opts.projectId() !== projectId) return
      activeRuns.value = Array.isArray(list) ? list : []
    } catch {
      // 指示性数据：失败静默。
    }
    void opts.onTick?.()
    if (disposed || opts.projectId() !== projectId) return
    if (activeRuns.value.length || opts.sending.value) {
      timer = setTimeout(() => {
        timer = null
        if (document.hidden) return
        void refresh()
      }, ACTIVE_POLL_MS)
    }
  }

  function onVisibilityChange() {
    if (!document.hidden && !timer) void refresh()
  }
  document.addEventListener('visibilitychange', onVisibilityChange)

  // 本地 run 开始 / 结束都刷新一次：开始时启动轮询，结束时及时去掉转圈。
  watch(opts.sending, () => { void refresh() })

  function dispose() {
    disposed = true
    clearTimer()
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }
  onScopeDispose(dispose)

  return { activeRuns, refresh, dispose }
}
