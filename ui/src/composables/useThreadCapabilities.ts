import { computed, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { t } from '@/locales'
import type { CloudAgent } from '@/services/api'
import type { AgentModuleInfo } from '@/services/api'
import type { AgentTask, AgentThread } from '@/services/types'

export interface UseThreadCapabilitiesOptions {
  activeThread: ComputedRef<AgentThread | undefined>
  activeThreadAgent: ComputedRef<CloudAgent | undefined>
  activeThreadAgentName: ComputedRef<string>
  modules: Ref<AgentModuleInfo[]>
  /** v3 协议版本；0 表示能力尚未返回，>0 且 <3 时只读。 */
  protocolVersion?: Ref<number>
  /** 协作轮次的任务（渲染路由卡 / 人工卡）。 */
  collabTasks: Ref<AgentTask[]>
}

/**
 * 当前会话的「能力」衍生状态：是否协作会话、轮次任务、助手能力提示（沙盒 / CLI 技能可用性）与 composer 下的提示文案。
 * 全是 computed，不持有状态、不发请求。
 */
export function useThreadCapabilities(opts: UseThreadCapabilitiesOptions) {
  /**
   * 当前会话是否为协作会话（群会话）。没有会话上下文（分区总览）时按群处理：
   * 新建的会话一律是 collaborative，固定助手会话只会来自历史数据。
   */
  const isCollaborativeThread = computed(() => {
    const thread = opts.activeThread.value
    if (!thread) return true
    return thread.dispatch_mode === 'collaborative'
  })

  /** 当前轮次的任务（Plan 卡渲染源）：只在协作会话展示。 */
  const activeTurnTasks = computed<AgentTask[]>(() => (isCollaborativeThread.value ? opts.collabTasks.value : []))
  /** 当前轮次里等人的任务（人工卡渲染源，保持任务顺序）。 */
  const humanTasksInTurn = computed<AgentTask[]>(() => activeTurnTasks.value.filter((task) => task.targetKind === 'human'))

  /** 当前选中 Agent 是否含沙盒工具；决定 composer 旁的提示文案。 */
  const activeAgentHasSandboxTools = computed(() => {
    const a = opts.activeThreadAgent.value
    if (!a) return false
    return (a.tool_ids || []).some((id) => id.startsWith('sandbox_'))
  })

  const activeCapabilityError = computed(() => {
    const protocol = opts.protocolVersion?.value ?? 0
    if (protocol > 0 && protocol < 3) return t('pages.agents.workspaceUpgradeRequired')
    const agent = opts.activeThreadAgent.value
    if (!agent) return ''
    if (agent.module === 'sandbox' && opts.modules.value.find((m) => m.id === 'sandbox')?.sandbox_available !== true) {
      return t('pages.agents.sandboxUnavailable')
    }
    if (['kv', 'gofunction', 'cron'].includes(agent.module) && !opts.modules.value.some((m) => m.id === agent.module)) {
      return t('pages.agents.cliSkillsUnavailable')
    }
    return ''
  })

  const toolsHint = computed(() => {
    if (activeCapabilityError.value) return activeCapabilityError.value
    const who = opts.activeThreadAgent.value
      ? t('pages.agents.mentionHint', { name: opts.activeThreadAgentName.value })
      : t('pages.agents.mentionHintGeneric')
    if (activeAgentHasSandboxTools.value) return t('pages.agents.mentionHintSandbox', { who })
    return t('pages.agents.mentionHintWritable', { who })
  })

  return { isCollaborativeThread, activeTurnTasks, humanTasksInTurn, activeAgentHasSandboxTools, activeCapabilityError, toolsHint }
}

/** 会话重命名弹窗状态：打开 → 编辑 → 提交（相同 / 空标题直接关闭，不请求）。 */
export function useRenameThread(opts: {
  threadsOf: () => AgentThread[]
  rename: (id: string, title: string) => Promise<void>
}) {
  const open = ref(false)
  const value = ref('')
  const saving = ref(false)
  const target = ref<AgentThread | null>(null)

  /** 侧栏「重命名」入口（分区制后线程 id 直接传入）。 */
  function start(id: string) {
    const th = opts.threadsOf().find((item) => item.id === id)
    if (!th) return
    target.value = th
    value.value = th.title
    open.value = true
  }

  /** 弹窗出现问题时不留半开状态。 */
  async function submit() {
    const current = target.value
    const next = value.value.trim()
    if (!current || !next || next === current.title) {
      open.value = false
      return
    }
    saving.value = true
    try {
      await opts.rename(current.id, next)
      open.value = false
    } finally {
      saving.value = false
    }
  }

  return { open, value, saving, target, start, submit }
}
