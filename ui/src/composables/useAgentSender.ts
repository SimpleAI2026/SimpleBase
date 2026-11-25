import { computed } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { toast } from 'vue-sonner'
import { t } from '@/locales'
import type { CloudAgent } from '@/services/api'
import type { AgentMentionTarget, AgentThread, ChatMsg } from '@/services/types'
import { errorCode } from '@/utils/api-errors'
import { errorMessage } from '@/utils/format'
import type { useAgentConversation } from '@/composables/useAgentConversation'
import type { useAgentCollaboration } from '@/composables/useAgentCollaboration'

/** composer 提交的点名目标（与 AiChatComposer 的 MentionSendTarget 同构）。 */
export interface SendMentionTarget {
  agent_id?: string
  kind?: 'agent' | 'human'
}

/** 协作错误码 → 提示文案 key：命中即 warning 并结束，否则走通用 error。 */
export const COLLAB_WARNINGS: Record<string, string> = {
  agent_not_in_tag: 'components.agentWorkspace.agentNotInTag',
  group_member_unavailable: 'components.agentWorkspace.agentNotInTag',
  group_archived: 'components.agentWorkspace.groupArchived',
  turn_busy: 'components.agentWorkspace.turnBusy',
  human_assignee_required: 'components.agentWorkspace.humanRequired'
}

/** 协作轮次的错误提示：命中登记的码走 warning，其余走 error。 */
export function notifyCollabError(err: unknown): void {
  const key = COLLAB_WARNINGS[errorCode(err) ?? '']
  if (key) {
    toast.warning(t(key))
    return
  }
  toast.error(errorMessage(err, t('pages.agents.runFailed')))
}

/** 从 composer 提交的 mentions 里取出去重后的助手 id，并标记是否 @ 了「当前人」。 */
export function collectMentionTargets(mentions: SendMentionTarget[]): { agentIds: string[]; wantsHuman: boolean } {
  const agentIds: string[] = []
  let wantsHuman = false
  for (const m of mentions) {
    if (m.kind === 'human') {
      wantsHuman = true
      continue
    }
    const id = m.agent_id?.trim()
    if (id && !agentIds.includes(id)) agentIds.push(id)
  }
  return { agentIds, wantsHuman }
}

export interface UseAgentSenderOptions {
  threadId: Ref<string>
  draft: Ref<string>
  chatMessages: Ref<ChatMsg[]>
  conv: ReturnType<typeof useAgentConversation>
  collab: ReturnType<typeof useAgentCollaboration>
  sending: ComputedRef<boolean>
  activeThread: ComputedRef<AgentThread | undefined>
  activeThreadAgent: ComputedRef<CloudAgent | undefined>
  isCollaborativeThread: ComputedRef<boolean>
  activeCapabilityError: ComputedRef<string>
  rawAgentById: ComputedRef<Map<string, CloudAgent>>
  agentNamesById: ComputedRef<Record<string, string>>
  /** 当前群成员 id（群外助手不能点名）。 */
  sectionAgentIds: () => string[]
  /** 登录态才有「人」。 */
  hasUser: () => boolean
  /** 没有会话时先新建一个。 */
  resetThread: () => Promise<void>
  refreshThreads: () => void
  loadHumanTasks: () => Promise<void>
}

/**
 * 发送 / 重试 / 停止 / 人工待办回复。
 *
 * 协作会话（含旧 auto）走 /turns，允许一条消息里多个目标；固定助手的老会话走 /runs，只支持单目标。
 */
export function useAgentSender(opts: UseAgentSenderOptions) {
  const { conv, collab } = opts

  /** 最后一条消息失败时可重试：复用最近一次失败的 run。 */
  const canRetry = computed(() => {
    if (opts.sending.value || !conv.failedRunId.value) return false
    const last = opts.chatMessages.value[opts.chatMessages.value.length - 1]
    return last?.role === 'assistant' && !!last.error
  })

  /** 发送前校验：返回要提示的文案（空串 = 通过）。 */
  function validateMentions(agentIds: string[], mentioned: CloudAgent[], wantsHuman: boolean): string {
    if (wantsHuman && !opts.hasUser()) return t('components.agentWorkspace.humanRequired')
    if (agentIds.length && mentioned.length !== agentIds.length) return t('pages.agents.selectAgentFirst')
    // 群分区点名了群外助手：提示后保留草稿（服务端也会拒绝，这里提前说清楚）。
    const members = opts.sectionAgentIds()
    if (mentioned.some((a) => !members.includes(a.id))) return t('components.agentWorkspace.agentNotInTag')
    return opts.activeCapabilityError.value
  }

  /** 单助手 run 的结束 / 失败处理：`warnCode` 命中时 warning（可附带回调），否则 error。 */
  function runHandlers(warnCode: string, warnKey: string, onWarn?: () => void) {
    return {
      onEnd: () => opts.refreshThreads(),
      onError: (e: unknown) => {
        if (errorCode(e) === warnCode) {
          toast.warning(t(warnKey))
          onWarn?.()
          return
        }
        toast.error(errorMessage(e, t('pages.agents.runFailed')))
      }
    }
  }

  /**
   * 发送：一轮可以 @ 多位助手，全部执行（按依赖串行）。
   * 显式 @ 的目标交给服务端校验成员资格；无 @ 时由群路由提示词决定分工。
   */
  async function onSend(text: string, mentions: SendMentionTarget[]) {
    const content = text.trim()
    if (!content || opts.sending.value) return
    const { agentIds, wantsHuman } = collectMentionTargets(mentions)
    const mentionedAgents = agentIds
      .map((id) => opts.rawAgentById.value.get(id))
      .filter((a): a is CloudAgent => !!a)
    const warning = validateMentions(agentIds, mentionedAgents, wantsHuman)
    if (warning) {
      toast.warning(warning)
      return
    }
    if (!opts.threadId.value) await opts.resetThread()
    if (!opts.threadId.value) return
    const thread = opts.activeThread.value
    const threadAgentId = thread?.agent_id

    // 协作会话：走 /turns（Plan 栅栏），允许一条消息里多个目标。
    if (opts.isCollaborativeThread.value) {
      opts.draft.value = ''
      const targets: AgentMentionTarget[] = mentionedAgents.map((a) => ({ kind: 'agent', id: a.id }))
      if (wantsHuman) targets.push({ kind: 'human', id: 'me' })
      if (!targets.length && threadAgentId) targets.push({ kind: 'agent', id: threadAgentId })
      collab.start(opts.threadId.value, content, targets, thread?.thread_kind)
      return
    }
    // 固定助手的老会话：/runs 只支持单目标，多目标仍明确拒绝并保留草稿。
    if (agentIds.length > 1) {
      toast.warning(t('pages.agents.multipleAgentsNotSupported'))
      return
    }
    opts.draft.value = ''
    // 无 @ 时回到会话绑定的助手（老语义：每个助手一份会话）。
    const targetIds = agentIds.length ? agentIds : threadAgentId ? [threadAgentId] : []
    conv.start(
      { content, mentions: targetIds.map((id) => ({ agent_id: id })), module: (mentionedAgents[0] || opts.activeThreadAgent.value)?.module },
      runHandlers('agent_not_in_tag', 'components.agentWorkspace.agentNotInTag', () => { opts.draft.value = content })
    )
  }

  /**
   * 重试最近一次失败：普通 run 复用原消息。
   * 原助手已被删除/移出标签时服务端拒绝，提示重新发送。
   */
  function retryLast() {
    if (opts.sending.value) return
    if (opts.activeCapabilityError.value) { toast.warning(opts.activeCapabilityError.value); return }
    if (!conv.failedRunId.value) return
    const retryId = conv.failedRunId.value
    // 移除末尾失败气泡（user 消息保留展示）。
    const last = opts.chatMessages.value[opts.chatMessages.value.length - 1]
    if (last?.role === 'assistant' && (last.error || last.canceled)) opts.chatMessages.value.pop()
    conv.start(
      { content: '', mentions: [], retry_of_run_id: retryId },
      runHandlers('agent_unavailable', 'components.agentWorkspace.retryAgentUnavailable')
    )
  }

  /** 停止：协作会话取消整轮（服务端置 canceled），固定助手会话沿用旧 stop。 */
  async function onStop() {
    if (opts.sending.value) {
      const reply = opts.chatMessages.value[opts.chatMessages.value.length - 1]
      if (reply?.role === 'assistant') reply.canceled = true
    }
    if (opts.isCollaborativeThread.value) {
      await collab.cancel()
      return
    }
    conv.stop()
  }

  /** 人工待办：回复并继续 / 无法处理（与工具审批严格分离）。 */
  async function replyHumanTask(taskId: string, outcome: 'continue' | 'unable', text?: string) {
    await collab.replyHuman(taskId, outcome, text)
    await opts.loadHumanTasks()
  }

  return { canRetry, validateMentions, onSend, retryLast, onStop, replyHumanTask }
}
