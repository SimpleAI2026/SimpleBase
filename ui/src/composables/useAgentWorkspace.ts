// useAgentWorkspace：左栏群与会话树的取数。
//
// 与页面组件的分工：
//   - 本 composable 只负责「每个群独立分页」「展开状态」「当前选中」，
//     不碰输入框、不碰消息流，便于单独测试；
//   - 页面（AgentManager）负责把选中的群/会话映射到消息加载。
//
// 设计要点：
//   - 每个标签各自维护一份 keyset 游标，互不影响；**当前群与其他群共用同一份 `pages`**，
//     是会话列表的唯一数据源。
//   - 群列表来自 /agent-tags；会话列表来自 /agent-threads?tag_id=...
//   - 所有请求都带「请求序号」守卫：切换项目/群时旧响应不会覆盖新列表。
import { computed, reactive, ref } from 'vue'
import { api } from '@/services/api'
import { t } from '@/locales'
import type { AgentTag, AgentThread, AgentThreadPage } from '@/services/types'
import { errorMessage } from '@/utils/format'

/** 每个群的会话分页状态。 */
export interface GroupThreadPage {
  threads: AgentThread[]
  cursor: string
  loading: boolean
  /** 是否还有下一页（cursor 非空即还有）。 */
  hasMore: boolean
  error: string
}

export interface UseAgentWorkspaceOptions {
  projectId: () => string
  /** 单页条数（默认 30，与后端 pagelimit 上限 200 兼容）。 */
  pageSize?: number
}

export function useAgentWorkspace(opts: UseAgentWorkspaceOptions) {
  const pageSize = opts.pageSize ?? 30
  const tags = ref<AgentTag[]>([])
  const tagsLoading = ref(false)
  const tagsError = ref('')
  /** 每个标签的会话分页（key = 标签 ID）。 */
  const pages = reactive<Record<string, GroupThreadPage>>({})

  // tagsSeq 守卫群列表；pageSeq 按群独立守卫会话分页（并行预取时互不丢弃）。
  let tagsSeq = 0
  const pageSeq: Record<string, number> = {}

  function emptyPage(): GroupThreadPage {
    return { threads: [], cursor: '', loading: false, hasMore: false, error: '' }
  }

  function pageOf(tagId: string): GroupThreadPage {
    if (!pages[tagId]) pages[tagId] = emptyPage()
    return pages[tagId]
  }

  /** 加载群列表（含内置群）。失败时保留旧列表，只置错误。 */
  async function loadTags(): Promise<AgentTag[]> {
    const projectId = opts.projectId()
    if (!projectId) return tags.value
    const seq = ++tagsSeq
    tagsLoading.value = true
    tagsError.value = ''
    try {
      const list = await api.agentTags.list(projectId)
      if (seq !== tagsSeq) return tags.value
      tags.value = Array.isArray(list) ? list : []
      return tags.value
    } catch (e) {
      if (seq === tagsSeq) tagsError.value = errorMessage(e, t('error.requestFailed'))
      return tags.value
    } finally {
      if (seq === tagsSeq) tagsLoading.value = false
    }
  }

  /**
   * 重新拉标签并预取「其他群」的首屏会话（当前群由页面的 loadSectionThreads 驱动，
   * 它有更完整的游标与选中态）。标签加载失败时 `onError` 收到原始错误，列表清空。
   */
  async function refreshTagsAndPrefetch(currentTagId: string, onError?: (e: unknown) => void): Promise<void> {
    const projectId = opts.projectId()
    if (!projectId) return
    try {
      const list = await api.agentTags.list(projectId)
      tags.value = Array.isArray(list) ? list : []
    } catch (e) {
      tags.value = []
      onError?.(e)
    }
    const others = tags.value.map((tag) => tag.id).filter((id) => id !== currentTagId)
    if (others.length) await prefetchAll(others)
  }

  /** 项目切换：丢弃全部本地状态（标签与各群分页）。 */
  function reset(): void {
    tags.value = []
    for (const id of Object.keys(pages)) delete pages[id]
  }

  /** 把一页结果整页写入某群（覆盖式）：游标与 hasMore 同步更新。 */
  function setPage(tagId: string, res: AgentThreadPage): void {
    const page = pageOf(tagId)
    page.threads = Array.isArray(res?.threads) ? res.threads : []
    page.cursor = res?.next_cursor || ''
    page.hasMore = Boolean(page.cursor)
  }

  /** 载入某个群的首屏会话（覆盖式）。 */
  async function loadThreads(tagId: string): Promise<AgentThread[]> {
    const projectId = opts.projectId()
    if (!projectId) return []
    const seq = (pageSeq[tagId] = (pageSeq[tagId] ?? 0) + 1)
    const page = pageOf(tagId)
    page.loading = true
    page.error = ''
    try {
      const res = await api.agentThreads.pageBy(projectId, { tagId, limit: pageSize })
      if (seq !== pageSeq[tagId]) return page.threads
      setPage(tagId, res)
      return page.threads
    } catch (e) {
      if (seq === pageSeq[tagId]) page.error = errorMessage(e, t('error.requestFailed'))
      return page.threads
    } finally {
      if (seq === pageSeq[tagId]) page.loading = false
    }
  }

  /**
   * 加载更多（追加式，去重保序）。
   * 与 `loadThreads` 共用同一个 pageSeq：加载期间若该群被重载 / 切项目，旧响应作废。
   */
  async function loadMore(tagId: string, limit = pageSize): Promise<AgentThread[]> {
    const page = pageOf(tagId)
    if (page.loading || !page.hasMore) return page.threads
    const projectId = opts.projectId()
    if (!projectId) return page.threads
    const seq = pageSeq[tagId] ?? 0
    page.loading = true
    page.error = ''
    try {
      const res = await api.agentThreads.pageBy(projectId, { tagId, limit, cursor: page.cursor })
      if (seq !== (pageSeq[tagId] ?? 0) || projectId !== opts.projectId()) return page.threads
      appendPage(tagId, res)
      return page.threads
    } catch (e) {
      if (seq === (pageSeq[tagId] ?? 0)) page.error = errorMessage(e, t('error.requestFailed'))
      return page.threads
    } finally {
      page.loading = false
    }
  }

  /**
   * 当前群的首屏：与 `loadThreads` 的差别只有两点——
   *   - 页面传入自己的取数后续（选中会话、加载消息），所以抛错由调用方处理（不吞）；
   *   - 切项目 / 切群的竞态由 `isCurrent` 判定，旧响应不落地并返回 false。
   */
  async function loadFirstPage(
    tagId: string,
    limit: number,
    isCurrent: () => boolean
  ): Promise<AgentThread[] | null> {
    const projectId = opts.projectId()
    const seq = (pageSeq[tagId] = (pageSeq[tagId] ?? 0) + 1)
    const page = pageOf(tagId)
    page.loading = true
    try {
      const res = await api.agentThreads.pageBy(projectId, { tagId, limit })
      if (seq !== pageSeq[tagId] || projectId !== opts.projectId() || !isCurrent()) return null
      setPage(tagId, res)
      return page.threads
    } finally {
      if (seq === pageSeq[tagId]) page.loading = false
    }
  }

  /** run 结束后刷新某群首屏：失败静默，保留本地新建但服务端分页尚未收录的会话。 */
  async function refreshFirstPage(tagId: string, limit: number): Promise<void> {
    const projectId = opts.projectId()
    try {
      const res = await api.agentThreads.pageBy(projectId, { tagId, limit })
      if (projectId !== opts.projectId()) return
      mergeFirstPage(tagId, res)
    } catch {
      /* 后台刷新失败静默：下一次动作会重新拉列表 */
    }
  }

  /** 把下一页追加到某群：按 id 去重保序，并更新游标。 */
  function appendPage(tagId: string, res: AgentThreadPage): void {
    const page = pageOf(tagId)
    const incoming = Array.isArray(res?.threads) ? res.threads : []
    const fresh = incoming.filter((th) => !page.threads.some((old) => old.id === th.id))
    page.threads = [...page.threads, ...fresh]
    page.cursor = res?.next_cursor || ''
    page.hasMore = Boolean(page.cursor)
  }

  /** 预取所有群的首屏会话（侧栏不再折叠，需要每个群都有数据）。 */
  async function prefetchAll(groups: string[]): Promise<void> {
    await Promise.all(groups.map((id) => loadThreads(id)))
  }

  /** 本地插入一条新会话（新建后立即出现在列表顶部）。 */
  function unshiftThread(tagId: string, thread: AgentThread): void {
    const page = pageOf(tagId)
    if (page.threads.some((th) => th.id === thread.id)) return
    page.threads = [thread, ...page.threads]
  }

  /** 本地移除已归档会话。 */
  function removeThread(tagId: string, threadId: string): void {
    const page = pageOf(tagId)
    page.threads = page.threads.filter((th) => th.id !== threadId)
  }

  /** 重命名后同步列表（后端返回新的 updated_at）。 */
  function replaceThread(tagId: string, thread: AgentThread): void {
    const page = pageOf(tagId)
    page.threads = page.threads.map((th) => (th.id === thread.id ? thread : th))
  }

  /**
   * 用服务端首屏刷新某群，但保留本地新建、服务端分页尚未收录的会话
   * （如 auto 会话首条，run 结束后标题/预览才会出现在分页里）。
   */
  function mergeFirstPage(tagId: string, res: AgentThreadPage): void {
    const page = pageOf(tagId)
    const fresh = Array.isArray(res?.threads) ? res.threads : []
    page.threads = fresh.concat(page.threads.filter((th) => !fresh.some((p) => p.id === th.id)))
    page.cursor = res?.next_cursor || ''
    page.hasMore = Boolean(page.cursor)
  }

  /** 向后端刷新某个群的首屏（run 结束后标题/预览会变）。 */
  async function refreshGroup(tagId: string): Promise<void> {
    const page = pageOf(tagId)
    const keep = page.threads
    await loadThreads(tagId)
    const fresh = pageOf(tagId)
    // 保留本地新建、服务端分页尚未收录的会话（如刚发首条的会话）。
    fresh.threads = fresh.threads.concat(keep.filter((th) => !fresh.threads.some((p) => p.id === th.id)))
  }

  const groupOf = (tagId: string): AgentTag | undefined => tags.value.find((tag) => tag.id === tagId)

  /** 会话所属标签。 */
  const tagIdOfThread = (thread: AgentThread | undefined): string => thread?.tag_id || ''

  return {
    tags,
    tagsLoading,
    tagsError,
    pages,
    computedPages: computed(() => pages),
    loadTags,
    refreshTagsAndPrefetch,
    reset,
    loadThreads,
    loadMore,
    loadFirstPage,
    refreshFirstPage,
    prefetchAll,
    unshiftThread,
    removeThread,
    replaceThread,
    setPage,
    appendPage,
    mergeFirstPage,
    refreshGroup,
    pageOf,
    groupOf,
    tagIdOfThread
  }
}
