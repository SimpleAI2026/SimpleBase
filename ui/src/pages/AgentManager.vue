<template>
  <ProjectScope>
    <PageContainer>
      <div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[184px_minmax(0,1fr)] xl:grid-cols-[200px_minmax(0,1fr)]">
        <!-- 左列：工作台导航（planv6.7 v2 §4.1：群列表 + 每群会话 + 群行 +）。 -->
        <AgentWorkspaceSidebar
          :tags="tags"
          :group-pages="groupPages"
          :section="section"
          :thread-id="threadId"
          :running-thread-ids="runningThreadIds"
          :pending-by-group="pendingByGroup"
          :pending="agentsPending"
          :error="agentsError"
          @select-section="selectSection"
          @select-thread="selectThreadInGroup"
          @create-tag="openTagCreate"
          @create-thread="createThreadInGroup"
          @load-more="loadMoreInGroup"
          @rename-thread="startRenameThread"
          @remove-thread="removeThread"
          @edit-tag="openTagEdit"
          @archive-tag="archiveTag"
          @add-members="openMembers"
          @create-member-agent="openMembersCreate"
          @reload="reloadAgents"
        />

        <!-- 右列：对话区（planv6.0 §3.2/§3.8，planv6.7 AGC-NAV 改为分区制）。
             高度 = 视口 - 顶栏 - 上下页面内边距（--page-pad 由 DefaultLayout 施加，
             这里必须引用同一个变量，不能写死数值）。 -->
        <Card class="flex h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))] min-h-[420px] min-w-0 flex-col sm:min-h-[520px]">
          <CardHeader class="border-b">
            <CardTitle class="flex w-full flex-wrap items-center gap-2 sm:gap-3">
              <span class="max-w-40 min-w-0 shrink truncate sm:max-w-56" data-section-title>{{ headerTitle }}</span>
              <Badge
                v-if="activeThreadAgent && threadAgentName"
                variant="outline"
                class="shrink-0 text-xs"
                data-thread-agent-badge
              >{{ threadAgentName }}</Badge>
            </CardTitle>
          </CardHeader>
          <CardContent class="flex min-h-0 min-w-0 flex-1 flex-col gap-2 p-4">
            <!-- 分区无会话：标签总览（助手条 + 提示词 + 最近会话）。 -->
            <AgentTagOverview
              v-if="!threadId"
              :name="sectionDisplayName"
              :agents="sectionAgents"
              :prompts="activeTag?.prompts || []"
              :recent-threads="recentTagThreads"
              :running-agent-ids="runningAgentIds"
              :loading="agentsPending"
              @new-thread="resetThread"
              @mention-agent="mentionAgentFromTag"
              @pick-prompt="pickPrompt"
              @open-thread="selectThread"
            />
            <SbAsyncRegion
              v-else
              class="flex min-h-0 flex-1 flex-col overflow-hidden"
              block="lines"
              :pending="threadPending"
              :show-skeleton="threadShowSkeleton"
              :show-empty="false"
              :show-error="threadShowError"
              :refreshing="threadRefreshing"
              :error="threadError"
              @retry="reloadThread"
            >
              <ConversationView
                :key="`${threadId}:${threadPending}`"
                :messages="chatMessages"
                :sending="sending"
                :confirmation="confirmation"
                :can-retry="canRetry"
                :history-loading="threadPending && !threadHasData"
                :agent-names="agentNamesById"
                @retry="retryLast"
                @confirm="conv.resolveConfirm(true)"
                @deny="conv.resolveConfirm(false)"
              >
                <template #empty>
                  <AgentWelcome
                    :agent-name="activeThreadAgentName"
                    :module="activeThreadAgentModule"
                    mode="thread-empty"
                    @pick="pickPrompt"
                  />
                </template>
              </ConversationView>
            </SbAsyncRegion>
            <!-- v2：路由分工卡与人工待办卡（只读展示 + 待办操作）。 -->
            <div v-if="activeTurnTasks.length" class="flex flex-col gap-2">
              <AgentDispatchCard
                :tasks="activeTurnTasks"
                :names="agentNamesById"
                @retry-task="retryTask"
              />
              <AgentHumanTaskCard
                v-for="task in humanTasksInTurn"
                :key="task.id"
                :task="task"
                :busy="collab.running.value"
                @reply="replyHumanTask"
              />
            </div>
            <div class="flex min-h-4 flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <p v-if="statusText" role="status" class="truncate">{{ statusText }}</p>
              <p v-else class="min-w-0 truncate">{{ toolsHint }}</p>
              <router-link v-if="activeAgentHasSandboxTools" :to="{ name: 'sandboxes' }" class="shrink-0 text-primary hover:underline">{{ t('pages.agents.viewInSandbox') }}</router-link>
            </div>
            <!-- 标签分区底部助手条（planv6.7 AGC-UI §4.3）。 -->
            <AgentRosterBar
              v-if="sectionAgents.length"
              :agents="sectionAgents"
              :running-agent-ids="runningAgentIds"
              @mention-agent="mentionAgentFromTag"
            />
            <AgentComposer
              ref="composer"
              v-model="draft"
              :sending="sending"
              :disabled="!threadId || !!activeCapabilityError"
              :placeholder="composerPlaceholder"
              :mention-agents="mentionAgents"
              :route-hint="routeHint"
              @send="onSend"
              @stop="onStop"
            />
          </CardContent>
        </Card>
      </div>

      <!-- 标签编辑弹窗（planv6.7 AGC-UI §4.4）。 -->
      <AgentTagModal
        :open="tagModalOpen"
        :tag="editingTag"
        :agents="agentOptionsForTag"
        :saving="tagSaving"
        :conflicted="tagConflicted"
        @update:open="(v: boolean) => (tagModalOpen = v)"
        @save="saveTag"
        @reload="reloadEditingTag"
      />

      <!-- 群成员弹窗（planv6.7 v2 §4.3）：加入已有 / 新建助手并加入。 -->
      <AgentGroupMembersModal
        :open="membersOpen"
        :group-name="membersGroupName"
        :agents="mentionAgents"
        :member-ids="membersGroup?.agentIds || []"
        :modules="modules"
        :initial-mode="membersMode"
        :saving="membersSaving"
        :conflicted="membersConflicted"
        @update:open="(v: boolean) => (membersOpen = v)"
        @save="saveMembers"
        @reload="reloadMembersGroup"
      />

      <!-- 会话重命名（planv6.0 §3.6 #7：会话操作收进「⋯」菜单）。 -->
      <SbModal
        :open="renameOpen"
        :title="t('pages.agents.renameThreadTitle')"
        :max-width="440"
        :confirm-loading="renaming"
        :ok-button-props="{ disabled: !renameValue.trim() }"
        @ok="submitRenameThread"
        @update:open="(v: boolean) => (renameOpen = v)"
      >
        <FieldGroup>
          <Field>
            <FieldLabel for="thread-rename">{{ t('pages.agents.renameThreadLabel') }}</FieldLabel>
            <Input
              id="thread-rename"
              v-model="renameValue"
              :placeholder="t('pages.agents.renameThreadPlaceholder')"
              @keydown.enter="submitRenameThread"
            />
          </Field>
        </FieldGroup>
      </SbModal>
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { i18n } from '@/locales'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { api } from '../services/api'
import type { AgentModuleInfo, CloudAgent } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useAuthStore } from '../stores/auth'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import SbModal from '../components/modal/SbModal.vue'
import AgentWelcome from '../components/agent/AgentWelcome.vue'
import AgentTagModal from '../components/modal/AgentTagModal.vue'
import AgentGroupMembersModal from '../components/modal/AgentGroupMembersModal.vue'
import AgentWorkspaceSidebar from '../components/agent/AgentWorkspaceSidebar.vue'
import type { SidebarGroupPage } from '../components/agent/AgentWorkspaceSidebar.vue'
import AgentTagOverview from '../components/agent/AgentTagOverview.vue'
import AgentRosterBar from '../components/agent/AgentRosterBar.vue'
import ConversationView from '../components/agent/ConversationView.vue'
import AgentDispatchCard from '../components/agent/AgentDispatchCard.vue'
import AgentHumanTaskCard from '../components/agent/AgentHumanTaskCard.vue'
import AgentComposer from '../components/agent/AgentComposer.vue'
import type { MentionAgent, MentionSendTarget } from '../components/ai/AiChatComposer.vue'
import { useAgentConversation } from '../composables/useAgentConversation'
import { useAgentWorkspace } from '../composables/useAgentWorkspace'
import { useAgentCollaboration } from '../composables/useAgentCollaboration'
import { findStarterTemplate } from '../constants/starter-prompts'
import { markStarterSent, starterAlreadySent } from '../utils/starter-storage'
import { useLoadState } from '@/composables/useLoadState'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import type { ActiveAgentRun, AgentGroupMembersInput, AgentHumanTask, AgentMentionTarget, AgentMessage, AgentTask, AgentTag, AgentTagInput, AgentThread, AgentThreadCreateOptions, ChatMsg, MentionAgentLike } from '../services/types'

const project = useProjectStore()
const route = useRoute()
const { t } = useI18n()
const router = useRouter()
const {
  pending: agentsPending,
  error: agentsError,
  run: runAgents
} = useLoadState({ fallback: 'pages.agents.loadAgentsFailed' })
const {
  pending: threadPending,
  hasData: threadHasData,
  showSkeleton: threadShowSkeleton,
  showError: threadShowError,
  refreshing: threadRefreshing,
  error: threadError,
  run: runThread,
  settle: settleThread
} = useLoadState({ fallback: 'pages.agents.loadThreadFailed' })
const agents = ref<CloudAgent[]>([])
const modules = ref<AgentModuleInfo[]>([])
const defaultModel = ref('')
const modelOptions = ref<string[]>([])
const chatMessages = ref<ChatMsg[]>([])
const draft = ref('')

/* ---------- 工作台分区状态（planv6.7 AGC-NAV） ---------- */
/** 全部标签（含预置 general/alerts/stats/scheduled 与自定义）。 */
const tags = ref<AgentTag[]>([])
/** 当前标签 ID，默认是包含全部助手的 #通用。 */
const section = ref('general')
/** 当前标签内的会话列表。 */
const sectionThreads = ref<AgentThread[]>([])
/** 当前标签的分页游标（空 = 没有下一页）。 */
const sectionCursor = ref('')
/** 当前标签会话是否在加载（侧栏据此显示 loading，不再用全局 pending）。 */
const threadsPending = ref(false)
/** 我的待办（跨会话聚合，用于群行角标与人工任务卡）。 */
const humanTasks = ref<AgentHumanTask[]>([])

/** 其他群的分页缓存与展开状态（当前分区仍由 sectionThreads 驱动）。 */
const ws = useAgentWorkspace({ projectId: () => project.id })
/** 登录态：只有它有「人」的身份（人工待办需要）。 */
const authStore = useAuthStore()
/** 当前会话（分区制后由选中线程直接持有）。 */
const threadId = ref('')
/** 标签弹窗状态。 */
const tagModalOpen = ref(false)
const editingTag = ref<AgentTag | null>(null)
const tagSaving = ref(false)
const tagConflicted = ref(false)

/* ---------- 群成员弹窗（planv6.7 v2 §4.3） ---------- */
const membersOpen = ref(false)
const membersGroupId = ref('')
const membersMode = ref<'existing' | 'create'>('existing')
const membersSaving = ref(false)
const membersConflicted = ref(false)

const composer = ref<{
  focus?: () => void
  insertMention?: (a: MentionAgentLike) => void
  clearMentions?: () => void
} | null>(null)
/** 会话重命名弹窗（原 ThreadSwitcher 无重命名入口，这里补上）。 */
const renameOpen = ref(false)
const renameValue = ref('')
const renaming = ref(false)
const renameTarget = ref<AgentThread | null>(null)

/** 会话状态层：运行续接与确认事件流。 */
const conv = useAgentConversation({
  projectId: () => project.id,
  threadId: () => threadId.value,
  messages: () => chatMessages.value
})

/**
 * 协作轮次层（planv6.7 v2 §6）：群会话走 /turns，一轮多任务。
 * 与 conv 并存：按会话类型二选一，互不干扰。
 */
const collab = useAgentCollaboration({
  projectId: () => project.id,
  messages: () => chatMessages.value,
  onTurnEnd: () => {
    refreshThreadsForAgent()
    void loadHumanTasks()
  },
  onError: (err) => {
    const e = err as { code?: string }
    const code = e?.code
    if (code === 'agent_not_in_tag' || code === 'group_member_unavailable') {
      toast.warning(t('components.agentWorkspace.agentNotInTag'))
      return
    }
    if (code === 'group_archived') {
      toast.warning(t('components.agentWorkspace.groupArchived'))
      return
    }
    if (code === 'turn_busy') {
      toast.warning(t('components.agentWorkspace.turnBusy'))
      return
    }
    if (code === 'human_assignee_required') {
      toast.warning(t('components.agentWorkspace.humanRequired'))
      return
    }
    toast.error(errorMessage(e, t('pages.agents.runFailed')))
  }
})
/** 发送中：单助手 run 或协作轮次正在执行。 */
const sending = computed(() => conv.sending.value || collab.running.value)
const confirmation = conv.confirmation
const statusText = conv.statusText
const failedRunId = conv.failedRunId

/**
 * 「正在回复」指示（助手列表头像进度环 + 会话切换器转圈）。
 * 服务端 /agent-runs/active 覆盖后台续跑与定时任务；当前会话的 sending 本地立即生效。
 * 有运行中的 run 时每 3s 轮询，全部结束即停；页面隐藏时暂停。
 */
const activeRuns = ref<ActiveAgentRun[]>([])
let activePollTimer: ReturnType<typeof setTimeout> | null = null
let activeDisposed = false
const ACTIVE_POLL_MS = 3000

const runningAgentIds = computed(() => {
  const ids = new Set(activeRuns.value.map((r) => r.agent_id))
  for (const m of chatMessages.value) if (m.agentId && m.routing) ids.add(m.agentId)
  return ids
})
const runningThreadIds = computed(() => {
  const ids = new Set(activeRuns.value.map((r) => r.thread_id))
  if (sending.value && threadId.value) ids.add(threadId.value)
  return ids
})

function clearActivePoll() {
  if (activePollTimer) clearTimeout(activePollTimer)
  activePollTimer = null
}

async function refreshActiveRuns() {
  clearActivePoll()
  const projectId = project.id
  if (!projectId || activeDisposed) return
  try {
    const list = await api.agentThreads.active(projectId)
    if (activeDisposed || project.id !== projectId) return
    activeRuns.value = Array.isArray(list) ? list : []
  } catch {
    // 指示性数据：失败静默，保留上一次结果，下轮再试。
  }
  // 待办角标与运行指示同一轮轮询：都只读、都失败静默。
  void loadHumanTasks()
  if (activeDisposed || project.id !== projectId) return
  if (activeRuns.value.length || sending.value) {
    activePollTimer = setTimeout(() => {
      activePollTimer = null
      // 页面隐藏时暂停，回到前台由 visibilitychange 续上。
      if (typeof document !== 'undefined' && document.hidden) return
      void refreshActiveRuns()
    }, ACTIVE_POLL_MS)
  }
}

function onVisibilityChange() {
  if (!document.hidden && !activePollTimer) void refreshActiveRuns()
}
if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibilityChange)

// 本地 run 开始 / 结束都刷新一次：开始时启动轮询，结束时及时去掉转圈。
watch(sending, () => { void refreshActiveRuns() })

const builtinDefaultNames: Record<string, string> = {
  general: '通用助手', database: 'Database', s3: 'S3', logs: 'Logs',
  kv: '键值助手', gofunction: '云函数助手', cron: '定时任务助手', sandbox: '沙盒助手'
}
/** 按当前语言解析内置助手显示名（planv6.7 AGC-NAV 改分区制后仍有多处引用）。 */
const displayedAgents = computed(() => {
  const locale = i18n.global.locale.value
  return agents.value.map((agent) => {
    const key = agent.builtin_key || ''
    if (agent.name !== builtinDefaultNames[key]) return agent
    return { ...agent, name: i18n.global.t(`pages.agents.builtinNames.${key}`, 1, { locale }) }
  })
})
/** agentId → 当前语言显示名（ConversationView 回答者行）。 */
const agentNamesById = computed(() => {
  const map: Record<string, string> = {}
  for (const a of displayedAgents.value) map[a.id] = a.name
  return map
})
const activeAgent = computed(() => displayedAgents.value.find((a) => a.id === activeThreadAgentId.value))
/** 当前会话固定助手（分区制：线程绑定，不再由左栏选择）。 */
const activeThreadAgentId = computed(() => sectionThreads.value.find((th) => th.id === threadId.value)?.agent_id || '')
const activeThreadAgent = computed(() => agents.value.find((a) => a.id === activeThreadAgentId.value))
const activeThreadAgentName = computed(() => activeThreadAgent.value ? displayedAgents.value.find((a) => a.id === activeThreadAgentId.value)?.name : '')
const activeThreadAgentModule = computed(() => activeThreadAgent.value?.module)
const threadAgentName = activeThreadAgentName
/**
 * @ 候选（planv6.7 v2 §4.3）：
 *   - 群分区只列本群成员，避免「点名群外助手 → 服务端拒绝」；
 *   - 登录态额外提供「当前人」，用于生成人工待办。
 */
const mentionAgents = computed<MentionAgent[]>(() => {
  const memberIds = new Set(sectionAgents.value.map((agent) => agent.id))
  const base = displayedAgents.value.filter((agent) => memberIds.has(agent.id))
  const list: MentionAgent[] = base.map((a) => ({
    id: a.id,
    name: a.name,
    module: a.module,
    legacyName: a.builtin_key ? agents.value.find((raw) => raw.id === a.id)?.name : undefined,
    builtinKey: a.builtin_key && agents.value.find((raw) => raw.id === a.id)?.name === builtinDefaultNames[a.builtin_key] ? a.builtin_key : undefined
  }))
  // 只有登录态才有「人」：API Key 主体不能担任人工接收人。
  const user = authStore.user
  if (user && activeTag.value?.humanPolicy !== 'disabled') {
    list.push({ id: 'me', name: t('components.agentWorkspace.mentionMe'), kind: 'human' })
  }
  return list
})
const composerPlaceholder = computed(() =>
  activeAgent.value
    ? t('pages.agents.composerWithAgent', { name: activeAgent.value.name })
    : t('pages.agents.composerIdle')
)

/* ---------- 分区导航计算（planv6.7 AGC-NAV §4.1） ---------- */

const activeTag = computed(() => tags.value.find((tag) => tag.id === section.value) || null)
/** 当前标签的本地化显示名。 */
const sectionDisplayName = computed(() => activeTag.value ? tagDisplayName(activeTag.value) : '')
const headerTitle = computed(() => sectionDisplayName.value || t('pages.agents.dialogLabel'))
/** 标签内可用助手（已过滤不存在/归档助手）。 */
const sectionAgents = computed(() => {
  const tag = activeTag.value
  if (!tag) return [] as MentionAgentLike[]
  const byId = new Map(displayedAgents.value.map((a) => [a.id, a]))
  return tag.agentIds
    .map((id) => byId.get(id))
    .filter((a): a is (typeof displayedAgents.value)[number] => !!a)
    .map((a) => ({ id: a.id, name: a.name, module: a.module }))
})
/** 标签最近会话（总览下半屏）。 */
const recentTagThreads = computed(() => sectionThreads.value.slice(0, 5))
/** 标签编辑弹窗的助手候选（含已不可用项，便于编辑器提示）。 */
const agentOptionsForTag = computed(() => {
  const tag = editingTag.value
  const used = new Set(tag?.agentIds || [])
  const out: MentionAgentLike[] = displayedAgents.value.map((a) => ({ id: a.id, name: a.name, module: a.module }))
  for (const id of tag?.unavailableAgentIds || []) {
    if (!used.has(id)) continue
    out.push({ id, name: t('components.agentTagOverview.unavailable') })
  }
  return out
})
/** composer routeHint：auto 线程显示「自动选择助手」，点名后显示「本轮：@X」（§4.3）。 */
const routeHint = computed(() => {
  const th = sectionThreads.value.find((item) => item.id === threadId.value)
  if (!th || th.dispatch_mode !== 'auto') return ''
  if (routeAgentName.value) return `@${routeAgentName.value} · ` + t('components.agentWorkspace.mentionThisTurn', { name: routeAgentName.value })
  return t('components.agentWorkspace.autoRoute')
})
/** route.done 记录的助手名（一轮结束后清空）。 */
const routeAgentName = ref('')
/** 标签显示名（预置走 i18n）。 */
function tagDisplayName(tag: AgentTag): string {
  return tag.builtinKey ? t(`components.agentWorkspace.builtinTags.${tag.builtinKey}`) : tag.name
}
/* ---------- 侧栏群树（planv6.7 v2 §4.1） ---------- */

/**
 * 每个群的会话分页。
 *
 * 当前分区直接复用 sectionThreads/sectionCursor（既有状态机不变）；
 * 其他群用 workspace 的独立分页缓存，展开时懒加载，互不干扰。
 */
const groupPages = computed<Record<string, SidebarGroupPage>>(() => {
  const out: Record<string, SidebarGroupPage> = {}
  for (const tag of tags.value) {
    const cached = ws.pages[tag.id]
    out[tag.id] = {
      threads: tag.id === section.value ? sectionThreads.value : cached?.threads ?? [],
      hasMore: tag.id === section.value ? Boolean(sectionCursor.value) : Boolean(cached?.hasMore),
      loading: tag.id === section.value ? threadsPending.value : Boolean(cached?.loading)
    }
  }
  return out
})

/** 每个群的待办数（群行角标）：按会话所属群聚合。 */
const pendingByGroup = computed<Record<string, number>>(() => {
  const threadToGroup: Record<string, string> = {}
  for (const [groupId, page] of Object.entries(groupPages.value)) {
    for (const th of page.threads) threadToGroup[th.id] = groupId
  }
  const out: Record<string, number> = {}
  for (const task of humanTasks.value) {
    const groupId = task.tagId || threadToGroup[task.threadId] || 'general'
    out[groupId] = (out[groupId] ?? 0) + 1
  }
  return out
})

/** 当前轮次的任务（路由卡渲染源）：只在协作会话展示。 */
const activeTurnTasks = computed<AgentTask[]>(() => {
  if (!isCollaborativeThread.value) return []
  return collab.tasks.value
})

/** 当前轮次里等人的任务（人工卡渲染源，保持任务顺序）。 */
const humanTasksInTurn = computed<AgentTask[]>(() =>
  activeTurnTasks.value.filter((task) => task.targetKind === 'human')
)

/**
 * 当前会话是否为协作会话（群会话）。
 *
 * 服务端已把旧 auto 读成 collaborative；这里再做一次兜底判断，
 * 保证「有 tag_id 但还没有协作行」的旧数据也能走协作路径。
 */
const isCollaborativeThread = computed(() => {
  const thread = sectionThreads.value.find((item) => item.id === threadId.value)
  if (!thread) return true
  return thread.dispatch_mode === 'collaborative' || thread.dispatch_mode === 'auto'
})

/** 侧栏新建会话：按钮带群 ID，先切分区再建。 */
async function createThreadInGroup(id: string) {
  if (id !== section.value) await selectSection(id)
  await resetThread()
}

/** 侧栏加载更多：按群走各自游标。 */
async function loadMoreInGroup(id: string) {
  if (id === section.value) {
    await loadMoreSectionThreads()
    return
  }
  await ws.loadMore(id)
}

/** 侧栏点会话：跨群时先切分区，再选中。 */
async function selectThreadInGroup(id: string, groupId: string) {
  if (groupId && groupId !== section.value) await selectSection(groupId)
  await selectThread(id)
}

// 当前选中 Agent 是否含沙盒工具；决定 composer 旁的提示文案。
const activeAgentHasSandboxTools = computed(() => {
  const a = activeThreadAgent.value
  if (!a) return false
  return (a.tool_ids || []).some((id) => id.startsWith('sandbox_'))
})

const activeCapabilityError = computed(() => {
  const agent = activeThreadAgent.value
  if (!agent) return ''
  if (agent.module === 'sandbox' && modules.value.find((m) => m.id === 'sandbox')?.sandbox_available !== true) {
    return t('pages.agents.sandboxUnavailable')
  }
  if (['kv', 'gofunction', 'cron'].includes(agent.module) && !modules.value.some((m) => m.id === agent.module)) {
    return t('pages.agents.cliSkillsUnavailable')
  }
  return ''
})

const toolsHint = computed(() => {
  if (activeCapabilityError.value) return activeCapabilityError.value
  const who = activeThreadAgent.value
    ? t('pages.agents.mentionHint', { name: activeThreadAgentName.value })
    : t('pages.agents.mentionHintGeneric')
  if (activeAgentHasSandboxTools.value) return t('pages.agents.mentionHintSandbox', { who })
  return t('pages.agents.mentionHintWritable', { who })
})
/** 最后一条消息失败时可重试：复用最近一次失败的 run。 */
const canRetry = computed(() => {
  if (sending.value || !failedRunId.value) return false
  const last = chatMessages.value[chatMessages.value.length - 1]
  return last?.role === 'assistant' && !!last.error
})

watch(
  () => project.id,
  () => {
    detachRun()
    section.value = 'general'
    threadId.value = ''
    sectionThreads.value = []
    sectionCursor.value = ''
    tags.value = []
    chatMessages.value = []
    activeRuns.value = []
    void bootstrap()
  }
)

async function loadModelOptions() {
  try {
    // BUG-07：优先用 /agents/models 提供模型候选。
    const res = await api.agents.models(project.id)
    defaultModel.value = res.default_model || ''
    modelOptions.value = res.models.map((m) => m.name)
    if (!modelOptions.value.length && defaultModel.value) modelOptions.value = [defaultModel.value]
  } catch (e) {
    defaultModel.value = ''
    modelOptions.value = []
    toast.error(errorMessage(e, t('pages.agents.loadModelsFailed')))
  }
}

async function bootstrap() {
  // 首次进入且尚未选定项目时 project.id 为空：此时请求会打到 /v1/projects//… 并弹出多条 404 提示；
  // 等 loadProjects 选中项目后由上面的 watch 再触发。
  if (!project.id) return
  // loadThreads 依赖 tags（决定分区），所以两者必须串行；
  // 模型候选与运行态与它们无关，可以并行。
  await Promise.all([loadModelOptions(), refreshActiveRuns(), (async () => {
    await loadAgents()
    await loadTags()
    await loadSectionThreads()
  })()])
  await maybeStartStarter()
}

/* ---------- 首页「一句话建应用」搭建会话（planv6.6 home-starter-prompts §2.3 / §2.7） ---------- */

const starterSite = ref<{ baseUrl?: string; publishable: boolean; reason?: string } | null>(null)
function buildStarterSeed(templateId: string): string {
  const tpl = findStarterTemplate(templateId)
  if (!tpl) return ''
  const site = starterSite.value
  return t(tpl.promptKey, {
    projectId: project.id,
    siteBaseUrl: site?.baseUrl || '-',
    publishable: site?.publishable ? 'true' : `false${site?.reason ? ` (${site.reason})` : ''}`
  })
}

/** 合并更新 query（值为 undefined 表示删除该键）；排队串行，始终基于上一次替换后的结果。 */
let queryChain: Promise<unknown> = Promise.resolve()
function replaceQuery(patch: Record<string, string | undefined>) {
  if (!router?.replace) return
  queryChain = queryChain.then(() => {
    const q: Record<string, unknown> = { ...router.currentRoute.value.query }
    for (const [k, v] of Object.entries(patch)) {
      if (v === undefined) delete q[k]
      else q[k] = v
    }
    return router.replace({ query: q as never })
  }).catch(() => undefined)
}

/**
 * 首页来的 `?starter=<id>`：在 #通用 新建协作会话并自动发送一次种子提示词。
 *
 * 这是对 planv6.0 §3.5「点击只填不发」的显式例外，仅限 starter 入口：用户已经在确认弹窗里点了
 * 「开始搭建」，等价于回车。发送前先去重（sessionStorage）并立即去掉 query，刷新不会重复发。
 */
async function maybeStartStarter() {
  const raw = route.query.starter
  const templateId = typeof raw === 'string' ? raw : ''
  if (!templateId) return
  const consumeQuery = () => replaceQuery({ starter: undefined })
  const tpl = findStarterTemplate(templateId)
  const projectId = project.id
  if (!tpl || !projectId) { consumeQuery(); return }
  if (starterAlreadySent(projectId, templateId)) { consumeQuery(); return }
  if (section.value !== 'general') {
    section.value = 'general'
    threadId.value = ''
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
    if (project.id !== projectId) return
    id = th.id
    sectionThreads.value.unshift(th)
    threadId.value = id
    chatMessages.value = []
    settleThread(false)
    replaceQuery({ thread: id })
  } catch (e) {
    toast.error(errorMessage(e, t('components.starter.sessionFailed')))
    return
  }
  const site = await api.s3.getSite(projectId).catch(() => null)
  if (project.id !== projectId || threadId.value !== id) return
  starterSite.value = site
  collab.start(id, buildStarterSeed(tpl.id), [], 'chat')
}

/** 侧栏「重命名」入口（分区制后线程 id 直接传入）。 */
function startRenameThread(id: string) {
  const th = sectionThreads.value.find((item) => item.id === id)
  if (!th) return
  renameTarget.value = th
  renameValue.value = th.title
  renameOpen.value = true
}

/** 会话重命名弹窗出现问题时不留半开状态。 */
async function submitRenameThread() {
  const target = renameTarget.value
  const next = renameValue.value.trim()
  if (!target || !next || next === target.title) {
    renameOpen.value = false
    return
  }
  renaming.value = true
  try {
    await renameThread(target.id, next)
    renameOpen.value = false
  } finally {
    renaming.value = false
  }
}

/**
 * 点提示词：写入输入框并聚焦，**不自动发送**（planv6.0 §3.5）。
 * 与「少操作」一致——一次点击把话术准备好，回车才执行。
 */
function pickPrompt(text: string) {
  if (draft.value.trim() && !window.confirm(t('components.agentWorkspace.replaceDraftConfirm'))) return
  draft.value = text
  composer.value?.focus?.()
}

/** 助手条点名：草稿插入 @名称（planv6.7 §4.3，不发送不切换）。 */
function mentionAgentFromTag(a: MentionAgentLike) {
  composer.value?.insertMention?.({ id: a.id, name: a.name, module: a.module })
}

/** BUG-06：持久化消息 → 聊天气泡。run 状态还原失败/取消态；assistant 按其归属助手补 module 与路由结论。 */
function toChatMessage(m: AgentMessage, agentModule?: string): ChatMsg {
  return {
    role: m.role === 'assistant' ? 'assistant' : 'user',
    content: m.content,
    module: m.role === 'assistant' ? agentModule : undefined,
    agentId: m.role === 'assistant' ? m.agent_id : undefined,
    routeKind: m.route_kind,
    toolCalls: m.tool_calls,
    error: m.error_code ? errorMessage({ code: m.error_code } as Error & { code?: string }, t('pages.agents.runFailed')) : undefined,
    canceled: m.run_status === 'canceled'
  }
}

async function loadThreadMessages(id: string): Promise<boolean> {
  const msgs = await api.agentThreads.messages(project.id, id)
  if (threadId.value !== id) return false
  chatMessages.value = msgs.map((m) => toChatMessage(m, agents.value.find((a) => a.id === m.agent_id)?.module))
  void resumeActiveRun(id)
  return chatMessages.value.length > 0
}

/**
 * 回到会话时续接后台仍在运行的 run：切换界面只断开 SSE，不取消 run，
 * 这里查最新一次 run，仍在运行就回放事件并继续跟随。
 */
async function resumeActiveRun(id: string) {
  if (sending.value) return
  const runs = await api.agentThreads.runs(project.id, id, 1).catch(() => [])
  const run = runs?.[0]
  if (threadId.value !== id || sending.value) return
  if (!run || (run.status !== "running" && run.status !== "queued")) return
  conv.attach(run.id, {
    module: agents.value.find((a) => a.id === run.agent_id)?.module,
    onEnd: () => refreshThreadsForAgent(),
    // 事件缓存已过期等：重新拉一次已落库的消息。
    onError: () => {
      void api.agentThreads.messages(project.id, id).then((list) => {
        if (threadId.value !== id || sending.value) return
        chatMessages.value = list.map((m) => toChatMessage(m, agents.value.find((a) => a.id === m.agent_id)?.module))
      }).catch(() => undefined)
    }
  })
}

async function selectThread(id: string) {
  if (!id) return
  // 深链/总览的线程可能不在当前分区列表：先补齐再选（404 → 会话不存在提示）。
  if (!sectionThreads.value.some((thread) => thread.id === id)) {
    const th = await Promise.resolve(api.agentThreads.get(project.id, id)).then((r) => r).catch(() => null)
    if (!th) {
      toast.error(t('components.agentWorkspace.threadNotFound'))
      return
    }
    if (!th.tag_id) {
      toast.error(t('components.agentWorkspace.threadNotFound'))
      return
    }
    if (th.tag_id !== section.value) await selectSection(th.tag_id)
    if (!sectionThreads.value.some((item) => item.id === id)) sectionThreads.value.unshift(th)
  }
  detachRun()
  routeAgentName.value = ''
  threadId.value = id
  replaceQuery({ thread: id })
  await runThread(async () => loadThreadMessages(id), { replace: true })
}

function reloadThread() {
  if (threadId.value) void selectThread(threadId.value)
  else void loadSectionThreads()
}

/** 切换标签：重置会话列表并按 query 或首条恢复选中。 */
async function selectSection(id: string) {
  if (!id || id === section.value) return
  detachRun()
  section.value = id
  threadId.value = ''
  replaceQuery({ thread: undefined, tag: id === 'general' ? undefined : id })
  await loadSectionThreads()
}

/* ---------- 分区会话列表（planv6.7 AGC-NAV §8.7） ---------- */

/** 拉当前分区的会话列表（首页 50 条 + 游标）。 */
async function loadSectionThreads() {
  const projectId = project.id
  const sec = section.value
  threadsPending.value = true
  await runThread(async () => {
    const page = await api.agentThreads.pageBy(projectId, { tagId: sec, limit: 50 })
    // 项目或分区可能在取数期间切换；旧响应不得覆盖当前列表。
    if (project.id !== projectId || section.value !== sec) return false
    sectionThreads.value = Array.isArray(page?.threads) ? page.threads : []
    sectionCursor.value = page.next_cursor
    const queryId = typeof route.query.thread === 'string' ? route.query.thread : ''
    const selected = queryId && page.threads.some((th) => th.id === queryId)
      ? queryId
      : page.threads[0]?.id || ''
    if (selected) {
      threadId.value = selected
      replaceQuery({ thread: selected })
      return loadThreadMessages(selected)
    }
    // 分区无会话：展示总览空态，不自动建会话（planv6.7 §8.7 AGC-NAV-5）。
    threadId.value = ''
    return false
  }, { replace: true })
  threadsPending.value = false
}

/**
 * 拉「我的待办」（planv6.7 v2 §7.4）。
 *
 * 只读列表，不产生副作用；失败静默（角标不该阻断主流程）。
 * 与 /agent-runs/active 的轮询合并成一轮，避免开两条定时器。
 */
async function loadHumanTasks() {
  if (!project.id) return
  try {
    const res = await api.agentTurns.humanTasks(project.id, 'me', 'pending')
    humanTasks.value = res.tasks
  } catch {
    // 未登录态（API Key）或未升级环境：不显示角标。
    humanTasks.value = []
  }
}

/** 当前标签加载下一页（侧栏「加载更多」）。 */
async function loadMoreSectionThreads() {
  if (!sectionCursor.value) return
  const projectId = project.id
  const sec = section.value
  const cursor = sectionCursor.value
  try {
    const page = await api.agentThreads.pageBy(projectId, { tagId: sec, limit: 50, cursor })
    if (project.id !== projectId || section.value !== sec) return
    const fresh = (page?.threads || []).filter((th) => !sectionThreads.value.some((old) => old.id === th.id))
    sectionThreads.value = [...sectionThreads.value, ...fresh]
    sectionCursor.value = page?.next_cursor || ''
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed')))
  }
}

async function renameThread(id: string, title: string) {
  try {
    const renamed = await api.agentThreads.rename(project.id, id, title)
    sectionThreads.value = sectionThreads.value.map((th) => th.id === id ? renamed : th)
  } catch (e) { toast.error(errorMessage(e, t('pages.agents.renameFailed'))) }
}

async function removeThread(id: string) {
  onStop()
  try {
    await api.agentThreads.remove(project.id, id)
    sectionThreads.value = sectionThreads.value.filter((th) => th.id !== id)
    if (threadId.value === id) {
      threadId.value = ''
      // 分区还有会话就切首条；否则回到总览空态（不再自动建会话）。
      if (sectionThreads.value.length) await selectThread(sectionThreads.value[0].id)
    }
  } catch (e) { toast.error(errorMessage(e, t('pages.agents.deleteThreadFailed'))) }
}

async function loadModules() {
  try {
    modules.value = await api.agents.modules(project.id)
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.loadModulesFailed')))
  }
}

async function loadAgents() {
  const projectId = project.id
  await runAgents(async () => {
    await loadModules()
    const list = await api.agents.list(projectId)
    if (project.id !== projectId) return false
    agents.value = list
    return agents.value.length > 0
  })
}

async function reloadAgents() {
  await Promise.all([loadAgents(), loadTags()])
}

/** 拉工作台标签（planv6.7 AGC-NAV）：预置 + 自定义。 */
async function loadTags() {
  try {
    const list = await api.agentTags.list(project.id)
    tags.value = Array.isArray(list) ? list : []
  } catch (e) {
    tags.value = []
    toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed')))
  }
  await prefetchGroupThreads()
}

/**
 * 预取各群的首屏会话（侧栏不折叠，每个群都要直接列出会话）。
 *
 * 当前分区由 sectionThreads 驱动（有更完整的游标与选中态），跳过它避免重复请求；
 * 其余群走 workspace 的独立分页缓存，加载更多仍按群各自进行。
 */
async function prefetchGroupThreads() {
  const current = section.value
  const others = tags.value.map((tag) => tag.id).filter((id) => id !== current)
  if (!others.length) return
  await ws.prefetchAll(others)
}

/**
 * 新建会话（planv6.7 v2 §4.2）：
 *
 * 所有标签都建协作会话，成员与路由规则由标签配置决定；
 * 会话类型缺省按标签推断（告警 → alert、统计 → statistics、定时 → scheduled）。
 */
async function resetThread() {
  detachRun()
  const projectId = project.id
  const sec = section.value
  if (!tags.value.some((item) => item.id === sec)) return
  const opts: AgentThreadCreateOptions = {
    title: t('nav.agents'),
    tagId: sec,
    dispatchMode: 'collaborative',
    source: 'workspace'
  }
  try {
    const th = await api.agentThreads.create(projectId, opts)
    if (project.id !== projectId || section.value !== sec) return
    if (!sectionThreads.value.some((item) => item.id === th.id)) sectionThreads.value.unshift(th)
    threadId.value = th.id
    chatMessages.value = []
    settleThread(false)
    replaceQuery({ thread: th.id })
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.createThreadFailed')))
  }
}

/** run 结束后刷新当前分区列表（标题/预览可能变化）。 */
function refreshThreadsForAgent() {
  const projectId = project.id
  const sec = section.value
  void Promise.resolve(api.agentThreads.pageBy(projectId, { tagId: sec, limit: 50 })).then((page) => {
    if (project.id !== projectId || section.value !== sec) return
    const fresh = Array.isArray(page?.threads) ? page.threads : []
    // 保留本地新建、尚未被服务端分页收录的线程（如 auto 会话首条）。
    sectionThreads.value = fresh.concat(sectionThreads.value.filter((th) => !fresh.some((p) => p.id === th.id)))
    sectionCursor.value = page?.next_cursor || ''
  }).catch(() => undefined)
}

/* ---------- 标签 CRUD（planv6.7 AGC-UI §4.4） ---------- */

function openTagCreate() {
  tagConflicted.value = false
  editingTag.value = null
  tagModalOpen.value = true
}

function openTagEdit(id: string) {
  const tag = tags.value.find((item) => item.id === id)
  if (!tag) return
  tagConflicted.value = false
  editingTag.value = tag
  tagModalOpen.value = true
}

async function saveTag(input: AgentTagInput) {
  const projectId = project.id
  const editing = editingTag.value
  tagSaving.value = true
  try {
    if (editing) await api.agentTags.patch(projectId, editing.id, input)
    else await api.agentTags.create(projectId, input)
    tagModalOpen.value = false
    tagConflicted.value = false
    await loadTags()
  } catch (e) {
    if ((e as { status?: number })?.status === 409) {
      tagConflicted.value = true
    } else {
      toast.error(errorMessage(e, t('pages.agents.saveFailed')))
    }
  } finally {
    tagSaving.value = false
  }
}

/** 版本冲突后「重新加载」：丢弃本地修改，用服务端版本回填。 */
async function reloadEditingTag() {
  const editing = editingTag.value
  if (!editing) return
  try {
    const list = await api.agentTags.list(project.id)
    tags.value = Array.isArray(list) ? list : []
    editingTag.value = list.find((item) => item.id === editing.id) || null
    tagConflicted.value = false
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed')))
  }
}

async function archiveTag(id: string) {
  try {
    await api.agentTags.remove(project.id, id)
    await loadTags()
    if (section.value === id) await selectSection('general')
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.deleteFailed')))
  }
}

/* ---------- 群成员（planv6.7 v2 §4.3） ---------- */

/** 打开成员弹窗（默认「加入已有助手」）。 */
function openMembers(groupId: string) {
  membersGroupId.value = groupId
  membersMode.value = 'existing'
  membersConflicted.value = false
  membersOpen.value = true
}

/** 从群行「新建助手并加入」打开：直接落在新建页签。 */
function openMembersCreate(groupId: string) {
  membersGroupId.value = groupId
  membersMode.value = 'create'
  membersConflicted.value = false
  membersOpen.value = true
}

/** 当前成员弹窗对应的群（渲染标题与成员置灰用）。 */
const membersGroup = computed(() => tags.value.find((tag) => tag.id === membersGroupId.value) || null)
const membersGroupName = computed(() => (membersGroup.value ? tagDisplayName(membersGroup.value) : ''))

/** 提交成员变更：加入已有 或 新建并加入（服务端组合事务）。 */
async function saveMembers(input: AgentGroupMembersInput) {
  const groupId = membersGroupId.value
  if (!groupId) return
  membersSaving.value = true
  try {
    const updated = await api.agentTags.addMembers(project.id, groupId, {
      ...input,
      expectedVersion: membersGroup.value?.version
    })
    membersOpen.value = false
    membersConflicted.value = false
    await loadTags()
    // 新建助手的场景下助手列表也变了：刷新一次，否则 @ 候选里没有它。
    if (input.newAgent) await loadAgents()
    // 群变更可能影响候选：刷新当前会话的成员提示。
    if (section.value === groupId) await refreshGroupMembers()
    void updated
  } catch (e) {
    if ((e as { status?: number })?.status === 409) {
      membersConflicted.value = true
    } else {
      toast.error(errorMessage(e, t('pages.agents.saveFailed')))
    }
  } finally {
    membersSaving.value = false
  }
}

/** 版本冲突后重新加载群（丢弃本地选择）。 */
async function reloadMembersGroup() {
  await loadTags()
  membersConflicted.value = false
}

/** 群成员变化后刷新当前分区助手（候选与身份名都依赖群配置）。 */
async function refreshGroupMembers() {
  await loadTags()
}

/**
 * 发送（planv6.7 v2 §3.3 / §7.1）。
 *
 * v2 的核心变化：一轮可以 @ 多位助手，全部执行（按依赖串行），不再是「多选只跑一个」。
 * 显式 @ 的目标交给服务端校验成员资格；无 @ 时由群路由提示词决定分工。
 */
async function onSend(text: string, mentions: MentionSendTarget[]) {
  const content = text.trim()
  if (!content || sending.value) return
  // 去重：同一助手被 @ 多次只算一次（服务端也会去重，这里提前收敛避免脏请求）。
  const agentIds: string[] = []
  // 人目标：composer 用 agent_id='me' + kind='human' 表达「@当前人」。
  let wantsHuman = false
  for (const m of mentions) {
    if (m.kind === 'human') {
      wantsHuman = true
      continue
    }
    const id = m.agent_id?.trim()
    if (id && !agentIds.includes(id)) agentIds.push(id)
  }
  if (wantsHuman && !authStore.user) {
    toast.warning(t('components.agentWorkspace.humanRequired'))
    return
  }
  const mentionedAgents = agentIds
    .map((id) => agents.value.find((a) => a.id === id))
    .filter((a): a is CloudAgent => !!a)
  if (agentIds.length && mentionedAgents.length !== agentIds.length) {
    toast.warning(t('pages.agents.selectAgentFirst'))
    return
  }
  // 群分区点名了群外助手：提示后保留草稿（服务端也会拒绝，这里提前说清楚）。
  if (mentionedAgents.some((a) => !sectionAgents.value.some((m) => m.id === a.id))) {
    toast.warning(t('components.agentWorkspace.agentNotInTag'))
    return
  }
  if (activeCapabilityError.value) {
    toast.warning(activeCapabilityError.value)
    return
  }
  routeAgentName.value = mentionedAgents.length
    ? mentionedAgents.map((a) => agentNamesById.value[a.id] || a.name).join('、')
    : ''
  if (!threadId.value) await resetThread()
  if (!threadId.value) return
  const thread = sectionThreads.value.find((item) => item.id === threadId.value)
  const threadAgentId = thread?.agent_id

  // 协作会话（含旧 auto）：走 /turns，允许一条消息里多个目标。
  if (isCollaborativeThread.value) {
    draft.value = ''
    const targets: AgentMentionTarget[] = mentionedAgents.map((a) => ({ kind: 'agent', id: a.id }))
    if (wantsHuman) targets.push({ kind: 'human', id: 'me' })
    if (!targets.length && threadAgentId) targets.push({ kind: 'agent', id: threadAgentId })
    collab.start(threadId.value, content, targets, thread?.thread_kind)
    return
  }
  // 固定助手的老会话：/runs 只支持单目标，多目标仍明确拒绝并保留草稿。
  if (agentIds.length > 1) {
    toast.warning(t('pages.agents.multipleAgentsNotSupported'))
    return
  }
  draft.value = ''
  // 无 @ 时回到会话绑定的助手（老语义：每个助手一份会话）。
  const usedMentions = agentIds.length
    ? agentIds.map((id) => ({ agent_id: id }))
    : threadAgentId
      ? [{ agent_id: threadAgentId }]
      : []
  conv.start({ content, mentions: usedMentions, module: (mentionedAgents[0] || activeThreadAgent.value)?.module }, {
    onEnd: () => {
      refreshThreadsForAgent()
    },
    onError: (e) => {
      if ((e as { code?: string })?.code === 'agent_not_in_tag') {
        toast.warning(t('components.agentWorkspace.agentNotInTag'))
        draft.value = content
        return
      }
      toast.error(errorMessage(e, t('pages.agents.runFailed')))
    }
  })
}

/** 重试最近一次失败：普通 run 复用原消息。
 *  原助手已被删除/移出标签时服务端拒绝，提示重新发送（§8.7）。 */
function retryLast() {
  if (sending.value) return
  if (activeCapabilityError.value) { toast.warning(activeCapabilityError.value); return }
  if (!failedRunId.value) return
  const retryId = failedRunId.value
  // 移除末尾失败气泡（user 消息保留展示）。
  const last = chatMessages.value[chatMessages.value.length - 1]
  if (last?.role === 'assistant' && (last.error || last.canceled)) chatMessages.value.pop()
  conv.start({ content: '', mentions: [], retry_of_run_id: retryId }, {
    onEnd: () => {
      refreshThreadsForAgent()
    },
    onError: (e) => {
      if ((e as { code?: string })?.code === 'agent_unavailable') {
        toast.warning(t('components.agentWorkspace.retryAgentUnavailable'))
        return
      }
      toast.error(errorMessage(e, t('pages.agents.runFailed')))
    }
  })
}

/** 停止：协作会话取消整轮（服务端置 canceled），固定助手会话沿用旧 stop。 */
async function onStop() {
  if (sending.value) {
    const reply = chatMessages.value[chatMessages.value.length - 1]
    if (reply?.role === 'assistant') reply.canceled = true
  }
  if (isCollaborativeThread.value) {
    await collab.cancel()
    return
  }
  conv.stop()
}

/** 人工待办：回复并继续 / 无法处理（与工具审批严格分离）。 */
async function replyHumanTask(taskId: string, outcome: 'continue' | 'unable', text?: string) {
  await collab.replyHuman(taskId, outcome, text)
  await loadHumanTasks()
}

/** 重试某个失败任务（协作会话）。 */
async function retryTask(taskId: string) {
  await collab.retryTask(taskId)
}

/** 切换会话 / 助手：只断开 SSE，后端 run 继续，回到该会话时自动续接。 */
function detachRun() {
  conv.clear()
}

// 离开页面（路由切换）：只关闭 SSE，不取消后端 run。
onBeforeUnmount(() => {
  detachRun()
  activeDisposed = true
  clearActivePoll()
  if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibilityChange)
})

void bootstrap()
</script>
