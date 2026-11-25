import { ref } from 'vue'
import type { Ref } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import { t } from '@/locales'
import { findStarterTemplate } from '@/constants/starter-prompts'
import type { AgentThread, ChatMsg } from '@/services/types'
import { errorMessage } from '@/utils/format'
import { markStarterSent, starterAlreadySent } from '@/utils/starter-storage'

export interface StarterSite {
  baseUrl?: string
  publishable: boolean
  reason?: string
}

export interface UseStarterThreadOptions {
  projectId: () => string
  /** 首页来的 `?starter=<id>`（非字符串视为没有）。 */
  starterQuery: () => unknown
  section: Ref<string>
  threadId: Ref<string>
  chatMessages: Ref<ChatMsg[]>
  /** 当前分区的会话列表：新建后立即插到顶部。 */
  addThread: (thread: AgentThread) => void
  replaceQuery: (patch: Record<string, string | undefined>) => void
  /** 清空会话加载态（新建的空会话没有历史可载）。 */
  settleThread: (hasData: boolean) => void
  /** 发起协作轮次（与普通发送同一条通路）。 */
  startTurn: (threadId: string, content: string) => void
}

/**
 * 首页「一句话建应用」的搭建会话：在 #通用 新建协作会话并自动发送一次种子提示词。
 *
 * 这是对「点击只填不发」的显式例外，仅限 starter 入口：用户已经在确认弹窗里点了「开始搭建」，
 * 等价于回车。发送前先去重（sessionStorage）并立即去掉 query，刷新不会重复发。
 */
export function useStarterThread(opts: UseStarterThreadOptions) {
  const starterSite = ref<StarterSite | null>(null)

  function buildStarterSeed(templateId: string): string {
    const tpl = findStarterTemplate(templateId)
    if (!tpl) return ''
    const site = starterSite.value
    return t(tpl.promptKey, {
      projectId: opts.projectId(),
      siteBaseUrl: site?.baseUrl || '-',
      publishable: site?.publishable ? 'true' : `false${site?.reason ? ` (${site.reason})` : ''}`
    })
  }

  async function maybeStartStarter(): Promise<void> {
    const raw = opts.starterQuery()
    const templateId = typeof raw === 'string' ? raw : ''
    if (!templateId) return
    const consumeQuery = () => opts.replaceQuery({ starter: undefined })
    const tpl = findStarterTemplate(templateId)
    const projectId = opts.projectId()
    if (!tpl || !projectId) { consumeQuery(); return }
    if (starterAlreadySent(projectId, templateId)) { consumeQuery(); return }
    if (opts.section.value !== 'general') {
      opts.section.value = 'general'
      opts.threadId.value = ''
    }
    markStarterSent(projectId, templateId)
    consumeQuery()
    let id = ''
    try {
      const th = await api.agentThreads.create(projectId, {
        title: t(tpl.titleKey),
        tagId: 'general',
        dispatchMode: 'collaborative',
        threadKind: 'chat',
        source: 'starter'
      })
      if (opts.projectId() !== projectId) return
      id = th.id
      opts.addThread(th)
      opts.threadId.value = id
      opts.chatMessages.value = []
      opts.settleThread(false)
      opts.replaceQuery({ thread: id })
    } catch (e) {
      toast.error(errorMessage(e, t('components.starter.sessionFailed')))
      return
    }
    const site = await api.s3.getSite(projectId).catch(() => null)
    if (opts.projectId() !== projectId || opts.threadId.value !== id) return
    starterSite.value = site
    opts.startTurn(id, buildStarterSeed(tpl.id))
  }

  return { starterSite, buildStarterSeed, maybeStartStarter }
}
