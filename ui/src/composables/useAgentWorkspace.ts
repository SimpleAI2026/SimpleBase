// useAgentWorkspace：左栏群与会话树的取数（planv6.7 v2 §4）。
//
// 与页面组件的分工：
//   - 本 composable 只负责「每个群独立分页」「展开状态」「当前选中」，
//     不碰输入框、不碰消息流，便于单独测试；
//   - 页面（AgentManager）负责把选中的群/会话映射到消息加载。
//
// 设计要点：
//   - 每个标签各自维护一份 keyset 游标，互不影响。
//   - 群列表来自 /agent-tags；会话列表来自 /agent-threads?tag_id=...
//   - 所有请求都带「请求序号」守卫：切换项目/群时旧响应不会覆盖新列表。
import { computed, reactive, ref } from 'vue'
import { api } from '@/services/api'
import type { AgentTag, AgentThread } from '@/services/types'

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
      if (seq === tagsSeq) tagsError.value = errorText(e)
      return tags.value
    } finally {
      if (seq === tagsSeq) tagsLoading.value = false
    }
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
      page.threads = res.threads
      page.cursor = res.next_cursor
      page.hasMore = Boolean(res.next_cursor)
      return page.threads
    } catch (e) {
      if (seq === pageSeq[tagId]) page.error = errorText(e)
      return page.threads
    } finally {
      if (seq === pageSeq[tagId]) page.loading = false
    }
  }

  /** 加载更多（追加式，去重保序）。 */
  async function loadMore(tagId: string): Promise<AgentThread[]> {
    const page = pageOf(tagId)
    if (page.loading || !page.hasMore) return page.threads
    const projectId = opts.projectId()
    if (!projectId) return page.threads
    page.loading = true
    page.error = ''
    try {
      const res = await api.agentThreads.pageBy(projectId, { tagId, limit: pageSize, cursor: page.cursor })
      const fresh = res.threads.filter((th) => !page.threads.some((old) => old.id === th.id))
      page.threads = [...page.threads, ...fresh]
      page.cursor = res.next_cursor
      page.hasMore = Boolean(res.next_cursor)
      return page.threads
    } catch (e) {
      page.error = errorText(e)
      return page.threads
    } finally {
      page.loading = false
    }
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
    loadThreads,
    loadMore,
    prefetchAll,
    unshiftThread,
    removeThread,
    replaceThread,
    refreshGroup,
    pageOf,
    groupOf,
    tagIdOfThread
  }
}

/** 统一的错误文案提取（避免各处重复 any）。 */
export function errorText(e: unknown): string {
  if (e instanceof Error && e.message) return e.message
  if (typeof e === 'string') return e
  return 'request failed'
}
