// useBiAgent.ts — 数据看板「Agent 生成图表」的会话与运行编排（planv6.7 §4.7）。
//
// 复用现有数据助手（builtin_key === 'database'）与 useAgentConversation，不新建对话层。
// 助手只产出配置块；块经 bi-agent-block 校验后进入「待确认区」，由用户决定是否加入看板。

import { computed, ref, shallowRef } from 'vue'
import { api } from '@/services/api'
import { t } from '@/locales'
import { agentErrorText } from '@/utils/agent-errors'
import { parseChartBlocksDetailed, stripChartBlocks, validateAgentWidget } from '@/utils/bi-agent-block'
import { newWidgetId } from '@/composables/useBiDashboard'
import { useAgentConversation } from '@/composables/useAgentConversation'
import { sourceKeyPart, type BiSourceKind } from '@/utils/bi-source'
import type {
  ApiError,
  BiAgentContext,
  BiChartType,
  BiWidget,
  ChatMsg,
  CloudAgent,
  CollectionField
} from '@/services/types'

const THREAD_PREFIX = 'sb_bi_thread_v1'
/** 内置数据助手的标识（systemdb defaultCloudAgentSpecs）。 */
export const DATA_AGENT_KEY = 'database'

export interface BiAgentScope {
  projectId: string
  databaseId: string
  /** 数据源名：集合名或 SQL 表名。 */
  collection: string
  /** 缺省为集合。 */
  kind?: BiSourceKind
}

export interface PendingWidget {
  widget: BiWidget
}

export interface RejectedBlock {
  title: string
  reason: string
}

/** 某个数据源的会话现场（切换到别处时暂存，切回来时恢复）。 */
interface ParkedState {
  threadId: string
  messages: ChatMsg[]
  pending: PendingWidget[]
  rejected: RejectedBlock[]
  notes: string
  noBlock: boolean
  failure: string
  lastPrompt: string
  /** 仍在后台运行的 run id（空表示没有在跑的）。 */
  runId: string
}

export function threadKey(scope: BiAgentScope): string {
  return `${THREAD_PREFIX}:${scope.projectId}:${scope.databaseId}:${sourceKeyPart({ kind: scope.kind ?? 'collection', name: scope.collection })}`
}

/**
 * 字段清单：隐藏字段不传。集合补上内置时间列（它们在 schema 里不存在，但恒可用）；
 * SQL 表只传真实列——不能谎称有 created_at，否则助手会据此生成必然 400 的图表。
 */
export function buildAgentFields(fields: CollectionField[], builtinTime: boolean = true): BiAgentContext['fields'] {
  const out = fields.filter((f) => !f.hidden).map((f) => ({ name: f.name, type: f.type }))
  if (builtinTime) {
    for (const name of ['created_at', 'updated_at']) {
      if (!out.some((f) => f.name === name)) out.push({ name, type: 'date' })
    }
  }
  return out
}

export interface BiAgentOptions {
  scope: () => BiAgentScope
  fields: () => CollectionField[]
  /** 当前类型选择器的值；'auto' 不传给后端。 */
  preferredType: () => BiChartType
  storage?: Storage
}

function safeStorage(): Storage | undefined {
  try {
    return typeof window !== 'undefined' ? window.localStorage : undefined
  } catch {
    return undefined
  }
}

export function useBiAgent(opts: BiAgentOptions) {
  const messages = ref<ChatMsg[]>([])
  const agent = shallowRef<CloudAgent | null>(null)
  const agentChecked = ref(false)
  const threadId = ref('')
  const pending = ref<PendingWidget[]>([])
  const rejected = ref<RejectedBlock[]>([])
  const notes = ref('')
  const noBlock = ref(false)
  const lastPrompt = ref('')
  const failure = ref('')
  /** 作用域令牌：切项目 / 库 / 集合后，旧会话的回调一律丢弃（同 useStarterRun 的 epoch）。 */
  let epoch = 0
  /**
   * 当前会话属于哪个数据源。reset() 发生在数据源已经切走之后（source 先变、再触发清理），
   * 不能用「当前 scope」判断旧现场属于谁，否则会把 A 的待确认区暂存到 B 名下、再被 B 恢复出来。
   */
  const liveThreadKey = ref('')

  const conv = useAgentConversation({
    projectId: () => opts.scope().projectId,
    threadId: () => threadId.value,
    messages: () => messages.value
  })

  const store = () => opts.storage ?? safeStorage()
  /** 准备阶段（查助手 / 建会话）也算忙：否则用户可以连发，建出多个会话。 */
  const preparing = ref(false)
  const sending = computed(() => preparing.value || conv.sending.value)
  const agentMissing = computed(() => agentChecked.value && !agent.value)

  /**
   * 每个数据源的会话现场：切到别的数据源时暂存，回到同一数据源时恢复。
   * 只存在内存里（刷新后靠 threadId + localStorage 重新探测），不新增持久化格式。
   */
  const parked = new Map<string, ParkedState>()

  /** 把当前现场存到「当前数据源」名下；没有会话（没发过消息）就不存。 */
  function park() {
    if (!liveThreadKey.value || !threadId.value) return
    parked.set(liveThreadKey.value, {
      threadId: threadId.value,
      messages: messages.value,
      pending: pending.value,
      rejected: rejected.value,
      notes: notes.value,
      noBlock: noBlock.value,
      failure: failure.value,
      lastPrompt: lastPrompt.value,
      runId: conv.runId.value
    })
  }

  /** 清空实时状态（不动 parked）：切数据源、新对话共用。 */
  function clearLive() {
    epoch++
    conv.clear()
    preparing.value = false
    messages.value = []
    pending.value = []
    rejected.value = []
    notes.value = ''
    noBlock.value = false
    failure.value = ''
    lastPrompt.value = ''
    threadId.value = ''
    liveThreadKey.value = ''
  }

  /**
   * 切换数据源：先把当前现场存起来，再清空实时状态。
   * 回到同一数据源时由 sync() 恢复（含仍在后台跑的 run）。
   */
  function reset() {
    park()
    clearLive()
  }

  /**
   * 数据源加载完成后调用：如果这个数据源之前有会话现场，恢复历史与待确认区；
   * 后台仍在运行的 run 用 attach 续接（只读回放 + 跟随到结束），而不是让用户看不到结果。
   */
  function sync() {
    const scope = opts.scope()
    if (!scope.collection) return
    const saved = parked.get(threadKey(scope))
    if (!saved) return
    parked.delete(threadKey(scope))
    const mine = ++epoch
    liveThreadKey.value = threadKey(scope)
    threadId.value = saved.threadId
    messages.value = saved.messages
    pending.value = saved.pending
    rejected.value = saved.rejected
    notes.value = saved.notes
    noBlock.value = saved.noBlock
    failure.value = saved.failure
    lastPrompt.value = saved.lastPrompt
    if (!saved.runId) return
    conv.attach(saved.runId, {
      onEnd: () => {
        if (mine !== epoch) return
        const reply = [...messages.value].reverse().find((m) => m.role === 'assistant')
        ingest(reply?.content ?? '')
      },
      onError: (e) => {
        if (mine !== epoch) return
        failure.value = agentErrorText(e)
      }
    })
  }

  async function ensureAgent(): Promise<CloudAgent | null> {
    if (agent.value) return agent.value
    const list = await api.agents.list(opts.scope().projectId)
    agent.value = list.find((a) => a.builtin_key === DATA_AGENT_KEY) ?? null
    agentChecked.value = true
    return agent.value
  }

  /** 读本集合已有会话；不存在（404 / 过期）就建新的。force 强制新建（「新对话」）。 */
  async function ensureThread(a: CloudAgent, force = false): Promise<string> {
    const scope = opts.scope()
    const key = threadKey(scope)
    if (!force) {
      const saved = store()?.getItem(key)
      if (saved) {
        try {
          // 契约里没有「按 id 取会话」：messages 在会话不存在时返回 404，用它探测会话是否还在。
          await api.agentThreads.messages(scope.projectId, saved)
          liveThreadKey.value = key
          return saved
        } catch (e) {
          // 404 才重建；其他错误（网络 / 鉴权）直接抛给调用方，不静默新建一堆会话。
          if ((e as ApiError)?.status !== 404) throw e
        }
      }
    }
    const created = await api.agentThreads.create(
      scope.projectId,
      t(scope.kind === 'table' ? 'components.bi.agent.threadTitleTable' : 'components.bi.agent.threadTitle', { collection: scope.collection }),
      a.id
    )
    try {
      store()?.setItem(key, created.id)
    } catch {
      /* 持久化失败只影响下次复用同一会话 */
    }
    liveThreadKey.value = key
    return created.id
  }

  function buildContext(): BiAgentContext {
    const scope = opts.scope()
    const preferred = opts.preferredType()
    const table = scope.kind === 'table'
    return {
      database_id: scope.databaseId,
      collection: scope.collection,
      // 集合不带 source 键：请求与改造前逐字一致，旧服务端也能处理。
      ...(table ? { source: 'table' as const } : {}),
      ...(preferred !== 'auto' ? { preferred_type: preferred } : {}),
      fields: buildAgentFields(opts.fields(), !table)
    }
  }

  /** run 结束：解析、校验，有效的进待确认区，无效的记录原因（不进看板）。 */
  function ingest(content: string) {
    const { blocks, broken } = parseChartBlocksDetailed(content)
    notes.value = stripChartBlocks(content)
    const ok: PendingWidget[] = []
    // 解析阶段就失败的块（坏 JSON / 超长 / 非对象）也要逐项说明，不能因为与好块混在一起就消失。
    const bad: RejectedBlock[] = broken.map((b) => ({ title: '—', reason: b.reason }))
    for (const block of blocks) {
      const r = validateAgentWidget(block, opts.fields(), newWidgetId('ai'), opts.scope().kind !== 'table')
      if (r.ok) ok.push({ widget: r.widget })
      else bad.push({ title: r.title, reason: r.reason })
    }
    pending.value = ok
    rejected.value = bad
    noBlock.value = blocks.length === 0 && broken.length === 0
  }

  async function send(text: string, retry?: { runId: string }): Promise<void> {
    const content = text.trim()
    if (!content && !retry) return
    // 准备阶段还没结束（或上一轮仍在运行）时不接受第二次发送：epoch 只能丢弃旧回调，
    // 撤销不了已经发出的建会话请求，连发会留下多余会话。
    if (sending.value) return
    const mine = ++epoch
    failure.value = ''
    if (content) lastPrompt.value = content
    preparing.value = true
    try {
      const a = await ensureAgent()
      if (mine !== epoch) return
      if (!a) return
      const id = threadId.value || (await ensureThread(a))
      if (mine !== epoch) return
      threadId.value = id
      pending.value = []
      rejected.value = []
      noBlock.value = false
      conv.start(
        {
          content: retry ? '' : content,
          mentions: [{ agent_id: a.id }],
          module: a.module,
          bi_context: buildContext(),
          ...(retry ? { retry_of_run_id: retry.runId } : {})
        },
        {
          onEnd: () => {
            if (mine !== epoch) return
            const reply = [...messages.value].reverse().find((m) => m.role === 'assistant')
            ingest(reply?.content ?? '')
          },
          onError: (e) => {
            if (mine !== epoch) return
            failure.value = agentErrorText(e)
          }
        }
      )
    } catch (e) {
      if (mine !== epoch) return
      // 走错误码映射：只读实例（writer_unavailable）等要给出准确原因，而不是后端英文原话。
      failure.value = agentErrorText(e)
    } finally {
      // 只有当前这次发送才有权关闭准备态；被 reset / 新发送接管后，旧调用不能把它误关。
      if (mine === epoch) preparing.value = false
    }
  }

  /** 重试最近一次失败的 run：重新携带 bi_context（它不落库）。 */
  function retry() {
    const runId = conv.failedRunId.value
    if (!runId) return Promise.resolve()
    // 重试前移除失败气泡，由 start 追加新的 assistant 气泡。
    const list = messages.value
    const last = list[list.length - 1]
    if (last?.role === 'assistant' && last.error) list.pop()
    return send('', { runId })
  }

  /** 新对话：丢掉旧会话 id 与暂存现场，下一次发送重建。 */
  async function newChat() {
    const scope = opts.scope()
    parked.delete(threadKey(scope))
    clearLive()
    try {
      store()?.removeItem(threadKey(scope))
    } catch {
      /* 忽略 */
    }
  }

  function stop() {
    conv.stop(true)
  }

  function dropPending(id: string) {
    pending.value = pending.value.filter((p) => p.widget.id !== id)
  }

  /**
   * 「调整」后保存：只更新待确认项（保持同一个 id，预览会按新配置重新查询），不写看板。
   * 是否加入仍由用户在预览成功后显式确认，调整不能绕过这一步。
   */
  function updatePending(widget: BiWidget) {
    pending.value = pending.value.map((p) => (p.widget.id === widget.id ? { widget } : p))
  }

  /** 预览查询失败后让助手修正：把后端错误文本作为跟进消息发出。 */
  function fixWithAgent(title: string, error: string) {
    return send(t('components.bi.agent.fixPrompt', { title, error }))
  }

  return {
    // 对话状态
    messages, phase: conv.phase, statusText: conv.statusText, sending, failedRunId: conv.failedRunId,
    // 助手
    agent, agentMissing, ensureAgent,
    // 结果
    pending, rejected, notes, noBlock, failure, lastPrompt,
    // 动作
    send, retry, stop, newChat, reset, sync, dropPending, updatePending, fixWithAgent
  }
}
