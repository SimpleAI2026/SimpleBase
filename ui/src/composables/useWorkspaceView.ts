import { computed } from 'vue'
import type { Ref } from 'vue'
import { t } from '@/locales'
import type { AgentTag, AgentThread, CloudAgent, MentionAgentLike } from '@/services/types'
import { isSeedName } from '@/utils/agent-names'

/** @ 候选（与 AiChatComposer 的 MentionAgent 结构一致：.ts 文件无法 import .vue 的具名类型）。 */
export interface WorkspaceMentionAgent extends MentionAgentLike {
  /** 服务端保存的旧内置名称，语言切换后仍可识别老草稿。 */
  legacyName?: string
  builtinKey?: string
  /** 'human' 表示「当前人」，发送时转成 {kind:'human', id:'me'}。缺省是助手。 */
  kind?: 'agent' | 'human'
}

export interface UseWorkspaceViewOptions {
  tags: Ref<AgentTag[]>
  section: Ref<string>
  threadId: Ref<string>
  /** 当前群的会话列表。 */
  sectionThreads: Ref<AgentThread[]>
  /** 按当前语言解析显示名后的助手 / 索引（来自 useAgentDirectory）。 */
  displayedAgents: Ref<CloudAgent[]>
  displayedAgentById: Ref<Map<string, CloudAgent>>
  rawAgentById: Ref<Map<string, CloudAgent>>
  /** 登录态才有「当前人」（API Key 主体不能担任人工接收人）。 */
  hasUser: () => boolean
}

/** 标签（群）显示名：预置走 i18n。 */
export function tagDisplayName(tag: AgentTag): string {
  return tag.builtinKey ? t(`components.agentWorkspace.builtinTags.${tag.builtinKey}`) : tag.name
}

/**
 * 工作台的只读衍生视图：当前群 / 当前会话 / 页头标题 / 成员助手条 / @ 候选 / 总览最近会话。
 * 全是 computed，不持有状态、不发请求。
 */
export function useWorkspaceView(opts: UseWorkspaceViewOptions) {
  const activeTag = computed(() => opts.tags.value.find((tag) => tag.id === opts.section.value) || null)
  const activeThread = computed(() => opts.sectionThreads.value.find((th) => th.id === opts.threadId.value))
  const sectionDisplayName = computed(() => (activeTag.value ? tagDisplayName(activeTag.value) : ''))
  const headerTitle = computed(() => sectionDisplayName.value || t('pages.agents.dialogLabel'))

  /** 标签内可用助手（已过滤不存在 / 归档助手）。 */
  const sectionAgents = computed<MentionAgentLike[]>(() => {
    const tag = activeTag.value
    if (!tag) return []
    return tag.agentIds
      .map((id) => opts.displayedAgentById.value.get(id))
      .filter((a): a is NonNullable<typeof a> => !!a)
      .map((a) => ({ id: a.id, name: a.name, module: a.module }))
  })
  /** 标签最近会话（总览下半屏）。 */
  const recentTagThreads = computed(() => opts.sectionThreads.value.slice(0, 5))

  /**
   * @ 候选：
   *   - 群分区只列本群成员，避免「点名群外助手 → 服务端拒绝」；
   *   - 登录态额外提供「当前人」，用于生成人工待办。
   */
  const mentionAgents = computed<WorkspaceMentionAgent[]>(() => {
    const memberIds = new Set(sectionAgents.value.map((agent) => agent.id))
    const list: WorkspaceMentionAgent[] = opts.displayedAgents.value
      .filter((agent) => memberIds.has(agent.id))
      .map((a) => {
        const raw = opts.rawAgentById.value.get(a.id)
        return {
          id: a.id,
          name: a.name,
          module: a.module,
          legacyName: a.builtin_key ? raw?.name : undefined,
          builtinKey: a.builtin_key && raw && isSeedName(raw) ? a.builtin_key : undefined
        }
      })
    if (opts.hasUser() && activeTag.value?.humanPolicy !== 'disabled') {
      list.push({ id: 'me', name: t('components.agentWorkspace.mentionMe'), kind: 'human' })
    }
    return list
  })

  /** 标签编辑弹窗的助手候选（含已不可用项，便于编辑器提示）。 */
  function agentOptionsFor(tag: AgentTag | null): MentionAgentLike[] {
    const used = new Set(tag?.agentIds || [])
    const out: MentionAgentLike[] = opts.displayedAgents.value.map((a) => ({ id: a.id, name: a.name, module: a.module }))
    for (const id of tag?.unavailableAgentIds || []) {
      if (!used.has(id)) continue
      out.push({ id, name: t('components.agentTagOverview.unavailable') })
    }
    return out
  }

  return { activeTag, activeThread, sectionDisplayName, headerTitle, sectionAgents, recentTagThreads, mentionAgents, agentOptionsFor }
}
