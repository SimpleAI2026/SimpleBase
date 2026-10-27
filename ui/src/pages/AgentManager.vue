<template>
  <ProjectScope>
    <PageContainer>
      <div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[200px_minmax(0,1fr)]">
        <!-- 左列：Agent 条目（拆到 components/agent/AgentListPanel.vue）。 -->
        <AgentListPanel
          :agents="displayedAgents"
          :active-id="activeId"
          :pending="agentsPending"
          :show-skeleton="agentsShowSkeleton"
          :show-empty="agentsShowEmpty"
          :show-error="agentsShowError"
          :refreshing="agentsRefreshing"
          :error="agentsError"
          :schedule-by-agent="scheduleByAgent"
          :schedule-summary="scheduleSummary"
          :running-agent-ids="runningAgentIds"
          @reload="loadAgents"
          @create="openCreate"
          @select="selectAgent"
        />

        <!-- 右列：对话区（planv6.0 §3.2/§3.8）。
             高度 = 视口 - 顶栏 - 上下页面内边距（--page-pad 由 DefaultLayout 施加，
             这里必须引用同一个变量，不能写死数值）。 -->
        <Card class="flex h-[calc(100svh-var(--header-height)-var(--page-pad)-var(--page-pad))] min-h-[420px] min-w-0 flex-col sm:min-h-[520px]">
          <CardHeader class="border-b">
            <CardTitle class="flex w-full flex-wrap items-center gap-2 sm:gap-3">
              <span class="max-w-40 min-w-0 shrink truncate sm:max-w-56">{{ activeAgent ? '@' + activeAgent.name : t('pages.agents.dialogLabel') }}</span>
              <Badge
                v-if="activeAgent && activeAgent.writable === false"
                variant="outline"
                class="shrink-0 cursor-pointer text-[11px]"
                data-agent-readonly-header
                role="button"
                tabindex="0"
                :title="t('components.agentCard.readonlyHint')"
                @click="onMenu('edit')"
                @keydown.enter="onMenu('edit')"
              >{{ t('components.agentCard.readonly') }}</Badge>
              <ThreadSwitcher
                :threads="threads"
                :active-id="threadId"
                :busy="sending"
                :pending="threadPending && !threadHasData"
                :running-thread-ids="runningThreadIds"
                @create="resetThread"
                @select="selectThread"
              />
              <Popover v-model:open="menuOpen">
                <PopoverTrigger as-child>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    class="shrink-0"
                    :aria-label="t('pages.agents.moreActions')"
                    :title="t('pages.agents.moreActions')"
                  >
                    <EllipsisIcon />
                  </Button>
                </PopoverTrigger>
                <PopoverContent align="end" class="w-52 p-1">
                  <p class="px-2 pt-1 pb-0.5 text-[11px] font-medium text-muted-foreground">{{ t('pages.agents.menuAgentGroup') }}</p>
                  <button
                    type="button"
                    class="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
                    :disabled="!activeAgent"
                    @click="onMenu('edit')"
                  >{{ t('pages.agents.menuEdit') }}</button>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
                    :disabled="!activeAgent"
                    @click="onMenu('schedule')"
                  >
                    {{ t('pages.agents.menuSchedule') }}
                    <span v-if="activeScheduleSummary" class="ml-auto truncate text-[11px] text-muted-foreground">{{ activeScheduleSummary }}</span>
                  </button>
                  <ConfirmAction
                    v-if="activeAgent && !activeAgent.builtin_key"
                    :title="t('components.agentCard.deleteConfirm', { name: activeAgent.name })"
                    @confirm="onMenu('remove')"
                  >
                    <button type="button" class="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs text-destructive hover:bg-muted">{{ t('components.agentCard.delete') }}</button>
                  </ConfirmAction>

                  <div class="my-1 h-px bg-border" />
                  <p class="px-2 pt-1 pb-0.5 text-[11px] font-medium text-muted-foreground">{{ t('pages.agents.menuThreadGroup') }}</p>
                  <button
                    type="button"
                    class="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
                    :disabled="!activeThread"
                    @click="onMenu('renameThread')"
                  >{{ t('pages.agents.menuRenameThread') }}</button>
                  <ConfirmAction
                    v-if="activeThread"
                    :title="t('components.threadSwitcher.deleteConfirm')"
                    @confirm="onMenu('removeThread')"
                  >
                    <button type="button" class="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs text-destructive hover:bg-muted">{{ t('pages.agents.menuDeleteThread') }}</button>
                  </ConfirmAction>
                </PopoverContent>
              </Popover>
            </CardTitle>
          </CardHeader>
          <CardContent class="flex min-h-0 min-w-0 flex-1 flex-col gap-2 p-4">
            <SbAsyncRegion
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
                @retry="retryLast"
                @confirm="conv.resolveConfirm(true)"
                @deny="conv.resolveConfirm(false)"
              >
                <template #empty>
                  <AgentWelcome
                    :agent-name="activeAgent?.name"
                    :module="activeAgent?.module"
                    @pick="pickPrompt"
                  />
                </template>
              </ConversationView>
            </SbAsyncRegion>
            <div class="flex min-h-4 flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <p v-if="statusText" role="status" class="truncate">{{ statusText }}</p>
              <p v-else class="min-w-0 truncate">{{ toolsHint }}</p>
              <router-link v-if="activeAgentHasSandboxTools" :to="{ name: 'sandboxes' }" class="shrink-0 text-primary hover:underline">{{ t('pages.agents.viewInSandbox') }}</router-link>
            </div>
            <div v-if="switchTarget" class="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-sm" role="status">
              <span>{{ t('pages.agents.switchAgentHint', { name: switchTarget.name }) }}</span>
              <Button type="button" size="sm" variant="outline" data-agent-switch @click="switchToMentionedAgent">
                {{ t('pages.agents.switchAgentAction', { name: switchTarget.name }) }}
              </Button>
            </div>
            <AgentComposer
              ref="composer"
              v-model="draft"
              :sending="sending"
              :disabled="!threadId || !!activeCapabilityError"
              :placeholder="composerPlaceholder"
              :mention-agents="mentionAgents"
              @send="onSend"
              @stop="onStop"
            />
          </CardContent>
        </Card>
      </div>

      <AgentFormModal
        :open="modalOpen"
        :editing="!!editing"
        :saving="saving"
        :form="form"
        :module-options="moduleOptions"
        :tool-options="toolOptions"
        :model-options="modelOptions"
        :default-model="defaultModel"
        @update:open="(v: boolean) => (modalOpen = v)"
        @save="saveAgent"
        @module-change="onModuleChange"
        @toggle-tool="toggleTool"
        @update:writable="(v: boolean) => (form.writable = v)"
      />

      <AgentScheduleModal
        :open="scheduleModalOpen"
        :agent="scheduleAgent"
        :schedule="scheduleAgent ? (scheduleByAgent[scheduleAgent.id] || null) : null"
        :project-id="project.id"
        @update:open="(v: boolean) => (scheduleModalOpen = v)"
        @saved="onScheduleSaved"
        @removed="onScheduleRemoved"
        @view-thread="onViewScheduleThread"
      />

      <!-- 会话重命名（planv6.0 §3.6 #7：会话操作收进「⋯」菜单）。 -->
      <SbModal
        :open="renameOpen"
        :title="t('pages.agents.renameThreadTitle')"
        :width="440"
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
import { BotIcon, EllipsisIcon, PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { api } from '../services/api'
import type { AgentModuleInfo, AgentThread, CloudAgent } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import SbModal from '../components/modal/SbModal.vue'
import AgentWelcome from '../components/agent/AgentWelcome.vue'
import AgentFormModal from '../components/modal/AgentFormModal.vue'
import type { AgentFormState } from '../components/modal/AgentFormModal.vue'
import AgentListPanel from '../components/agent/AgentListPanel.vue'
import ThreadSwitcher from '../components/agent/ThreadSwitcher.vue'
import ConversationView from '../components/agent/ConversationView.vue'
import AgentComposer from '../components/agent/AgentComposer.vue'
import AgentScheduleModal from '../components/ai/AgentScheduleModal.vue'
import { useAgentConversation } from '../composables/useAgentConversation'
import { useLoadState } from '@/composables/useLoadState'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import type { ActiveAgentRun, AgentMessage } from '../services/types'
import type { ChatMsg } from '../composables/useAiChat'
import type { AgentSchedule } from '../services/types'

const project = useProjectStore()
const route = useRoute()
const { t } = useI18n()
const router = useRouter()
const {
  pending: agentsPending,
  showSkeleton: agentsShowSkeleton,
  showEmpty: agentsShowEmpty,
  showError: agentsShowError,
  refreshing: agentsRefreshing,
  error: agentsError,
  run: runAgents
} = useLoadState({ fallback: 'pages.agents.loadAgentsFailed' })
const {
  pending: threadPending,
  hasData: threadHasData,
  showSkeleton: threadShowSkeleton,
  showEmpty: threadShowEmpty,
  showError: threadShowError,
  refreshing: threadRefreshing,
  error: threadError,
  run: runThread,
  settle: settleThread
} = useLoadState({ fallback: 'pages.agents.loadThreadFailed' })
const saving = ref(false)
const agents = ref<CloudAgent[]>([])
const modules = ref<AgentModuleInfo[]>([])
const activeId = ref('')
const modalOpen = ref(false)
const editing = ref<CloudAgent | null>(null)
const form = ref({ name: '', module: 'database', description: '', system_prompt: '', model_override: '', tool_ids: [] as string[], writable: false })
const defaultModel = ref('')
const modelOptions = ref<string[]>([])
const threads = ref<AgentThread[]>([])
const nextCursor = ref('')
const threadId = ref('')
const chatMessages = ref<ChatMsg[]>([])
const draft = ref('')
const switchTargetId = ref('')

const scheduleByAgent = ref<Record<string, AgentSchedule>>({})
const scheduleModalOpen = ref(false)
const scheduleAgent = ref<CloudAgent | null>(null)
/** 头部「⋯」菜单开合（planv6.0 §3.8）。 */
const menuOpen = ref(false)
const composer = ref<{ focus?: () => void } | null>(null)
/** 会话重命名弹窗（原 ThreadSwitcher 无重命名入口，这里补上）。 */
const renameOpen = ref(false)
const renameValue = ref('')
const renaming = ref(false)
const renameTarget = ref<AgentThread | null>(null)

/** 会话状态层（planv4.1 BUG-01/03/09/10）。 */
const conv = useAgentConversation({
  projectId: () => project.id,
  threadId: () => threadId.value,
  messages: () => chatMessages.value
})
const sending = conv.sending
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
  if (sending.value && activeId.value) ids.add(activeId.value)
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
const displayedAgents = computed(() => {
  const locale = i18n.global.locale.value
  return agents.value.map((agent) => {
    const key = agent.builtin_key || ''
    if (agent.name !== builtinDefaultNames[key]) return agent
    return { ...agent, name: i18n.global.t(`pages.agents.builtinNames.${key}`, 1, { locale }) }
  })
})
const activeAgent = computed(() => displayedAgents.value.find((a) => a.id === activeId.value))
const switchTarget = computed(() => displayedAgents.value.find((a) => {
  if (a.id !== switchTargetId.value) return false
  const original = agents.value.find((raw) => raw.id === a.id)
  if (draft.value.includes(`@${a.name}`) || (!!original && draft.value.includes(`@${original.name}`))) return true
  if (original?.builtin_key === 'kv' && original.name === builtinDefaultNames.kv && draft.value.includes('@KV助手')) return true
  if (original?.builtin_key && original.name === builtinDefaultNames[original.builtin_key]) {
    return (['zh-CN', 'en-US'] as const).some((locale) => draft.value.includes(`@${i18n.global.t(`pages.agents.builtinNames.${original.builtin_key}`, {}, { locale })}`))
  }
  return false
}))
const mentionAgents = computed(() => displayedAgents.value.map((a) => ({
  id: a.id, name: a.name, module: a.module,
  legacyName: a.builtin_key ? agents.value.find((raw) => raw.id === a.id)?.name : undefined,
  builtinKey: a.builtin_key && agents.value.find((raw) => raw.id === a.id)?.name === builtinDefaultNames[a.builtin_key] ? a.builtin_key : undefined
})))
const composerPlaceholder = computed(() =>
  activeAgent.value
    ? t('pages.agents.composerWithAgent', { name: activeAgent.value.name })
    : t('pages.agents.composerIdle')
)

const moduleOptions = computed(() =>
  modules.value.map((m) => ({
    label: `${m.name}（${m.id}）`,
    value: m.id,
    disabled: m.id === 'sandbox' && m.sandbox_available !== true
  }))
)

// 当前选中 Agent 是否含沙盒工具；决定 composer 旁的提示文案。
const activeAgentHasSandboxTools = computed(() => {
  const a = activeAgent.value
  if (!a) return false
  return (a.tool_ids || []).some((id) => id.startsWith('sandbox_'))
})

const activeCapabilityError = computed(() => {
  const agent = activeAgent.value
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
  const who = activeAgent.value
    ? t('pages.agents.mentionHint', { name: activeAgent.value.name })
    : t('pages.agents.mentionHintGeneric')
  if (activeAgentHasSandboxTools.value) return t('pages.agents.mentionHintSandbox', { who })
  if (['kv', 'gofunction', 'cron'].includes(activeAgent.value?.module || '')) return t('pages.agents.mentionHintCli', { who })
  return t('pages.agents.mentionHintReadonly', { who })
})
const toolOptions = computed(() => {
  const m = modules.value.find((x) => x.id === form.value.module)
  const ids = m ? m.default_tools : ['list_databases', 'list_collections', 'readonly_sql', 'list_objects', 'head_object', 'search_logs', 'log_level_stats']
  return ids.map((id) => ({ label: id, value: id }))
})

/** 最后一条消息失败且记录了失败 run id 时可重试（BUG-03：走后端 retry_of_run_id）。 */
const canRetry = computed(() => {
  if (sending.value || !failedRunId.value) return false
  const last = chatMessages.value[chatMessages.value.length - 1]
  return last?.role === 'assistant' && !!last.error
})

watch(
  () => project.id,
  () => {
    detachRun()
    switchTargetId.value = ''
    activeId.value = ''
    threadId.value = ''
    threads.value = []
    nextCursor.value = ''
    chatMessages.value = []
    scheduleByAgent.value = {}
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
  // loadThreads 依赖 activeId（由 loadAgents 决定），所以两者必须串行；
  // 调度与模型候选与它们无关，可以和 loadAgents 并行。
  await Promise.all([loadSchedules(), loadModelOptions(), refreshActiveRuns(), (async () => {
    await loadAgents()
    await loadThreads()
  })()])
}

async function loadSchedules() {
  try {
    const list = await api.agentSchedules.list(project.id)
    const map: Record<string, AgentSchedule> = {}
    for (const s of list) map[s.agent_id] = s
    scheduleByAgent.value = map
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.loadSchedulesFailed')))
  }
}

/** 预设 cron → i18n key；未命中原样展示表达式。 */
const CRON_PRESET_KEYS: Record<string, string> = {
  '*/15 * * * *': 'components.agentSchedule.preset15Min',
  '0 * * * *': 'components.agentSchedule.presetHourly',
  '0 8 * * *': 'components.agentSchedule.presetDaily8',
  '0 8 * * 1': 'components.agentSchedule.presetWeeklyMon8'
}

function scheduleSummary(s: AgentSchedule): string {
  const key = CRON_PRESET_KEYS[s.cron_expr]
  const cron = key ? t(key) : s.cron_expr
  return t('pages.agents.scheduleSummary', {
    cron,
    state: s.enabled ? t('components.agentSchedule.enabled') : t('components.agentSchedule.disabled')
  })
}

/** 当前选中 Agent 的定时任务摘要（头部「⋯」菜单里显示一行）。 */
const activeScheduleSummary = computed(() => {
  const s = activeAgent.value ? scheduleByAgent.value[activeAgent.value.id] : undefined
  return s ? scheduleSummary(s) : ''
})

/** 当前会话（菜单里的会话级动作需要它）。 */
const activeThread = computed(() => threads.value.find((th) => th.id === threadId.value))

type MenuAction = 'edit' | 'schedule' | 'remove' | 'renameThread' | 'removeThread'

/**
 * 头部「⋯」菜单动作。
 *
 * 「编辑/定时/删除」作用于当前选中 Agent；「重命名/删除」作用于当前会话——
 * 这两项原本挂在 ThreadSwitcher 上，收进菜单后由这里接管（planv6.0 §3.3/§3.6 #7）。
 */
function onMenu(action: MenuAction) {
  menuOpen.value = false
  if (action === 'renameThread') {
    const th = activeThread.value
    if (!th) return
    renameTarget.value = th
    renameValue.value = th.title
    renameOpen.value = true
    return
  }
  if (action === 'removeThread') {
    const th = activeThread.value
    if (th) void removeThread(th.id)
    return
  }
  const a = agents.value.find((agent) => agent.id === activeId.value)
  if (!a) return
  if (action === 'edit') openEdit(a)
  else if (action === 'schedule') openSchedule(a)
  else void removeAgent(a)
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
  draft.value = text
  composer.value?.focus?.()
}

/** 定时任务已统一到「定时任务」页：跳转并预选该助手（旧 /agent-schedules 写接口已返回 410）。 */
function openSchedule(a: CloudAgent) {
  void router.push({ name: 'cron-jobs', query: { agent: a.id } })
}

function onScheduleSaved(s: AgentSchedule) {
  scheduleByAgent.value = { ...scheduleByAgent.value, [s.agent_id]: s }
}

function onScheduleRemoved(scheduleId: string) {
  const map = { ...scheduleByAgent.value }
  for (const [agentId, s] of Object.entries(map)) {
    if (s.id === scheduleId) delete map[agentId]
  }
  scheduleByAgent.value = map
}

async function onViewScheduleThread(threadIdToView: string) {
  scheduleModalOpen.value = false
  await selectThread(threadIdToView)
}

/** BUG-06：持久化消息 → 聊天气泡。run 状态还原失败/取消态；assistant 按其归属助手补 module。 */
function toChatMessage(m: AgentMessage, agentModule?: string): ChatMsg {
  return {
    role: m.role === 'assistant' ? 'assistant' : 'user',
    content: m.content,
    module: m.role === 'assistant' ? agentModule : undefined,
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
  if (!run || threadId.value !== id || sending.value) return
  if (run.status !== 'running' && run.status !== 'queued') return
  conv.attach(run.id, {
    module: agents.value.find((a) => a.id === run.agent_id)?.module,
    onEnd: () => {
      refreshThreadsForAgent()
    },
    // 事件缓存已过期等：不打扰用户，重新拉一次已落库的消息。
    onError: () => {
      void api.agentThreads.messages(project.id, id).then((list) => {
        if (threadId.value !== id || sending.value) return
        chatMessages.value = list.map((m) => toChatMessage(m, agents.value.find((a) => a.id === m.agent_id)?.module))
      }).catch(() => undefined)
    }
  })
}

async function selectThread(id: string) {
  if (!id || !threads.value.some((thread) => thread.id === id)) return
  detachRun()
  switchTargetId.value = ''
  threadId.value = id
  if (router?.replace) void router.replace({ query: { ...route.query, thread: id } })
  await runThread(async () => loadThreadMessages(id), { replace: true })
}

function reloadThread() {
  if (threadId.value) void selectThread(threadId.value)
  else void loadThreads()
}

/**
 * 选中助手：切到「该助手自己的会话」（planv6.0 §3.7 修订）。
 *
 * 会话与助手绑定后，每个助手一份列表；没有会话就为它建一个。
 * 同一次点击里把 activeId 定下来，再由 `threadsForActive` 重新取列表。
 */
async function selectAgent(id: string) {
  if (!id || id === activeId.value) return
  detachRun()
  switchTargetId.value = ''
  activeId.value = id
  threadId.value = ''
  await loadThreads()
}

async function switchToMentionedAgent() {
  const target = switchTarget.value
  if (!target) return
  await selectAgent(target.id)
  composer.value?.focus?.()
}

/** 拉当前助手的会话列表；空则新建一个。 */
async function loadThreads() {
  const agentId = activeId.value
  const projectId = project.id
  if (!agentId) return
  await runThread(async () => {
    const page = await api.agentThreads.page(projectId, agentId).catch(async () => ({ threads: await api.agentThreads.list(projectId, agentId), next_cursor: '' }))
    // 项目或助手可能在取数期间切换；旧响应不得覆盖当前列表。
    if (project.id !== projectId || activeId.value !== agentId) return false
    threads.value = page.threads
    nextCursor.value = page.next_cursor
    const queryId = typeof route.query.thread === 'string' ? route.query.thread : ''
    if (threads.value.length) {
      const selected = threads.value.find((th) => th.id === queryId)
      threadId.value = selected?.id || threads.value[0].id
    } else {
      const th = await api.agentThreads.create(projectId, t('nav.agents'), agentId)
      if (project.id !== projectId || activeId.value !== agentId) return false
      threads.value = [th]
      threadId.value = th.id
    }
    if (router?.replace) void router.replace({ query: { ...route.query, thread: threadId.value } })
    return loadThreadMessages(threadId.value)
  }, { replace: true })
}

async function renameThread(id: string, title: string) {
  try {
    const renamed = await api.agentThreads.rename(project.id, id, title)
    threads.value = threads.value.map((th) => th.id === id ? renamed : th)
  } catch (e) { toast.error(errorMessage(e, t('pages.agents.renameFailed'))) }
}

async function removeThread(id: string) {
  onStop()
  try {
    await api.agentThreads.remove(project.id, id)
    threads.value = threads.value.filter((th) => th.id !== id)
    if (threadId.value === id) {
      threadId.value = ''
      if (threads.value.length) await selectThread(threads.value[0].id)
      else await resetThread()
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
    // BUG-11：通用助手默认选中；切项目后旧助手 ID 不得沿用。
    if (!agents.value.some((a) => a.id === activeId.value)) {
      const general = agents.value.find((a) => a.builtin_key === 'general')
      activeId.value = general?.id || agents.value[0]?.id || ''
    }
    return agents.value.length > 0
  })
}

/** 新建表单：取第一个模块的名称/说明/默认工具（无模块回退 database）。 */
function createAgentForm(first: AgentModuleInfo | undefined): AgentFormState {
  return {
    name: first?.name || '',
    module: first?.id || 'database',
    description: first?.description || '',
    system_prompt: '',
    model_override: '',
    tool_ids: [...(first?.default_tools || [])],
    writable: false
  }
}

/** 编辑表单：回填已有助手；工具数组复制，避免改动原对象。 */
function editAgentForm(a: CloudAgent): AgentFormState {
  return {
    name: a.name,
    module: a.module,
    description: a.description,
    system_prompt: a.system_prompt,
    model_override: a.model_override || '',
    tool_ids: [...(a.tool_ids || [])],
    writable: !!a.writable
  }
}

function openCreate() {
  editing.value = null
  form.value = createAgentForm(modules.value[0])
  modalOpen.value = true
}

function openEdit(a: CloudAgent) {
  editing.value = a
  form.value = editAgentForm(a)
  modalOpen.value = true
}

function onModuleChange(mod: string) {
  form.value.module = mod
  const m = modules.value.find((x) => x.id === mod)
  if (m && !editing.value) {
    form.value.tool_ids = [...(m.default_tools || [])]
    if (!form.value.name) form.value.name = m.name
  }
}

function toggleTool(id: string) {
  const cur = form.value.tool_ids
  form.value.tool_ids = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]
}

async function saveAgent() {
  if (!form.value.name.trim()) {
    toast.warning(t('pages.agents.nameRequired'))
    return
  }
  const mod = modules.value.find((m) => m.id === form.value.module)
  if (form.value.module === 'sandbox' && mod?.sandbox_available !== true) {
    toast.warning(t('pages.agents.sandboxUnconfigured'))
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.agents.patch(project.id, editing.value.id, form.value)
    } else {
      const created = await api.agents.create(project.id, form.value)
      activeId.value = created.id
    }
    modalOpen.value = false
    await loadAgents()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function removeAgent(a: CloudAgent) {
  try {
    await api.agents.remove(project.id, a.id)
    const wasActive = activeId.value === a.id
    if (wasActive) {
      activeId.value = ''
      threadId.value = ''
      threads.value = []
      chatMessages.value = []
      settleThread(false)
    }
    await loadAgents()
    // 删掉的是当前助手：会话列表也要跟着换到新选中的助手。
    if (wasActive) await loadThreads()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.deleteFailed')))
  }
}

async function resetThread() {
  detachRun()
  switchTargetId.value = ''
  if (!activeId.value) return
  try {
    const th = await api.agentThreads.create(project.id, t('nav.agents'), activeId.value)
    threads.value.unshift(th)
    threadId.value = th.id
    chatMessages.value = []
    settleThread(false)
    if (router?.replace) void router.replace({ query: { ...route.query, thread: th.id } })
  } catch (e) {
    toast.error(errorMessage(e, t('pages.agents.createThreadFailed')))
  }
}

function refreshThreadsForAgent() {
  const projectId = project.id
  const agentId = activeId.value
  if (!agentId) return
  void api.agentThreads.page(projectId, agentId).then((page) => {
    if (project.id !== projectId || activeId.value !== agentId) return
    threads.value = page.threads
    nextCursor.value = page.next_cursor
  }).catch((e) => {
    if (project.id === projectId && activeId.value === agentId) {
      toast.error(errorMessage(e, t('pages.agents.loadThreadsFailed')))
    }
  })
}

async function onSend(text: string, mentions: { agent_id: string }[]) {
  const content = text.trim()
  if (!content || sending.value) return
  if (new Set(mentions.map((m) => m.agent_id)).size > 1) {
    toast.warning(t('pages.agents.multipleAgentsNotSupported'))
    return
  }
  const target = mentions.length ? agents.value.find((a) => a.id === mentions[0]?.agent_id) : undefined
  if (mentions.length && !target) { toast.warning(t('pages.agents.selectAgentFirst')); return }
  if (target && target.id !== activeId.value) {
    switchTargetId.value = target.id
    return
  }
  if (activeCapabilityError.value) {
    toast.warning(activeCapabilityError.value)
    return
  }
  switchTargetId.value = ''
  if (!threadId.value) await loadThreads()
  if (!threadId.value) return
  let used = mentions
  if (!used.length && activeAgent.value) used = [{ agent_id: activeAgent.value.id }]
  if (!used.length) { toast.warning(t('pages.agents.selectAgentFirst')); return }
  const speaker = agents.value.find((a) => a.id === used[0]?.agent_id)
  const thread = threads.value.find((item) => item.id === threadId.value)
  if (thread?.agent_id && thread.agent_id !== speaker?.id) {
    if (speaker) switchTargetId.value = speaker.id
    return
  }
  draft.value = ''
  conv.start({ content, mentions: [...used], module: speaker?.module }, {
    onEnd: () => {
      refreshThreadsForAgent()
    },
    onError: (e) => {
      if ((e as { code?: string })?.code === 'agent_switch_required' && target) {
        draft.value = content
        switchTargetId.value = target.id
        return
      }
      toast.error(errorMessage(e, t('pages.agents.runFailed')))
    }
  })
}

/** 重试：移除失败气泡后走后端 retry_of_run_id（BUG-03：不重复落 user 消息）。 */
function retryLast() {
  if (sending.value || !failedRunId.value) return
  if (activeCapabilityError.value) { toast.warning(activeCapabilityError.value); return }
  const retryId = failedRunId.value
  // 移除末尾失败气泡（user 消息保留展示）。
  const last = chatMessages.value[chatMessages.value.length - 1]
  if (last?.role === 'assistant' && (last.error || last.canceled)) chatMessages.value.pop()
  conv.start({ content: '', mentions: [], retry_of_run_id: retryId }, {
    onEnd: () => {
      refreshThreadsForAgent()
    },
    onError: (e) => { toast.error(errorMessage(e, t('pages.agents.runFailed'))) }
  })
}

function onStop() {
  if (sending.value) {
    const reply = chatMessages.value[chatMessages.value.length - 1]
    if (reply?.role === 'assistant') reply.canceled = true
  }
  conv.stop()
}

/** 切换会话 / 助手：只断开 SSE，后端 run 继续，回到该会话时自动续接。 */
function detachRun() {
  conv.stop(false)
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
