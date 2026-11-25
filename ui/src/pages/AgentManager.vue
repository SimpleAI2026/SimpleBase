<template>
  <ProjectScope>
    <PageContainer>
      <div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[184px_minmax(0,1fr)] xl:grid-cols-[200px_minmax(0,1fr)]">
        <!-- 左列：工作台导航（群列表 + 每群会话 + 群行 +）。 -->
        <AgentWorkspaceSidebar
          :tags="tags"
          :group-pages="ws.pages"
          :section="section"
          :thread-id="threadId"
          :running-thread-ids="runningThreadIds"
          :pending-by-group="pendingByGroup"
          :group-members="groupMembers"
          :pending="agentsPending"
          :error="agentsError"
          @select-section="selectSection"
          @select-thread="selectThreadInGroup"
          @create-tag="tagEditor.openTag()"
          @create-thread="createThreadInGroup"
          @load-more="(id: string) => ws.loadMore(id)"
          @rename-thread="startRenameThread"
          @remove-thread="removeThread"
          @edit-tag="tagEditor.openTag"
          @archive-tag="tagEditor.archiveTag"
          @add-members="(id: string) => tagEditor.openMembers(id)"
          @create-member-agent="(id: string) => tagEditor.openMembers(id, 'create')"
          @reload="reloadAgents"
        />

        <!-- 右列：对话区（分区制）。
             高度 = 视口 - 顶栏 - 上下页面内边距（--page-pad 由 DefaultLayout 施加，
             这里必须引用同一个变量，不能写死数值）。 -->
        <Card class="flex h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))] min-h-[420px] min-w-0 flex-col sm:min-h-[520px]">
          <CardHeader class="border-b">
            <CardTitle class="flex w-full flex-wrap items-center gap-2 sm:gap-3">
              <span class="max-w-40 min-w-0 shrink truncate sm:max-w-56" data-section-title>{{ headerTitle }}</span>
              <Badge
                v-if="activeThreadAgent && activeThreadAgentName"
                variant="outline"
                class="shrink-0 text-xs"
                data-thread-agent-badge
              >{{ activeThreadAgentName }}</Badge>
              <!-- 群会话：群名 · N 位成员 + 成员头像叠放（最多 3 个）+ 进度摘要。 -->
              <span
                v-if="isCollaborativeThread && sectionAgents.length"
                class="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground"
                data-thread-group-badge
              >
                <span class="flex -space-x-1.5" aria-hidden="true">
                  <span
                    v-for="a in sectionAgents.slice(0, 3)"
                    :key="a.id"
                    class="flex size-5 items-center justify-center rounded-full border border-background bg-card text-foreground"
                    :data-group-avatar="a.id"
                  >
                    <component :is="iconForAgentModule(a.module)" class="size-3" />
                  </span>
                </span>
                <span>{{ t('components.agentWorkspace.groupMembers', { n: sectionAgents.length }) }}</span>
                <span v-if="collab.progress.value.total" data-group-progress>{{ collab.progress.value.done }}/{{ collab.progress.value.total }}</span>
              </span>
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
              :state="threadRegionState"
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
                :plan-instructions="planInstructions"
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
            <!-- v3：Plan 卡（目标 / 步骤 / 分工 / 进度）与人工待办卡。 -->
            <div v-if="activeTurnTasks.length" class="flex flex-col gap-2">
              <AgentPlanCard
                :plan="collab.turn.value?.plan ?? null"
                :tasks="activeTurnTasks"
                :names="agentNamesById"
                @retry-task="collab.retryTask"
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
            <!-- 标签分区底部助手条。 -->
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
              :plan-hint="planHint"
              :planning="collab.planning.value"
              @send="onSend"
              @stop="onStop"
            />
          </CardContent>
        </Card>
      </div>

      <!-- v3：整轮 Plan 确认；与 AgentConfirmCard 的单次工具确认分离。 -->
      <PlanConfirmDialog
        :open="!!collab.pendingPlan.value"
        :plan="collab.pendingPlan.value?.decision ?? null"
        :names="agentNamesById"
        :busy="collab.confirmingPlan.value"
        @submit="collab.confirmPlan"
      />

      <!-- 提示词替换已有草稿前的确认（CB-FE-13：统一 ConfirmAction，不用 window.confirm）。 -->
      <ConfirmAction
        controlled
        :open="replaceDraftOpen"
        :title="t('components.agentWorkspace.replaceDraftConfirm')"
        @confirm="applyPickPrompt"
        @cancel="closeReplaceDraft"
        @update:open="setReplaceDraftOpen"
      />

      <!-- 标签编辑弹窗。 -->
      <AgentTagModal
        :open="tagEditor.tagModal.value.open"
        :tag="tagEditor.editingTag.value"
        :agents="agentOptionsForTag"
        :saving="tagEditor.tagModal.value.saving"
        :conflicted="tagEditor.tagModal.value.conflicted"
        @update:open="(v: boolean) => (tagEditor.tagModal.value.open = v)"
        @save="tagEditor.saveTag"
        @reload="tagEditor.reloadTag"
      />

      <!-- 群成员弹窗：加入已有 / 新建助手并加入。 -->
      <AgentGroupMembersModal
        :open="tagEditor.membersModal.value.open"
        :group-name="membersGroupName"
        :agents="mentionAgents"
        :member-ids="tagEditor.membersGroup.value?.agentIds || []"
        :modules="modules"
        :initial-mode="tagEditor.membersMode.value"
        :saving="tagEditor.membersModal.value.saving"
        :conflicted="tagEditor.membersModal.value.conflicted"
        @update:open="(v: boolean) => (tagEditor.membersModal.value.open = v)"
        @save="tagEditor.saveMembers"
        @reload="tagEditor.reloadMembers"
      />

      <ThreadRenameModal
        v-model:open="renameOpen"
        v-model="renameValue"
        :saving="renaming"
        @submit="submitRenameThread"
      />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useRoute, useRouter } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import PageContainer from '@/components/PageContainer.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import ThreadRenameModal from '@/components/modal/ThreadRenameModal.vue'
import AgentWelcome from '@/components/agent/AgentWelcome.vue'
import AgentTagModal from '@/components/modal/AgentTagModal.vue'
import AgentGroupMembersModal from '@/components/modal/AgentGroupMembersModal.vue'
import AgentWorkspaceSidebar from '@/components/agent/AgentWorkspaceSidebar.vue'
import AgentTagOverview from '@/components/agent/AgentTagOverview.vue'
import AgentRosterBar from '@/components/agent/AgentRosterBar.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import AgentPlanCard from '@/components/agent/AgentPlanCard.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import { iconForAgentModule } from '@/components/nav-icons'
import PlanConfirmDialog from '@/components/agent/PlanConfirmDialog.vue'
import AgentHumanTaskCard from '@/components/agent/AgentHumanTaskCard.vue'
import AgentComposer from '@/components/agent/AgentComposer.vue'
import { useLoadState } from '@/composables/useLoadState'
import { useAgentRunLayer } from '@/composables/useAgentRunLayer'
import { useAgentWorkspace } from '@/composables/useAgentWorkspace'
import { useHumanTasks } from '@/composables/useHumanTasks'
import { useAgentDirectory } from '@/composables/useAgentDirectory'
import { useAgentSender } from '@/composables/useAgentSender'
import { useWorkspaceView, tagDisplayName } from '@/composables/useWorkspaceView'
import { useThreadCapabilities, useRenameThread } from '@/composables/useThreadCapabilities'
import { useThreadNavigation } from '@/composables/useThreadNavigation'
import { useAgentTagEditor } from '@/composables/useAgentTagEditor'
import { useQueryPatch } from '@/composables/useQueryPatch'
import { useStarterThread } from '@/composables/useStarterThread'
import { api } from '@/services/api'
import type { AgentThread, ChatMsg, MentionAgentLike } from '@/services/types'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { errorMessage } from '@/utils/format'

const project = useProjectStore()
const route = useRoute()
const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const replaceQuery = useQueryPatch(router)

const threadLoad = useLoadState({ fallback: 'pages.agents.loadThreadFailed' })
const { pending: threadPending, hasData: threadHasData, settle: settleThread } = threadLoad
/** 会话区的空态交给 ConversationView 的 empty 插槽，所以这里恒不显示 SbAsyncRegion 的空态。 */
const threadRegionState = { ...threadLoad, showEmpty: false }

/** 助手目录：助手 / 模块 / 模型候选与按当前语言解析的显示名。 */
const directory = useAgentDirectory({ projectId: () => project.id })
const { agents, modules, modelOptions, rawAgentById, displayedAgents, displayedAgentById, agentNamesById, loadAgents, loadModules } = directory
const { pending: agentsPending, error: agentsError } = directory

const chatMessages = ref<ChatMsg[]>([])
const draft = ref('')
/** 当前标签 ID，默认是包含全部助手的 #通用。 */
const section = ref('general')
/** 当前会话（分区制后由选中线程直接持有）。 */
const threadId = ref('')
const protocolVersion = ref(0)

/** 每个群（含当前群）的会话分页：会话列表的唯一数据源；`tags` 是全部标签（预置 + 自定义）。 */
const ws = useAgentWorkspace({ projectId: () => project.id })
const { tags } = ws
const sectionThreads = computed<AgentThread[]>({
  get: () => ws.pageOf(section.value).threads,
  set: (v) => { ws.pageOf(section.value).threads = v }
})
const sectionCursor = computed<string>({
  get: () => ws.pageOf(section.value).cursor,
  set: (v) => {
    const page = ws.pageOf(section.value)
    page.cursor = v
    page.hasMore = Boolean(v)
  }
})

const composer = ref<{ focus?: () => void; insertMention?: (a: MentionAgentLike) => void } | null>(null)

/** 我的待办：群行角标与人工任务卡。 */
const { pendingByGroup, loadHumanTasks } = useHumanTasks({ projectId: () => project.id, ws })

/** 运行层：单助手 run + 协作轮次 + 「正在回复」指示（conv 与 collab 按会话类型二选一）。 */
const { conv, collab, sending, activeRuns, refreshActiveRuns, runningAgentIds, runningThreadIds } = useAgentRunLayer({
  projectId: () => project.id,
  threadId,
  chatMessages,
  onTurnEnd: () => {
    refreshThreadsForAgent()
    void loadHumanTasks()
  },
  onTick: () => loadHumanTasks()
})
const { confirmation, statusText, failedRunId } = conv

/** 只读衍生视图：当前群 / 会话 / 页头 / 助手条 / @ 候选。 */
const view = useWorkspaceView({
  tags,
  section,
  threadId,
  sectionThreads,
  displayedAgents,
  displayedAgentById,
  rawAgentById,
  hasUser: () => !!authStore.user
})
const { activeTag, activeThread, sectionDisplayName, headerTitle, sectionAgents, recentTagThreads, mentionAgents } = view

/** 当前会话固定助手（分区制：线程绑定，不再由左栏选择）。 */
const activeThreadAgent = computed(() => rawAgentById.value.get(activeThread.value?.agent_id || ''))
const activeAgent = computed(() => displayedAgentById.value.get(activeThread.value?.agent_id || ''))
const activeThreadAgentName = computed(() => activeAgent.value?.name ?? '')
const activeThreadAgentModule = computed(() => activeThreadAgent.value?.module)

/** 会话能力：协作判定、轮次任务、助手能力提示。 */
const {
  isCollaborativeThread,
  activeTurnTasks,
  humanTasksInTurn,
  activeAgentHasSandboxTools,
  activeCapabilityError,
  toolsHint
} = useThreadCapabilities({ activeThread, activeThreadAgent, activeThreadAgentName, modules, protocolVersion, collabTasks: collab.tasks })

/** 会话导航：切分区 / 选会话 / 新建 / 删除 / 加载消息并续接后台 run。 */
const nav = useThreadNavigation({
  projectId: () => project.id,
  tags,
  section,
  threadId,
  chatMessages,
  sectionThreads,
  sectionCursor,
  sending,
  ws,
  conv,
  collab,
  isCollaborative: () => isCollaborativeThread.value,
  load: threadLoad,
  queryThread: () => route.query.thread,
  replaceQuery,
  moduleOf: (agentId) => rawAgentById.value.get(agentId)?.module,
  stop: () => onStop()
})
const {
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
  createThreadInGroup,
  selectThreadInGroup
} = nav
function refreshThreadsForAgent() {
  nav.refreshThreads()
}

/** 发送 / 重试 / 停止 / 人工待办回复。 */
const { canRetry, onSend, retryLast, onStop, replyHumanTask } = useAgentSender({
  threadId,
  draft,
  chatMessages,
  conv,
  collab,
  sending,
  activeThread,
  activeThreadAgent,
  isCollaborativeThread,
  activeCapabilityError,
  rawAgentById,
  agentNamesById,
  sectionAgentIds: () => sectionAgents.value.map((a) => a.id),
  hasUser: () => !!authStore.user,
  resetThread,
  refreshThreads: refreshThreadsForAgent,
  loadHumanTasks
})

/** 会话重命名弹窗。 */
const rename = useRenameThread({ threadsOf: () => sectionThreads.value, rename: renameThread })
const { open: renameOpen, value: renameValue, saving: renaming, start: startRenameThread, submit: submitRenameThread } = rename

const tagEditor = useAgentTagEditor({
  projectId: () => project.id,
  tags,
  section,
  loadTags,
  loadAgents,
  selectSection
})
const agentOptionsForTag = computed(() => view.agentOptionsFor(tagEditor.editingTag.value))
const membersGroupName = computed(() => (tagEditor.membersGroup.value ? tagDisplayName(tagEditor.membersGroup.value) : ''))

const composerPlaceholder = computed(() =>
  activeAgent.value ? t('pages.agents.composerWithAgent', { name: activeAgent.value.name }) : t('pages.agents.composerIdle')
)
/**
 * composer 的 Plan 状态条（CB-FE-10）：群会话里分工由 Plan 栅栏决定。
 * 草稿里点了 @助手 时提示「本轮 @A、@B」——@ 是硬约束，Plan 会为每人生成独立指令。
 */
const planHint = computed(() => {
  if (!isCollaborativeThread.value) return ''
  const text = draft.value
  const mentioned = mentionAgents.value
    .filter((a) => {
      const token = `@${a.name}`
      const at = text.indexOf(token)
      if (at < 0) return false
      const after = text[at + token.length]
      return !after || /\s/.test(after)
    })
    .map((a) => `@${a.name}`)
  return mentioned.length
    ? t('components.agentWorkspace.planMentioned', { names: mentioned.join('、') })
    : t('components.agentWorkspace.planDecides')
})

/** 左栏群头像：每个群最多 3 位成员的 id 与模块（叠放渲染）。 */
const groupMembers = computed<Record<string, { id: string; module?: string }[]>>(() =>
  Object.fromEntries(
    tags.value.map((tag) => [
      tag.id,
      tag.agentIds
        .map((id) => rawAgentById.value.get(id))
        .filter((a): a is NonNullable<typeof a> => !!a)
        .slice(0, 3)
        .map((a) => ({ id: a.id, module: a.module }))
    ])
  )
)

/** 当前轮次里 taskId → Plan 给该助手的指令（气泡上方「来自 Plan」引用）。 */
const planInstructions = computed<Record<string, string>>(() => {
  const out: Record<string, string> = {}
  for (const task of collab.tasks.value) if (task.instruction) out[task.id] = task.instruction
  return out
})

/** 提示词替换草稿的确认（受控 ConfirmAction）。 */
const replaceDraftOpen = ref(false)
let pendingPrompt = ''

/** 点提示词：写入输入框并聚焦，**不自动发送**——一次点击把话术准备好，回车才执行。 */
function pickPrompt(text: string) {
  if (!draft.value.trim()) {
    applyPickPromptTo(text)
    return
  }
  if (draft.value === text) return
  pendingPrompt = text
  replaceDraftOpen.value = true
}

function applyPickPrompt() {
  applyPickPromptTo(pendingPrompt)
  replaceDraftOpen.value = false
}

function closeReplaceDraft() {
  replaceDraftOpen.value = false
}

function setReplaceDraftOpen(open: boolean) {
  replaceDraftOpen.value = open
}

function applyPickPromptTo(text: string) {
  draft.value = text
  composer.value?.focus()
}
watch(
  () => project.id,
  () => {
    detachRun()
    section.value = 'general'
    threadId.value = ''
    ws.reset()
    chatMessages.value = []
    activeRuns.value = []
    void bootstrap()
  }
)

async function bootstrap() {
  // 首次进入且尚未选定项目时 project.id 为空：此时请求会打到 /v1/projects//… 并弹出多条 404 提示；
  // 等 loadProjects 选中项目后由上面的 watch 再触发。
  if (!project.id) return
  // loadThreads 依赖 tags（决定分区），所以两者必须串行；
  // 模型候选与运行态与它们无关，可以并行。
  await Promise.all([
    api.agentThreads.capabilities(project.id).then((caps) => { protocolVersion.value = caps.protocolVersion }).catch(() => { protocolVersion.value = 0 }),
    directory.loadModelOptions(),
    refreshActiveRuns(),
    (async () => {
    await loadAgents()
    await loadTags()
    await loadSectionThreads()
  })()])
  await maybeStartStarter()
}

/** 首页「一句话建应用」搭建会话。 */
const { maybeStartStarter } = useStarterThread({
  projectId: () => project.id,
  starterQuery: () => route.query.starter,
  section,
  threadId,
  chatMessages,
  addThread: (th) => ws.unshiftThread(section.value, th),
  replaceQuery,
  settleThread,
  startTurn: (id, seed) => collab.start(id, seed, [], 'chat')
})

/** 拉工作台标签并预取其他群首屏；失败 toast。 */
async function loadTags() {
  await ws.refreshTagsAndPrefetch(section.value, (e) => toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed'))))
}

async function reloadAgents() {
  await Promise.all([loadAgents(), loadTags()])
}

/** 点提示词：写入输入框并聚焦，**不自动发送**——一次点击把话术准备好，回车才执行。 */
/** 助手条点名：草稿插入 @名称（不发送不切换）。 */
function mentionAgentFromTag(a: MentionAgentLike) {
  composer.value?.insertMention?.({ id: a.id, name: a.name, module: a.module })
}

// 离开页面（路由切换）：只关闭 SSE，不取消后端 run（轮询由 useActiveRunsPoll 自行回收）。
onBeforeUnmount(detachRun)

void bootstrap()
</script>
