import { computed, ref } from 'vue'
import { api } from '@/services/api'
import type { AgentHumanTask } from '@/services/types'
import type { useAgentWorkspace } from '@/composables/useAgentWorkspace'

export interface UseHumanTasksOptions {
  projectId: () => string
  /** 每个群的会话分页：待办按会话所属群聚合群行角标。 */
  ws: ReturnType<typeof useAgentWorkspace>
}

/**
 * 「我的待办」：跨会话聚合，用于群行角标与人工任务卡。
 * 只读列表，不产生副作用；失败静默（角标不该阻断主流程）。
 */
export function useHumanTasks(opts: UseHumanTasksOptions) {
  const humanTasks = ref<AgentHumanTask[]>([])

  /** 与 /agent-runs/active 的轮询合并成一轮，避免开两条定时器。 */
  async function loadHumanTasks() {
    const projectId = opts.projectId()
    if (!projectId) return
    try {
      const res = await api.agentTurns.humanTasks(projectId, 'me', 'pending')
      humanTasks.value = res.tasks
    } catch {
      // 未登录态（API Key）或未升级环境：不显示角标。
      humanTasks.value = []
    }
  }

  /** 每个群的待办数（群行角标）：先看任务自带的群，再按会话所属群回退，最后归 general。 */
  const pendingByGroup = computed<Record<string, number>>(() => {
    const threadToGroup: Record<string, string> = {}
    for (const [groupId, page] of Object.entries(opts.ws.pages)) {
      for (const th of page.threads) threadToGroup[th.id] = groupId
    }
    const out: Record<string, number> = {}
    for (const task of humanTasks.value) {
      const groupId = task.tagId || threadToGroup[task.threadId] || 'general'
      out[groupId] = (out[groupId] ?? 0) + 1
    }
    return out
  })

  return { humanTasks, pendingByGroup, loadHumanTasks }
}
