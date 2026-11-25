import type { ComputedRef, Ref } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '@/services/api'
import { t } from '@/locales'
import type { AgentTag, AgentThread, AgentThreadCreateOptions, ChatMsg } from '@/services/types'
import { toChatMessages } from '@/utils/chat-messages'
import { errorMessage } from '@/utils/format'
import type { useAgentConversation } from '@/composables/useAgentConversation'
import type { useAgentCollaboration } from '@/composables/useAgentCollaboration'
import type { useAgentWorkspace } from '@/composables/useAgentWorkspace'

/** 当前群首屏 / 续页的单页条数（侧栏预取其他群沿用 workspace 默认值）。 */
export const SECTION_PAGE_SIZE = 50

/** useLoadState 的 `run` / `settle`：页面把会话加载态传进来，这里只负责驱动。 */
export interface ThreadLoadRunner {
  run: (task: () => Promise<boolean>, opts?: { replace?: boolean }) => Promise<unknown>
  settle: (hasData: boolean) => void
}

export interface UseThreadNavigationOptions {
  projectId: () => string
  tags: Ref<AgentTag[]>
  section: Ref<string>
  threadId: Ref<string>
  chatMessages: Ref<ChatMsg[]>
  sectionThreads: ComputedRef<AgentThread[]>
  sectionCursor: ComputedRef<string>
  sending: ComputedRef<boolean>
  ws: ReturnType<typeof useAgentWorkspace>
  conv: ReturnType<typeof useAgentConversation>
  collab?: ReturnType<typeof useAgentCollaboration>
  isCollaborative?: () => boolean
  load: ThreadLoadRunner
  /** 当前 URL 里的 `?thread=`：首次进入 / 刷新时恢复选中。 */
  queryThread: () => unknown
  replaceQuery: (patch: Record<string, string | undefined>) => void
  moduleOf: (agentId: string) => string | undefined
  /** 停止当前运行（删除会话前先停）。 */
  stop: () => void | Promise<void>
}

/**
 * 会话导航：切分区 / 选会话 / 新建 / 删除 / 重命名 / 加载消息并续接后台 run。
 *
 * 竞态都在这里收口：项目或分区在请求期间切换时，旧响应既不写列表也不写消息。
 */
export function useThreadNavigation(opts: UseThreadNavigationOptions) {
  const { ws, conv } = opts

  /** 切换会话 / 助手：只断开 SSE，后端 run 继续，回到该会话时自动续接。 */
  function detachRun() {
    conv.clear()
    opts.collab?.detach()
  }

  async function loadThreadMessages(id: string): Promise<boolean> {
    const msgs = await api.agentThreads.messages(opts.projectId(), id)
    if (opts.threadId.value !== id) return false
    opts.chatMessages.value = toChatMessages(msgs, opts.moduleOf)
    void resumeActiveRun(id)
    return opts.chatMessages.value.length > 0
  }

  /**
   * 回到会话时续接后台仍在运行的 run：切换界面只断开 SSE，不取消 run，
   * 这里查最新一次 run，仍在运行就回放事件并继续跟随。
   */
  async function resumeActiveRun(id: string) {
    if (opts.sending.value) return
    const projectId = opts.projectId()
    if (opts.collab && opts.isCollaborative?.()) {
      const turns = await api.agentThreads.turns(projectId, id, 1).catch(() => [])
      const latest = turns?.[0]
      if (opts.threadId.value !== id || opts.sending.value) return
      if (latest) {
        if (['running', 'planning', 'awaiting_plan_confirm', 'waiting_human'].includes(latest.status)) {
          await opts.collab.refresh(latest.id)
        }
        return
      }
    }
    const runs = await api.agentThreads.runs(projectId, id, 1).catch(() => [])
    const run = runs?.[0]
    if (opts.threadId.value !== id || opts.sending.value) return
    if (!run || (run.status !== 'running' && run.status !== 'queued')) return
    conv.attach(run.id, {
      module: opts.moduleOf(run.agent_id),
      onEnd: () => refreshThreads(),
      // 事件缓存已过期等：重新拉一次已落库的消息。
      onError: () => {
        void api.agentThreads.messages(projectId, id).then((list) => {
          if (opts.threadId.value !== id || opts.sending.value) return
          opts.chatMessages.value = toChatMessages(list, opts.moduleOf)
        }).catch(() => undefined)
      }
    })
  }

  async function selectThread(id: string) {
    if (!id) return
    // 深链/总览的线程可能不在当前分区列表：先补齐再选（404 → 会话不存在提示）。
    if (!opts.sectionThreads.value.some((thread) => thread.id === id)) {
      // api-mock 的 get 默认返回 undefined（非 Promise）：Promise.resolve 让同步值也走 catch 路径。
      const th = await Promise.resolve(api.agentThreads.get(opts.projectId(), id)).catch(() => null)
      if (!th?.tag_id) {
        toast.error(t('components.agentWorkspace.threadNotFound'))
        return
      }
      if (th.tag_id !== opts.section.value) await selectSection(th.tag_id)
      ws.unshiftThread(opts.section.value, th)
    }
    detachRun()
    opts.threadId.value = id
    opts.replaceQuery({ thread: id })
    await opts.load.run(async () => loadThreadMessages(id), { replace: true })
  }

  /** 切换标签：重置会话列表并按 query 或首条恢复选中。 */
  async function selectSection(id: string) {
    if (!id || id === opts.section.value) return
    detachRun()
    opts.section.value = id
    opts.threadId.value = ''
    opts.replaceQuery({ thread: undefined, tag: id === 'general' ? undefined : id })
    await loadSectionThreads()
  }

  /** 拉当前分区的会话列表（首页 50 条 + 游标），并选中 query 指定的会话或首条。 */
  async function loadSectionThreads() {
    const sec = opts.section.value
    await opts.load.run(async () => {
      // 项目或分区可能在取数期间切换：workspace 的 seq / isCurrent 守卫让旧响应不落地（返回 null）。
      const threads = await ws.loadFirstPage(sec, SECTION_PAGE_SIZE, () => opts.section.value === sec)
      if (!threads) return false
      const query = opts.queryThread()
      const queryId = typeof query === 'string' ? query : ''
      const selected = queryId && threads.some((th) => th.id === queryId) ? queryId : threads[0]?.id || ''
      if (selected) {
        opts.threadId.value = selected
        opts.replaceQuery({ thread: selected })
        return loadThreadMessages(selected)
      }
      // 分区无会话：展示总览空态，不自动建会话。
      opts.threadId.value = ''
      return false
    }, { replace: true })
  }

  /** 当前标签加载下一页：竞态与去重在 workspace，这里只负责失败 toast。 */
  async function loadMoreSectionThreads() {
    const sec = opts.section.value
    if (!opts.sectionCursor.value) return
    await ws.loadMore(sec, SECTION_PAGE_SIZE)
    const page = ws.pageOf(sec)
    if (page.error) toast.error(page.error)
  }

  function reloadThread() {
    if (opts.threadId.value) void selectThread(opts.threadId.value)
    else void loadSectionThreads()
  }

  /**
   * 新建会话：所有标签都建协作会话，成员与路由规则由标签配置决定；
   * 会话类型缺省按标签推断（告警 → alert、统计 → statistics、定时 → scheduled）。
   */
  async function resetThread() {
    detachRun()
    const projectId = opts.projectId()
    const sec = opts.section.value
    if (!opts.tags.value.some((item) => item.id === sec)) return
    const createOpts: AgentThreadCreateOptions = {
      title: t('nav.agents'),
      tagId: sec,
      dispatchMode: 'collaborative',
      source: 'workspace'
    }
    try {
      const th = await api.agentThreads.create(projectId, createOpts)
      if (opts.projectId() !== projectId || opts.section.value !== sec) return
      ws.unshiftThread(sec, th)
      opts.threadId.value = th.id
      opts.chatMessages.value = []
      opts.load.settle(false)
      opts.replaceQuery({ thread: th.id })
    } catch (e) {
      toast.error(errorMessage(e, t('pages.agents.createThreadFailed')))
    }
  }

  async function renameThread(id: string, title: string) {
    try {
      const renamed = await api.agentThreads.rename(opts.projectId(), id, title)
      ws.replaceThread(opts.section.value, renamed)
    } catch (e) { toast.error(errorMessage(e, t('pages.agents.renameFailed'))) }
  }

  async function removeThread(id: string) {
    void opts.stop()
    try {
      await api.agentThreads.remove(opts.projectId(), id)
      ws.removeThread(opts.section.value, id)
      if (opts.threadId.value === id) {
        opts.threadId.value = ''
        // 分区还有会话就切首条；否则回到总览空态（不再自动建会话）。
        if (opts.sectionThreads.value.length) await selectThread(opts.sectionThreads.value[0].id)
      }
    } catch (e) { toast.error(errorMessage(e, t('pages.agents.deleteThreadFailed'))) }
  }

  /** run 结束后刷新当前分区列表（标题/预览可能变化）；失败静默，保留本地新建但服务端未收录的会话。 */
  function refreshThreads() {
    void ws.refreshFirstPage(opts.section.value, SECTION_PAGE_SIZE)
  }

  /** 侧栏新建会话：按钮带群 ID，先切分区再建。 */
  async function createThreadInGroup(id: string) {
    if (id !== opts.section.value) await selectSection(id)
    await resetThread()
  }

  /** 侧栏点会话：跨群时先切分区，再选中。 */
  async function selectThreadInGroup(id: string, groupId: string) {
    if (groupId && groupId !== opts.section.value) await selectSection(groupId)
    await selectThread(id)
  }

  return {
    detachRun,
    loadThreadMessages,
    selectThread,
    selectSection,
    loadSectionThreads,
    loadMoreSectionThreads,
    reloadThread,
    resetThread,
    renameThread,
    removeThread,
    refreshThreads,
    createThreadInGroup,
    selectThreadInGroup
  }
}
